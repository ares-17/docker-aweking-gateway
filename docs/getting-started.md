---
title: Getting Started
nav_order: 2
---

# Getting Started
{: .no_toc }

<details open markdown="block">
  <summary>Contents</summary>
  {: .text-delta }
- TOC
{:toc}
</details>

---

## Prerequisites

- Docker Engine 20.10+
- Docker Compose v2 (plugin, not standalone)
- A host with access to `/var/run/docker.sock`

---

## Quick Start

```bash
git clone https://github.com/your-user/docker-gateway.git
cd docker-gateway

# Build and start the gateway + two test containers
docker compose up -d --build
```

Add entries to `/etc/hosts` for local DNS resolution:

```
127.0.0.1  slow-app.localhost
127.0.0.1  fail-app.localhost
```

Then test an awakening:

```bash
# Open in your browser or:
curl -I http://slow-app.localhost:8080/
```

The gateway shows the loading page while `slow-app` boots (~15 s), then redirects automatically.

---

## Compose Setup

Minimal `docker-compose.yml` for the gateway itself:

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
> The Docker socket is mounted **read-only** — the gateway only uses `ContainerInspect`, `ContainerStart`, `ContainerStop`, and `ContainerLogs`.

---

## Test Scenarios

The included `docker-compose.yml` ships with two containers designed for testing:

| Container | What to test | Key config |
|-----------|--------------|------------|
| `slow-app` | Normal boot delay (~15 s), live log box, auto-redirect | `start_timeout: 90s`, `idle_timeout: 5m` |
| `fail-app` | Container always crashes → inline error page | `start_timeout: 8s` |

```bash
# 1. Awakening — watch the loading page, logs appear, redirect after ~15s
curl -I http://slow-app.localhost:8080/

# 2. Failure — error page shown after 8s timeout
curl -I http://fail-app.localhost:8080/

# 3. Force idle-stop: set idle_timeout: 1m, wait, then re-request
docker kill -s HUP docker-gateway   # hot-reload after editing config.yaml
```

---

## Building

```bash
# Local binary
go build -o docker-gateway .

# Docker image (uses vendored deps — no network needed during build)
docker build -t docker-gateway .
```

The final image is based on `gcr.io/distroless/static` — no shell, no package manager, ~22 MB.

---

## Next steps

- Configure your own containers: **[Configuration Guide →](configuration.md)**
- Understand the full request lifecycle: **[How It Works →](how-it-works.md)**
- Secure admin endpoints: **[Security →](security.md)**
