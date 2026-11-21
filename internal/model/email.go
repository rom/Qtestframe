package model

import (
	"errors"
	"time"
)

// E-mail for notifications (Preferences.EmailNotifications).
const (
	EmailOff       = "off"
	EmailImmediate = "immediate"
	EmailDigest    = "digest"
)

// EmailModes are the valid values of Preferences.EmailNotifications ("" =
// off).
var EmailModes = []string{EmailOff, EmailImmediate, EmailDigest}

// Invitation invites a person to a tenant by e-mail. The token itself is
// only in the e-mail; the database holds its hash.
type Invitation struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	TokenHash string    `json:"-"`
	SSO       bool      `json:"sso"`
	InvitedBy string    `json:"invited_by"`
	CreatedAt time.Time `json:"created_at"`
	SentAt    time.Time `json:"sent_at"`
	ExpiresAt time.Time `json:"expires_at"`
	// AcceptedAt is set when the person set a password or first signed in.
	AcceptedAt *time.Time `json:"accepted_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

// Pending reports whether the invitation can still be accepted at now.
func (i *Invitation) Pending(now time.Time) bool {
	return i.AcceptedAt == nil && i.RevokedAt == nil && now.Before(i.ExpiresAt)
}

// Report subscription frequencies.
const (
	FrequencyDaily   = "daily"
	FrequencyWeekly  = "weekly"
	FrequencyMonthly = "monthly"
)

// Frequencies are the schedules of a report subscription.
var Frequencies = []string{FrequencyDaily, FrequencyWeekly, FrequencyMonthly}

// ReportSubscription e-mails a saved report on a schedule.
type ReportSubscription struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	DefinitionID string `json:"definition_id"`
	// DefinitionName is filled in when listing.
	DefinitionName string `json:"definition_name,omitempty"`
	// OwnerID is the user the report is generated as (with their
	// permissions at the time it is sent).
	OwnerID    string `json:"owner_id"`
	OwnerEmail string `json:"owner_email,omitempty"`
	Frequency  string `json:"frequency"`
	// Weekday 0 (Sunday) – 6 for weekly; DayOfMonth 1–28 for monthly.
	Weekday    int    `json:"weekday"`
	DayOfMonth int    `json:"day_of_month"`
	TimeOfDay  string `json:"time"` // HH:MM
	Timezone   string `json:"timezone"`
	Format     string `json:"format"`
	// Recipients are user IDs of tenant members; Addresses other e-mail
	// addresses (allowed with project.update only).
	Recipients []string `json:"recipients"`
	Addresses  []string `json:"addresses"`
	Message    string   `json:"message"`
	// Language is the language the report and its e-mail are in (en, sv:
	// the UI language of whoever created or last changed the
	// subscription); "" = the owner's preferred language.
	Language   string     `json:"lang"`
	Enabled    bool       `json:"enabled"`
	NextRunAt  *time.Time `json:"next_run_at"`
	LastRunAt  *time.Time `json:"last_run_at"`
	LastSentAt *time.Time `json:"last_sent_at"`
	LastError  string     `json:"last_error"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// ParseTimeOfDay parses "HH:MM" (24 hours).
func ParseTimeOfDay(s string) (hour, minute int, ok bool) {
	t, err := time.Parse("15:04", s)
	if err != nil || len(s) != 5 {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}

// NextRun returns the first scheduled time strictly after after, or an
// error when the schedule is invalid.
func (s *ReportSubscription) NextRun(after time.Time) (time.Time, error) {
	if s.Timezone == "" || s.Timezone == "Local" {
		return time.Time{}, errors.New("unknown time zone")
	}
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return time.Time{}, errors.New("unknown time zone")
	}
	h, m, ok := ParseTimeOfDay(s.TimeOfDay)
	if !ok {
		return time.Time{}, errors.New("time must be HH:MM")
	}
	t := after.In(loc)
	y, mo, d := t.Date()
	var c time.Time
	switch s.Frequency {
	case FrequencyDaily:
		c = time.Date(y, mo, d, h, m, 0, 0, loc)
		if !c.After(after) {
			c = time.Date(y, mo, d+1, h, m, 0, 0, loc)
		}
	case FrequencyWeekly:
		if s.Weekday < 0 || s.Weekday > 6 {
			return time.Time{}, errors.New("weekday must be 0 (Sunday) to 6")
		}
		ahead := (s.Weekday - int(t.Weekday()) + 7) % 7
		c = time.Date(y, mo, d+ahead, h, m, 0, 0, loc)
		if !c.After(after) {
			c = time.Date(y, mo, d+ahead+7, h, m, 0, 0, loc)
		}
	case FrequencyMonthly:
		if s.DayOfMonth < 1 || s.DayOfMonth > 28 {
			return time.Time{}, errors.New("day of month must be 1 to 28")
		}
		c = time.Date(y, mo, s.DayOfMonth, h, m, 0, 0, loc)
		if !c.After(after) {
			c = time.Date(y, mo+1, s.DayOfMonth, h, m, 0, 0, loc)
		}
	default:
		return time.Time{}, errors.New("frequency must be daily, weekly or monthly")
	}
	return c.UTC(), nil
}
