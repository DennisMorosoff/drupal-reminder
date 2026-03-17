package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	logChildAge(db)

	botAPI, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		log.Fatalf("telegram init error: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Printf("sleep bot started as @%s (version=%s, build_time=%s, commit=%s)", botAPI.Self.UserName, version, buildTime, commitHash)

	bot := NewSleepBot(botAPI, store, cfg)

	go bot.RunReminders(ctx)

	if err := bot.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("bot stopped with error: %v", err)
	}
}

func logChildAge(db *sql.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var birthRaw sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT birth_date
		FROM children
		WHERE birth_date IS NOT NULL AND TRIM(birth_date) != ''
		ORDER BY id
		LIMIT 1
	`).Scan(&birthRaw)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("failed to load child birth date: %v", err)
		return
	}
	if !birthRaw.Valid {
		return
	}

	birthDate, err := time.Parse("2006-01-02", birthRaw.String)
	if err != nil {
		log.Printf("failed to parse child birth date %q: %v", birthRaw.String, err)
		return
	}

	age := time.Since(birthDate)
	if age < 0 {
		return
	}

	hours := int(age.Hours())
	days := hours / 24

	log.Printf("child age: %d days (~%d hours) since %s", days, hours, birthDate.Format("2006-01-02"))
}
