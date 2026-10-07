FROM python:3.13-alpine

WORKDIR /app
COPY sender.py /app/sender.py

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    INTERVAL_SECONDS=30 \
    BUCKET=0.1 \
    GATEWAY_URL=http://prom-aggregation-gateway-local:8080/metrics/job/histogram-sender

USER 65532:65532
ENTRYPOINT ["python", "/app/sender.py"]
