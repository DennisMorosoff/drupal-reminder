package main

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

func escapeTelegramMarkdown(s string) string {
	return tgbotapi.EscapeText(tgbotapi.ModeMarkdown, s)
}
