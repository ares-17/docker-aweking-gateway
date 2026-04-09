# Schedule Timezone Support — Design Spec

**Date:** 2026-04-09  
**Status:** Approved

## Problem

`cron.New()` uses `time.Local`, which is UTC inside a Docker container. Users writing cron expressions in local time (e.g. Italian, UTC+2) get silent misfires — the gateway fires 2 hours late or shows the wrong offline window.

## Goal

Allow operators to declare a single global timezone for all `schedule_start` / `schedule_stop` expressions so they can write cron times in their local time without UTC mental math.

## Config

New field in `GlobalConfig`:

```go
ScheduleTimezone string `yaml:"schedule_timezone"`
```

Example `config.yaml`:

```yaml
gateway:
  port: "8080"
  schedule_timezone: "Europe/Rome"
```

- Default: `""` → `time.Local` (backward-compatible, no breaking change)
- Env var override: `SCHEDULE_TIMEZONE` — consistent with existing `ADMIN_AUTH_*` / `DISCOVERY_INTERVAL` pattern
- Validated at load time via `time.LoadLocation`; invalid value → fatal config error

## Architecture

`Validate()` resolves `ScheduleTimezone → *time.Location` once and passes it to all callers that parse cron expressions. The location is threaded as a parameter rather than stored globally.

### Affected functions

| Function | Change |
|---|---|
| `validateScheduleCompatibility(start, stop string)` | Add `loc *time.Location` param; prefix expressions with `CRON_TZ=<tz>` before parsing |
| `IsInScheduleWindow(cfg, now)` | Add `loc *time.Location` param; same prefix strategy |
| `ScheduleManager.Sync(containers)` | Add `loc *time.Location` param; prefix expressions before `sm.cron.AddFunc` |
| `Server.ReloadConfig` / boot path | Resolve `loc` from config, pass to `Sync` and gate check |

robfig/cron v3 natively supports the `CRON_TZ=<tz>` prefix in expression strings, so prefixing is zero-dependency.

### Location resolution helper

```go
// resolveLocation returns the *time.Location for the given IANA name.
// Returns time.Local if name is empty.
func resolveLocation(name string) (*time.Location, error)
```

Called once in `Validate()`; the result is passed downstream — no repeated `LoadLocation` calls at request time.

## Docker Label

`dag.schedule_timezone` supported in `docker.go` label discovery, consistent with `dag.schedule_start` / `dag.schedule_stop`.

## Testing

- `scheduler_test.go`: existing tests use explicit `time.UTC` — no breakage. New case: `loc = Europe/Rome`, verifies gate correctly blocks/allows in local time.
- `config_test.go`: invalid timezone string → validation error; valid timezone → propagated correctly.
- `docker.go` label parsing: `dag.schedule_timezone` round-trip.

## Non-Goals

- Per-container timezone override (not needed; global covers the use case)
- Changing the `cron.Cron` instance timezone via `cron.WithLocation` (the `CRON_TZ=` prefix approach is simpler and already supported)
