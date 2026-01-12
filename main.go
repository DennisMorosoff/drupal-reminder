package main

import (
	"log"
	"os"

	"github.com/PuerkitoBio/goquery"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

func fetchWebsiteContent(url string) (string, error) {
	resp, err := goquery.NewDocument(url)
	if err != nil {
		return "", err
	}
	content, err := resp.Html()
	if err != nil {
		return "", err
	}
	return content, nil
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Print("No .env file found")
	}

	token := os.Getenv("TELEGRAM_BOT_TOKEN")

	if token == "" {
		log.Panic("TELEGRAM_BOT_TOKEN is not set")
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updatesChan := bot.GetUpdatesChan(u)

	for update := range updatesChan {
		if update.Message != nil && update.Message.IsCommand() {
			switch update.Message.Command() {
			case "start":
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Hello! I'm the Drupal Update Notification Bot.")
				bot.Send(msg)
			case "fetch":
				url := os.Getenv("DRUPAL_SITE_URL")
				if url == "" {
					bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "DRUPAL_SITE_URL is not set"))
					continue
				}

				content, err := fetchWebsiteContent(url)
				if err != nil {
					bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Failed to fetch website content: "+err.Error()))
					continue
				}

				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, content))
			default:
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Unknown command. Try /start or /fetch")
				bot.Send(msg)
			}
		} else if update.Message != nil {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "I'm sorry, but I can only handle commands.")
			bot.Send(msg)
		}
	}
}
