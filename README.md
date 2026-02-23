# Docker Awakening Gateway 🐳💤→⚡

An ultra-lightweight reverse proxy that **wakes up stopped Docker containers on-demand**. When a request arrives for a sleeping container, the gateway shows a sleek loading page with live container logs, starts the container, and transparently proxies once it's ready. Idle containers can be auto-stopped to save resources.

Built as a single static Go binary — ideal for home labs, edge devices, and resource-constrained environments.

## Features

- **On-demand container startup** — containers sleep until needed
- **Dual timeout system** — independent `start_timeout` and `idle_timeout` per container
- **Live log box** — loading page polls container logs every 3s in real time
- **Configurable redirect path** — redirect to `/dashboard` or any path after startup
- **Per-container configuration** — one gateway manages N containers via `config.yaml`
- **Transparent reverse proxy** — once running, requests flow through with zero overhead
- **Concurrency-safe** — multiple simultaneous requests won't trigger duplicate starts
- **Ultra-lightweight** — static Go binary, distroless final image (~17 MB)

## Quick Start

```bash
git clone https://github.com/your-user/docker-gateway.git
cd docker-gateway

# Start the gateway + test containers
docker compose up -d --build

# Test the slow-app awakening (add entry to /etc/hosts first — see below)
curl http://slow-app.localhost:8080/
```

Add to `/etc/hosts` for local testing:
```
127.0.0.1  slow-app.localhost
127.0.0.1  fail-app.localhost
```

## Configuration

The gateway is configured via a **YAML file** mounted at `/etc/gateway/config.yaml`. The path can be overridden with the `CONFIG_PATH` environment variable.

### `config.yaml` reference

```yaml
gateway:
  port: "8080"        # port the gateway listens on (default: 8080)
  log_lines: 30       # container log lines shown in the loading page UI (default: 30)

containers:
  - name: "my-app"               # Docker container name (required)
    host: "my-app.example.com"   # Host header to match incoming requests (required)
    target_port: "80"            # Port the container listens on (e.g., `80`, `3000`) (default: 80)
    start_timeout: "60s"         # Max time to wait for container to start (Docker) + TCP probe. Gateway will error if timeout reached (default: 60s)
    idle_timeout: "30m"          # Auto-stop container after X time of no requests (e.g., `10m`). `0` disables auto-stop (default: 0)
    network: ""                  # Instructs gateway to resolve container IP from this specific Docker network name. Useful if container is on multiple networks.
    redirect_path: "/"           # Absolute path to redirect the browser to once target is running (default: /)
    icon: "docker"               # [Simple Icons](https://simpleicons.org/) slug for the `/_status` dashboard (e.g. `nginx`, `redis`) (default: `docker`)
```

### Options reference

| Field | Scope | Default | Description |
|---|---|---|---|
| `gateway.port` | Global | `8080` | Listening port |
| `gateway.log_lines` | Global | `30` | Log lines shown in the UI |
| `name` | Container | — | Docker container name |
| `host` | Container | — | Incoming `Host` header to match |
| `target_port` | Container | `80` | Port on the container to proxy to |
| `start_timeout` | Container | `60s` | Max time to wait for startup before showing the error page |
| `idle_timeout` | Container | `0` | Inactivity period before auto-stopping the container (`0` = disabled) |
| `redirect_path` | Container | `/` | Path the browser navigates to once the container is running |

### Timeout behaviour

```
start_timeout  — from the moment the gateway triggers docker start
    │
    └─► container enters "running" → redirect to redirect_path
    └─► timeout exceeded           → error page shown

idle_timeout   — checked every 60 seconds (background goroutine)
    │
    └─► last request > idle_timeout ago AND container running → docker stop
    └─► next request arrives → back to start_timeout path
```

## Docker Compose

```yaml
services:
  gateway:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./config.yaml:/etc/gateway/config.yaml:ro
    environment:
      - CONFIG_PATH=/etc/gateway/config.yaml
```

> [!NOTE]
> The Docker socket is mounted **read-only** — the gateway only needs `ContainerInspect`, `ContainerStart`, `ContainerStop`, and `ContainerLogs`.

## How It Works

```
 Incoming Request (Host: my-app.example.com)
       │
       ▼
 resolve Host → ContainerConfig
       │
       ├─ container running? ──YES──► RecordActivity → Reverse Proxy
       │
       └─ container stopped?
              │
              ├──► serve loading page (log box + progress bar)
              └──► async: docker start → poll until running
                                               │
                                         browser polls /_health
                                               │
                                         status = running
                                               │
                                         redirect to redirect_path
```

### Internal endpoints

| Endpoint | Description |
|---|---|
| `/_health?container=NAME` | Returns `{"status":"running"}` — polled by the loading page |
| `/_logs?container=NAME` | Returns `{"lines":["..."]}` — polled every 3s by the log box |
| `/_status` | Admin dashboard showing all managed containers with live status |
| `/_status/api` | JSON snapshot of all containers — polled every 5s by the dashboard |
| `/_status/wake?container=NAME` | POST — triggers container start from the dashboard |
| `/_metrics` | Prometheus metrics endpoint. **[→ Read the Monitoring Guide](docs/prometheus.md)** |

## Test Scenarios (docker compose)

| Container | Scenario to test | Config |
|---|---|---|
| `slow-app` | Normal boot delay (15s), log box, auto-redirect | `start_timeout: 90s`, `idle_timeout: 5m` |
| `fail-app` | Container always crashes → error page | `start_timeout: 8s` |

Trigger each:
```bash
# 1. Normal awakening — visit loading page, watch logs appear, auto-redirect after ~15s
curl -I http://slow-app.localhost:8080/

# 2. Force idle-stop test — set idle_timeout to 1m in config.yaml, wait, re-request
# 3. Error/timeout page — request fail-app, wait 8s
curl -I http://fail-app.localhost:8080/
```

## Build

```bash
# Local build
go build -o docker-gateway .

# Docker image (uses vendored deps — no network needed during build)
docker build -t docker-gateway .
```

## Architecture

```
docker-gateway/
├── main.go                    # Entry point: load config → start idle watcher → serve
├── config.yaml                # Per-container configuration (mount into container)
├── gateway/
│   ├── config.go              # YAML config structs + loader + host index builder
│   ├── docker.go              # Docker client: inspect, start, stop, logs
│   ├── manager.go             # Concurrency-safe start + idle auto-stop watcher
│   ├── server.go              # HTTP server: routing, /_health, /_logs, proxy
│   └── templates/
│       ├── loading.html       # Awakening page: log box + progress + auto-redirect
│       ├── error.html         # Failure state page
│       └── status.html        # Admin status dashboard
├── Dockerfile                 # Multi-stage: golang:1.24 → distroless/static
└── docker-compose.yml         # Dev/test environment
```

## Security

- Docker socket mounted **read-only** (`:ro`)
- Log lines rendered via `textContent` (no HTML injection)
- Final image: `gcr.io/distroless/static` — no shell, no package manager
- No credentials, secrets, or sensitive paths exposed via `/_logs` or `/_health`

## License

MIT
