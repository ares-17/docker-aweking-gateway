package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ─── ProbeHTTP ────────────────────────────────────────────────────────────────

func TestProbeHTTP(t *testing.T) {
	d := &DockerClient{} // no real Docker client needed for HTTP probe tests

	t.Run("success on 200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		// Extract host and port from the test server URL
		addr := srv.Listener.Addr().String()
		parts := strings.SplitN(addr, ":", 2)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := d.ProbeHTTP(ctx, parts[0], parts[1], "/health")
		if err != nil {
			t.Errorf("ProbeHTTP() error = %v, want nil", err)
		}
	})

	t.Run("retries on 503 then succeeds on 200", func(t *testing.T) {
		var callCount atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if callCount.Add(1) <= 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		addr := srv.Listener.Addr().String()
		parts := strings.SplitN(addr, ":", 2)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := d.ProbeHTTP(ctx, parts[0], parts[1], "/health")
		if err != nil {
			t.Errorf("ProbeHTTP() error = %v, want nil", err)
		}
		if callCount.Load() < 3 {
			t.Errorf("expected at least 3 calls, got %d", callCount.Load())
		}
	})

	t.Run("timeout on cancelled context", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		addr := srv.Listener.Addr().String()
		parts := strings.SplitN(addr, ":", 2)

		ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
		defer cancel()

		err := d.ProbeHTTP(ctx, parts[0], parts[1], "/health")
		if err == nil {
			t.Error("ProbeHTTP() expected timeout error, got nil")
		}
	})
}

// ─── stripDockerLogHeaders ────────────────────────────────────────────────────

func TestStripDockerLogHeaders(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{
			name:  "single stdout frame",
			input: makeDockerFrame(1, []byte("hello world")),
			want:  "hello world",
		},
		{
			name:  "single stderr frame",
			input: makeDockerFrame(2, []byte("error msg")),
			want:  "error msg",
		},
		{
			name: "multiple frames concatenated",
			input: append(
				makeDockerFrame(1, []byte("line1\n")),
				makeDockerFrame(1, []byte("line2\n"))...,
			),
			want: "line1\nline2\n",
		},
		{
			name:  "empty input",
			input: []byte{},
			want:  "",
		},
		{
			name:  "input shorter than header (7 bytes)",
			input: []byte{1, 0, 0, 0, 0, 0, 3},
			want:  "",
		},
		{
			name:  "frame with zero payload",
			input: makeDockerFrame(1, []byte{}),
			want:  "",
		},
		{
			name: "frame size larger than remaining data (graceful)",
			input: func() []byte {
				// Header says 100 bytes but only 5 follow
				header := []byte{1, 0, 0, 0, 0, 0, 0, 100}
				return append(header, []byte("short")...)
			}(),
			want: "short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripDockerLogHeaders(tt.input)
			if got != tt.want {
				t.Errorf("stripDockerLogHeaders() = %q, want %q", got, tt.want)
			}
		})
	}
}

// makeDockerFrame builds a Docker multiplexed log frame:
// [stream_type(1), 0, 0, 0, size(4 big-endian)] + payload
func makeDockerFrame(streamType byte, payload []byte) []byte {
	size := len(payload)
	header := []byte{
		streamType, 0, 0, 0,
		byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size),
	}
	return append(header, payload...)
}

// ─── joinNetworkNames ─────────────────────────────────────────────────────────

func TestJoinNetworkNames(t *testing.T) {
	tests := []struct {
		name string
		// We can't use the Docker types directly in a simple test without importing them,
		// so we test the general behaviour via string assertions.
		inputLen int
		wantLen  int
	}{
		{"empty map", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with nil map
			result := joinNetworkNames(nil)
			if result != "" {
				t.Errorf("joinNetworkNames(nil) = %q, want empty string", result)
			}
		})
	}
}
