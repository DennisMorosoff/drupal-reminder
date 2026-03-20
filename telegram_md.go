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
// Если обрезка попадает внутрь незакрытого блока ```, сдвигает границу назад, чтобы не ломать Markdown.
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
		pieceRunes := rs[start:cut]
		for strings.Count(string(pieceRunes), "```")%2 != 0 {
			j := -1
			for i := len(pieceRunes) - 1; i >= 0; i-- {
				if pieceRunes[i] == '\n' {
					j = i
					break
				}
			}
			if j <= 0 {
				break
			}
			cut = start + j + 1
			pieceRunes = rs[start:cut]
		}
		if len(pieceRunes) == 0 {
			cut = start + maxRunes
			pieceRunes = rs[start:cut]
		}
		out = append(out, string(pieceRunes))
		start = cut
		for start < len(rs) && rs[start] == '\n' {
			start++
		}
	}
	return out
}
