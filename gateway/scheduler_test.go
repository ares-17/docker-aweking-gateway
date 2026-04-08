package gateway

import (
	"testing"
	"time"
)

// ─── validateScheduleCompatibility ───────────────────────────────────────────

func TestScheduleCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		start   string
		stop    string
		wantErr bool
	}{
		{"both empty", "", "", false},
		{"only start valid", "0 8 * * 1-5", "", false},
		{"only stop valid", "", "0 18 * * 1-5", false},
		{"both valid daily", "0 8 * * *", "0 20 * * *", false},
		{"both valid overnight", "0 22 * * *", "0 6 * * *", false},
		{"same minute conflict", "0 8 * * *", "0 8 * * *", true},
		{"invalid start", "not-a-cron", "0 8 * * *", true},
		{"invalid stop", "0 8 * * *", "not-a-cron", true},
		{"both invalid", "bad", "bad", true},
		{"zero-minute split valid", "0 0 * * *", "30 0 * * *", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateScheduleCompatibility(tt.start, tt.stop)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateScheduleCompatibility(%q, %q) error = %v, wantErr %v",
					tt.start, tt.stop, err, tt.wantErr)
			}
		})
	}
}

// ─── IsInScheduleWindow ───────────────────────────────────────────────────────

func TestIsInScheduleWindow(t *testing.T) {
	// Reference point: Monday 2026-04-13 10:00:00 UTC
	// schedule_start: "0 8 * * 1-5" → fires at 08:00 Mon-Fri
	// schedule_stop:  "0 18 * * 1-5" → fires at 18:00 Mon-Fri
	mon10am := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC) // inside window
	mon20pm := time.Date(2026, 4, 13, 20, 0, 0, 0, time.UTC) // outside window (after stop)
	mon07am := time.Date(2026, 4, 13, 7, 0, 0, 0, time.UTC)  // outside window (before start)
	tue08am := time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC)  // exactly on start boundary

	weekdayStart := "0 8 * * 1-5"
	weekdayStop := "0 18 * * 1-5"

	tests := []struct {
		name          string
		cfg           ContainerConfig
		now           time.Time
		wantAllowed   bool
		wantNextStart bool // true = nextStart should be non-zero
	}{
		{
			name:        "no schedule always allowed",
			cfg:         ContainerConfig{},
			now:         mon10am,
			wantAllowed: true,
		},
		{
			name:        "only start schedule always allowed",
			cfg:         ContainerConfig{ScheduleStart: weekdayStart},
			now:         mon10am,
			wantAllowed: true,
		},
		{
			name:        "only stop schedule always allowed",
			cfg:         ContainerConfig{ScheduleStop: weekdayStop},
			now:         mon10am,
			wantAllowed: true,
		},
		{
			name:          "both: inside window (10am)",
			cfg:           ContainerConfig{ScheduleStart: weekdayStart, ScheduleStop: weekdayStop},
			now:           mon10am,
			wantAllowed:   true,
			wantNextStart: false,
		},
		{
			name:          "both: outside window after stop (8pm)",
			cfg:           ContainerConfig{ScheduleStart: weekdayStart, ScheduleStop: weekdayStop},
			now:           mon20pm,
			wantAllowed:   false,
			wantNextStart: true,
		},
		{
			name:          "both: outside window before start (7am Monday)",
			cfg:           ContainerConfig{ScheduleStart: weekdayStart, ScheduleStop: weekdayStop},
			now:           mon07am,
			wantAllowed:   false,
			wantNextStart: true,
		},
		{
			name:          "exactly on start boundary is inside window",
			cfg:           ContainerConfig{ScheduleStart: weekdayStart, ScheduleStop: weekdayStop},
			now:           tue08am,
			wantAllowed:   true,
			wantNextStart: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, nextStart := IsInScheduleWindow(&tt.cfg, tt.now)
			if allowed != tt.wantAllowed {
				t.Errorf("IsInScheduleWindow() allowed = %v, want %v", allowed, tt.wantAllowed)
			}
			if tt.wantNextStart && nextStart.IsZero() {
				t.Error("expected non-zero nextStart when outside window")
			}
			if !tt.wantNextStart && !nextStart.IsZero() {
				t.Errorf("expected zero nextStart when inside/no window, got %v", nextStart)
			}
		})
	}
}
