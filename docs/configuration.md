# Configuration Guide

The Docker Awakening Gateway provides two distinct ways to manage the containers it wakes up and proxies traffic to: **Label-based Auto-Discovery** and **Static Configuration**. 

You can use either method exclusively, or combine them together. When combined, the static configuration always takes precedence in case of identical host mapping.

---

## 1. Label-based Auto-Discovery (Recommended)

The easiest and most dynamic way to configure the gateway is by attaching Docker labels directly to your application's `docker-compose.yml` or container run command.

The gateway periodically polls the Docker daemon (every 15 seconds) to find any containers (running or stopped) that have the `dag.enabled=true` label.

### Required Labels

To make a container discoverable, it **must** have these two labels:

| Label | Example | Description |
|-------|---------|-------------|
| `dag.enabled` | `true` | Tells the gateway to manage this container. |
| `dag.host` | `app.example.com` | The exact `Host` HTTP header to match incoming traffic against. |

### Optional Labels

You can fine-tune the container behavior by adding any of these optional labels:

| Label | Default | Description |
|-------|---------|-------------|
| `dag.target_port` | `80` | The internal port the container listens on (e.g., `8080`, `3000`). |
| `dag.start_timeout` | `60s` | Maximum time to wait for the container to start and boot before showing an error page. |
| `dag.idle_timeout`  | `0` (Disabled)| Time of inactivity (no HTTP requests) before the gateway automatically stops the container to save resources (e.g., `15m`, `1h`). |
| `dag.network` | `""` | The specific Docker network to look for the container IP on. If empty, the first attached network is used. |
| `dag.redirect_path` | `/` | The URL path to redirect the user to once the container successfully boots. |
| `dag.icon` | `docker` | A [Simple Icons](https://simpleicons.org/) slug (e.g., `nginx`, `redis`) used for the `/_status` dashboard. |

### Example `docker-compose.yml`

```yaml
services:
  my-app:
    image: my-app:latest
    container_name: my-app
    # No ports exposed directly! The gateway handles traffic.
    labels:
      - "dag.enabled=true"
      - "dag.host=my-app.localhost"
      - "dag.target_port=3000"
      - "dag.start_timeout=120s"
      - "dag.idle_timeout=30m"
      - "dag.icon=nodedotjs"
```

---

## 2. Static Configuration (`config.yaml`)

Static configuration is useful when you want all route definitions centralized in a single file, or if you need to configure global gateway settings. 

The gateway expects a YAML file mounted at `/etc/gateway/config.yaml` (you can override this path using the `CONFIG_PATH` environment variable).

### Global Settings

The `config.yaml` file is the **only** place where you can configure global gateway behavior:

```yaml
gateway:
  port: "8080"        # Port the gateway proxy listens on
  log_lines: 30       # Number of container log lines shown in the browser loading UI
```

### Static Container Definitions

You define targets in the `containers` array. The fields map exactly to their label counterparts.

```yaml
containers:
  - name: "my-app"               # (Required) Docker container name
    host: "my-app.example.com"   # (Required) Host header to match
    target_port: "3000"          # (Default: 80)
    start_timeout: "120s"        # (Default: 60s)
    idle_timeout: "30m"          # (Default: 0)
    network: "backend-net"       # (Default: "")
    redirect_path: "/login"      # (Default: /)
    icon: "postgresql"           # (Default: docker)
```

### Hot-Reloading

A major advantage of the `config.yaml` approach is **Hot-Reloading**. If you edit the `config.yaml` file on disk, you can tell the gateway to reload its configuration without dropping any active connections by sending a `SIGHUP` signal:

```bash
docker kill -s HUP docker-gateway
```

*(Note: Sending a `SIGHUP` also forces an immediate auto-discovery polling pass for labels, instead of waiting for the next 15-second tick).*

---

## Mixing Both Methods

You can freely mix both configurations:
- Use `config.yaml` just for the `gateway:` block, and use labels for all your dynamic containers.
- Statically define your critical core containers in `config.yaml`, but let temporary/testing apps join via labels.

**Conflict Resolution:** If a container is defined both in `config.yaml` and via labels (e.g., both try to claim the `Host: app.example.com`), the **Static Configuration always wins**, and the conflicting discovered label will be skipped and logged.
