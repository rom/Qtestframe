package model

import (
	"testing"
	"time"
)

func TestNextRun(t *testing.T) {
	utc := func(s string) time.Time {
		v, err := time.Parse("2006-01-02 15:04", s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	// 2026-11-20 is a Friday
	now := utc("2026-11-20 10:00")
	for _, c := range []struct {
		sub  ReportSubscription
		want string
	}{
		{ReportSubscription{Frequency: FrequencyDaily, TimeOfDay: "11:30", Timezone: "UTC"}, "2026-11-20 11:30"},
		{ReportSubscription{Frequency: FrequencyDaily, TimeOfDay: "10:00", Timezone: "UTC"}, "2026-11-21 10:00"},
		{ReportSubscription{Frequency: FrequencyDaily, TimeOfDay: "07:00", Timezone: "Europe/Stockholm"}, "2026-11-21 06:00"},
		{ReportSubscription{Frequency: FrequencyWeekly, Weekday: 1, TimeOfDay: "08:00", Timezone: "UTC"}, "2026-11-23 08:00"},
		{ReportSubscription{Frequency: FrequencyWeekly, Weekday: 5, TimeOfDay: "09:00", Timezone: "UTC"}, "2026-11-27 09:00"},
		{ReportSubscription{Frequency: FrequencyWeekly, Weekday: 5, TimeOfDay: "12:00", Timezone: "UTC"}, "2026-11-20 12:00"},
		{ReportSubscription{Frequency: FrequencyMonthly, DayOfMonth: 1, TimeOfDay: "06:00", Timezone: "UTC"}, "2026-12-01 06:00"},
		{ReportSubscription{Frequency: FrequencyMonthly, DayOfMonth: 25, TimeOfDay: "06:00", Timezone: "UTC"}, "2026-11-25 06:00"},
	} {
		got, err := c.sub.NextRun(now)
		if err != nil || !got.Equal(utc(c.want)) {
			t.Errorf("%+v: %v %v, want %s", c.sub, got, err, c.want)
		}
	}
	for _, bad := range []ReportSubscription{
		{Frequency: "hourly", TimeOfDay: "07:00", Timezone: "UTC"},
		{Frequency: FrequencyDaily, TimeOfDay: "7:00", Timezone: "UTC"},
		{Frequency: FrequencyDaily, TimeOfDay: "25:00", Timezone: "UTC"},
		{Frequency: FrequencyDaily, TimeOfDay: "07:00", Timezone: "Mars/Base"},
		{Frequency: FrequencyDaily, TimeOfDay: "07:00", Timezone: ""},
		{Frequency: FrequencyWeekly, Weekday: 7, TimeOfDay: "07:00", Timezone: "UTC"},
		{Frequency: FrequencyMonthly, DayOfMonth: 31, TimeOfDay: "07:00", Timezone: "UTC"},
	} {
		if _, err := bad.NextRun(now); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	i := Invitation{ExpiresAt: now.Add(time.Hour)}
	if !i.Pending(now) || i.Pending(now.Add(2*time.Hour)) {
		t.Fatal("pending")
	}
	i.AcceptedAt = &now
	if i.Pending(now) {
		t.Fatal("accepted is pending")
	}
}
