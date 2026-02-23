package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ─── validateOrigin ───────────────────────────────────────────────────────────

func TestValidateOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string // Origin header value ("" = absent)
		host   string // request Host header
		want   bool
	}{
		{
			name: "no origin header (curl/script) → allowed",
			host: "localhost:8080",
			want: true,
		},
		{
			name:   "same origin → allowed",
			origin: "http://localhost:8080",
			host:   "localhost:8080",
			want:   true,
		},
		{
			name:   "cross origin → blocked",
			origin: "http://evil.com",
			host:   "localhost:8080",
			want:   false,
		},
		{
			name:   "cross origin with port → blocked",
			origin: "http://attacker.local:9999",
			host:   "gateway.local:8080",
			want:   false,
		},
		{
			name:   "same host different scheme → allowed (host match)",
			origin: "https://mygateway.com",
			host:   "mygateway.com",
			want:   true,
		},
		{
			name:   "origin with path → allowed if host matches",
			origin: "http://localhost:8080/some/path",
			host:   "localhost:8080",
			want:   true,
		},
		{
			name:   "malformed origin → blocked",
			origin: "://not-a-url",
			host:   "localhost:8080",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://"+tt.host+"/_status/wake", nil)
			r.Host = tt.host
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if got := validateOrigin(r); got != tt.want {
				t.Errorf("validateOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ─── rateLimiter ──────────────────────────────────────────────────────────────

func TestRateLimiter_Allow(t *testing.T) {
	rl := newRateLimiter(100 * time.Millisecond)

	// First request from an IP should always be allowed
	if !rl.Allow("10.0.0.1") {
		t.Fatal("first request should be allowed")
	}

	// Immediate second request from same IP should be blocked
	if rl.Allow("10.0.0.1") {
		t.Fatal("immediate second request should be rate-limited")
	}

	// Different IP should be allowed
	if !rl.Allow("10.0.0.2") {
		t.Fatal("first request from different IP should be allowed")
	}

	// Wait for interval to expire
	time.Sleep(120 * time.Millisecond)

	// Now the original IP should be allowed again
	if !rl.Allow("10.0.0.1") {
		t.Fatal("request after interval should be allowed")
	}
}

func TestRateLimiter_EvictStale(t *testing.T) {
	rl := newRateLimiter(50 * time.Millisecond)

	// Populate with several IPs
	for i := 0; i < 100; i++ {
		rl.Allow("192.168.0." + string(rune('0'+i%10)))
	}

	// Verify map has entries
	rl.mu.Lock()
	before := len(rl.lastSeen)
	rl.mu.Unlock()
	if before == 0 {
		t.Fatal("expected entries in lastSeen map")
	}

	// Wait for entries to become stale (2× interval = 100ms)
	time.Sleep(120 * time.Millisecond)

	rl.evictStale()

	rl.mu.Lock()
	after := len(rl.lastSeen)
	rl.mu.Unlock()
	if after != 0 {
		t.Errorf("expected 0 entries after eviction, got %d", after)
	}
}

func TestRateLimiter_EvictStale_KeepsFresh(t *testing.T) {
	rl := newRateLimiter(50 * time.Millisecond) // cutoff = 2×50ms = 100ms

	rl.Allow("old-ip")
	time.Sleep(120 * time.Millisecond) // old-ip is now stale (>100ms)

	rl.Allow("fresh-ip") // fresh-ip just recorded

	rl.evictStale()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if _, exists := rl.lastSeen["old-ip"]; exists {
		t.Error("old-ip should have been evicted")
	}
	if _, exists := rl.lastSeen["fresh-ip"]; !exists {
		t.Error("fresh-ip should have been kept")
	}
}

func TestRateLimiter_StartCleanup(t *testing.T) {
	rl := newRateLimiter(10 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl.startCleanup(ctx, 50*time.Millisecond)

	rl.Allow("auto-clean-ip")

	// Wait long enough for at least one cleanup pass
	time.Sleep(100 * time.Millisecond)

	rl.mu.Lock()
	count := len(rl.lastSeen)
	rl.mu.Unlock()

	if count != 0 {
		t.Errorf("expected auto-cleanup to evict stale entries, got %d remaining", count)
	}

	// Verify cancellation stops the goroutine (no panic/leak)
	cancel()
	time.Sleep(20 * time.Millisecond)
}

// ─── Trusted Proxy ────────────────────────────────────────────────────────────

func TestParseTrustedProxies(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantLen int
	}{
		{
			name:    "empty list",
			input:   nil,
			wantLen: 0,
		},
		{
			name:    "valid CIDRs",
			input:   []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
			wantLen: 3,
		},
		{
			name:    "invalid CIDR is skipped",
			input:   []string{"10.0.0.0/8", "not-a-cidr", "192.168.0.0/16"},
			wantLen: 2,
		},
		{
			name:    "all invalid",
			input:   []string{"garbage", "also-garbage"},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTrustedProxies(tt.input)
			if len(got) != tt.wantLen {
				t.Errorf("parseTrustedProxies() returned %d CIDRs, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestIsTrustedProxy(t *testing.T) {
	cidrs := parseTrustedProxies([]string{"10.0.0.0/8", "172.16.0.0/12"})

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"IP in 10.x range", "10.0.0.5", true},
		{"IP in 172.16.x range", "172.20.0.1", true},
		{"IP outside all ranges", "8.8.8.8", false},
		{"IP in 192.168.x (not configured)", "192.168.1.1", false},
		{"empty IP", "", false},
		{"malformed IP", "not-an-ip", false},
		{"loopback", "127.0.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTrustedProxy(tt.ip, cidrs); got != tt.want {
				t.Errorf("isTrustedProxy(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestIsTrustedProxy_EmptyCIDRs(t *testing.T) {
	// With no trusted CIDRs, nothing should be trusted
	if isTrustedProxy("10.0.0.1", nil) {
		t.Error("expected false with nil CIDRs")
	}
	if isTrustedProxy("10.0.0.1", parseTrustedProxies(nil)) {
		t.Error("expected false with empty CIDRs")
	}
}

// ─── Server.clientIP integration ──────────────────────────────────────────────

func TestServerClientIP(t *testing.T) {
	tests := []struct {
		name           string
		remoteAddr     string
		xff            string
		trustedProxies []string
		wantIP         string
	}{
		{
			name:       "no trusted proxies → use RemoteAddr",
			remoteAddr: "1.2.3.4:12345",
			xff:        "5.6.7.8",
			wantIP:     "1.2.3.4",
		},
		{
			name:           "trusted proxy + XFF → use XFF",
			remoteAddr:     "10.0.0.1:12345",
			xff:            "5.6.7.8",
			trustedProxies: []string{"10.0.0.0/8"},
			wantIP:         "5.6.7.8",
		},
		{
			name:           "untrusted source + XFF → ignore XFF",
			remoteAddr:     "8.8.8.8:12345",
			xff:            "1.1.1.1",
			trustedProxies: []string{"10.0.0.0/8"},
			wantIP:         "8.8.8.8",
		},
		{
			name:           "trusted proxy but no XFF → use RemoteAddr",
			remoteAddr:     "10.0.0.1:12345",
			xff:            "",
			trustedProxies: []string{"10.0.0.0/8"},
			wantIP:         "10.0.0.1",
		},
		{
			name:           "XFF chain → take first IP",
			remoteAddr:     "10.0.0.1:12345",
			xff:            "203.0.113.50, 70.41.3.18, 150.172.238.178",
			trustedProxies: []string{"10.0.0.0/8"},
			wantIP:         "203.0.113.50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				cfg: &GatewayConfig{
					Gateway: GlobalConfig{
						TrustedProxies: tt.trustedProxies,
					},
				},
				trustedCIDRs: parseTrustedProxies(tt.trustedProxies),
			}

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}

			got := s.clientIP(r)
			if got != tt.wantIP {
				t.Errorf("clientIP() = %q, want %q", got, tt.wantIP)
			}
		})
	}
}
