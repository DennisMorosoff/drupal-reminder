package main

import (
	"strings"
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

func TestBuildSleepTableRowMarksHalfHourSlots(t *testing.T) {
	loc := time.FixedZone("UTC+3", 3*60*60)
	day := time.Date(2026, 3, 16, 0, 0, 0, 0, loc)

	makeSession := func(startHour int, startMinute int, endHour int, endMinute int) SleepSession {
		start := time.Date(day.Year(), day.Month(), day.Day(), startHour, startMinute, 0, 0, loc).UTC()
		end := time.Date(day.Year(), day.Month(), day.Day(), endHour, endMinute, 0, 0, loc).UTC()
		return SleepSession{StartAt: start, EndAt: &end}
	}

	row := buildSleepTableRow(day, []SleepSession{
		makeSession(1, 0, 1, 30),
		makeSession(2, 30, 2, 40),
	}, loc)

	if !strings.HasPrefix(row, "16.03  .. #. .#") {
		t.Fatalf("unexpected row prefix: %s", row)
	}
}

func TestBuildSleepTableRowSplitsSleepAcrossMidnight(t *testing.T) {
	loc := time.FixedZone("UTC+3", 3*60*60)
	day := time.Date(2026, 3, 16, 0, 0, 0, 0, loc)
	start := time.Date(2026, 3, 16, 23, 30, 0, 0, loc).UTC()
	end := time.Date(2026, 3, 17, 0, 20, 0, 0, loc).UTC()
	session := SleepSession{StartAt: start, EndAt: &end}

	firstRow := buildSleepTableRow(day, []SleepSession{session}, loc)
	secondRow := buildSleepTableRow(day.AddDate(0, 0, 1), []SleepSession{session}, loc)

	if !strings.HasSuffix(firstRow, " .. .#") {
		t.Fatalf("expected last slot on first day to be filled, got %s", firstRow)
	}
	if !strings.HasPrefix(secondRow, "17.03  #. ..") {
		t.Fatalf("expected first slot on second day to be filled, got %s", secondRow)
	}
}

func TestBuildRangeReportIncludesSleepTable(t *testing.T) {
	loc := time.FixedZone("UTC+3", 3*60*60)
	end := time.Date(2026, 3, 16, 12, 0, 0, 0, loc)
	start := time.Date(2026, 3, 16, 1, 0, 0, 0, loc).UTC()
	finish := time.Date(2026, 3, 16, 2, 0, 0, 0, loc).UTC()
	sessions := []SleepSession{{ID: 1, StartAt: start, EndAt: &finish}}

	report := BuildRangeReport(sessions, nil, end, 1, loc)

	if !strings.Contains(report, "Таблица сна за 1 дн.") {
		t.Fatalf("expected sleep table heading, got %s", report)
	}
	if !strings.Contains(report, "```") {
		t.Fatalf("expected markdown code block, got %s", report)
	}
	if !strings.Contains(report, "16.03  .. ##") {
		t.Fatalf("expected sleep cells in table, got %s", report)
	}
}
