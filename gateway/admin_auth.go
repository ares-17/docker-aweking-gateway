package gateway

import (
	"crypto/subtle"
	"encoding/base64"
	"log/slog"
	"net/http"
	"strings"
)

// adminAuthMiddleware wraps an http.Handler and enforces the configured
// authentication scheme (basic / bearer) on every request.
// If method is "none", the handler is returned unchanged (zero overhead).
func adminAuthMiddleware(next http.Handler, cfg *AdminAuthConfig) http.Handler {
	switch cfg.Method {
	case "none":
		return next
	case "basic":
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !checkBasicAuth(r, cfg.Username, cfg.Password) {
				w.Header().Set("WWW-Authenticate", `Basic realm="DAG Admin"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				slog.Warn("admin auth failed",
					"method", "basic",
					"remote", r.RemoteAddr,
					"path", r.URL.Path,
				)
				return
			}
			next.ServeHTTP(w, r)
		})
	case "bearer":
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !checkBearerToken(r, cfg.Token) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				slog.Warn("admin auth failed",
					"method", "bearer",
					"remote", r.RemoteAddr,
					"path", r.URL.Path,
				)
				return
			}
			next.ServeHTTP(w, r)
		})
	default:
		// Should never happen after Validate(), but be defensive.
		return next
	}
}

// checkBasicAuth parses the Authorization header and compares credentials
// using constant-time comparison to prevent timing attacks.
func checkBasicAuth(r *http.Request, wantUser, wantPass string) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Basic ") {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(auth[len("Basic "):])
	if err != nil {
		return false
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return false
	}
	userOK := subtle.ConstantTimeCompare([]byte(parts[0]), []byte(wantUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(parts[1]), []byte(wantPass)) == 1
	return userOK && passOK
}

// checkBearerToken validates the Authorization: Bearer <token> header
// using constant-time comparison to prevent timing attacks.
func checkBearerToken(r *http.Request, wantToken string) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	got := auth[len("Bearer "):]
	return subtle.ConstantTimeCompare([]byte(got), []byte(wantToken)) == 1
}
