package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ─── isWebSocketRequest ───────────────────────────────────────────────────────

func TestIsWebSocketRequest(t *testing.T) {
	tests := []struct {
		name       string
		upgrade    string
		connection string
		want       bool
	}{
		{
			name:       "valid WebSocket upgrade",
			upgrade:    "websocket",
			connection: "upgrade",
			want:       true,
		},
		{
			name:       "case insensitive headers",
			upgrade:    "WebSocket",
			connection: "Upgrade",
			want:       true,
		},
		{
			name:       "connection with keep-alive",
			upgrade:    "websocket",
			connection: "keep-alive, Upgrade",
			want:       true,
		},
		{
			name:       "only Upgrade header (no Connection)",
			upgrade:    "websocket",
			connection: "",
			want:       false,
		},
		{
			name:       "only Connection header (no Upgrade)",
			upgrade:    "",
			connection: "upgrade",
			want:       false,
		},
		{
			name:       "normal HTTP request",
			upgrade:    "",
			connection: "",
			want:       false,
		},
		{
			name:       "upgrade to something else",
			upgrade:    "h2c",
			connection: "upgrade",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.upgrade != "" {
				r.Header.Set("Upgrade", tt.upgrade)
			}
			if tt.connection != "" {
				r.Header.Set("Connection", tt.connection)
			}
			if got := isWebSocketRequest(r); got != tt.want {
				t.Errorf("isWebSocketRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ─── setForwardedHeaders ──────────────────────────────────────────────────────

func TestSetForwardedHeaders(t *testing.T) {
	t.Run("sets all headers for fresh request", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)
		r.RemoteAddr = "192.168.1.100:12345"
		r.Host = "example.com"

		setForwardedHeaders(r, "10.0.0.5")

		if got := r.Header.Get("X-Forwarded-For"); got != "192.168.1.100" {
			t.Errorf("X-Forwarded-For = %q, want %q", got, "192.168.1.100")
		}
		if got := r.Header.Get("X-Real-IP"); got != "192.168.1.100" {
			t.Errorf("X-Real-IP = %q, want %q", got, "192.168.1.100")
		}
		if got := r.Header.Get("X-Forwarded-Proto"); got != "http" {
			t.Errorf("X-Forwarded-Proto = %q, want %q", got, "http")
		}
		if got := r.Header.Get("X-Forwarded-Host"); got != "example.com" {
			t.Errorf("X-Forwarded-Host = %q, want %q", got, "example.com")
		}
	})

	t.Run("appends to existing X-Forwarded-For chain", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.0.0.1:9999"
		r.Header.Set("X-Forwarded-For", "203.0.113.50")

		setForwardedHeaders(r, "10.0.0.5")

		got := r.Header.Get("X-Forwarded-For")
		if !strings.Contains(got, "203.0.113.50") || !strings.Contains(got, "10.0.0.1") {
			t.Errorf("X-Forwarded-For = %q, should contain both IPs", got)
		}
	})

	t.Run("does not overwrite existing X-Real-IP", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.0.0.1:9999"
		r.Header.Set("X-Real-IP", "original-ip")

		setForwardedHeaders(r, "10.0.0.5")

		if got := r.Header.Get("X-Real-IP"); got != "original-ip" {
			t.Errorf("X-Real-IP = %q, should remain %q", got, "original-ip")
		}
	})
}

// ─── requestID ────────────────────────────────────────────────────────────────

func TestRequestID(t *testing.T) {
	t.Run("has correct prefix", func(t *testing.T) {
		id := requestID("req")
		if !strings.HasPrefix(id, "req-") {
			t.Errorf("requestID() = %q, want prefix %q", id, "req-")
		}
	})

	t.Run("different prefix", func(t *testing.T) {
		id := requestID("err")
		if !strings.HasPrefix(id, "err-") {
			t.Errorf("requestID() = %q, want prefix %q", id, "err-")
		}
	})

	t.Run("contains hex suffix", func(t *testing.T) {
		id := requestID("test")
		parts := strings.SplitN(id, "-", 2)
		if len(parts) != 2 || len(parts[1]) == 0 {
			t.Errorf("requestID() = %q, expected prefix-hex format", id)
		}
	})
}

// ─── metricsResponseWriter ───────────────────────────────────────────────────

func TestMetricsResponseWriter(t *testing.T) {
	t.Run("captures status code", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mw := &metricsResponseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

		mw.WriteHeader(http.StatusNotFound)

		if mw.statusCode != http.StatusNotFound {
			t.Errorf("statusCode = %d, want %d", mw.statusCode, http.StatusNotFound)
		}
	})

	t.Run("default is 200", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mw := &metricsResponseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

		// No WriteHeader call → should remain 200
		if mw.statusCode != http.StatusOK {
			t.Errorf("statusCode = %d, want %d", mw.statusCode, http.StatusOK)
		}
	})

	t.Run("proxies write to underlying writer", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mw := &metricsResponseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

		mw.WriteHeader(http.StatusCreated)
		if rec.Code != http.StatusCreated {
			t.Errorf("underlying ResponseWriter code = %d, want %d", rec.Code, http.StatusCreated)
		}
	})
}

// ─── resolveConfig ────────────────────────────────────────────────────────────

func TestResolveConfig(t *testing.T) {
	s := &Server{
		cfg: &GatewayConfig{
			Containers: []ContainerConfig{
				{Name: "app1", Host: "app1.local:8080"},
				{Name: "app2", Host: "app2.local"},
			},
		},
	}
	s.hostIndex = BuildHostIndex(s.cfg)

	tests := []struct {
		name     string
		host     string
		query    string
		wantName string
		wantNil  bool
	}{
		{
			name:     "exact host match with port",
			host:     "app1.local:8080",
			wantName: "app1",
		},
		{
			name:     "host match without port",
			host:     "app2.local",
			wantName: "app2",
		},
		{
			name:    "unknown host",
			host:    "unknown.com",
			wantNil: true,
		},
		{
			name:     "query param fallback",
			host:     "unknown.com",
			query:    "container=app1",
			wantName: "app1",
		},
		{
			name:    "query param unknown container",
			host:    "unknown.com",
			query:   "container=nope",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/"
			if tt.query != "" {
				url = "/?" + tt.query
			}
			r := httptest.NewRequest(http.MethodGet, url, nil)
			r.Host = tt.host

			got := s.resolveConfig(r)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil config")
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}
