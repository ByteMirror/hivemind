package config

import (
	"testing"
	"time"
)

func TestParseSchedule(t *testing.T) {
	tests := []struct {
		input    string
		wantErr  bool
		interval time.Duration // zero if clock-based
		isClock  bool
		hour     int
		minute   int
	}{
		{"hourly", false, 60 * time.Minute, false, 0, 0},
		{"daily", false, 24 * time.Hour, false, 0, 0},
		{"weekly", false, 7 * 24 * time.Hour, false, 0, 0},
		{"every 4h", false, 4 * time.Hour, false, 0, 0},
		{"every 30m", false, 30 * time.Minute, false, 0, 0},
		{"every 1h", false, 1 * time.Hour, false, 0, 0},
		{"@06:00", false, 0, true, 6, 0},
		{"@23:59", false, 0, true, 23, 59},
		{"@00:00", false, 0, true, 0, 0},
		// Error cases
		{"every 0h", true, 0, false, 0, 0},
		{"every 0m", true, 0, false, 0, 0},
		{"every h", true, 0, false, 0, 0},
		{"@25:00", true, 0, false, 0, 0},
		{"@12:60", true, 0, false, 0, 0},
		{"@12", true, 0, false, 0, 0},
		{"monthly", true, 0, false, 0, 0},
		{"", true, 0, false, 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			spec, err := ParseSchedule(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ParseSchedule(%q) expected error, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseSchedule(%q) unexpected error: %v", tc.input, err)
				return
			}
			if spec.isClockBased != tc.isClock {
				t.Errorf("ParseSchedule(%q).isClockBased = %v, want %v", tc.input, spec.isClockBased, tc.isClock)
			}
			if tc.isClock {
				if spec.clockHour != tc.hour {
					t.Errorf("ParseSchedule(%q).clockHour = %d, want %d", tc.input, spec.clockHour, tc.hour)
				}
				if spec.clockMinute != tc.minute {
					t.Errorf("ParseSchedule(%q).clockMinute = %d, want %d", tc.input, spec.clockMinute, tc.minute)
				}
			} else {
				if spec.interval != tc.interval {
					t.Errorf("ParseSchedule(%q).interval = %v, want %v", tc.input, spec.interval, tc.interval)
				}
			}
		})
	}
}

func TestNextRunTime_IntervalBased(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		schedule string
		lastRun  time.Time
		now      time.Time
		want     time.Time
	}{
		{
			schedule: "hourly",
			lastRun:  base,
			now:      base.Add(30 * time.Minute),
			want:     base.Add(60 * time.Minute), // lastRun + interval
		},
		{
			schedule: "daily",
			lastRun:  base,
			now:      base.Add(2 * time.Hour),
			want:     base.Add(24 * time.Hour),
		},
		{
			schedule: "every 4h",
			lastRun:  base,
			now:      base.Add(1 * time.Hour),
			want:     base.Add(4 * time.Hour),
		},
		{
			schedule: "every 30m",
			lastRun:  base,
			now:      base.Add(10 * time.Minute),
			want:     base.Add(30 * time.Minute),
		},
		// When lastRun + interval is in the past, next run should be now.
		{
			schedule: "hourly",
			lastRun:  base.Add(-3 * time.Hour),
			now:      base,
			want:     base,
		},
		// Zero lastRun — first ever run should happen at now.
		{
			schedule: "daily",
			lastRun:  time.Time{},
			now:      base,
			want:     base,
		},
	}

	for _, tc := range tests {
		t.Run(tc.schedule, func(t *testing.T) {
			got, err := NextRunTime(tc.schedule, tc.lastRun, tc.now)
			if err != nil {
				t.Fatalf("NextRunTime(%q) unexpected error: %v", tc.schedule, err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("NextRunTime(%q, lastRun=%v, now=%v) = %v, want %v",
					tc.schedule, tc.lastRun, tc.now, got, tc.want)
			}
		})
	}
}
