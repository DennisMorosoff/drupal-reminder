package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramBotToken string
	DatabasePath     string
	DefaultTimezone  string
	PollTimeout      int
	InviteTTL        time.Duration
	ReminderTick     time.Duration
	MaxBackdate      time.Duration
}

func LoadConfig() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		TelegramBotToken: strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		DatabasePath:     defaultString(os.Getenv("SLEEPBOT_DB_PATH"), "sleepbot.db"),
		DefaultTimezone:  defaultString(os.Getenv("SLEEPBOT_DEFAULT_TIMEZONE"), "Europe/Moscow"),
		PollTimeout:      defaultInt(os.Getenv("SLEEPBOT_POLL_TIMEOUT"), 60),
		InviteTTL:        defaultDurationMinutes(os.Getenv("SLEEPBOT_INVITE_TTL_MINUTES"), 1440),
		ReminderTick:     defaultDurationSeconds(os.Getenv("SLEEPBOT_REMINDER_TICK_SECONDS"), 60),
		MaxBackdate:      defaultDurationMinutes(os.Getenv("SLEEPBOT_MAX_BACKDATE_MINUTES"), 2880),
	}

	if cfg.TelegramBotToken == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}

	if _, err := time.LoadLocation(cfg.DefaultTimezone); err != nil {
		return Config{}, fmt.Errorf("invalid SLEEPBOT_DEFAULT_TIMEZONE: %w", err)
	}

	return cfg, nil
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func defaultInt(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func defaultDurationMinutes(value string, fallback int) time.Duration {
	return time.Duration(defaultInt(value, fallback)) * time.Minute
}

func defaultDurationSeconds(value string, fallback int) time.Duration {
	return time.Duration(defaultInt(value, fallback)) * time.Second
}
