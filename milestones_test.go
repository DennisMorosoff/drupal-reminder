package main

import (
	"testing"
	"time"
)

func TestBirthAnchorLocal(t *testing.T) {
	birth := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	loc := time.FixedZone("MSK", 3*3600)
	anchor, ok := BirthAnchorLocal(&birth, loc)
	if !ok {
		t.Fatal("expected ok")
	}
	want := time.Date(2026, 3, 16, 0, 0, 0, 0, loc)
	if !anchor.Equal(want) {
		t.Fatalf("anchor = %v, want %v", anchor, want)
	}
	if _, ok := BirthAnchorLocal(nil, loc); ok {
		t.Fatal("nil birth should fail")
	}

	// Рождение «вечером» по UTC, но уже следующий календарный день в MSK — якорь по дню в таймзоне семьи.
	birthLateUTC := time.Date(2026, 3, 15, 22, 30, 0, 0, time.UTC)
	anchor2, ok := BirthAnchorLocal(&birthLateUTC, loc)
	if !ok {
		t.Fatal("expected ok")
	}
	want2 := time.Date(2026, 3, 16, 0, 0, 0, 0, loc)
	if !anchor2.Equal(want2) {
		t.Fatalf("anchor cross-midnight = %v, want %v", anchor2, want2)
	}
}

func TestCollectBeautifulIntsIncludesPatterns(t *testing.T) {
	m := collectBeautifulInts(10_000)
	for _, n := range []int{1, 10, 100, 11, 111, 121, 123, 1234, 234} {
		if _, ok := m[n]; !ok {
			t.Fatalf("expected %d in beautiful set", n)
		}
	}
}

func TestMilestoneScheduleHasKnownIDs(t *testing.T) {
	var seenHour100, seenDay100 bool
	for _, m := range milestoneScheduleSorted() {
		switch m.ID {
		case "hour-100":
			seenHour100 = true
		case "day-100":
			seenDay100 = true
		}
	}
	if !seenHour100 {
		t.Fatal("expected hour-100 milestone")
	}
	if !seenDay100 {
		t.Fatal("expected day-100 milestone")
	}
}

func TestMilestonesOnLocalCalendarDay(t *testing.T) {
	loc := time.UTC
	anchor := time.Date(2026, 1, 10, 0, 0, 0, 0, loc)
	// 100 минут после полуночи — тот же календарный день
	day := time.Date(2026, 1, 10, 23, 0, 0, 0, loc)
	ms := MilestonesOnLocalCalendarDay(anchor, day, loc)
	found := false
	for _, m := range ms {
		if m.ID == "min-100" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected min-100 on same day, got %d milestones", len(ms))
	}
}

func TestForEachMilestoneDueForNotifyWindow(t *testing.T) {
	loc := time.UTC
	anchor := time.Date(2020, 1, 1, 0, 0, 0, 0, loc)
	now := time.Date(2020, 1, 5, 15, 0, 0, 0, loc)
	var ids []string
	ForEachMilestoneDueForNotify(anchor, now, func(m Milestone) {
		ids = append(ids, m.ID)
	})
	if len(ids) == 0 {
		t.Fatal("expected some milestones in 24h window before now")
	}
}

func TestMilestonesOccurredBetween(t *testing.T) {
	loc := time.UTC
	anchor := time.Date(2020, 1, 1, 0, 0, 0, 0, loc)
	// 100 ч от полуночи 1 янв = 5 янв 04:00 UTC
	from := time.Date(2020, 1, 5, 3, 0, 0, 0, loc)
	to := time.Date(2020, 1, 5, 5, 0, 0, 0, loc)
	ms := MilestonesOccurredBetween(anchor, from, to)
	found := false
	for _, m := range ms {
		if m.ID == "hour-100" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected hour-100 between from and to, got %#v", ms)
	}
}

func TestPalindromeFromHalf(t *testing.T) {
	p, err := palindromeFromHalf(12, false)
	if err != nil || p != 1221 {
		t.Fatalf("even palindrome: got %d err=%v", p, err)
	}
	p, err = palindromeFromHalf(12, true)
	if err != nil || p != 121 {
		t.Fatalf("odd palindrome: got %d err=%v", p, err)
	}
}
