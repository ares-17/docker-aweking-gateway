# Testing Guide

The Docker Awakening Gateway includes a security-focused test suite written in pure Go, using only the standard library (`testing`, `net/http/httptest`). No external testing frameworks or dependencies are required.

---

## Running the Tests

```bash
# Run all tests with verbose output and race detector
go test -v -race ./gateway/...

# Run only security tests (by name pattern)
go test -v -race -run "TestValidateOrigin|TestRateLimiter|TestTrustedProxy|TestServerClientIP" ./gateway/...

# Short summary (no verbose)
go test -race ./gateway/...
```

> [!TIP]
> The `-race` flag enables Go's built-in race detector. All tests are designed to pass under race detection, ensuring thread-safety of the security primitives.

---

## Test Coverage

The test suite is located in `gateway/security_test.go` and covers three security domains:

### 1. CSRF / Origin Validation (`TestValidateOrigin`)

Verifies that the `validateOrigin()` function correctly blocks cross-origin POST requests while allowing legitimate same-origin and non-browser requests.

| Sub-test | Input | Expected |
|----------|-------|----------|
| No Origin header (curl/script) | `Origin: ""` | ✅ Allowed |
| Same origin | `Origin: http://localhost:8080` | ✅ Allowed |
| Cross origin | `Origin: http://evil.com` | ❌ Blocked (403) |
| Cross origin with port | `Origin: http://attacker:9999` | ❌ Blocked |
| Same host, different scheme | `Origin: https://mygateway.com` | ✅ Allowed |
| Origin with path | `Origin: http://localhost:8080/path` | ✅ Allowed |
| Malformed Origin | `Origin: ://not-a-url` | ❌ Blocked |

### 2. Rate Limiter (`TestRateLimiter_*`)

Tests the IP-based rate limiter for correctness, memory management, and goroutine lifecycle.

| Test | What it verifies |
|------|------------------|
| `TestRateLimiter_Allow` | First request allowed, immediate duplicate blocked, different IP allowed, request after interval allowed |
| `TestRateLimiter_EvictStale` | 100 IPs inserted → all evicted after becoming stale |
| `TestRateLimiter_EvictStale_KeepsFresh` | Old IP evicted while freshly-seen IP is preserved |
| `TestRateLimiter_StartCleanup` | Background goroutine auto-cleans stale entries; context cancellation stops the goroutine cleanly |

### 3. Trusted Proxy (`TestParseTrustedProxies`, `TestIsTrustedProxy*`, `TestServerClientIP`)

Tests the CIDR-based trusted proxy system that controls when `X-Forwarded-For` is trusted for rate limiting.

| Test | What it verifies |
|------|------------------|
| `TestParseTrustedProxies` | Valid CIDRs parsed, invalid CIDRs skipped with warning, empty input returns nil |
| `TestIsTrustedProxy` | IP in range → true, IP outside all ranges → false, empty/malformed IP → false |
| `TestIsTrustedProxy_EmptyCIDRs` | Nil or empty CIDR list → nothing is trusted |
| `TestServerClientIP` | Full integration: XFF ignored without trusted proxies, XFF used when RemoteAddr is trusted, XFF chain extracts first IP |

---

## Docker Image Safety

Test files have **zero impact** on the production Docker image:

1. **Go toolchain**: Files ending in `_test.go` are **automatically excluded** from `go build` — this is a built-in Go compiler behaviour, not a convention.
2. **`.dockerignore`**: The project's `.dockerignore` explicitly excludes `*_test.go` from the Docker build context, so test files are never even sent to the Docker daemon during image builds.

This means you can safely add as many `_test.go` files as needed without affecting the final image size (~17 MB).

---

## Writing New Tests

When adding tests, follow these conventions:

1. **Table-driven tests** — Use `[]struct{}` with `t.Run()` for each case.
2. **Standard library only** — Avoid external assertion libraries; use `t.Errorf` / `t.Fatalf`.
3. **Race-safe** — All tests must pass with `-race`. Use proper synchronization in time-dependent tests.
4. **Naming** — Test functions should be `Test<FunctionName>` or `Test<FunctionName>_<Scenario>`.

```go
func TestMyFunction(t *testing.T) {
    tests := []struct {
        name string
        input string
        want  bool
    }{
        {"valid input", "foo", true},
        {"empty input", "", false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := MyFunction(tt.input); got != tt.want {
                t.Errorf("MyFunction(%q) = %v, want %v", tt.input, got, tt.want)
            }
        })
    }
}
```
