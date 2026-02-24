# Admin Endpoint Authentication

Protect admin endpoints (`/_status`, `/_status/api`, `/_status/wake`, `/_metrics`) with optional **Basic Auth** or **Bearer Token** authentication. Disabled by default.

---

## Available Methods

| Method | Header | Use case |
|--------|--------|----------|
| `none` (default) | — | No authentication, open access |
| `basic` | `Authorization: Basic <base64>` | Browser-friendly login dialog, dashboard access |
| `bearer` | `Authorization: Bearer <token>` | Machine-to-machine, Prometheus scraping |

---

## Configuration

### Via `config.yaml`

**Basic Auth:**

```yaml
gateway:
  admin_auth:
    method: "basic"
    username: "admin"
    password: "s3cret-passw0rd"
```

**Bearer Token:**

```yaml
gateway:
  admin_auth:
    method: "bearer"
    token: "my-super-secret-token"
```

**Disabled (default):**

```yaml
gateway:
  admin_auth:
    method: "none"
```

> [!TIP]
> You can omit the `admin_auth` block entirely — the gateway defaults to `method: "none"`.

### Via Environment Variables

Environment variables **override** `config.yaml` values:

| Variable | Description |
|----------|-------------|
| `ADMIN_AUTH_METHOD` | `none`, `basic`, or `bearer` |
| `ADMIN_AUTH_USERNAME` | Username (required for `basic`) |
| `ADMIN_AUTH_PASSWORD` | Password (required for `basic`) |
| `ADMIN_AUTH_TOKEN` | Token (required for `bearer`) |

### Docker Compose Example

```yaml
services:
  gateway:
    build: .
    ports:
      - "8080:8080"
    environment:
      - ADMIN_AUTH_METHOD=bearer
      - ADMIN_AUTH_TOKEN=my-super-secret-token
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./config.yaml:/etc/gateway/config.yaml:ro
```

---

## What's Protected

| Endpoint | Protected | Reason |
|----------|-----------|--------|
| `/_status` | ✅ | Dashboard — exposes container names, status, images |
| `/_status/api` | ✅ | JSON with container details |
| `/_status/wake` | ✅ | Privileged action — starts containers |
| `/_metrics` | ✅ | Prometheus metrics — reveals internal architecture |
| `/_health` | ❌ | Used by loading page JS |
| `/_logs` | ❌ | Used by loading page JS |
| `/` (proxy) | ❌ | End-user traffic to managed containers |

---

## Usage Examples

### Accessing the dashboard (Basic Auth)

With Basic Auth enabled, the browser automatically shows a login dialog when you visit `http://gateway:8080/_status`.

Via `curl`:

```bash
curl -u admin:s3cret-passw0rd http://gateway:8080/_status/api
```

### Prometheus scraping (Bearer Token)

Configure your `prometheus.yml` to include the token:

```yaml
scrape_configs:
  - job_name: "docker-gateway"
    bearer_token: "my-super-secret-token"
    static_configs:
      - targets: ["gateway:8080"]
    metrics_path: "/_metrics"
```

Via `curl`:

```bash
curl -H "Authorization: Bearer my-super-secret-token" http://gateway:8080/_metrics
```

---

## Security Notes

> [!WARNING]
> Basic Auth and Bearer Token transmit credentials **in cleartext** over HTTP.
> In production, always place a TLS-terminating reverse proxy (Nginx, Caddy, Traefik) in front of the gateway.
> This is consistent with the gateway's design: "HTTP only — TLS expected to be handled by an upstream proxy".

- Credential comparison uses **constant-time algorithms** (`crypto/subtle`) to prevent timing attacks.
- Failed authentication attempts are logged with source IP and path (credentials are **never** logged).
- Changing auth settings requires a **gateway restart** — `SIGHUP` hot-reload does not update authentication configuration. This is intentional for security (avoids partial credential update windows).
