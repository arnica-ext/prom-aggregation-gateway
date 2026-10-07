"""Push one observation in the configured bucket every 30 seconds."""

import logging
import math
import os
import random
import signal
import threading
import time
import urllib.error
import urllib.request


METRIC_NAME = "demo_periodic_duration_seconds"
BUCKETS = (0.1, 0.25, 0.5, 1, 2, 5)


def parse_bucket(raw):
    raw = raw.strip()
    if raw == "+Inf":
        return len(BUCKETS)
    try:
        return BUCKETS.index(float(raw))
    except ValueError:
        allowed = ", ".join(str(bound) for bound in BUCKETS) + ", +Inf"
        raise ValueError(f"BUCKET must be one of: {allowed}") from None


def random_observation(index):
    """Generate one value in the configured bucket's exclusive range."""
    lower = BUCKETS[index - 1] if index else 0
    upper = BUCKETS[index] if index < len(BUCKETS) else 10
    bucket = str(BUCKETS[index]) if index < len(BUCKETS) else "+Inf"
    # Use integer microseconds so rounding cannot cross the lower boundary.
    value = random.randint(int(lower * 1_000_000) + 1, int(upper * 1_000_000)) / 1_000_000
    return value, bucket


def histogram_payload(value):
    lines = [
        f"# HELP {METRIC_NAME} Random duration in seconds from the demo sender",
        f"# TYPE {METRIC_NAME} histogram",
    ]
    for bound in BUCKETS:
        lines.append(
            f'{METRIC_NAME}_bucket{{le="{bound}"}} {int(value <= bound)}'
        )
    lines.extend([
        f'{METRIC_NAME}_bucket{{le="+Inf"}} 1',
        f"{METRIC_NAME}_sum {value:.6f}",
        f"{METRIC_NAME}_count 1",
    ])
    return ("\n".join(lines) + "\n").encode("utf-8")


def main():
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(message)s")
    gateway_url = os.environ.get(
        "GATEWAY_URL",
        "http://prom-aggregation-gateway-local:8080/metrics/job/histogram-sender",
    )
    try:
        interval = float(os.environ.get("INTERVAL_SECONDS", "30"))
    except ValueError:
        raise SystemExit("INTERVAL_SECONDS must be a positive, finite number")
    if not math.isfinite(interval) or interval <= 0:
        raise SystemExit("INTERVAL_SECONDS must be a positive, finite number")
    try:
        bucket_index = parse_bucket(os.environ.get("BUCKET", "0.1"))
    except ValueError as error:
        raise SystemExit(str(error))

    stopping = threading.Event()
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: stopping.set())

    bucket_label = str(BUCKETS[bucket_index]) if bucket_index < len(BUCKETS) else "+Inf"
    logging.info(
        "Sending to %s every %g seconds with assigned_bucket=%s",
        gateway_url, interval, bucket_label,
    )
    while not stopping.is_set():
        started = time.monotonic()
        value, bucket = random_observation(bucket_index)
        request = urllib.request.Request(
            gateway_url,
            data=histogram_payload(value),
            headers={"Content-Type": "text/plain; version=0.0.4"},
            method="POST",
        )
        try:
            with urllib.request.urlopen(request, timeout=10) as response:
                if response.status != 202:
                    raise RuntimeError(f"Unexpected HTTP status: {response.status}")
                logging.info(
                    "Pushed observation=%.6f selected_bucket=%s count=1 HTTP=%s",
                    value, bucket, response.status,
                )
        except (urllib.error.URLError, OSError, RuntimeError) as error:
            logging.error("Push failed: %s; next attempt in %g seconds", error, interval)
        stopping.wait(max(0, interval - (time.monotonic() - started)))
    logging.info("Sender stopped")


if __name__ == "__main__":
    main()
