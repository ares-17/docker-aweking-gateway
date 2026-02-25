# Docker Awakening Gateway 🐳💤→⚡

An ultra-lightweight reverse proxy that **wakes up stopped Docker containers on demand**. When a request arrives for a sleeping container, the gateway shows an animated loading page with live logs, starts the container, and transparently proxies once it's ready.

Built as a single static Go binary — ideal for home labs, edge devices, and resource-constrained environments. Final image: **~22 MB** (distroless).

---

## Quick Start

```bash
git clone https://github.com/your-user/docker-gateway.git
cd docker-gateway
docker compose up -d --build
```

Add to `/etc/hosts`:
```
127.0.0.1  slow-app.localhost
127.0.0.1  fail-app.localhost
```

```bash
curl http://slow-app.localhost:8080/   # watch the loading page → auto-redirect ~15s later
```

---

## Features at a glance

| | |
|---|---|
| 💤 On-demand startup | Containers sleep until a request arrives |
| 📺 Live loading page | Animated UI with real-time container logs |
| ⏱ Idle auto-stop | `idle_timeout` per container stops unused services |
| 🔍 Label discovery | Add `dag.enabled=true` to any container — no static config needed |
| 🔗 Dependency ordering | `depends_on` starts `postgres` before `app`, automatically |
| ⚖️ Load balancing | Round-robin across container groups behind one hostname |
| 🔒 Admin auth | Basic Auth or Bearer Token on `/_status` and `/_metrics` |
| 🔄 Hot-reload | `docker kill -s HUP` reloads config without dropping connections |
| 📊 Prometheus | Per-container counters for requests, starts, durations, idle stops |

---

## Documentation

**📚 Full documentation → [docs/](docs/index.md)**

| Page | Description |
|------|-------------|
| [Getting Started](docs/getting-started.md) | Installation, docker-compose, test scenarios |
| [How It Works](docs/how-it-works.md) | Request lifecycle, component architecture |
| [Configuration](docs/configuration.md) | All `config.yaml` options and Docker labels |
| [Security](docs/security.md) | Admin auth, trusted proxies, Docker socket, distroless |
| [Hot-Reload](docs/hot-reload.md) | Live config updates without restarts |
| [Groups & Dependencies](docs/groups-and-dependencies.md) | Load balancing, dependency ordering |
| [Health Probe & Discovery](docs/health-probe-and-discovery.md) | HTTP readiness probes, discovery interval |
| [Prometheus Monitoring](docs/prometheus.md) | Metrics, Grafana, PromQL examples |
| [Testing](docs/testing.md) | Running the test suite |
| [Roadmap](docs/roadmap.md) | What's done, what's planned |

---

## License

MIT
