# Docker Awakening Gateway — Roadmap

## ✅ Completed

### Core
- [x] **On-demand container startup** — containers sleep until a request arrives, then are started via Docker API
- [x] **Transparent reverse proxy** — once running, requests are proxied with zero loading page overhead
- [x] **Concurrency-safe start** — per-container mutex prevents duplicate start attempts on concurrent requests
- [x] **WebSocket support** — upgrade requests are tunnelled via raw TCP hijack to the backend
- [x] **Host-header routing** — O(1) lookup maps `Host` header → container config; supports N containers on one gateway
- [x] **Query-param fallback** — `?container=NAME` for testing without DNS

### Configuration & Operations
- [x] **YAML config file** (`config.yaml`) — per-container settings, mounted via volume
- [x] **`CONFIG_PATH` env override** — point to any path for the config file
- [x] **Config validation at startup** — gateway fails-fast if `config.yaml` is missing required fields (`name`, `host`, `target_port`) or contains duplicate definitions
- [x] **Config hot-reload** — `docker kill -s HUP docker-gateway` reloads `config.yaml` at runtime without dropping connections or altering gateway state
- [x] **Label-based auto-discovery** — gateway reads Docker labels (`dag.host`, `dag.target_port`, etc.) to automatically discover containers without static config file entries
- [x] **Per-container `start_timeout`** — max time to wait for docker start + TCP probe
- [x] **Per-container `idle_timeout`** — auto-stop containers idle longer than threshold (0 = disabled)
- [x] **Per-container `target_port`** — proxy to any port on the container
- [x] **Per-container `network`** — prefer a specific Docker network for IP resolution
- [x] **Per-container `redirect_path`** — browser navigates to a specific path after startup (e.g. `/dashboard`)
- [x] **Global `log_lines`** — number of container log lines shown in the loading UI

### Reliability
- [x] **TCP readiness probe** — after Docker reports "running", dial `ip:port` until the app responds before redirect
- [x] **Early crash detection** — if container enters `exited`/`dead` during start, fail immediately (no timeout wait)
- [x] **Start state tracking** — `starting` / `running` / `failed` states with error message, exported via `/_health`
- [x] **Idle watcher goroutine** — background loop (every 60s) auto-stops containers exceeding `idle_timeout`
- [x] **Multi-network support** — resolves container IP from a named Docker network; falls back to first available

### Security
- [x] **Read-only Docker socket** — gateway only needs `ContainerInspect`, `ContainerStart`, `ContainerStop`, `ContainerLogs`
- [x] **Distroless final image** (`gcr.io/distroless/static`) — no shell, no package manager, ~18 MB
- [x] **Rate limiter on internal endpoints** — 1 req/s per IP on `/_health` and `/_logs`
- [x] **XSS-safe log rendering** — log lines injected via `textContent`, not `innerHTML`
- [x] **Vendored dependencies** — no network access needed during Docker build

### Proxy Headers
- [x] **`X-Forwarded-For`** — appends client IP to the forwarding chain
- [x] **`X-Real-IP`** — original client IP (not overwritten if already set upstream)
- [x] **`X-Forwarded-Proto`** — `http` or `https`
- [x] **`X-Forwarded-Host`** — original `Host` header value

### Frontend (loading page)
- [x] **Animated loading page** — dark-themed, breathing container icon, barber-pole progress bar
- [x] **Live log box** — polls `/_logs` every 3s, renders last N lines with auto-scroll
- [x] **Inline error state** — on `status=failed`, swaps progress bar for error box in-place (no redirect); shows retry button
- [x] **Auto-redirect on ready** — polls `/_health` every 2s; navigates to `redirect_path` when running
- [x] **Start timeout visible** — displays the configured timeout value in the subtitle
- [x] **Error page** — separate template for initial request errors (container not found, Docker error)

### Tooling
- [x] **Multi-stage Dockerfile** — `golang:1.24` builder → `distroless/static` runner
- [x] **`docker-compose.yml`** — gateway + `slow-app` (15s boot) + `fail-app` (always crashes) for testing
- [x] **`ROADMAP.md`** — this file

### Admin & Observability
- [x] **`/_status` dashboard page** — HTML admin dashboard showing all managed containers with live status, heartbeat bars, uptime, last request, idle timeout, and dark/light mode toggle
- [x] **`/_status/api` JSON endpoint** — returns a snapshot of all containers (status, image, timestamps, config) polled every 5s by the dashboard
- [x] **`/_status/wake` action** — POST endpoint to trigger container start from the dashboard UI
- [x] **Prometheus `/metrics` endpoint** — per-container counters: requests proxied, start events, idle stops, duration histograms

---

## 📅 Medium-term

### Features
- [ ] **Customisable loading page** — per-container colour/logo/message overrides
- [ ] **HTTP health probe** — optionally call a container's `/health` endpoint instead of TCP to confirm readiness
- [ ] **Configurable discovery interval** — allow tuning the label polling frequency via config or env var (default: 15s)

### Security
- [ ] **Admin endpoint authentication** — optional basic-auth or bearer token to protect `/_status/*` and `/_metrics`
- [x] **CORS / CSRF protection on `/_status/wake`** — prevent cross-origin container start abuse
- [x] **Rate limiter memory cleanup** — periodic eviction of stale IPs to prevent unbounded memory growth
- [x] **Trusted proxy configuration** — only trust `X-Forwarded-For` from known upstream proxies for rate limiting

### Reliability & Quality
- [x] **Graceful shutdown** — `SIGTERM` / `SIGINT` triggers `http.Server.Shutdown()` with grace period + cancels all background goroutines
- [x] **Structured logging** — migrate from `log.Printf` / `fmt.Printf` to Go 1.21+ `log/slog` for JSON-structured output
- [x] **Discovery change detection** — only trigger `ReloadConfig` when the merged config actually differs from the current one
- [x] **Unit tests** — table-driven tests for config validation, discovery merging, rate limiter, and proxy routing

## 🔭 Long-term

- [ ] **Multi-instance / distributed state** — share `startStates` and `lastSeen` via Redis or etcd for horizontal scaling
- [ ] **Built-in TLS termination** — ACME/Let's Encrypt via `golang.org/x/crypto/acme/autocert`
- [ ] **Container grouping / weighted routing** — start a group of containers, load-balance across replicas
- [ ] **Admin UI** — lightweight web interface to view states, force wake/sleep, view logs, edit config

## Known Limitations (by design)

- **Single host only** — communicates with the local Docker socket; remote Docker hosts not supported
- **HTTP only** — TLS expected to be handled by an upstream proxy (nginx, Caddy, Traefik)
- **In-memory state** — start states and activity timestamps reset on gateway restart
