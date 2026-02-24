# Container Grouping, Dependencies & Load Balancing

The gateway supports **dependency-ordered startup** and **round-robin load balancing** across container groups. These features let you model production-like topologies (e.g., a web app that depends on a database, or an API cluster with multiple replicas behind one host).

---

## Container Dependencies (`depends_on`)

Any container can declare dependencies. When a request arrives, the gateway starts dependencies **in topological order** — each must pass its readiness probe before the next begins.

### YAML Configuration

```yaml
containers:
  - name: "web-app"
    host: "app.localhost"
    target_port: "3000"
    depends_on: ["postgres", "redis"]   # ← started first, in order

  - name: "postgres"
    target_port: "5432"
    # No `host` needed — dependency-only containers
    # just need to be running

  - name: "redis"
    target_port: "6379"
```

### Docker Labels

```yaml
labels:
  - "dag.enabled=true"
  - "dag.host=app.localhost"
  - "dag.depends_on=postgres,redis"
```

### How it works

```
Request → app.localhost
   │
   ├─ Is "postgres" running? No → docker start postgres → wait for readiness
   ├─ Is "redis" running?     No → docker start redis    → wait for readiness
   └─ Is "web-app" running?   No → docker start web-app  → wait for readiness
   │
   └─ All ready → proxy request to web-app:3000
```

### Rules

- Dependencies are resolved via **topological sort** (DFS) — diamond and chain shapes are fully supported.
- **Cycles are detected** at config validation time and rejected with a clear error message.
- Dependencies that are **already running** are skipped (no unnecessary restarts).
- A dependency **does not need a `host` field** — it only needs `name` and `target_port`.
- If any dependency fails to start, the entire startup sequence is aborted.

---

## Container Groups (Load Balancing)

A **group** maps a single host to multiple containers and distributes requests via round-robin.

### YAML Configuration

```yaml
groups:
  - name: "api-cluster"
    host: "api.localhost"
    strategy: "round-robin"     # default
    containers: ["api-1", "api-2", "api-3"]

containers:
  - name: "api-1"
    target_port: "8080"
    depends_on: ["postgres"]

  - name: "api-2"
    target_port: "8080"
    depends_on: ["postgres"]

  - name: "api-3"
    target_port: "8080"
    depends_on: ["postgres"]

  - name: "postgres"
    target_port: "5432"
```

### How it works

```
Request → api.localhost   (round-robin counter: 0)
   │
   ├─ Resolve dependencies for all group members
   │    └─ postgres: running? Yes → skip
   │
   ├─ Start all group members (api-1, api-2, api-3)
   │    └─ Each must pass readiness probe
   │
   └─ Pick api-1 (counter % 3 = 0) → proxy request
```

Subsequent requests cycle through: `api-1` → `api-2` → `api-3` → `api-1` → ...

### Configuration Reference

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | ✅ | — | Unique group identifier |
| `host` | ✅ | — | Host header to match incoming requests |
| `strategy` | ❌ | `round-robin` | Load balancing algorithm |
| `containers` | ✅ | — | List of container names in this group |

### Rules

- All containers listed in `containers` must be defined in the `containers[]` array.
- Group hosts **must not conflict** with container hosts or other group hosts.
- Group members don't need their own `host` field (routing is via the group's host).
- When the group is triggered, **all members + their dependencies** are started.
- Each group member manages its own `idle_timeout` independently.

---

## Combining Groups and Dependencies

Groups and dependencies compose naturally:

```yaml
groups:
  - name: "web-cluster"
    host: "web.localhost"
    containers: ["web-1", "web-2"]

containers:
  - name: "web-1"
    target_port: "3000"
    depends_on: ["api", "postgres"]

  - name: "web-2"
    target_port: "3000"
    depends_on: ["api", "postgres"]

  - name: "api"
    target_port: "8080"
    depends_on: ["postgres"]

  - name: "postgres"
    target_port: "5432"
```

Startup order: `postgres` → `api` → `web-1` + `web-2` (all probed before traffic flows).

---

## Validation

The gateway validates the configuration at startup and rejects:

| Error | Message |
|-------|---------|
| Circular dependency | `dependency cycle detected: a → b → a` |
| Self-dependency | `container "app" cannot depend on itself` |
| Unknown dependency | `container "app" depends on unknown container "missing"` |
| Empty group | `group "api" has no containers` |
| Unknown group member | `group "api" references unknown container "unknown"` |
| Host conflict | `group "api" host "app.local" conflicts with an existing host` |
| Duplicate group name | `duplicate group name found: "api"` |
