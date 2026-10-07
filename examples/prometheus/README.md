# Prometheus for the local aggregation gateway

Prometheus scrapes the gateway's aggregated metrics every 10 seconds over the
existing `prom-aggregation-demo` Docker network. `honor_labels: true` preserves
the pushed `job="histogram-sender"` label. Port 8080 serves the accumulated
metrics; port 8888 serves the gateway's own monitoring metrics.

From the repository root, start Prometheus with the gateway already running
and connected to `prom-aggregation-demo`:

```bash
docker run -d --name prometheus-local --network prom-aggregation-demo -p 127.0.0.1:9090:9090 --mount "type=bind,source=$(pwd)/examples/prometheus/prometheus.yml,target=/etc/prometheus/prometheus.yml,readonly" --mount type=volume,source=prometheus-local-data,target=/prometheus prom/prometheus:latest
```

The named volume `prometheus-local-data` stores the collected time series.

Open http://127.0.0.1:9090/query to enter a PromQL expression and click Execute, or
http://127.0.0.1:9090/targets to check scrape health. Use the explicit IPv4
address: another process listening on IPv6 port 9090 can receive requests to
`localhost` instead. The Query page initially displays no result until you
execute an expression. Prometheus provides a built-in query and graph UI; this
setup does not include a Grafana dashboard. Try:

```promql
demo_periodic_duration_seconds_bucket
demo_periodic_duration_seconds_count
rate(demo_periodic_duration_seconds_count[5m])
histogram_quantile(0.95, sum by (le) (rate(demo_periodic_duration_seconds_bucket[5m])))
```

Query the current count through the API:

```bash
curl -sS -G http://127.0.0.1:9090/api/v1/query --data-urlencode 'query=demo_periodic_duration_seconds_count'
```

Rate and quantile queries need multiple scrapes; allow at least 30 seconds
after startup for fresh observations from the senders.

```bash
docker logs -f prometheus-local
docker stop prometheus-local
docker start prometheus-local
```

See the [Prometheus configuration documentation](https://prometheus.io/docs/prometheus/latest/configuration/configuration/)
and [Docker installation documentation](https://prometheus.io/docs/prometheus/latest/installation/).
