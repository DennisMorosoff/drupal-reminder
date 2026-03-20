package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Максимумы для генерации вех (покрывают несколько десятилетий жизни).
const (
	milestoneMaxDays    = 8000
	milestoneMaxHours   = milestoneMaxDays * 24
	milestoneMaxMinutes = milestoneMaxDays * 24 * 60
	milestoneMaxSeconds = milestoneMaxDays * 24 * 60 * 60
)

// MilestoneNotifyMaxAge не уведомлять о вехе старше этого интервала (включение настроек без «залпа» в прошлое).
const MilestoneNotifyMaxAge = 24 * time.Hour

// Milestone описывает одну «красивую» веху от якоря рождения.
type Milestone struct {
	ID     string
	Offset time.Duration
	Title  string
}

var (
	milestoneSchedule     []Milestone
	milestoneScheduleOnce sync.Once
)

// BirthAnchorLocal полночь календарной даты рождения в таймзоне семьи.
func BirthAnchorLocal(birth *time.Time, loc *time.Location) (time.Time, bool) {
	if birth == nil || loc == nil {
		return time.Time{}, false
	}
	y, m, d := birth.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc), true
}

func milestoneScheduleSorted() []Milestone {
	milestoneScheduleOnce.Do(func() {
		milestoneSchedule = buildMilestoneSchedule()
	})
	out := make([]Milestone, len(milestoneSchedule))
	copy(out, milestoneSchedule)
	return out
}

func buildMilestoneSchedule() []Milestone {
	byNano := make(map[int64]Milestone)

	merge := func(off time.Duration, id, title string) {
		if off <= 0 {
			return
		}
		nano := off.Nanoseconds()
		if _, ok := byNano[nano]; ok {
			return
		}
		byNano[nano] = Milestone{ID: id, Offset: off, Title: title}
	}

	beautDays := collectBeautifulInts(milestoneMaxDays)
	for n := range beautDays {
		merge(time.Duration(n)*24*time.Hour, fmt.Sprintf("day-%d", n), milestoneTitleDays(n))
	}

	beautH := collectBeautifulInts(milestoneMaxHours)
	for n := range beautH {
		merge(time.Duration(n)*time.Hour, fmt.Sprintf("hour-%d", n), milestoneTitleHours(n))
	}

	beautM := collectBeautifulInts(milestoneMaxMinutes)
	for n := range beautM {
		merge(time.Duration(n)*time.Minute, fmt.Sprintf("min-%d", n), milestoneTitleMinutes(n))
	}

	beautS := collectBeautifulInts(milestoneMaxSeconds)
	for n := range beautS {
		merge(time.Duration(n)*time.Second, fmt.Sprintf("sec-%d", n), milestoneTitleSeconds(n))
	}

	list := make([]Milestone, 0, len(byNano))
	for _, m := range byNano {
		list = append(list, m)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Offset < list[j].Offset })
	return list
}

func collectBeautifulInts(max int) map[int]struct{} {
	if max < 1 {
		return map[int]struct{}{}
	}
	res := make(map[int]struct{})

	res[1] = struct{}{}

	for p := 10; p <= max; p *= 10 {
		res[p] = struct{}{}
	}

	for d := 1; d <= 9; d++ {
		v := 0
		for length := 2; ; length++ {
			v = v*10 + d
			if v > max {
				break
			}
			res[v] = struct{}{}
		}
	}

	for length := 3; length <= 9; length++ {
		for start := 1; start <= 9; start++ {
			if start+length-1 > 9 {
				continue
			}
			n := 0
			for i := 0; i < length; i++ {
				n = n*10 + (start + i)
			}
			if n <= max {
				res[n] = struct{}{}
			}
		}
	}

	addPalindromesUpTo(max, res)

	return res
}

func intPow10(e int) int {
	n := 1
	for i := 0; i < e; i++ {
		n *= 10
	}
	return n
}

// addPalindromesUpTo перебирает палиндромы по длине, без цикла до max.
func addPalindromesUpTo(max int, res map[int]struct{}) {
	if max < 1 {
		return
	}
	maxDigits := len(strconv.Itoa(max))
	for L := 2; L <= maxDigits+1; L++ {
		odd := L%2 == 1
		halfLen := (L + 1) / 2
		lo := intPow10(halfLen - 1)
		hi := intPow10(halfLen)
		for h := lo; h < hi; h++ {
			p, err := palindromeFromHalf(h, odd)
			if err != nil || p < 1 || p > max {
				continue
			}
			res[p] = struct{}{}
		}
	}
}

func palindromeFromHalf(h int, odd bool) (int, error) {
	s := strconv.Itoa(h)
	if !odd {
		return strconv.Atoi(s + reverseASCII(s))
	}
	if len(s) < 2 {
		return strconv.Atoi(s)
	}
	return strconv.Atoi(s + reverseASCII(s[:len(s)-1]))
}

func reverseASCII(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

func milestoneTitleDays(n int) string {
	return fmt.Sprintf("%d %s жизни", n, ruPlural(n, "день", "дня", "дней"))
}

func milestoneTitleHours(n int) string {
	return fmt.Sprintf("%d %s жизни", n, ruPlural(n, "час", "часа", "часов"))
}

func milestoneTitleMinutes(n int) string {
	return fmt.Sprintf("%d %s жизни", n, ruPlural(n, "минута", "минуты", "минут"))
}

func milestoneTitleSeconds(n int) string {
	return fmt.Sprintf("%d %s жизни", n, ruPlural(n, "секунда", "секунды", "секунд"))
}

func ruPlural(n int, one, few, many string) string {
	nAbs := n
	if nAbs < 0 {
		nAbs = -nAbs
	}
	n100 := nAbs % 100
	n10 := nAbs % 10
	if n100 >= 11 && n100 <= 14 {
		return many
	}
	switch n10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}

// MilestonesOccurredBetween возвращает вехи, наступившие в [from, to] (полуинтервал [from,to) по at).
func MilestonesOccurredBetween(anchor time.Time, from, to time.Time) []Milestone {
	if !to.After(from) {
		return nil
	}
	var out []Milestone
	for _, m := range milestoneScheduleSorted() {
		at := anchor.Add(m.Offset)
		if !at.Before(from) && at.Before(to) {
			out = append(out, m)
		}
	}
	return out
}

// ForEachMilestoneDueForNotify вызывает fn для вех в окне (now-24h, now], уже наступивших.
func ForEachMilestoneDueForNotify(anchor, now time.Time, fn func(m Milestone)) {
	sched := milestoneScheduleSorted()
	cutoff := now.Add(-MilestoneNotifyMaxAge)
	minDur := cutoff.Sub(anchor)
	start := sort.Search(len(sched), func(i int) bool {
		return sched[i].Offset >= minDur
	})
	for i := start; i < len(sched); i++ {
		m := sched[i]
		at := anchor.Add(m.Offset)
		if at.After(now) {
			break
		}
		fn(m)
	}
}

// MilestonesOnLocalCalendarDay — все вехи, чей момент попадает на календарный день `day` в локали loc.
func MilestonesOnLocalCalendarDay(anchor time.Time, day time.Time, loc *time.Location) []Milestone {
	if loc == nil {
		return nil
	}
	d := day.In(loc)
	start := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
	end := start.Add(24 * time.Hour)

	var out []Milestone
	for _, m := range milestoneScheduleSorted() {
		at := anchor.Add(m.Offset).In(loc)
		if !at.Before(start) && at.Before(end) {
			out = append(out, m)
		}
	}
	return out
}

// FormatMilestoneReportBlock форматирует блок для отчёта; пустая строка, если нечего показать.
func FormatMilestoneReportBlock(childName string, milestones []Milestone, loc *time.Location, anchor time.Time) string {
	if len(milestones) == 0 {
		return ""
	}
	var b strings.Builder
	if childName != "" {
		b.WriteString(fmt.Sprintf("Красивые даты сегодня (%s):\n", childName))
	} else {
		b.WriteString("Красивые даты сегодня:\n")
	}
	for _, m := range milestones {
		at := anchor.Add(m.Offset).In(loc)
		b.WriteString(fmt.Sprintf("• %s — в %s\n", m.Title, at.Format("15:04")))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// FormatMilestonePushMessage текст push-уведомления о наступившей вехе.
func FormatMilestonePushMessage(childName, title string) string {
	if childName == "" {
		return fmt.Sprintf("Красивая дата: %s.", title)
	}
	return fmt.Sprintf("Красивая дата у %s: %s.", childName, title)
}
