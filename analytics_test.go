package main

import (
	"testing"
	"time"
)

func TestAnalyzeLatestNap(t *testing.T) {
	loc := time.FixedZone("UTC+3", 3*60*60)
	base := time.Date(2026, 3, 16, 10, 0, 0, 0, loc)

	makeSession := func(id int64, dayOffset int, startHour int, durationMin int) SleepSession {
		start := time.Date(base.Year(), base.Month(), base.Day()+dayOffset, startHour, 0, 0, 0, loc).UTC()
		end := start.Add(time.Duration(durationMin) * time.Minute)
		return SleepSession{ID: id, StartAt: start, EndAt: &end}
	}

	sessions := []SleepSession{
		makeSession(1, -2, 10, 55),
		makeSession(2, -1, 10, 60),
		makeSession(3, 0, 10, 70),
	}

	insight := AnalyzeLatestNap(sessions, sessions[2], loc)
	if insight.NapIndex != 1 {
		t.Fatalf("expected first nap, got %d", insight.NapIndex)
	}
	if insight.Yesterday == nil || insight.YesterdayDelta == nil {
		t.Fatalf("expected yesterday comparison")
	}
	if got := insight.Duration; got != 70*time.Minute {
		t.Fatalf("expected 70 minutes, got %s", got)
	}
	if got := *insight.YesterdayDelta; got != 10*time.Minute {
		t.Fatalf("expected +10 minutes vs yesterday, got %s", got)
	}
}

func TestParseSleepRange(t *testing.T) {
	loc := time.FixedZone("UTC+3", 3*60*60)
	now := time.Date(2026, 3, 16, 20, 0, 0, 0, loc)

	startAt, endAt, err := parseSleepRange("11:10 - 12:35", now, loc)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if startAt.In(loc).Hour() != 11 || startAt.In(loc).Minute() != 10 {
		t.Fatalf("unexpected start time: %s", startAt.In(loc))
	}
	if endAt.In(loc).Hour() != 12 || endAt.In(loc).Minute() != 35 {
		t.Fatalf("unexpected end time: %s", endAt.In(loc))
	}
}
