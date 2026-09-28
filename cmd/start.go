package cmd

import (
	"fmt"

	"github.com/arnica-ext/prom-aggregation-gateway/routers"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(startCmd)
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "starts up the server",
	Long:  `Starts up the aggregation server`,
	RunE:  startFunc,
}

func startFunc(cmd *cobra.Command, args []string) error {
	if cfg.MetricTTL <= 0 {
		return fmt.Errorf("metric-ttl must be positive")
	}

	apiCfg := routers.ApiRouterConfig{
		CorsDomain: cfg.CorsDomain,
		Accounts:   cfg.AuthUsers,
	}

	routers.RunServers(apiCfg, cfg.ApiListen, cfg.LifecycleListen, cfg.MetricTTL)

	return nil
}
