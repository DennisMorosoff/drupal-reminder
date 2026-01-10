# Tech Stack Context
- Language: Go 1.23+
- Telegram Lib: github.com/go-telegram-bot-api/telegram-bot-api/v5
- Database: PostgreSQL with `pgx/v5` driver (no ORM preferred).
- Config: `godotenv` or `viper`.

IMPORTANT: Do not use methods from `tucnak/telebot` or other libraries. Use strictly `tgbotapi` structures.
