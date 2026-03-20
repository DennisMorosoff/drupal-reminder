package main

import (
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Лимит длины одного сообщения в Telegram Bot API.
const telegramMaxMessageRunes = 4096

func escapeTelegramMarkdown(s string) string {
	return tgbotapi.EscapeText(tgbotapi.ModeMarkdown, s)
}

// splitTelegramMessage режет текст на части не длиннее maxRunes, по возможности по переводу строки.
func splitTelegramMessage(text string, maxRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = telegramMaxMessageRunes
	}
	rs := []rune(text)
	if len(rs) <= maxRunes {
		return []string{text}
	}
	var out []string
	start := 0
	for start < len(rs) {
		end := start + maxRunes
		if end >= len(rs) {
			out = append(out, string(rs[start:]))
			break
		}
		lo := start + (maxRunes*3)/4
		if lo >= end {
			lo = start + 1
		}
		cut := -1
		for i := end - 1; i >= lo; i-- {
			if rs[i] == '\n' {
				cut = i + 1
				break
			}
		}
		if cut < 0 {
			for i := lo - 1; i > start; i-- {
				if rs[i] == '\n' {
					cut = i + 1
					break
				}
			}
		}
		if cut < 0 {
			cut = end
		}
		if cut <= start {
			cut = end
		}
		out = append(out, string(rs[start:cut]))
		start = cut
		for start < len(rs) && rs[start] == '\n' {
			start++
		}
	}
	return out
}

func telegramSendPlainFallback(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "parse") ||
		strings.Contains(s, "entity") ||
		strings.Contains(s, "entities")
}
