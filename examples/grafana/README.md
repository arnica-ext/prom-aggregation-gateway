# Gateway histogram in Grafana

This dashboard shows the estimated number of new observations per duration
range during the selected time window, defaulting to the last five minutes.
It refreshes every 10 seconds. It uses Prometheus `increase` and subtracts
adjacent cumulative buckets to show separate interval counts. Counts may be
fractional because Prometheus extrapolates to the selected time boundaries.

With `prometheus-local` on `prom-aggregation-demo`, run from the repository root:

```bash
docker run -d --name grafana-local --network prom-aggregation-demo -p 127.0.0.1:3000:3000 -e GF_AUTH_ANONYMOUS_ENABLED=true -e GF_AUTH_ANONYMOUS_ORG_ROLE=Viewer -e GF_USERS_ALLOW_SIGN_UP=false --mount "type=bind,source=$(pwd)/examples/grafana/provisioning,target=/etc/grafana/provisioning,readonly" --mount "type=bind,source=$(pwd)/examples/grafana/dashboards,target=/var/lib/grafana/dashboards,readonly" --mount type=volume,source=grafana-local-data,target=/var/lib/grafana grafana/grafana:latest
```

Open http://127.0.0.1:3000/d/gateway-histogram/push-gateway-histogram?from=now-5m&to=now&refresh=10s

The local container permits read-only viewing without login. Change the time
picker to inspect another recent window. Dashboard and data source definitions
are provisioned from this directory. Grafana reaches Prometheus using its Docker
container name.

```bash
docker logs -f grafana-local
docker stop grafana-local
docker start grafana-local
```

See [Grafana provisioning](https://grafana.com/docs/grafana/latest/administration/provisioning/)
and [bar chart documentation](https://grafana.com/docs/grafana/latest/visualizations/panels-visualizations/visualizations/bar-chart/).
