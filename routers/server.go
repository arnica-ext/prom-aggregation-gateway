package routers

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arnica-ext/prom-aggregation-gateway/metrics"
	"github.com/gin-gonic/gin"
	promMetrics "github.com/slok/go-http-metrics/metrics/prometheus"
)

func RunServers(cfg ApiRouterConfig, apiListen string, lifecycleListen string, metricTTL time.Duration) {
	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigChannel)

	agg := metrics.NewAggregate(metrics.SetTTLMetricTime(&metricTTL))
	cleanupTicker := time.NewTicker(min(metricTTL, time.Minute))
	defer cleanupTicker.Stop()

	promMetricsConfig := promMetrics.Config{
		Registry: metrics.PromRegistry,
	}

	apiRouter := setupAPIRouter(cfg, agg, promMetricsConfig)
	go runServer("api", apiRouter, apiListen)

	lifecycleRouter := setupLifecycleRouter(metrics.PromRegistry)
	go runServer("lifecycle", lifecycleRouter, lifecycleListen)

	for {
		select {
		case <-cleanupTicker.C:
			agg.RemoveExpiredMetrics()
		case <-sigChannel:
			return
		}
	}
}

func runServer(label string, r *gin.Engine, listen string) {
	log.Printf("%s server listening at %s", label, listen)
	if err := r.Run(listen); err != nil {
		log.Panicf("error while serving %s: %v", label, err)
	}
}
