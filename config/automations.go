package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Automation represents a scheduled agent task.
type Automation struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Instructions string    `json:"instructions"`
	Schedule     string    `json:"schedule"`
	Enabled      bool      `json:"enabled"`
	LastRun      time.Time `json:"last_run,omitempty"`
	NextRun      time.Time `json:"next_run"`
}

// scheduleSpec describes a parsed schedule.
type scheduleSpec struct {
	// interval is non-zero for interval-based schedules ("hourly", "daily", "every Nh/Nm")
	interval time.Duration
	// clockHour and clockMinute are used for @HH:MM daily schedules.
	clockHour   int
	clockMinute int
	// isClockBased is true when this is an @HH:MM schedule.
	isClockBased bool
}

// ParseSchedule parses a schedule string and returns its spec.
// Supported formats: "hourly", "daily", "weekly", "every 30m", "every 4h", "@06:00".
// Returns an error for unrecognised or invalid strings.
func ParseSchedule(s string) (scheduleSpec, error) {
	s = strings.TrimSpace(s)
	switch s {
	case "hourly":
		return scheduleSpec{interval: 60 * time.Minute}, nil
	case "daily":
		return scheduleSpec{interval: 24 * time.Hour}, nil
	case "weekly":
		return scheduleSpec{interval: 7 * 24 * time.Hour}, nil
	}

	if strings.HasPrefix(s, "every ") {
		rest := strings.TrimPrefix(s, "every ")
		rest = strings.TrimSpace(rest)
		if strings.HasSuffix(rest, "h") {
			n, err := strconv.Atoi(strings.TrimSuffix(rest, "h"))
			if err != nil || n <= 0 {
				return scheduleSpec{}, fmt.Errorf("invalid schedule %q: hour count must be a positive integer", s)
			}
			return scheduleSpec{interval: time.Duration(n) * time.Hour}, nil
		}
		if strings.HasSuffix(rest, "m") {
			n, err := strconv.Atoi(strings.TrimSuffix(rest, "m"))
			if err != nil || n <= 0 {
				return scheduleSpec{}, fmt.Errorf("invalid schedule %q: minute count must be a positive integer", s)
			}
			return scheduleSpec{interval: time.Duration(n) * time.Minute}, nil
		}
		return scheduleSpec{}, fmt.Errorf("invalid schedule %q: 'every' must be followed by Nh or Nm", s)
	}

	if strings.HasPrefix(s, "@") {
		timeStr := strings.TrimPrefix(s, "@")
		parts := strings.SplitN(timeStr, ":", 2)
		if len(parts) != 2 {
			return scheduleSpec{}, fmt.Errorf("invalid schedule %q: expected @HH:MM format", s)
		}
		hour, err1 := strconv.Atoi(parts[0])
		minute, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return scheduleSpec{}, fmt.Errorf("invalid schedule %q: expected valid time in @HH:MM format", s)
		}
		return scheduleSpec{
			isClockBased: true,
			clockHour:    hour,
			clockMinute:  minute,
		}, nil
	}

	return scheduleSpec{}, fmt.Errorf("unrecognised schedule format %q", s)
}

// NextRunTime computes the next run time for a schedule based on the last run time and now.
func NextRunTime(schedule string, lastRun time.Time, now time.Time) (time.Time, error) {
	spec, err := ParseSchedule(schedule)
	if err != nil {
		return time.Time{}, err
	}

	if spec.isClockBased {
		// Next occurrence of HH:MM today or tomorrow.
		candidate := time.Date(now.Year(), now.Month(), now.Day(), spec.clockHour, spec.clockMinute, 0, 0, now.Location())
		if !candidate.After(now) {
			candidate = candidate.Add(24 * time.Hour)
		}
		return candidate, nil
	}

	// Interval-based: next run = lastRun + interval, but not before now.
	next := lastRun.Add(spec.interval)
	if next.Before(now) {
		next = now
	}
	return next, nil
}

// NewAutomation creates a new Automation with a generated ID and a computed NextRun.
func NewAutomation(name, instructions, schedule string) (*Automation, error) {
	now := time.Now()
	next, err := NextRunTime(schedule, time.Time{}, now)
	if err != nil {
		return nil, err
	}
	a := &Automation{
		ID:           fmt.Sprintf("auto-%d", now.UnixNano()),
		Name:         name,
		Instructions: instructions,
		Schedule:     schedule,
		Enabled:      true,
		NextRun:      next,
	}
	return a, nil
}

const automationsFileName = "automations.json"

// automationsPath returns the path to the automations file.
func automationsPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, automationsFileName), nil
}

// LoadAutomations loads automations from disk.
// Returns an empty slice (not an error) when the file does not exist yet.
func LoadAutomations() ([]*Automation, error) {
	path, err := automationsPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get automations path: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Automation{}, nil
		}
		return nil, fmt.Errorf("failed to read automations file: %w", err)
	}
	var automations []*Automation
	if err := json.Unmarshal(data, &automations); err != nil {
		return nil, fmt.Errorf("failed to parse automations file: %w", err)
	}
	return automations, nil
}

// SaveAutomations persists automations to disk atomically.
func SaveAutomations(automations []*Automation) error {
	path, err := automationsPath()
	if err != nil {
		return fmt.Errorf("failed to get automations path: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	data, err := json.MarshalIndent(automations, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal automations: %w", err)
	}
	return atomicWriteFile(path, data, 0600)
}
