# Drupal Update Notification Bot

This bot is designed to be added to Telegram group chats. It monitors a Drupal site for updates and notifies users when new articles are published.

## Objective
- Monitor a Drupal website.
- Detect when new articles are published.
- Send notifications in a Telegram group chat.

## Getting Started

### Prerequisites
- Go programming language installed (version 1.16 or later).
- Telegram bot token.
- A running Telegram bot server.

### Setup

1. **Clone the repository:**
   ```sh
   git clone https://github.com/DennisMorosoff/drupal-reminder.git
   cd drupal-reminder
   ```

2. **Set up environment variables:**
   Create a `.env` file with the following content:
   ```env
   TELEGRAM_BOT_TOKEN=your_telegram_bot_token_here
   DRUPAL_SITE_URL=https://example.com
   ```

3. **Build and run the bot:**
   ```sh
   go build -o drupal-bot main.go
   ./drupal-bot
   ```

### Configuration
- The bot will monitor the Drupal site specified in `DRUPAL_SITE_URL`.
- It will send notifications to the Telegram group chat where it is added.

## Contributing
Contributions are welcome! Please open an issue or submit a pull request with your changes.

## License
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
