---
title: Health Probe & Discovery
nav_order: 9
---

# HTTP Health Probe & Configurable Discovery Interval

Two features that improve container readiness detection and give operators control over the auto-discovery polling frequency.

---

## HTTP Health Probe

By default, the gateway confirms a container is ready by performing a **TCP dial** on `target_port`. For applications that need more time after the TCP port is open (e.g., database migrations, cache warm-up), you can configure an **HTTP health probe** instead.

When `health_path` is set, the gateway makes an `HTTP GET` to the specified path and considers the container ready only when it receives a **2xx** response.

### Configuration

**YAML (`config.yaml`):**

```yaml
containers:
  - name: "my-app"
    host: "app.example.com"
    target_port: "3000"
    health_path: "/healthz"      # ← HTTP probe instead of TCP
```

**Docker label (auto-discovery):**

```yaml
labels:
  - "dag.enabled=true"
  - "dag.host=app.example.com"
  - "dag.target_port=3000"
  - "dag.health_path=/healthz"
```

### Behaviour

| `health_path` value | Probe type | Success condition |
|---------------------|------------|-------------------|
| `""` (empty/absent) | TCP dial   | Port accepts connection |
| `"/healthz"`        | HTTP GET   | 2xx status code |

- The HTTP probe retries every **500 ms** with a **2 s** timeout per attempt.
- The total budget is still governed by the container's `start_timeout`.
- If the container crashes during boot, the gateway detects it immediately (same as TCP mode).

### When to use

- Application runs migrations on startup before serving traffic.
- A warm-up phase is required (loading ML models, building caches).
- The app binds the port early but returns `503` until fully initialized.

---

## Configurable Discovery Interval

The gateway polls Docker for labeled containers at a fixed interval. Previously this was hardcoded to **15 seconds**. It can now be tuned via config or environment variable.

### Configuration

**YAML (`config.yaml`):**

```yaml
gateway:
  discovery_interval: "30s"    # poll every 30 seconds
```

**Environment variable (takes priority over YAML):**

```bash
DISCOVERY_INTERVAL=5s
```

**Docker Compose example:**

```yaml
services:
  gateway:
    build: .
    environment:
      - DISCOVERY_INTERVAL=10s
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./config.yaml:/etc/gateway/config.yaml:ro
```

### Defaults & priority

| Source | Priority | Default |
|--------|----------|---------|
| `DISCOVERY_INTERVAL` env | Highest | — |
| `gateway.discovery_interval` in YAML | Medium | — |
| Built-in default | Lowest | `15s` |

### When to tune

| Scenario | Suggested value |
|----------|----------------|
| Rapidly spinning up/down containers | `5s` |
| Stable, few containers | `30s` – `60s` |
| Reducing Docker socket chatter | `60s`+ |
| Default (most use cases) | `15s` |

> [!TIP]
> Sending `SIGHUP` to the gateway (`docker kill -s HUP docker-gateway`) always triggers an **immediate** discovery pass, regardless of the configured interval. Use this for instant reconfiguration.
