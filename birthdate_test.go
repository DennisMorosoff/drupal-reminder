package main

import (
	"testing"
	"time"
)

func TestParseBirthDateStored(t *testing.T) {
	t.Parallel()
	if _, ok := ParseBirthDateStored(""); ok {
		t.Fatal("empty")
	}
	got, ok := ParseBirthDateStored("2026-03-16")
	if !ok || !got.Equal(time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("legacy date: %v ok=%v", got, ok)
	}
	got, ok = ParseBirthDateStored("2026-03-16T14:30:00+03:00")
	if !ok {
		t.Fatal("rfc3339")
	}
	if got.Year() != 2026 || got.Month() != time.March || got.Day() != 16 || got.Hour() != 14 || got.Minute() != 30 {
		t.Fatalf("rfc3339 fields: %v", got)
	}
}

func TestParseBirthDateInput(t *testing.T) {
	t.Parallel()
	loc := time.FixedZone("MSK", 3*3600)
	got, err := ParseBirthDateInput("16.03.2026 14:30", loc)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 3, 16, 14, 30, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("datetime: got %v want %v", got, want)
	}
	got, err = ParseBirthDateInput("16.03.2026", loc)
	if err != nil {
		t.Fatal(err)
	}
	want = time.Date(2026, 3, 16, 0, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("date only: got %v want %v", got, want)
	}
}

func TestFormatChildBirthForSettings(t *testing.T) {
	t.Parallel()
	loc := time.FixedZone("MSK", 3*3600)
	legacy := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	if g := formatChildBirthForSettings(legacy, loc); g != "16.03.2026" {
		t.Fatalf("legacy display: %q", g)
	}
	withTime := time.Date(2026, 3, 16, 14, 5, 0, 0, loc)
	if g := formatChildBirthForSettings(withTime, loc); g != "16.03.2026 14:05" {
		t.Fatalf("with time: %q", g)
	}
}
