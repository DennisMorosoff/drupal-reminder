package main

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestBotCommand(t *testing.T) {
	// Mock the bot API to test command handling
	bot := &tgbotapi.BotAPI{
		SendMessage: func(msg tgbotapi.MessageConfig) error {
			return nil
		},
	}

	token := "mock_token"
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updatesChan, err := bot.GetUpdatesChan(u)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for update := range updatesChan {
			if update.Message != nil && update.Message.IsCommand() {
				switch update.Message.Command() {
				case "start":
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Hello! I'm the Drupal Update Notification Bot.")
					bot.SendMessage(msg)
				default:
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Unknown command. Try /start")
					bot.SendMessage(msg)
				}
			} else if update.Message != nil {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "I'm sorry, but I can only handle commands.")
				bot.SendMessage(msg)
			}
		}
	}()

	// Simulate a /start command
	startCmd := tgbotapi.NewMessage(123456789, "/start")
	updatesChan <- startCmd

	// Simulate an unexpected message
	unexpectedMsg := tgbotapi.NewMessage(123456789, "Hello, world!")
	updatesChan <- unexpectedMsg

	close(updatesChan)
}
