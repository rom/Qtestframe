package backup

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is when scheduled backups run (setting backup.schedule), in UTC:
//
//	daily HH:MM          every day at HH:MM
//	weekly DAY HH:MM     every week (DAY: mon, tue, wed, thu, fri, sat, sun)
//	every DURATION       at a fixed interval, e.g. every 6h (15m to 720h)
type Schedule struct {
	// Kind is "daily", "weekly" or "every".
	Kind string
	// Minute is the time of day of daily and weekly schedules, in minutes
	// after midnight UTC.
	Minute int
	// Weekday is the day of weekly schedules.
	Weekday time.Weekday
	// Every is the interval of "every" schedules.
	Every time.Duration
}

// Interval bounds of "every" schedules.
const (
	MinInterval = 15 * time.Minute
	MaxInterval = 720 * time.Hour
)

var weekdays = map[string]time.Weekday{"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday}

var errSchedule = errors.New(`must be "daily HH:MM", "weekly DAY HH:MM" (DAY mon–sun) or "every DURATION" (e.g. every 6h, 15m to 720h); times are UTC`)

func parseClock(s string) (int, bool) {
	h, m, ok := strings.Cut(s, ":")
	if !ok || len(h) == 0 || len(h) > 2 || len(m) != 2 {
		return 0, false
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, false
	}
	return hh*60 + mm, true
}

// ParseSchedule reads a schedule (case-insensitive, extra spaces allowed).
func ParseSchedule(s string) (Schedule, error) {
	f := strings.Fields(strings.ToLower(s))
	switch {
	case len(f) == 2 && f[0] == "daily":
		if m, ok := parseClock(f[1]); ok {
			return Schedule{Kind: "daily", Minute: m}, nil
		}
	case len(f) == 3 && f[0] == "weekly":
		d, okd := weekdays[f[1]]
		if m, ok := parseClock(f[2]); ok && okd {
			return Schedule{Kind: "weekly", Minute: m, Weekday: d}, nil
		}
	case len(f) == 2 && f[0] == "every":
		d, err := time.ParseDuration(f[1])
		if err == nil && d >= MinInterval && d <= MaxInterval {
			return Schedule{Kind: "every", Every: d}, nil
		}
	}
	return Schedule{}, errSchedule
}

// CheckSchedule validates a schedule (for the system settings).
func CheckSchedule(s string) error {
	_, err := ParseSchedule(s)
	return err
}

// String is the canonical form of the schedule.
func (s Schedule) String() string {
	clock := fmt.Sprintf("%02d:%02d", s.Minute/60, s.Minute%60)
	switch s.Kind {
	case "daily":
		return "daily " + clock
	case "weekly":
		return "weekly " + strings.ToLower(s.Weekday.String()[:3]) + " " + clock
	case "every":
		d := strings.TrimSuffix(s.Every.String(), "m0s")
		if d != s.Every.String() {
			d += "m"
		}
		return "every " + strings.Replace(d, "h0m", "h", 1)
	}
	return ""
}

// prev is the latest slot of a daily or weekly schedule at or before now.
func (s Schedule) prev(now time.Time) time.Time {
	now = now.UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	t := day.Add(time.Duration(s.Minute) * time.Minute)
	if s.Kind == "weekly" {
		back := (int(now.Weekday()) - int(s.Weekday) + 7) % 7
		t = t.AddDate(0, 0, -back)
	}
	if t.After(now) {
		if s.Kind == "weekly" {
			t = t.AddDate(0, 0, -7)
		} else {
			t = t.AddDate(0, 0, -1)
		}
	}
	return t
}

// Next is the next time the schedule runs after the recorded run last
// (nil = none) as of now.
func (s Schedule) Next(now time.Time, last *time.Time) time.Time {
	if s.Kind == "every" {
		if last == nil {
			return now.UTC()
		}
		next := last.UTC().Add(s.Every)
		if next.Before(now) {
			return now.UTC()
		}
		return next
	}
	p := s.prev(now)
	if last != nil && last.Before(p) {
		return now.UTC() // a slot was missed: at the next check
	}
	if s.Kind == "weekly" {
		return p.AddDate(0, 0, 7)
	}
	return p.AddDate(0, 0, 1)
}

// Due reports whether a backup is due at now, given the recorded run last
// (nil = never), and the time to record for it. A daily or weekly schedule
// that has never run records its latest slot without running (the first
// backup is at the next slot); a slot missed while no server ran is run
// once, at the next check.
func (s Schedule) Due(now time.Time, last *time.Time) (due bool, record time.Time) {
	now = now.UTC()
	if s.Kind == "every" {
		if last == nil || !now.Before(last.Add(s.Every)) {
			return true, now
		}
		return false, time.Time{}
	}
	p := s.prev(now)
	if last == nil {
		return false, p
	}
	if last.Before(p) {
		return true, p
	}
	return false, time.Time{}
}
