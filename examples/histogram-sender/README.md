# Periodic histogram sender

One image supports multiple senders. Set `BUCKET` to assign each container a
fixed bucket. On startup and every 30 seconds, the container generates a random
observation of `demo_periodic_duration_seconds` within its assigned range:

| `BUCKET` | Generated value in seconds |
| --- | --- |
| 0.1 (default) | 0 < value <= 0.1 |
| 0.25 | 0.1 < value <= 0.25 |
| 0.5 | 0.25 < value <= 0.5 |
| 1 | 0.5 < value <= 1 |
| 2 | 1 < value <= 2 |
| 5 | 2 < value <= 5 |
| +Inf | 5 < value <= 10 |

The bucket assignment stays fixed for the lifetime of the container. Unsupported
bucket values prevent startup. Values use microsecond precision so rounding
cannot move an observation into a lower bucket.

Each POST contains a fresh histogram with `_count 1`, its observation as
`_sum`, and cumulative bucket counts. Buckets below the observation have count
zero; the assigned bucket and every higher bucket have count one. For example,
a container with `BUCKET=0.5` produces values above 0.25 and at most 0.5,
incrementing `le="0.5"`, `le="1"`, `le="2"`, `le="5"`, and `le="+Inf"`.

All five senders use the same metric name and `job="histogram-sender"` label,
so the gateway combines their observations in one histogram. Sender logs show
the assigned bucket, each generated value, and the HTTP response.

Run these commands from the repository root with the existing
`prom-aggregation-gateway-local` container listening on port 8080:

```bash
docker build -t prom-histogram-sender:local examples/histogram-sender

# Create the network and connect the gateway only once, if not already done.
docker network create prom-aggregation-demo
docker network connect prom-aggregation-demo prom-aggregation-gateway-local

# Start five containers from the same image, each with a different bucket.
for bucket in 0.1 0.25 0.5 1 2; do
  docker run -d --name "prom-histogram-sender-bucket-$bucket" --network prom-aggregation-demo -e BUCKET="$bucket" prom-histogram-sender:local
done

docker logs -f prom-histogram-sender-bucket-0.5
```

These five senders collectively add five observations every 30 seconds. The
cumulative bucket increments per cycle are 1, 2, 3, 4, 5, 5, and 5 for bucket
bounds 0.1, 0.25, 0.5, 1, 2, 5, and +Inf respectively.

Retrieve the aggregated metrics:

```bash
docker run --rm --network prom-aggregation-demo curlimages/curl:8.12.1 --fail-with-body -sS http://prom-aggregation-gateway-local:8080/metrics
```

Override `GATEWAY_URL` or the positive `INTERVAL_SECONDS` with `docker run -e`
as needed. Failed pushes are logged; the sender tries a new observation in its
assigned bucket on its next scheduled iteration.

Stop and remove all five senders before recreating them with a rebuilt image:

```bash
for bucket in 0.1 0.25 0.5 1 2; do
  docker stop "prom-histogram-sender-bucket-$bucket"
  docker rm "prom-histogram-sender-bucket-$bucket"
done
```
