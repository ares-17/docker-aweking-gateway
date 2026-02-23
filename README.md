# Docker Awakening Gateway 🐳💤→⚡

An ultra-lightweight reverse proxy that **wakes up stopped Docker containers on-demand**. When a request arrives for a sleeping container, the gateway shows a sleek loading page, starts the container, and transparently proxies once it's ready.

Built as a single static Go binary — ideal for home labs, edge devices, and resource-constrained environments.

## Features

- **On-demand container startup** — containers sleep until needed, saving resources
- **Transparent reverse proxy** — once the container is running, requests flow through directly
- **Minimal loading page** — dark-themed, responsive UI with CSS animations (no heavy JS frameworks)
- **Concurrency-safe** — multiple simultaneous requests won't trigger duplicate starts
- **Ultra-lightweight** — static Go binary, distroless final image (< 20 MB)
- **Zero external dependencies at runtime** — HTML/CSS templates embedded in the binary via `go:embed`

## Quick Start

### With Docker Compose

```bash
# Clone the repo
git clone https://github.com/your-user/docker-gateway.git
cd docker-gateway

# Build and start the gateway + a test whoami container
docker compose up -d --build

# The whoami container starts stopped. Visit the gateway to wake it:
curl http://localhost:8080?container=whoami-test

# You'll see the loading page. After a few seconds, the container is up and proxied.
```

### Standalone Docker

```bash
# Build the image
docker build -t docker-gateway .

# Run with Docker socket mounted
docker run -d \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e PORT=8080 \
  -e TARGET_PORT=80 \
  docker-gateway
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Port the gateway listens on |
| `TARGET_PORT` | `80` | Port to proxy to on the target container |

## How It Works

```
 Client Request
       │
       ▼
 ┌─────────────┐
 │   Gateway    │──── Container running? ──── YES ──► Reverse Proxy
 │   :8080      │                                      to container
 └─────────────┘
       │ NO
       ▼
 ┌─────────────┐     ┌──────────────┐
 │ Loading Page │────►│ Start via    │
 │ (polling)    │     │ Docker API   │
 └─────────────┘     └──────────────┘
       │                    │
       │  ◄── running ─────┘
       ▼
  Auto-redirect
  to service
```

### Container Resolution

The gateway resolves which container to wake using:

1. **Query parameter**: `?container=my-app` (useful for testing)
2. **Host subdomain**: `my-app.localhost:8080` → wakes container `my-app`

## Architecture

```
docker-gateway/
├── main.go                    # Entry point
├── gateway/
│   ├── docker.go              # Docker client wrapper (inspect, start)
│   ├── manager.go             # Concurrency-safe container manager
│   ├── server.go              # HTTP server, proxy, and template rendering
│   └── templates/
│       ├── loading.html       # Awakening state page
│       └── error.html         # Failure state page
├── Dockerfile                 # Multi-stage build (golang → distroless)
├── docker-compose.yml         # Dev/test environment
└── mokups/                    # UI mockups from Stitch
```

## Security Notes

- The Docker socket is mounted **read-only** (`:ro`). The gateway only needs `ContainerInspect` and `ContainerStart`.
- The final image uses `gcr.io/distroless/static` — no shell, no package manager.
- Consider limiting which containers the gateway can manage via labels or a whitelist (roadmap feature).

## Development

```bash
# Prerequisites: Go 1.24+, Docker

# Run locally (requires Docker socket access)
go run .

# Build binary
go build -o docker-gateway .

# Build Docker image
docker build -t docker-gateway .
```

## Roadmap

- [ ] Container whitelist / label-based filtering
- [ ] Per-container port configuration via Docker labels
- [ ] Configurable start timeout
- [ ] Auto-stop idle containers (inactivity timer)
- [ ] Health check endpoint for the gateway itself
- [ ] Light mode support in the loading page

## License

MIT
