package gateway

import (
	"testing"
)

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
