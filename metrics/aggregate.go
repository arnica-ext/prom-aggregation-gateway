package metrics

import (
	"errors"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

const DefaultMetricTTL = 30 * time.Minute

type metricFamily struct {
	*dto.MetricFamily
	lock     sync.RWMutex
	lastSeen map[*dto.Metric]time.Time
}

type Aggregate struct {
	familiesLock sync.RWMutex
	families     map[string]*metricFamily
	jobsLastSeen map[string]time.Time
	options      aggregateOptions
}

type ignoredLabels []string

type aggregateOptions struct {
	ignoredLabels     ignoredLabels
	metricTTLDuration time.Duration
}

type aggregateOptionsFunc func(a *Aggregate)

func AddIgnoredLabels(ignoredLabels ...string) aggregateOptionsFunc {
	return func(a *Aggregate) {
		a.options.ignoredLabels = ignoredLabels
	}
}

func SetTTLMetricTime(duration *time.Duration) aggregateOptionsFunc {
	return func(a *Aggregate) {
		if duration != nil {
			a.options.metricTTLDuration = *duration
		}
	}
}

func NewAggregate(opts ...aggregateOptionsFunc) *Aggregate {
	a := &Aggregate{
		families:     map[string]*metricFamily{},
		jobsLastSeen: map[string]time.Time{},
		options: aggregateOptions{
			ignoredLabels:     []string{},
			metricTTLDuration: DefaultMetricTTL,
		},
	}

	for _, opt := range opts {
		opt(a)
	}

	a.options.formatOptions()

	return a
}

func (ao *aggregateOptions) formatOptions() {
	ao.formatIgnoredLabels()
}

func (ao *aggregateOptions) formatIgnoredLabels() {
	if ao.ignoredLabels != nil {
		for i, v := range ao.ignoredLabels {
			ao.ignoredLabels[i] = strings.ToLower(v)
		}
	}

	sort.Strings(ao.ignoredLabels)
}

func (a *Aggregate) Len() int {
	a.familiesLock.RLock()
	count := len(a.families)
	a.familiesLock.RUnlock()
	return count
}

func (a *Aggregate) saveFamily(familyName string, family *dto.MetricFamily) error {
	a.familiesLock.Lock()
	defer a.familiesLock.Unlock()

	now := time.Now()
	existingFamily := a.families[familyName]
	if existingFamily != nil {
		existingFamily.removeExpired(now.Add(-a.options.metricTTLDuration))
	}
	if existingFamily == nil || len(existingFamily.Metric) == 0 {
		existingFamily = &metricFamily{
			MetricFamily: family,
			lastSeen:     make(map[*dto.Metric]time.Time, len(family.Metric)),
		}
		for _, metric := range family.Metric {
			existingFamily.lastSeen[metric] = now
		}
		a.families[familyName] = existingFamily
	} else {
		if err := existingFamily.mergeFamily(family); err != nil {
			return err
		}
	}

	MetricCountByFamily.WithLabelValues(familyName).Set(float64(len(existingFamily.Metric)))
	TotalFamiliesGauge.Set(float64(len(a.families)))
	return nil
}

func (mf *metricFamily) removeExpired(cutoff time.Time) {
	mf.lock.Lock()
	defer mf.lock.Unlock()

	retained := mf.Metric[:0]
	for _, metric := range mf.Metric {
		if mf.lastSeen[metric].After(cutoff) {
			retained = append(retained, metric)
		}
	}
	if len(retained) == len(mf.Metric) {
		return
	}

	// Rebuild storage so expired series and the old capacity can be collected.
	mf.Metric = append([]*dto.Metric(nil), retained...)
	lastSeen := make(map[*dto.Metric]time.Time, len(retained))
	for _, metric := range retained {
		lastSeen[metric] = mf.lastSeen[metric]
	}
	mf.lastSeen = lastSeen
}

func (a *Aggregate) RemoveExpiredMetrics() {
	a.familiesLock.Lock()
	defer a.familiesLock.Unlock()

	cutoff := time.Now().Add(-a.options.metricTTLDuration)
	for name, family := range a.families {
		family.removeExpired(cutoff)
		if len(family.Metric) == 0 {
			delete(a.families, name)
			MetricCountByFamily.DeleteLabelValues(name)
		} else {
			MetricCountByFamily.WithLabelValues(name).Set(float64(len(family.Metric)))
		}
	}
	for job, lastSeen := range a.jobsLastSeen {
		if !lastSeen.After(cutoff) {
			delete(a.jobsLastSeen, job)
			MetricPushes.DeleteLabelValues(job)
		}
	}
	TotalFamiliesGauge.Set(float64(len(a.families)))
}

func (a *Aggregate) parseAndMerge(r io.Reader, labels []labelPair) error {
	var parser expfmt.TextParser
	inFamilies, err := parser.TextToMetricFamilies(r)
	if err != nil {
		return err
	}

	for name, family := range inFamilies {
		// Sort labels in case source sends them inconsistently
		for _, m := range family.Metric {
			if err := a.formatLabels(m, labels); err != nil {
				return err
			}
		}

		if err := validateFamily(family); err != nil {
			return err
		}

		// family must be sorted for the merge
		sort.Sort(byLabel(family.Metric))

		if err := a.saveFamily(name, family); err != nil {
			return err
		}
	}

	return nil
}

func (a *Aggregate) HandleRender(c *gin.Context) {
	contentType := expfmt.Negotiate(c.Request.Header)
	c.Header("Content-Type", string(contentType))
	a.encodeAllMetrics(c.Writer, contentType)
}

func (a *Aggregate) encodeAllMetrics(writer io.Writer, contentType expfmt.Format) {
	a.RemoveExpiredMetrics()
	enc := expfmt.NewEncoder(writer, contentType)

	a.familiesLock.RLock()
	defer a.familiesLock.RUnlock()

	metricNames := []string{}
	metricTypeCounts := make(map[string]int)
	for name, family := range a.families {
		metricNames = append(metricNames, name)
		var typeName string
		if family.Type == nil {
			typeName = "unknown"
		} else {
			typeName = dto.MetricType_name[int32(*family.Type)]
		}
		metricTypeCounts[typeName]++
	}

	sort.Strings(metricNames)

	for _, name := range metricNames {
		if a.encodeMetric(name, enc) {
			return
		}
	}

	MetricCountByType.Reset()
	for typeName, count := range metricTypeCounts {
		MetricCountByType.WithLabelValues(typeName).Set(float64(count))
	}

}

func (a *Aggregate) encodeMetric(name string, enc expfmt.Encoder) bool {
	a.families[name].lock.RLock()
	defer a.families[name].lock.RUnlock()

	if err := enc.Encode(a.families[name].MetricFamily); err != nil {
		log.Printf("An error has occurred during metrics encoding:\n\n%s\n", err.Error())
		return true
	}
	return false
}

var ErrOddNumberOfLabelParts = errors.New("labels must be defined in pairs")

func (a *Aggregate) HandleInsert(c *gin.Context) {
	labelParts, jobName, err := parseLabelsInPath(c)
	if err != nil {
		log.Println(err)
		http.Error(c.Writer, err.Error(), http.StatusBadRequest)
		return
	}

	if err := a.parseAndMerge(c.Request.Body, labelParts); err != nil {
		log.Println(err)
		http.Error(c.Writer, err.Error(), http.StatusBadRequest)
		return
	}

	a.familiesLock.Lock()
	a.jobsLastSeen[jobName] = time.Now()
	MetricPushes.WithLabelValues(jobName).Inc()
	a.familiesLock.Unlock()
	c.Status(http.StatusAccepted)
}

type labelPair struct {
	name, value string
}

func parseLabelsInPath(c *gin.Context) ([]labelPair, string, error) {
	labelString := c.Param("labels")
	labelString = strings.Trim(labelString, "/")
	if labelString == "" {
		return nil, "", nil
	}

	labelParts := strings.Split(labelString, "/")
	if len(labelParts)%2 != 0 {
		return nil, "", ErrOddNumberOfLabelParts
	}

	var (
		labelPairs []labelPair
		jobName    string
	)
	for idx := 0; idx < len(labelParts); idx += 2 {
		name := labelParts[idx]
		value := labelParts[idx+1]
		labelPairs = append(labelPairs, labelPair{name, value})
		if name == "job" {
			jobName = value
		}
	}

	return labelPairs, jobName, nil
}
