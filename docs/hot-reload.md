# Configuration Hot-Reload

The Docker Awakening Gateway supports updating its routing configuration at runtime without process restarts or dropping active connections.

## How to trigger a reload

The gateway listens for the `SIGHUP` signal. You can trigger it via Docker:

```bash
docker kill -s HUP docker-gateway
```

When a `SIGHUP` is received:
1. The `config.yaml` file is re-read from disk.
2. A new auto-discovery pass is immediately triggered for Docker labels.
3. The internal routing index (Host mapping) is updated safely.

---

## What IS reloaded

The following settings are updated dynamically:

- **Static Container Mappings**: Adding/removing containers under `containers:` in `config.yaml`.
- **Group Definitions**: Changes to `groups:` and load-balancing memberships.
- **Auto-Discovery Results**: Any changes to Docker labels on your containers.
- **Trusted Proxies**: Changes to the `trusted_proxies` CIDR list for rate-limiting.

---

## What is NOT reloaded

Certain global gateway settings are bound at startup and require a **container restart** to change:

| Setting | Reason |
|---------|--------|
| `gateway.port` | The TCP socket is opened at startup. Moving it requires a process restart. |
| `gateway.admin_auth` | Authentication middleware is applied to routes during initialization. |
| **Environmental Overrides** | Standard process behavior; environment variables are read once at startup. |

> [!NOTE]
> If a hot-reload fails (e.g., due to a syntax error in the new `config.yaml`), the gateway will log an error and continue running with its **previously valid configuration**.
