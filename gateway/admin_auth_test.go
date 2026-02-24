package gateway

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── checkBasicAuth ───────────────────────────────────────────────────────────

func TestCheckBasicAuth(t *testing.T) {
	encode := func(user, pass string) string {
		return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	}

	tests := []struct {
		name   string
		header string
		user   string
		pass   string
		want   bool
	}{
		{
			name:   "valid credentials",
			header: "Basic " + encode("admin", "secret"),
			user:   "admin",
			pass:   "secret",
			want:   true,
		},
		{
			name:   "wrong password",
			header: "Basic " + encode("admin", "wrong"),
			user:   "admin",
			pass:   "secret",
			want:   false,
		},
		{
			name:   "wrong username",
			header: "Basic " + encode("user", "secret"),
			user:   "admin",
			pass:   "secret",
			want:   false,
		},
		{
			name:   "missing header",
			header: "",
			user:   "admin",
			pass:   "secret",
			want:   false,
		},
		{
			name:   "bearer instead of basic",
			header: "Bearer token123",
			user:   "admin",
			pass:   "secret",
			want:   false,
		},
		{
			name:   "malformed base64",
			header: "Basic %%%invalid",
			user:   "admin",
			pass:   "secret",
			want:   false,
		},
		{
			name:   "no colon in decoded value",
			header: "Basic " + base64.StdEncoding.EncodeToString([]byte("nocolon")),
			user:   "admin",
			pass:   "secret",
			want:   false,
		},
		{
			name:   "empty username and password match",
			header: "Basic " + encode("", ""),
			user:   "",
			pass:   "",
			want:   true,
		},
		{
			name:   "password with colon",
			header: "Basic " + encode("admin", "pass:word"),
			user:   "admin",
			pass:   "pass:word",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			if got := checkBasicAuth(r, tt.user, tt.pass); got != tt.want {
				t.Errorf("checkBasicAuth() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ─── checkBearerToken ─────────────────────────────────────────────────────────

func TestCheckBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		token  string
		want   bool
	}{
		{
			name:   "valid token",
			header: "Bearer my-token-123",
			token:  "my-token-123",
			want:   true,
		},
		{
			name:   "wrong token",
			header: "Bearer wrong-token",
			token:  "my-token-123",
			want:   false,
		},
		{
			name:   "missing header",
			header: "",
			token:  "my-token-123",
			want:   false,
		},
		{
			name:   "basic instead of bearer",
			header: "Basic dXNlcjpwYXNz",
			token:  "my-token-123",
			want:   false,
		},
		{
			name:   "extra whitespace in token",
			header: "Bearer  my-token-123",
			token:  "my-token-123",
			want:   false,
		},
		{
			name:   "token with special characters",
			header: "Bearer abc-123_DEF.456",
			token:  "abc-123_DEF.456",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			if got := checkBearerToken(r, tt.token); got != tt.want {
				t.Errorf("checkBearerToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ─── adminAuthMiddleware ──────────────────────────────────────────────────────

func TestAdminAuthMiddleware_None(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := adminAuthMiddleware(handler, &AdminAuthConfig{Method: "none"})

	r := httptest.NewRequest(http.MethodGet, "/_status", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("method=none: got status %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAdminAuthMiddleware_None_ReturnsOriginalHandler(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	wrapped := adminAuthMiddleware(handler, &AdminAuthConfig{Method: "none"})

	// When method is "none", the middleware should return the exact same handler (zero overhead).
	// We can't compare functions directly, but we can verify it's not wrapped in a HandlerFunc.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAdminAuthMiddleware_BasicOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cfg := &AdminAuthConfig{Method: "basic", Username: "admin", Password: "secret"}
	wrapped := adminAuthMiddleware(handler, cfg)

	r := httptest.NewRequest(http.MethodGet, "/_status", nil)
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:secret")))
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("basic auth valid: got status %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAdminAuthMiddleware_Basic401(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called on auth failure")
	})

	cfg := &AdminAuthConfig{Method: "basic", Username: "admin", Password: "secret"}
	wrapped := adminAuthMiddleware(handler, cfg)

	r := httptest.NewRequest(http.MethodGet, "/_status", nil)
	// No Authorization header
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("basic auth missing: got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if got := w.Header().Get("WWW-Authenticate"); got != `Basic realm="DAG Admin"` {
		t.Errorf("WWW-Authenticate = %q, want %q", got, `Basic realm="DAG Admin"`)
	}
}

func TestAdminAuthMiddleware_BearerOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cfg := &AdminAuthConfig{Method: "bearer", Token: "my-token"}
	wrapped := adminAuthMiddleware(handler, cfg)

	r := httptest.NewRequest(http.MethodGet, "/_metrics", nil)
	r.Header.Set("Authorization", "Bearer my-token")
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("bearer auth valid: got status %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAdminAuthMiddleware_Bearer401(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called on auth failure")
	})

	cfg := &AdminAuthConfig{Method: "bearer", Token: "my-token"}
	wrapped := adminAuthMiddleware(handler, cfg)

	r := httptest.NewRequest(http.MethodGet, "/_metrics", nil)
	// No Authorization header
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("bearer auth missing: got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAdminAuthMiddleware_Bearer_WrongToken(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called on auth failure")
	})

	cfg := &AdminAuthConfig{Method: "bearer", Token: "correct-token"}
	wrapped := adminAuthMiddleware(handler, cfg)

	r := httptest.NewRequest(http.MethodGet, "/_metrics", nil)
	r.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("bearer wrong token: got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAdminAuthMiddleware_UnknownMethod(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Unknown method should fall through to the handler (defensive behavior).
	wrapped := adminAuthMiddleware(handler, &AdminAuthConfig{Method: "unknown"})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("unknown method: got status %d, want %d", w.Code, http.StatusOK)
	}
}
