---
title: Prometheus Monitoring
nav_order: 10
---

# Prometheus Monitoring Guide

The Docker Awakening Gateway natively exposes application telemetrics via a standard Prometheus `/_metrics` endpoint. This allows administrators to monitor gateway performance, container awakening activity, and resource optimization (idle shutdowns) globally or per-container.

## 1. Overview

By default, the gateway serves metrics on port `8080` (or whatever `gateway.port` is configured to) under the `/_metrics` HTTP path.
This endpoint serves:
- Standard Go runtime metrics (`go_gc_*`, `go_memstats_*`, `go_goroutines`, etc.)
- Custom Gateway metrics (prefixed with `gateway_*`).

*Note: The `/_metrics` endpoint is considered internal. It is excluded from proxy routing and is rate-limited exactly like the `/_health` checks.*

## 2. Prometheus Scrape Configuration

To instruct Prometheus to scrape the gateway, add a job to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'docker-gateway'
    scrape_interval: 15s
    static_configs:
      - targets: ['gateway:8080'] # Replace with your gateway IP/hostname and port
```

## 3. Available Custom Metrics

All custom metrics use the `container` label. This allows you to differentiate traffic and lifecycle events for `my-app` vs `slow-app`, ensuring granular observability.

| Metric Name | Type | Labels | Description |
| :--- | :--- | :--- | :--- |
| `gateway_requests_total` | Counter | `container`, `status_code` | Total number of HTTP requests that successfully passed through the reverse proxy. `status_code` allows distinguishing between `200 OK`, `502 Bad Gateway`, etc. |
| `gateway_request_duration_seconds` | Histogram | `container` | Tracks the entire latency of the HTTP request, including proxying time. |
| `gateway_starts_total` | Counter | `container`, `result` | Counts every attempt to wake up a sleeping container. `result` is either `success` (container started and TCP answered) or `error` (timeout, crash, network issue). |
| `gateway_start_duration_seconds` | Histogram | `container` | Tracks the time it takes for a container to go from "starting" to fully "running" (TCP port responding). Crucial for optimizing `start_timeout` values. |
| `gateway_idle_stops_total` | Counter | `container` | Increments every time a container is automatically stopped by the gateway because its `idle_timeout` threshold was exceeded. |

## 4. Useful PromQL Queries (Grafana Examples)

Here are some standard queries you can use to build a Grafana dashboard monitoring your sleep/wake environment.

### Traffic & Performance
**Requests per second (RPS) per container**
```promql
rate(gateway_requests_total[5m])
```

**95th Percentile Response Time (Latency)**
```promql
histogram_quantile(0.95, rate(gateway_request_duration_seconds_bucket[5m]))
```

**Error Rate (HTTP 5xx)**
```promql
sum by (container) (rate(gateway_requests_total{status_code=~"5.."}[5m]))
```

### Awakening & Lifecyles
**Awakening Success Rate vs Failure Rate**
```promql
sum by (result) (rate(gateway_starts_total[1h]))
```

**Average Awakening Time (Cold Start Penalty)**
```promql
rate(gateway_start_duration_seconds_sum[1h]) 
/ 
rate(gateway_start_duration_seconds_count[1h])
```

**Containers stopped to save resources (last 24h)**
```promql
increase(gateway_idle_stops_total[24h])
```
