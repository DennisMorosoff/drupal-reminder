package main

import (
	"fmt"
	"strings"
	"time"
)

// ParseBirthDateStored разбирает birth_date из БД: RFC3339 или устаревший формат только даты Y-M-D.
func ParseBirthDateStored(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// ParseBirthDateInput разбирает ввод пользователя: дата и опционально время в таймзоне семьи, либо RFC3339.
func ParseBirthDateInput(raw string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("пустая строка")
	}
	for _, layout := range []string{
		"02.01.2006 15:04:05",
		"02.01.2006 15:04",
		"02.01.2006",
	} {
		if t, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return t, nil
		}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("не удалось разобрать дату и время")
}

// FormatBirthDateStored сериализует момент рождения для колонки birth_date.
func FormatBirthDateStored(t time.Time) string {
	return t.Format(time.RFC3339Nano)
}

// formatChildBirthForSettings показывает дату/время в настройках (таймзона семьи).
// Старые записи «только дата» хранились как полночь UTC — показываем без лишнего времени.
func formatChildBirthForSettings(t time.Time, familyLoc *time.Location) string {
	if familyLoc == nil {
		familyLoc = time.UTC
	}
	u := t.UTC()
	if u.Nanosecond() == 0 && u.Second() == 0 && u.Minute() == 0 && u.Hour() == 0 {
		return u.Format("02.01.2006")
	}
	return t.In(familyLoc).Format("02.01.2006 15:04")
}
