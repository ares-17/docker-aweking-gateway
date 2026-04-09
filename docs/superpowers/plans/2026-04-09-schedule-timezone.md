# Schedule Timezone Support — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a global `schedule_timezone` config field (+ `SCHEDULE_TIMEZONE` env var) so cron expressions are interpreted in the operator's local timezone instead of the container's UTC.

**Architecture:** A `ScheduleTimezone` string is added to `GlobalConfig`, validated with `time.LoadLocation` in `Validate()`, then threaded as a `tzName string` parameter to the three functions that parse cron expressions. A `cronExpr(expr, tzName)` helper prepends the `CRON_TZ=<tz>` prefix that robfig/cron v3 already supports natively. The `Server` stores the current timezone name alongside its config and passes it to `IsInScheduleWindow` and `Sync` on every reload.

**Tech Stack:** Go, robfig/cron v3 (`vendor/github.com/robfig/cron/v3`), `time.LoadLocation` (stdlib)

> **Note on Docker labels:** `dag.schedule_timezone` as a per-container label is not implemented — `ScheduleTimezone` is a global gateway setting, settable only via `config.yaml` or `SCHEDULE_TIMEZONE` env var.

---

### Task 1: `resolveLocation` helper + `GlobalConfig` field + env var + validation

**Files:**
- Modify: `gateway/config.go`
- Test: `gateway/config_test.go`

- [ ] **Step 1: Write failing tests**

In `gateway/config_test.go`, add a new test function after the existing schedule tests:

```go
func TestScheduleTimezone(t *testing.T) {
    base := func() *GatewayConfig {
        return &GatewayConfig{
            Gateway: GlobalConfig{Port: "8080"},
            Containers: []ContainerConfig{
                {Name: "app", Host: "app.local", TargetPort: "80"},
            },
        }
    }

    t.Run("empty timezone is valid (uses time.Local)", func(t *testing.T) {
        cfg := base()
        cfg.Gateway.ScheduleTimezone = ""
        if err := cfg.Validate(); err != nil {
            t.Errorf("unexpected error: %v", err)
        }
    })

    t.Run("valid IANA timezone accepted", func(t *testing.T) {
        cfg := base()
        cfg.Gateway.ScheduleTimezone = "Europe/Rome"
        if err := cfg.Validate(); err != nil {
            t.Errorf("unexpected error: %v", err)
        }
    })

    t.Run("invalid timezone returns error", func(t *testing.T) {
        cfg := base()
        cfg.Gateway.ScheduleTimezone = "Not/ATimezone"
        if err := cfg.Validate(); err == nil {
            t.Error("expected error for invalid timezone, got nil")
        }
    })
}

func TestResolveLocation(t *testing.T) {
    t.Run("empty string returns time.Local", func(t *testing.T) {
        loc, err := resolveLocation("")
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if loc != time.Local {
            t.Errorf("expected time.Local, got %v", loc)
        }
    })

    t.Run("valid IANA name returns location", func(t *testing.T) {
        loc, err := resolveLocation("Europe/Rome")
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if loc.String() != "Europe/Rome" {
            t.Errorf("expected Europe/Rome, got %v", loc)
        }
    })

    t.Run("invalid name returns error", func(t *testing.T) {
        _, err := resolveLocation("Not/ATimezone")
        if err == nil {
            t.Error("expected error for invalid timezone")
        }
    })
}

func TestLoadConfig_ScheduleTimezoneEnvVar(t *testing.T) {
    yamlContent := `
gateway:
  port: "8080"
containers:
  - name: "app"
    host: "app.local"
    target_port: "80"
`
    tmp := t.TempDir()
    path := filepath.Join(tmp, "config.yaml")
    if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
        t.Fatal(err)
    }
    t.Setenv("CONFIG_PATH", path)
    t.Setenv("SCHEDULE_TIMEZONE", "Europe/Rome")

    cfg, err := LoadConfig()
    if err != nil {
        t.Fatalf("LoadConfig() error: %v", err)
    }
    if cfg.Gateway.ScheduleTimezone != "Europe/Rome" {
        t.Errorf("ScheduleTimezone = %q, want %q", cfg.Gateway.ScheduleTimezone, "Europe/Rome")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./gateway/ -run "TestScheduleTimezone|TestResolveLocation|TestLoadConfig_ScheduleTimezoneEnvVar" -v -mod=vendor
```
Expected: FAIL — `ScheduleTimezone` field and `resolveLocation` do not exist yet.

- [ ] **Step 3: Add `ScheduleTimezone` to `GlobalConfig` and implement `resolveLocation`**

In `gateway/config.go`:

Add the field to `GlobalConfig` (after the `AdminAuth` field):
```go
// ScheduleTimezone is the IANA timezone name used to interpret schedule_start
// and schedule_stop cron expressions (e.g. "Europe/Rome", "America/New_York").
// Default: "" uses the process's local timezone (time.Local).
// Overridable via SCHEDULE_TIMEZONE env var.
ScheduleTimezone string `yaml:"schedule_timezone"`
```

Add the helper (after the `LoadConfig` function):
```go
// resolveLocation parses an IANA timezone name and returns the corresponding
// *time.Location. Returns time.Local when name is empty.
func resolveLocation(name string) (*time.Location, error) {
	if name == "" {
		return time.Local, nil
	}
	return time.LoadLocation(name)
}
```

In `LoadConfig`, add env var handling after the `ADMIN_AUTH_TOKEN` block:
```go
if envTZ := os.Getenv("SCHEDULE_TIMEZONE"); envTZ != "" {
    cfg.Gateway.ScheduleTimezone = envTZ
}
```

In `Validate()`, add timezone validation after the `admin_auth` switch block (before `seenNames`):
```go
if _, err := resolveLocation(c.Gateway.ScheduleTimezone); err != nil {
    return fmt.Errorf("schedule_timezone: invalid IANA timezone %q: %w", c.Gateway.ScheduleTimezone, err)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./gateway/ -run "TestScheduleTimezone|TestResolveLocation|TestLoadConfig_ScheduleTimezoneEnvVar" -v -mod=vendor
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add gateway/config.go gateway/config_test.go
git commit -m "feat: add schedule_timezone to GlobalConfig with env var and validation"
```

---

### Task 2: `cronExpr` helper + update `validateScheduleCompatibility` and `IsInScheduleWindow`

**Files:**
- Modify: `gateway/scheduler.go`
- Modify: `gateway/scheduler_test.go`

- [ ] **Step 1: Write failing tests**

In `gateway/scheduler_test.go`, add tests for `cronExpr` and the timezone-aware window check:

```go
func TestCronExpr(t *testing.T) {
    tests := []struct {
        expr   string
        tzName string
        want   string
    }{
        {"0 8 * * *", "", "0 8 * * *"},
        {"0 8 * * *", "Europe/Rome", "CRON_TZ=Europe/Rome 0 8 * * *"},
        {"", "Europe/Rome", ""},
    }
    for _, tt := range tests {
        got := cronExpr(tt.expr, tt.tzName)
        if got != tt.want {
            t.Errorf("cronExpr(%q, %q) = %q, want %q", tt.expr, tt.tzName, got, tt.want)
        }
    }
}

func TestIsInScheduleWindowWithTimezone(t *testing.T) {
    // 10:10 Rome time (CEST, UTC+2) = 08:10 UTC
    // stop: "0 8 * * *" Rome = 08:00 Rome = 06:00 UTC — fired 2h10m ago UTC
    // start: "0 11 * * *" Rome = 11:00 Rome = 09:00 UTC — fires in ~50min UTC
    rome, err := time.LoadLocation("Europe/Rome")
    if err != nil {
        t.Fatal(err)
    }
    // 10:10 Rome time
    now := time.Date(2026, 4, 9, 10, 10, 0, 0, rome)

    cfg := ContainerConfig{
        ScheduleStart: "0 11 * * *",
        ScheduleStop:  "0 8 * * *",
    }

    t.Run("outside window with Rome timezone", func(t *testing.T) {
        allowed, nextStart := IsInScheduleWindow(&cfg, now, "Europe/Rome")
        if allowed {
            t.Error("expected blocked outside window, got allowed")
        }
        if nextStart.IsZero() {
            t.Error("expected non-zero nextStart")
        }
    })

    t.Run("inside window with Rome timezone (12:00 Rome)", func(t *testing.T) {
        noon := time.Date(2026, 4, 9, 12, 0, 0, 0, rome)
        allowed, _ := IsInScheduleWindow(&cfg, noon, "Europe/Rome")
        if !allowed {
            t.Error("expected allowed inside window at noon Rome time")
        }
    })
}
```

Also update the existing `TestIsInScheduleWindow` and `TestScheduleCompatibility` calls to pass `""` as `tzName`:

In `TestIsInScheduleWindow`, change:
```go
allowed, nextStart := IsInScheduleWindow(&tt.cfg, tt.now, "")
```

In `TestScheduleCompatibility`, change:
```go
err := validateScheduleCompatibility(tt.start, tt.stop, "")
```

In `TestScheduleManagerSync`, change:
```go
sm.Sync(containers, "")
// ...
sm.Sync(updated, "")
// ...
sm.Sync(nil, "")
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./gateway/ -run "TestCronExpr|TestIsInScheduleWindow|TestScheduleCompatibility|TestScheduleManagerSync" -v -mod=vendor
```
Expected: FAIL — wrong number of arguments.

- [ ] **Step 3: Implement `cronExpr`, update `validateScheduleCompatibility` and `IsInScheduleWindow`**

In `gateway/scheduler.go`:

Add `cronExpr` helper after the imports:
```go
// cronExpr prepends a CRON_TZ=<tzName> prefix to expr when tzName is non-empty.
// robfig/cron v3 natively parses this prefix to set the schedule's timezone.
// Returns expr unchanged when tzName is empty or expr is empty.
func cronExpr(expr, tzName string) string {
	if tzName == "" || expr == "" {
		return expr
	}
	return "CRON_TZ=" + tzName + " " + expr
}
```

Update `validateScheduleCompatibility` signature and body:
```go
func validateScheduleCompatibility(startExpr, stopExpr, tzName string) error {
	var startSched, stopSched cron.Schedule
	var err error

	if startExpr != "" {
		startSched, err = cron.ParseStandard(cronExpr(startExpr, tzName))
		if err != nil {
			return fmt.Errorf("schedule_start: invalid cron expression %q: %w", startExpr, err)
		}
	}
	if stopExpr != "" {
		stopSched, err = cron.ParseStandard(cronExpr(stopExpr, tzName))
		if err != nil {
			return fmt.Errorf("schedule_stop: invalid cron expression %q: %w", stopExpr, err)
		}
	}
    // ... rest of function unchanged
```

Update `IsInScheduleWindow` signature and body:
```go
func IsInScheduleWindow(cfg *ContainerConfig, now time.Time, tzName string) (allowed bool, nextStart time.Time) {
	if cfg.ScheduleStart == "" || cfg.ScheduleStop == "" {
		return true, time.Time{}
	}

	startSched, err1 := cron.ParseStandard(cronExpr(cfg.ScheduleStart, tzName))
	stopSched, err2 := cron.ParseStandard(cronExpr(cfg.ScheduleStop, tzName))
	if err1 != nil || err2 != nil {
		return true, time.Time{}
	}
    // ... rest of function unchanged
```

Update call site in `gateway/config.go` `Validate()`:
```go
if err := validateScheduleCompatibility(ctr.ScheduleStart, ctr.ScheduleStop, c.Gateway.ScheduleTimezone); err != nil {
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./gateway/ -run "TestCronExpr|TestIsInScheduleWindow|TestScheduleCompatibility" -v -mod=vendor
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add gateway/scheduler.go gateway/scheduler_test.go gateway/config.go
git commit -m "feat: add tzName param to validateScheduleCompatibility and IsInScheduleWindow"
```

---

### Task 3: Update `ScheduleManager.Sync` + `Server` + `main.go`

**Files:**
- Modify: `gateway/scheduler.go`
- Modify: `gateway/scheduler_test.go`
- Modify: `gateway/server.go`
- Modify: `main.go`

- [ ] **Step 1: Write failing tests for `Sync` with timezone**

In `gateway/scheduler_test.go`, add a test after `TestScheduleManagerSync`:

```go
func TestScheduleManagerSyncWithTimezone(t *testing.T) {
	sm := NewScheduleManager(nil, nil)

	containers := []ContainerConfig{
		{Name: "app", ScheduleStart: "0 8 * * *", ScheduleStop: "0 20 * * *", StartTimeout: 60 * time.Second},
	}
	sm.Sync(containers, "Europe/Rome")

	entries := sm.cron.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 cron entries, got %d", len(entries))
	}
	// Verify both entries have Rome timezone in their schedule
	// (robfig/cron stores Schedule; we verify indirectly by checking Next fires in expected range)
	rome, _ := time.LoadLocation("Europe/Rome")
	ref := time.Date(2026, 4, 9, 7, 59, 0, 0, rome) // just before 08:00 Rome
	next := entries[0].Schedule.Next(ref)
	want := time.Date(2026, 4, 9, 8, 0, 0, 0, rome)
	if !next.Equal(want) {
		t.Errorf("next firing = %v, want %v", next, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./gateway/ -run TestScheduleManagerSyncWithTimezone -v -mod=vendor
```
Expected: FAIL — `Sync` still takes one argument.

- [ ] **Step 3: Update `Sync` signature in `scheduler.go`**

Change `Sync` to accept `tzName string`:
```go
func (sm *ScheduleManager) Sync(containers []ContainerConfig, tzName string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for name, ids := range sm.entries {
		for _, id := range ids {
			sm.cron.Remove(id)
		}
		delete(sm.entries, name)
	}

	for _, c := range containers {
		if c.ScheduleStart == "" && c.ScheduleStop == "" {
			continue
		}
		cfg := c
		var ids []cron.EntryID

		if cfg.ScheduleStart != "" {
			id, err := sm.cron.AddFunc(cronExpr(cfg.ScheduleStart, tzName), func() {
				ctx, cancel := context.WithTimeout(context.Background(), cfg.StartTimeout)
				defer cancel()
				sm.manager.InitStartState(cfg.Name)
				if err := sm.manager.EnsureRunning(ctx, &cfg); err != nil {
					slog.Error("scheduled start failed", "container", cfg.Name, "error", err)
				} else {
					slog.Info("scheduled start succeeded", "container", cfg.Name)
				}
			})
			if err != nil {
				slog.Error("failed to register schedule_start", "container", cfg.Name, "error", err)
				continue
			}
			ids = append(ids, id)
		}

		if cfg.ScheduleStop != "" {
			id, err := sm.cron.AddFunc(cronExpr(cfg.ScheduleStop, tzName), func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := sm.client.StopContainer(ctx, cfg.Name); err != nil {
					slog.Error("scheduled stop failed", "container", cfg.Name, "error", err)
				} else {
					slog.Info("scheduled stop succeeded", "container", cfg.Name)
				}
			})
			if err != nil {
				slog.Error("failed to register schedule_stop", "container", cfg.Name, "error", err)
				continue
			}
			ids = append(ids, id)
		}

		if len(ids) > 0 {
			sm.entries[cfg.Name] = ids
		}
	}
}
```

- [ ] **Step 4: Update `Server` to store and thread `tzName`**

In `gateway/server.go`, add `scheduleTZ string` field to the `Server` struct (after `scheduler`):
```go
scheduler    *ScheduleManager
scheduleTZ   string
```

In `NewServer`, set it from cfg:
```go
func NewServer(manager *ContainerManager, scheduler *ScheduleManager, cfg *GatewayConfig) (*Server, error) {
	// ... existing code ...
	s := &Server{
		// ... existing fields ...
		scheduleTZ: cfg.Gateway.ScheduleTimezone,
	}
```

In `ReloadConfig`, update both lines that use timezone:
```go
func (s *Server) ReloadConfig(newCfg *GatewayConfig) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	s.cfg = newCfg
	s.scheduleTZ = newCfg.Gateway.ScheduleTimezone
	s.hostIndex = BuildHostIndex(newCfg)
	s.groupIndex = BuildGroupHostIndex(newCfg)
	s.containerMap = BuildContainerMap(newCfg)
	s.trustedCIDRs = parseTrustedProxies(newCfg.Gateway.TrustedProxies)
	s.scheduler.Sync(newCfg.Containers, newCfg.Gateway.ScheduleTimezone)
}
```

In `handleRequest`, update the `IsInScheduleWindow` call:
```go
if allowed, nextStart := IsInScheduleWindow(cfg, time.Now(), s.scheduleTZ); !allowed {
```

Note: `s.scheduleTZ` is read inside `handleRequest` which is called while holding `configMu` via `resolveConfig`. The field is updated under `configMu` in `ReloadConfig`, so reads in `handleRequest` (also under the same lock flow) are safe.

- [ ] **Step 5: Update `main.go`**

Change the `scheduler.Sync` call:
```go
scheduler.Sync(cfg.Containers, cfg.Gateway.ScheduleTimezone)
```

- [ ] **Step 6: Run all tests**

```bash
go test ./gateway/... -mod=vendor -v
```
Expected: all PASS

- [ ] **Step 7: Build to verify no compile errors**

```bash
go build -mod=vendor -o docker-gateway .
```
Expected: success, no errors.

- [ ] **Step 8: Commit**

```bash
git add gateway/scheduler.go gateway/scheduler_test.go gateway/server.go main.go
git commit -m "feat: thread schedule_timezone through Sync, Server, and IsInScheduleWindow"
```

---

### Task 4: Update `config.yaml` test containers to use `schedule_timezone`

**Files:**
- Modify: `config.yaml`

- [ ] **Step 1: Add `schedule_timezone` to config.yaml**

In `config.yaml`, under `gateway:`, add:
```yaml
gateway:
  port: "8080"
  schedule_timezone: "Europe/Rome"
```

- [ ] **Step 2: Verify hot-reload works**

```bash
docker compose up -d --build
# Visit http://sched-offline.localhost:8080 — should show offline page
docker kill -s HUP docker-gateway
```

Expected: `sched-offline` shows "Scheduled Downtime" page; `sched-live` cron jobs fire at 10:16 and 10:17 IT.

- [ ] **Step 3: Commit**

```bash
git add config.yaml
git commit -m "chore: set schedule_timezone=Europe/Rome in test config"
```

---

## Self-Review

**Spec coverage:**
- ✅ `ScheduleTimezone string` in `GlobalConfig`
- ✅ `SCHEDULE_TIMEZONE` env var override
- ✅ `time.LoadLocation` validation in `Validate()`
- ✅ `resolveLocation` helper
- ✅ `validateScheduleCompatibility` updated
- ✅ `IsInScheduleWindow` updated
- ✅ `ScheduleManager.Sync` updated
- ✅ `Server.ReloadConfig` and boot path updated
- ✅ Tests: config, scheduler, timezone-specific
- ⚠️ Docker label `dag.schedule_timezone` intentionally skipped — timezone is global, not per-container; a per-container label cannot affect `GlobalConfig`

**Placeholder scan:** None found.

**Type consistency:** All usages of `tzName string`, `cronExpr(expr, tzName)`, `Sync(containers, tzName)`, `IsInScheduleWindow(cfg, now, tzName)`, `validateScheduleCompatibility(start, stop, tzName)` are consistent across all tasks.
