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
	for _, n := range []int{1, 10, 100, 11, 111, 123, 1234, 234} {
		if _, ok := m[n]; !ok {
			t.Fatalf("expected %d in beautiful set", n)
		}
	}
	if _, ok := m[121]; ok {
		t.Fatal("121 is too short a step palindrome for milestones")
	}
	mLarge := collectBeautifulInts(20_000)
	if _, ok := mLarge[12321]; !ok {
		t.Fatal("expected 12321 (5-digit step palindrome) in beautiful set")
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
	ForEachMilestoneDueForNotify(anchor, now, loc, func(m Milestone) {
		ids = append(ids, m.ID)
	})
	if len(ids) == 0 {
		t.Fatal("expected some milestones in 24h window before now")
	}
}

func TestForEachMilestoneDueForNotifyMatchesDailyReportFilter(t *testing.T) {
	loc := time.UTC
	anchor := time.Date(2020, 1, 1, 0, 0, 0, 0, loc)
	// 7777 мин = 5д 09:37, 7887 мин = 5д 11:27 — в одном календарном дне.
	// При наличии 7777 в этот день 7887 должен быть отфильтрован и в push.
	now := anchor.Add(5*24*time.Hour + 12*time.Hour)

	var got []string
	ForEachMilestoneDueForNotify(anchor, now, loc, func(m Milestone) {
		got = append(got, m.ID)
	})
	has7777 := false
	has7887 := false
	for _, id := range got {
		if id == "min-7777" {
			has7777 = true
		}
		if id == "min-7887" {
			has7887 = true
		}
	}
	if !has7777 {
		t.Fatal("expected min-7777 in notify window")
	}
	if has7887 {
		t.Fatal("min-7887 should be filtered out to match report")
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

func TestNextMilestoneAtOrAfter(t *testing.T) {
	loc := time.UTC
	anchor := time.Date(2020, 1, 1, 0, 0, 0, 0, loc)

	m, at, ok := NextMilestoneAtOrAfter(anchor, anchor.Add(99*time.Hour+30*time.Minute))
	if !ok {
		t.Fatal("expected next milestone")
	}
	if m.ID != "hour-100" {
		t.Fatalf("expected hour-100, got %s", m.ID)
	}
	wantAt := anchor.Add(100 * time.Hour)
	if !at.Equal(wantAt) {
		t.Fatalf("at = %v, want %v", at, wantAt)
	}
}

func TestNextMilestoneShownInDailyReportRespectsFilter(t *testing.T) {
	loc := time.UTC
	anchor := time.Date(2020, 1, 1, 0, 0, 0, 0, loc)

	// В 2020-01-06 в списке минут при наличии 7777 7887 фильтруется.
	from := anchor.Add(5*24*time.Hour + 10*time.Hour)
	day := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	ms := MilestonesOnLocalCalendarDay(anchor, day, loc)
	for _, m := range ms {
		if m.ID == "min-7887" {
			t.Fatal("sanity: min-7887 should be filtered out of daily report")
		}
	}

	next, _, ok := NextMilestoneShownInDailyReportAtOrAfter(anchor, from, loc)
	if !ok {
		t.Fatal("expected a next milestone for daily report")
	}
	if next.ID == "min-7887" {
		t.Fatal("next milestone should not be a filtered-out one")
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

func TestIsStepPalindrome(t *testing.T) {
	for _, n := range []int{121, 1221, 12321, 1234321, 2345432} {
		if !isStepPalindrome(n) {
			t.Fatalf("%d should be step palindrome", n)
		}
	}
	for _, n := range []int{131, 1331, 1001, 10201} {
		if isStepPalindrome(n) {
			t.Fatalf("%d should not be step palindrome", n)
		}
	}
}

func TestStepPalindromeSuppressedByRepdigitTwin(t *testing.T) {
	max := 500_000
	m := collectBeautifulInts(max)
	if _, ok := m[456654]; ok {
		t.Fatal("456654 should lose to repdigit 444444")
	}
	if _, ok := m[444444]; !ok {
		t.Fatal("444444 should remain")
	}
	if _, ok := m[123321]; ok {
		t.Fatal("123321 (6-digit step) should lose to 111111")
	}
	if _, ok := m[12321]; !ok {
		t.Fatal("12321 (5-digit) should remain")
	}
}

func TestMilestoneKindPriority(t *testing.T) {
	if milestoneKindPriority(444444) <= milestoneKindPriority(456654) {
		t.Fatal("repdigit should rank above step palindrome for merge")
	}
	if milestoneKindPriority(456654) <= milestoneKindPriority(3600) {
		t.Fatal("step palindrome should rank above a plain milestone number")
	}
	if isStepPalindrome(3600) {
		t.Fatal("sanity: 3600 should not be step palindrome")
	}
}

func TestMilestonesSameCalendarDayPreferRepdigitOverStepPal(t *testing.T) {
	ms := []Milestone{
		{ID: "min-7777", Offset: time.Hour, Title: "a"},
		{ID: "min-7887", Offset: 2 * time.Hour, Title: "b"},
	}
	out := milestonesSameCalendarDayPreferRepdigitOverStepPal(ms)
	if len(out) != 1 || out[0].ID != "min-7777" {
		t.Fatalf("expected only min-7777, got %#v", out)
	}

	sec := []Milestone{
		{ID: "sec-444444", Offset: 3 * time.Minute, Title: "a"},
		{ID: "sec-456789", Offset: 4 * time.Minute, Title: "b"},
	}
	out2 := milestonesSameCalendarDayPreferRepdigitOverStepPal(sec)
	if len(out2) != 1 || out2[0].ID != "sec-444444" {
		t.Fatalf("expected only sec-444444, got %#v", out2)
	}

	// Нет репдигита — обе «украшалки» остаются
	mixed := []Milestone{
		{ID: "min-7887", Offset: time.Hour, Title: "a"},
		{ID: "min-1234", Offset: 2 * time.Hour, Title: "b"},
	}
	out3 := milestonesSameCalendarDayPreferRepdigitOverStepPal(mixed)
	if len(out3) != 2 {
		t.Fatalf("expected both without repdigit, got %d", len(out3))
	}

	// Другая шкала: sec-репдигит не трогает min-палиндром
	cross := []Milestone{
		{ID: "sec-111111", Offset: time.Minute, Title: "a"},
		{ID: "min-7887", Offset: 2 * time.Hour, Title: "b"},
	}
	out4 := milestonesSameCalendarDayPreferRepdigitOverStepPal(cross)
	if len(out4) != 2 {
		t.Fatalf("expected cross-scale keep both, got %d", len(out4))
	}
}
