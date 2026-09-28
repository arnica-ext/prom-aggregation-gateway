package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	Name      = "prom-aggregation-gateway"
	Version   = "local"
	CommitSHA = "xxxxxx"
)

type Server struct {
	ApiListen       string
	LifecycleListen string
	CorsDomain      string
	AuthUsers       []string
	MetricTTL       time.Duration
}

const (
	configFileName             = "prom-agg-conf"
	envPrefix                  = "PAG"
	replaceHyphenWithCamelCase = true
)

func Initialize(cmd *cobra.Command) error {
	v := viper.New()

	v.SetConfigName(configFileName)
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		// It's okay if there isn't a config file
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		}
	}

	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()
	if err := v.BindEnv("metricttl", "METRIC_TTL"); err != nil {
		return err
	}

	return bindFlags(cmd, v)
}

func bindFlags(cmd *cobra.Command, v *viper.Viper) error {
	var err error
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if err != nil {
			return
		}
		configName := f.Name

		if replaceHyphenWithCamelCase {
			configName = strings.ReplaceAll(f.Name, "-", "")
		}

		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if !f.Changed && v.IsSet(configName) {
			val := v.Get(configName)
			err = cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val))
		}
	})
	return err
}
