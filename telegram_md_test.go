package main

import (
	"strings"
	"testing"
)

func TestSplitTelegramMessageSingleChunk(t *testing.T) {
	s := strings.Repeat("a", 100)
	parts := splitTelegramMessage(s, telegramMaxMessageRunes)
	if len(parts) != 1 || parts[0] != s {
		t.Fatalf("want one part, got %d", len(parts))
	}
}

func TestSplitTelegramMessageRespectsMaxRunes(t *testing.T) {
	const max = 200
	var b strings.Builder
	for i := 0; i < 80; i++ {
		b.WriteString(strings.Repeat("я", 15))
		b.WriteByte('\n')
	}
	parts := splitTelegramMessage(b.String(), max)
	for i, p := range parts {
		if len([]rune(p)) > max {
			t.Fatalf("part %d len %d > %d", i, len([]rune(p)), max)
		}
	}
	if len(parts) < 2 {
		t.Fatalf("expected multiple parts, got %d", len(parts))
	}
}

func TestSplitTelegramMessageEvenCodeFences(t *testing.T) {
	var mid strings.Builder
	for i := 0; i < 400; i++ {
		mid.WriteString("12.34  .. ##\n")
	}
	text := "Ввод\n\n```\n" + mid.String() + "```\n\nХвост"
	parts := splitTelegramMessage(text, 300)
	for i, p := range parts {
		if n := strings.Count(p, "```"); n%2 != 0 {
			t.Fatalf("part %d has odd ``` count (%d): %q", i, n, truncateRunes(p, 80))
		}
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
