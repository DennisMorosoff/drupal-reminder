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

## Deployment

### Automatic Deployment via GitHub Actions

The bot can be automatically deployed to a server when changes are pushed to the `master` branch.

#### Setup GitHub Secrets

Configure the following secrets in your GitHub repository settings (Settings → Secrets and variables → Actions):

- `DEPLOY_HOST` - Server IP address or domain name (e.g., `192.168.1.100` or `example.com`)
- `DEPLOY_USER` - SSH username for server access (e.g., `root` or your username)
- `DEPLOY_SSH_KEY` - Private SSH key for server access (copy the entire private key content)
- `DEPLOY_PATH` - Deployment path on the server (e.g., `/home/user/drupal-reminder`)

#### How to Generate SSH Key

1. Generate an SSH key pair (if you don't have one):
   ```sh
   ssh-keygen -t ed25519 -C "github-actions"
   ```

2. Copy the public key to your server:
   ```sh
   ssh-copy-id -i ~/.ssh/id_ed25519.pub user@your-server
   ```

3. Copy the private key content and add it to GitHub Secrets as `DEPLOY_SSH_KEY`:
   ```sh
   cat ~/.ssh/id_ed25519
   ```

#### Deployment Process

When you push changes to the `master` branch:
1. GitHub Actions compiles the bot for Linux
2. The binary is copied to the server via SCP
3. The old bot process is stopped
4. The new bot process is started in the background

The bot logs will be written to `bot.log` in the deployment directory.

## Contributing
Contributions are welcome! Please open an issue or submit a pull request with your changes.

## License
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
