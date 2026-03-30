package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const sleepTableSlot = 30 * time.Minute

type NapInsight struct {
	NapIndex         int
	Duration         time.Duration
	Yesterday        *time.Duration
	WeekAverage      *time.Duration
	MonthAverage     *time.Duration
	YesterdayDelta   *time.Duration
	WeekAverageDelta *time.Duration
	MonthAverageDiff *time.Duration
}

type DaySummary struct {
	Date         time.Time
	SleepCount   int
	TotalSleep   time.Duration
	AverageSleep time.Duration
}

func AnalyzeLatestNap(sessions []SleepSession, latest SleepSession, loc *time.Location) NapInsight {
	ordered := append([]SleepSession(nil), sessions...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].StartAt.Before(ordered[j].StartAt)
	})

	napsByDay := map[string][]SleepSession{}
	for _, session := range ordered {
		if session.EndAt == nil {
			continue
		}
		dayKey := session.StartAt.In(loc).Format("2006-01-02")
		napsByDay[dayKey] = append(napsByDay[dayKey], session)
	}

	latestDay := latest.StartAt.In(loc)
	latestKey := latestDay.Format("2006-01-02")
	latestIndex := 0
	for idx, session := range napsByDay[latestKey] {
		if session.ID == latest.ID {
			latestIndex = idx + 1
			break
		}
	}
	if latestIndex == 0 {
		latestIndex = 1
	}

	duration := latest.EndAt.Sub(latest.StartAt)
	insight := NapInsight{
		NapIndex: latestIndex,
		Duration: duration,
	}

	yesterdayKey := latestDay.AddDate(0, 0, -1).Format("2006-01-02")
	if naps := napsByDay[yesterdayKey]; len(naps) >= latestIndex {
		ref := naps[latestIndex-1].EndAt.Sub(naps[latestIndex-1].StartAt)
		insight.Yesterday = &ref
		delta := duration - ref
		insight.YesterdayDelta = &delta
	}

	var weekDurations []time.Duration
	var monthDurations []time.Duration
	for offset := 1; offset <= 30; offset++ {
		dayKey := latestDay.AddDate(0, 0, -offset).Format("2006-01-02")
		naps := napsByDay[dayKey]
		if len(naps) < latestIndex {
			continue
		}
		ref := naps[latestIndex-1].EndAt.Sub(naps[latestIndex-1].StartAt)
		monthDurations = append(monthDurations, ref)
		if offset <= 7 {
			weekDurations = append(weekDurations, ref)
		}
	}

	if avg, ok := averageDuration(weekDurations); ok {
		insight.WeekAverage = &avg
		delta := duration - avg
		insight.WeekAverageDelta = &delta
	}
	if avg, ok := averageDuration(monthDurations); ok {
		insight.MonthAverage = &avg
		delta := duration - avg
		insight.MonthAverageDiff = &delta
	}

	return insight
}

func SummarizeDay(sessions []SleepSession, day time.Time, loc *time.Location) DaySummary {
	var summary DaySummary
	summary.Date = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)

	for _, session := range sessions {
		if session.EndAt == nil {
			continue
		}
		startLocal := session.StartAt.In(loc)
		if startLocal.Year() != day.In(loc).Year() || startLocal.YearDay() != day.In(loc).YearDay() {
			continue
		}
		summary.SleepCount++
		summary.TotalSleep += session.EndAt.Sub(session.StartAt)
	}

	if summary.SleepCount > 0 {
		summary.AverageSleep = summary.TotalSleep / time.Duration(summary.SleepCount)
	}
	return summary
}

func SummarizeRange(sessions []SleepSession, end time.Time, days int, loc *time.Location) (int, time.Duration, time.Duration) {
	var count int
	var total time.Duration
	endLocal := end.In(loc)
	startDate := time.Date(endLocal.Year(), endLocal.Month(), endLocal.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(days - 1))
	// Эксклюзивная граница: следующий локальный полуночный "тик".
	endExclusive := time.Date(endLocal.Year(), endLocal.Month(), endLocal.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	for _, session := range sessions {
		if session.EndAt == nil {
			continue
		}
		startLocal := session.StartAt.In(loc)
		if startLocal.Before(startDate) || !startLocal.Before(endExclusive) {
			continue
		}
		count++
		total += session.EndAt.Sub(session.StartAt)
	}

	average := time.Duration(0)
	if count > 0 {
		average = total / time.Duration(count)
	}
	return count, total, average
}

func BuildLatestSleepReport(childName string, sessions []SleepSession, latest SleepSession, loc *time.Location) string {
	insight := AnalyzeLatestNap(sessions, latest, loc)
	var lines []string

	lines = append(lines, fmt.Sprintf("Последний сон %s", childName))
	lines = append(lines, fmt.Sprintf("%s сон длился %s.", ordinalNap(insight.NapIndex), formatDurationRU(insight.Duration)))

	if insight.Yesterday != nil && insight.YesterdayDelta != nil {
		lines = append(lines, compareSentence(*insight.YesterdayDelta, "чем вчера"))
	} else {
		lines = append(lines, "Сравнение со вчера пока недоступно.")
	}

	if insight.WeekAverage != nil && insight.WeekAverageDelta != nil {
		lines = append(lines, compareSentence(*insight.WeekAverageDelta, "чем среднее за неделю"))
	} else {
		lines = append(lines, "Среднего по неделе пока недостаточно.")
	}

	if insight.MonthAverage != nil && insight.MonthAverageDiff != nil {
		lines = append(lines, compareSentence(*insight.MonthAverageDiff, "чем среднее за месяц"))
	} else {
		lines = append(lines, "Среднего по месяцу пока недостаточно.")
	}

	return strings.Join(lines, "\n")
}

func BuildDashboardReport(childName string, sessions []SleepSession, active *SleepSession, loc *time.Location, now time.Time) string {
	var blocks []string

	if active != nil {
		blocks = append(blocks,
			fmt.Sprintf("Сейчас %s спит уже %s.", childName, formatDurationRU(now.Sub(active.StartAt))),
		)
	}

	if latest := latestCompletedSleep(sessions); latest != nil {
		blocks = append(blocks, BuildLatestSleepReport(childName, sessions, *latest, loc))
	}

	today := SummarizeDay(sessions, now.In(loc), loc)
	blocks = append(blocks, formatDaySummary("Сегодня", today))

	weekCount, weekTotal, weekAverage := SummarizeRange(sessions, now, 7, loc)
	blocks = append(blocks, fmt.Sprintf("За 7 дней: %d снов, всего %s, средняя длительность %s.", weekCount, formatDurationRU(weekTotal), formatDurationRU(weekAverage)))

	monthCount, monthTotal, monthAverage := SummarizeRange(sessions, now, 30, loc)
	blocks = append(blocks, fmt.Sprintf("За 30 дней: %d снов, всего %s, средняя длительность %s.", monthCount, formatDurationRU(monthTotal), formatDurationRU(monthAverage)))
	blocks = append(blocks, BuildSleepTableSection(sessionsWithActive(sessions, active, now), now, 7, loc))

	return strings.Join(blocks, "\n\n")
}

func BuildDayReport(sessions []SleepSession, active *SleepSession, day time.Time, loc *time.Location) string {
	summary := SummarizeDay(sessions, day, loc)
	table := BuildSleepTableSection(sessionsWithActive(sessions, active, day), day, 7, loc)
	return strings.Join([]string{
		formatDaySummary("Сегодня", summary),
		table,
	}, "\n\n")
}

func BuildRangeReport(sessions []SleepSession, active *SleepSession, end time.Time, days int, loc *time.Location) string {
	count, total, average := SummarizeRange(sessions, end, days, loc)
	summary := fmt.Sprintf("За %d дней: %d снов, всего %s, средняя длительность %s.", days, count, formatDurationRU(total), formatDurationRU(average))
	table := BuildSleepTableSection(sessionsWithActive(sessions, active, end), end, days, loc)
	return strings.Join([]string{
		summary,
		table,
	}, "\n\n")
}

func formatDaySummary(label string, summary DaySummary) string {
	if summary.SleepCount == 0 {
		return fmt.Sprintf("%s: записей о сне пока нет.", label)
	}
	return fmt.Sprintf("%s: %d снов, всего %s, средняя длительность %s.", label, summary.SleepCount, formatDurationRU(summary.TotalSleep), formatDurationRU(summary.AverageSleep))
}

func latestCompletedSleep(sessions []SleepSession) *SleepSession {
	for i := len(sessions) - 1; i >= 0; i-- {
		if sessions[i].EndAt != nil {
			session := sessions[i]
			return &session
		}
	}
	return nil
}

func averageDuration(values []time.Duration) (time.Duration, bool) {
	if len(values) == 0 {
		return 0, false
	}
	var total time.Duration
	for _, value := range values {
		total += value
	}
	return total / time.Duration(len(values)), true
}

func formatDurationRU(duration time.Duration) string {
	if duration < 0 {
		duration = -duration
	}
	duration = duration.Round(time.Minute)
	hours := int(duration / time.Hour)
	minutes := int((duration % time.Hour) / time.Minute)
	parts := make([]string, 0, 2)
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d ч", hours))
	}
	if minutes > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d мин", minutes))
	}
	return strings.Join(parts, " ")
}

func compareSentence(delta time.Duration, suffix string) string {
	if delta == 0 {
		return fmt.Sprintf("Это равно %s.", suffix)
	}
	if delta > 0 {
		return fmt.Sprintf("Это на %s длиннее, %s.", formatDurationRU(delta), suffix)
	}
	return fmt.Sprintf("Это на %s короче, %s.", formatDurationRU(-delta), suffix)
}

func ordinalNap(index int) string {
	labels := map[int]string{
		1: "Первый",
		2: "Второй",
		3: "Третий",
		4: "Четвертый",
		5: "Пятый",
	}
	if label, ok := labels[index]; ok {
		return label
	}
	return fmt.Sprintf("%d-й", index)
}

func BuildSleepTableSection(sessions []SleepSession, end time.Time, days int, loc *time.Location) string {
	if days < 1 {
		days = 1
	}
	return strings.Join([]string{
		fmt.Sprintf("Таблица сна за %d дн. (`#` = сон, `.` = нет; 1 символ = 30 мин):", days),
		wrapCodeBlock(BuildSleepTable(sessions, end, days, loc)),
	}, "\n")
}

func BuildSleepTable(sessions []SleepSession, end time.Time, days int, loc *time.Location) string {
	if days < 1 {
		days = 1
	}

	endDay := startOfDay(end, loc)
	startDay := endDay.AddDate(0, 0, -(days - 1))

	lines := []string{buildSleepTableHeader()}
	for day := startDay; !day.After(endDay); day = day.AddDate(0, 0, 1) {
		if !dayHasAnySleep(sessions, day, loc) {
			continue
		}
		lines = append(lines, buildSleepTableRow(day, sessions, loc))
	}
	return strings.Join(lines, "\n")
}

func dayHasAnySleep(sessions []SleepSession, day time.Time, loc *time.Location) bool {
	dayStart := startOfDay(day, loc)
	// Конец дня — следующий локальный полуночный "тик", а не start+24h.
	dayEnd := dayStart.AddDate(0, 0, 1)
	for _, session := range sessions {
		if session.EndAt == nil {
			continue
		}
		startAt := session.StartAt.In(loc)
		endAt := session.EndAt.In(loc)
		if startAt.Before(dayEnd) && endAt.After(dayStart) {
			return true
		}
	}
	return false
}

func buildSleepTableHeader() string {
	groups := make([]string, 0, 24)
	for hour := 0; hour < 24; hour++ {
		groups = append(groups, fmt.Sprintf("%02d", hour))
	}
	return "дата   " + strings.Join(groups, " ")
}

func buildSleepTableRow(day time.Time, sessions []SleepSession, loc *time.Location) string {
	groups := make([]string, 0, 24)
	dayStart := startOfDay(day, loc)
	for hour := 0; hour < 24; hour++ {
		cells := []byte{'.', '.'}
		for half := 0; half < 2; half++ {
			slotStart := dayStart.Add(time.Duration(hour*2+half) * sleepTableSlot)
			slotEnd := slotStart.Add(sleepTableSlot)
			if hasSleepOverlap(sessions, slotStart, slotEnd, loc) {
				cells[half] = '#'
			}
		}
		groups = append(groups, string(cells))
	}
	return fmt.Sprintf("%s  %s", dayStart.Format("02.01"), strings.Join(groups, " "))
}

func hasSleepOverlap(sessions []SleepSession, slotStart time.Time, slotEnd time.Time, loc *time.Location) bool {
	for _, session := range sessions {
		if session.EndAt == nil {
			continue
		}
		startAt := session.StartAt.In(loc)
		endAt := session.EndAt.In(loc)
		if startAt.Before(slotEnd) && endAt.After(slotStart) {
			return true
		}
	}
	return false
}

func sessionsWithActive(sessions []SleepSession, active *SleepSession, now time.Time) []SleepSession {
	merged := append([]SleepSession(nil), sessions...)
	if active == nil {
		return merged
	}

	session := *active
	endAt := now
	session.EndAt = &endAt
	merged = append(merged, session)
	return merged
}

func wrapCodeBlock(text string) string {
	return "```\n" + text + "\n```"
}

func startOfDay(value time.Time, loc *time.Location) time.Time {
	local := value.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

type SleepNorm struct {
	TotalMin  float64
	TotalMax  float64
	DayMin    float64
	DayMax    float64
	NightMin  float64
	NightMax  float64
}

type NormSystemResult struct {
	SystemID string
	Band     string
	Norm     SleepNorm
	Actual   struct {
		Total float64
		Day   float64
		Night float64
	}
	Delta struct {
		TotalPercent float64
		DayPercent   float64
		NightPercent float64
		Score        float64
	}
	Category string
}

var sleepNormsA = map[string]SleepNorm{
	"0-1": {TotalMin: 16, TotalMax: 20, DayMin: 7, DayMax: 9, NightMin: 9, NightMax: 11},
	"1-2": {TotalMin: 15, TotalMax: 18, DayMin: 6, DayMax: 8, NightMin: 9, NightMax: 11},
	"2-3": {TotalMin: 15, TotalMax: 17, DayMin: 5, DayMax: 7, NightMin: 9, NightMax: 10},
	"3-4": {TotalMin: 14, TotalMax: 16, DayMin: 4, DayMax: 5.5, NightMin: 9, NightMax: 10.5},
	"4-6": {TotalMin: 13, TotalMax: 15, DayMin: 3, DayMax: 4.5, NightMin: 9, NightMax: 11},
}

var sleepNormsB = map[string]SleepNorm{
	"0-1": {TotalMin: 14, TotalMax: 17, DayMin: 7, DayMax: 9, NightMin: 7, NightMax: 9},
	"1-2": {TotalMin: 14, TotalMax: 17, DayMin: 6, DayMax: 8, NightMin: 8, NightMax: 9},
	"2-3": {TotalMin: 14, TotalMax: 17, DayMin: 5, DayMax: 7, NightMin: 9, NightMax: 10},
	"3-4": {TotalMin: 12, TotalMax: 16, DayMin: 4, DayMax: 5, NightMin: 8, NightMax: 10},
	"4-6": {TotalMin: 12, TotalMax: 15, DayMin: 3, DayMax: 4.5, NightMin: 9, NightMax: 10.5},
}

var sleepNormsC = map[string]SleepNorm{
	"0-3": {TotalMin: 14, TotalMax: 17, DayMin: 6, DayMax: 9, NightMin: 8, NightMax: 11},
	"3-6": {TotalMin: 12, TotalMax: 16, DayMin: 3, DayMax: 5, NightMin: 9, NightMax: 11},
}

func ageBandA(ageMonths float64) string {
	switch {
	case ageMonths < 1:
		return "0-1"
	case ageMonths < 2:
		return "1-2"
	case ageMonths < 3:
		return "2-3"
	case ageMonths < 4:
		return "3-4"
	default:
		return "4-6"
	}
}

func ageBandB(ageMonths float64) string {
	return ageBandA(ageMonths)
}

func ageBandC(ageMonths float64) string {
	if ageMonths < 3 {
		return "0-3"
	}
	return "3-6"
}

func deviationPercent(value float64, min float64, max float64) float64 {
	if value >= min && value <= max {
		return 0
	}
	if value < min {
		if min == 0 {
			return 0
		}
		return (min - value) / min * -100
	}
	if max == 0 {
		return 0
	}
	return (value - max) / max * 100
}

func categoryByScore(score float64) string {
	switch {
	case score <= 10:
		return "В пределах нормы"
	case score <= 25:
		return "Лёгкое отклонение"
	case score <= 40:
		return "Заметное отклонение"
	default:
		return "Сильное отклонение"
	}
}

func evaluateSystem(systemID string, norms map[string]SleepNorm, band string, dayHours float64, nightHours float64) NormSystemResult {
	norm := norms[band]
	total := dayHours + nightHours

	deltaTotal := deviationPercent(total, norm.TotalMin, norm.TotalMax)
	deltaDay := deviationPercent(dayHours, norm.DayMin, norm.DayMax)
	deltaNight := deviationPercent(nightHours, norm.NightMin, norm.NightMax)

	score := 0.5*absFloat(deltaTotal) + 0.25*absFloat(deltaDay) + 0.25*absFloat(deltaNight)

	var res NormSystemResult
	res.SystemID = systemID
	res.Band = band
	res.Norm = norm
	res.Actual.Total = total
	res.Actual.Day = dayHours
	res.Actual.Night = nightHours
	res.Delta.TotalPercent = deltaTotal
	res.Delta.DayPercent = deltaDay
	res.Delta.NightPercent = deltaNight
	res.Delta.Score = score
	res.Category = categoryByScore(score)
	return res
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func splitDayNightLast24h(sessions []SleepSession, now time.Time, loc *time.Location) (time.Duration, time.Duration) {
	var (
		dayTotal   time.Duration
		nightTotal time.Duration
	)
	since := now.Add(-24 * time.Hour)

	for _, session := range sessions {
		if session.EndAt == nil {
			continue
		}
		start := session.StartAt.In(loc)
		end := session.EndAt.In(loc)
		if end.Before(since) || start.After(now) {
			continue
		}
		if start.Before(since) {
			start = since
		}
		if end.After(now) {
			end = now
		}
		if !end.After(start) {
			continue
		}

		total := end.Sub(start)
		dayTotal += overlapWithDayWindow(start, end, loc)
		nightTotal += total - overlapWithDayWindow(start, end, loc)
	}

	return dayTotal, nightTotal
}

func overlapWithDayWindow(start time.Time, end time.Time, loc *time.Location) time.Duration {
	const dayStartHour = 7
	const dayEndHour = 19

	var total time.Duration
	current := start
	for current.Before(end) {
		dayStart := time.Date(current.Year(), current.Month(), current.Day(), dayStartHour, 0, 0, 0, loc)
		dayEnd := time.Date(current.Year(), current.Month(), current.Day(), dayEndHour, 0, 0, 0, loc)
		if dayEnd.Before(dayStart) {
			dayEnd = dayStart
		}

		windowStart := maxTime(dayStart, start)
		windowEnd := minTime(dayEnd, end)

		if windowEnd.After(windowStart) {
			total += windowEnd.Sub(windowStart)
		}

		current = time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, loc)
	}
	return total
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func childAgeMonths(child Child, now time.Time) (float64, bool) {
	if child.BirthDate == nil {
		return 0, false
	}
	age := now.Sub(*child.BirthDate)
	if age <= 0 {
		return 0, false
	}
	days := age.Hours() / 24
	months := days / 30.4375
	return months, true
}

func BuildNormsReport(child Child, sessions []SleepSession, loc *time.Location, now time.Time) string {
	ageMonths, ok := childAgeMonths(child, now.In(loc))
	if !ok {
		return "Возраст ребенка неизвестен или некорректен. Укажите дату рождения через `/setbirthdate`, чтобы оценивать сон относительно норм для возраста до 6 месяцев."
	}
	if ageMonths > 6 {
		return "Эта оценка рассчитана для детей до 6 месяцев. Сейчас возраст ребенка больше 6 месяцев, поэтому используйте обычные отчеты или проконсультируйтесь с педиатром."
	}

	dayDur, nightDur := splitDayNightLast24h(sessions, now, loc)
	if dayDur == 0 && nightDur == 0 {
		return "За последние 24 часа нет сохраненных снов, поэтому оценка относительно норм пока недоступна."
	}

	dayHours := dayDur.Hours()
	nightHours := nightDur.Hours()

	bandA := ageBandA(ageMonths)
	bandB := ageBandB(ageMonths)
	bandC := ageBandC(ageMonths)

	resA := evaluateSystem("A", sleepNormsA, bandA, dayHours, nightHours)
	resB := evaluateSystem("B", sleepNormsB, bandB, dayHours, nightHours)
	resC := evaluateSystem("C", sleepNormsC, bandC, dayHours, nightHours)

	avgScore := (resA.Delta.Score + resB.Delta.Score + resC.Delta.Score) / 3
	overallCategory := categoryByScore(avgScore)

	safeName := escapeTelegramMarkdown(child.Name)
	lines := []string{
		fmt.Sprintf("Оценка сна %s за последние 24 часа:", safeName),
		fmt.Sprintf("*Сводка:* в среднем по 3 системам — _%s_ (среднее отклонение %.1f%%).", overallCategory, avgScore),
		"",
		formatSystemBlock(resA, "Система A (русская таблица)"),
		"",
		formatSystemBlock(resB, "Система B (Sleep Foundation)"),
		"",
		formatSystemBlock(resC, "Система C (международные рекомендации)"),
		"",
		"Это ориентировочная оценка по открытым педиатрическим источникам и не является медицинским диагнозом. При заметных отклонениях или беспокойстве по поводу сна ребенка обсудите режим с педиатром или детским сомнологом.",
	}

	return strings.Join(lines, "\n")
}

func formatSystemBlock(res NormSystemResult, title string) string {
	return strings.Join([]string{
		fmt.Sprintf("*%s* (возрастная группа %s)", title, res.Band),
		fmt.Sprintf("Норма: всего %.1f–%.1f ч, день %.1f–%.1f ч, ночь %.1f–%.1f ч.",
			res.Norm.TotalMin, res.Norm.TotalMax,
			res.Norm.DayMin, res.Norm.DayMax,
			res.Norm.NightMin, res.Norm.NightMax,
		),
		fmt.Sprintf("У вас: всего %.1f ч (%.1f%%), день %.1f ч (%.1f%%), ночь %.1f ч (%.1f%%).",
			res.Actual.Total, res.Delta.TotalPercent,
			res.Actual.Day, res.Delta.DayPercent,
			res.Actual.Night, res.Delta.NightPercent,
		),
		fmt.Sprintf("Итог по системе: _%s_.", res.Category),
	}, "\n")
}

