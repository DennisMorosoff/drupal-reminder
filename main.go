package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	version    = "dev"
	buildTime  = "unknown"
	commitHash = "unknown"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	db, err := sql.Open("sqlite", cfg.DatabasePath)
	if err != nil {
		log.Fatalf("db open error: %v", err)
	}
	defer db.Close()

	store, err := NewStore(db, cfg)
	if err != nil {
		log.Fatalf("store init error: %v", err)
	}

	botAPI, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		log.Fatalf("telegram init error: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Printf("sleep bot started as @%s (%s, %s, %s)", botAPI.Self.UserName, version, buildTime, commitHash)

	bot := NewSleepBot(botAPI, store, cfg)

	go bot.RunReminders(ctx)

	if err := bot.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("bot stopped with error: %v", err)
	}
}
