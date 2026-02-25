---
title: Security
nav_order: 6
---

# Security
{: .no_toc }

<details open markdown="block">
  <summary>Contents</summary>
  {: .text-delta }
- TOC
{:toc}
</details>

---

## Admin Endpoint Authentication

The admin endpoints (`/_status`, `/_status/api`, `/_status/wake`, `/_metrics`) can be optionally protected with **Basic Auth** or **Bearer Token** authentication. By default, authentication is **disabled** for backward compatibility.

### Available Methods

| Method | Header sent by client | Best for |
|--------|-----------------------|----------|
| `none` (default) | — | Trusted internal networks, no exposure needed |
| `basic` | `Authorization: Basic <base64>` | Browser access to `/_status` dashboard |
| `bearer` | `Authorization: Bearer <token>` | Prometheus scraping, automation, CI |

### What is protected

| Endpoint | Protected | Why |
|----------|-----------|-----|
| `/_status` | ✅ | Exposes container names, images, and statuses |
| `/_status/api` | ✅ | JSON snapshot with full container details |
| `/_status/wake` | ✅ | Privileged action — starts containers |
| `/_metrics` | ✅ | Reveals internal architecture details |
| `/_health` | ❌ | Required by loading page JS |
| `/_logs` | ❌ | Required by loading page JS |
| `/` (proxy) | ❌ | End-user traffic |

### Configuration

Auth is configured via `config.yaml` or environment variables. See _[Configuration → Admin Auth](configuration.md#admin-auth)_ for the full reference.

**Config snippet:**

```yaml
gateway:
  admin_auth:
    method: "basic"
    username: "admin"
    password: "s3cret-passw0rd"
```

**Environment variables** (higher priority than YAML):

| Variable | Description |
|----------|-------------|
| `ADMIN_AUTH_METHOD` | `none`, `basic`, or `bearer` |
| `ADMIN_AUTH_USERNAME` | Username (required for `basic`) |
| `ADMIN_AUTH_PASSWORD` | Password (required for `basic`) |
| `ADMIN_AUTH_TOKEN` | Token (required for `bearer`) |

### Usage examples

**Browser / curl with Basic Auth:**

```bash
curl -u admin:s3cret-passw0rd http://gateway:8080/_status/api
```

**Prometheus scraping with Bearer Token (`prometheus.yml`):**

```yaml
scrape_configs:
  - job_name: "docker-gateway"
    bearer_token: "my-super-secret-token"
    static_configs:
      - targets: ["gateway:8080"]
    metrics_path: "/_metrics"
```

```bash
curl -H "Authorization: Bearer my-super-secret-token" http://gateway:8080/_metrics
```

### Security notes

> [!WARNING]
> Both Basic Auth and Bearer Token transmit credentials **in cleartext** over HTTP.
> Always place a **TLS-terminating reverse proxy** (Nginx, Caddy, Traefik) in front
> of the gateway in production. This is consistent with the gateway's design: TLS is
> the upstream proxy's responsibility.

- Credential comparison uses **constant-time algorithms** (`crypto/subtle`) to prevent timing attacks.
- Failed authentication is logged with the source IP and path — credentials are **never** logged.
- `SIGHUP` hot-reload does **not** update `admin_auth` settings. A container restart is required. This is intentional — it prevents partial credential update windows during reload.

---

## Docker Socket

The gateway mounts the Docker socket **read-only**:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
```

Only the following API operations are used:

- `ContainerInspect` — read container state and IP
- `ContainerStart` — wake a sleeping container
- `ContainerStop` — idle auto-stop
- `ContainerLogs` — stream logs to the loading page

No write operations (create, remove, pull) are ever performed.

---

## Distroless Image

The final Docker image is based on `gcr.io/distroless/static`:

- **No shell** — `sh`, `bash`, `ash` do not exist in the image
- **No package manager** — `apt`, `apk`, `yum` are absent
- **No runtime dependencies** — the Go binary links statically with `CGO_ENABLED=0`
- **~22 MB** total image size

---

## XSS-Safe Log Rendering

Container log lines are injected into the loading page via JavaScript `textContent`, never `innerHTML`. This prevents any HTML or script injection from container output reaching the browser DOM.

---

## Trusted Proxies & Rate Limiting

Admin and utility endpoints are rate-limited to **1 request/s per source IP**.

By default, rate limiting uses the direct TCP connection (`RemoteAddr`). If the gateway sits behind a known upstream proxy, you can configure trusted CIDR ranges so that the real client IP is extracted from `X-Forwarded-For`:

```yaml
gateway:
  trusted_proxies:
    - "10.0.0.0/8"
    - "172.16.0.0/12"
    - "192.168.0.0/16"
```

> [!NOTE]
> Only trust proxies you fully control. An attacker can forge `X-Forwarded-For` if they can reach the gateway directly. With no `trusted_proxies` configured (the default), `X-Forwarded-For` is always ignored.

---

## Proxy Headers

The gateway sets the following forwarding headers on proxied requests:

| Header | Behaviour |
|--------|-----------|
| `X-Forwarded-For` | Appends client IP to any existing chain |
| `X-Real-IP` | Original client IP — **not overwritten** if already set upstream |
| `X-Forwarded-Proto` | Upstream value **preserved** if already present; defaults to `http` |
| `X-Forwarded-Host` | Original `Host` header value (always set) |
