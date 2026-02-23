# Testing Guide

The Docker Awakening Gateway includes a comprehensive test suite written in pure Go, using only the standard library (`testing`, `net/http/httptest`). No external testing frameworks or dependencies are required.

---

## Running the Tests

```bash
# Run all tests with verbose output and race detector
go test -v -race ./gateway/...

# Run only security tests
go test -v -race -run "TestValidateOrigin|TestRateLimiter|TestTrustedProxy|TestServerClientIP" ./gateway/...

# Run only config tests
go test -v -race -run "TestApplyDefaults|TestValidate|TestBuildHostIndex|TestLoadConfig" ./gateway/...

# Short summary (no verbose)
go test -race ./gateway/...
```

> [!TIP]
> The `-race` flag enables Go's built-in race detector. All tests are designed to pass under race detection, ensuring thread-safety of the security primitives and concurrent state management.

---

## Test Files Overview

| File | Target | Tests | Sub-tests |
|------|--------|------:|----------:|
| `security_test.go` | CSRF, rate limiter, trusted proxy | 10 | 26 |
| `config_test.go` | YAML loading, defaults, validation, host index | 7 | 22 |
| `discovery_test.go` | Config merging, conflict resolution, thread-safety | 3 | 10 |
| `server_test.go` | Routing, WebSocket detection, proxy headers, request ID | 5 | 21 |
| `docker_test.go` | Log header stripping, network name joining | 2 | 8 |
| `manager_test.go` | State lifecycle, activity tracking, per-container locks | 4 | 11 |
| **Total** | | **31** | **~98** |

---

## Test Coverage by Domain

### Security (`security_test.go`)

| Test | What it verifies |
|------|------------------|
| `TestValidateOrigin` | CSRF: same-origin ✅, cross-origin ❌, no Origin ✅, malformed ❌ |
| `TestRateLimiter_Allow` | First request allowed, duplicate blocked, different IP allowed, post-interval allowed |
| `TestRateLimiter_EvictStale` | 100 IPs inserted → all evicted after becoming stale |
| `TestRateLimiter_EvictStale_KeepsFresh` | Stale IP evicted, fresh IP preserved |
| `TestRateLimiter_StartCleanup` | Background cleanup goroutine + context cancellation |
| `TestParseTrustedProxies` | Valid CIDRs parsed, invalid skipped, empty returns nil |
| `TestIsTrustedProxy` | IP in range, out of range, empty, malformed, loopback |
| `TestServerClientIP` | XFF with/without trusted proxy, XFF chain extraction |

### Configuration (`config_test.go`)

| Test | What it verifies |
|------|------------------|
| `TestApplyDefaults` | All default values applied; explicit values never overridden |
| `TestValidate` | Missing fields, duplicates, empty config, multi-container configs |
| `TestBuildHostIndex` | O(1) lookup, empty hosts excluded, unknown hosts return nil |
| `TestLoadConfig_*` | Missing file, invalid YAML, valid parse, validation failure |

### Discovery (`discovery_test.go`)

| Test | What it verifies |
|------|------------------|
| `TestMergeConfigs` | 8 conflict scenarios: static wins, name/host dedup, empty configs |
| `TestMergeConfigs_ConcurrentAccess` | Thread-safety of merge under concurrent access |
| `TestMergeConfigs_PreservesFields` | All container fields survive the merge unchanged |

### Server (`server_test.go`)

| Test | What it verifies |
|------|------------------|
| `TestIsWebSocketRequest` | Upgrade detection, case sensitivity, partial headers |
| `TestSetForwardedHeaders` | XFF append, X-Real-IP preservation, X-Forwarded-Proto/Host |
| `TestRequestID` | Prefix format, hex suffix, uniqueness |
| `TestMetricsResponseWriter` | Status code capture, default 200, proxy to underlying writer |
| `TestResolveConfig` | Host matching, port stripping, query param fallback |

### Docker (`docker_test.go`)

| Test | What it verifies |
|------|------------------|
| `TestStripDockerLogHeaders` | Single/multi frame, empty input, truncated frames |
| `TestJoinNetworkNames` | Nil map handling |

### Manager (`manager_test.go`)

| Test | What it verifies |
|------|------------------|
| `TestStartStateLifecycle` | unknown → starting → running → failed transitions |
| `TestRecordActivity` | Visibility, timestamp ordering |
| `TestGetLock` | Same name → same mutex, different names → different mutexes |
| `TestStartState_ConcurrentAccess` | Race-free concurrent state reads/writes |

---

## Docker Image Safety

Test files have **zero impact** on the production Docker image:

1. **Go toolchain**: Files ending in `_test.go` are **automatically excluded** from `go build`.
2. **`.dockerignore`**: Explicitly excludes `*_test.go` from the Docker build context.

---

## Writing New Tests

Follow these conventions:

1. **Table-driven tests** — Use `[]struct{}` with `t.Run()` for each case.
2. **Standard library only** — Use `t.Errorf` / `t.Fatalf`, no external assertion libraries.
3. **Race-safe** — All tests must pass with `-race`.
4. **Naming** — `Test<FunctionName>` or `Test<FunctionName>_<Scenario>`.
5. **File naming** — Place tests in `gateway/<source>_test.go` matching the source file.
