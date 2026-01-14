# Drupal Update Notification Bot

This bot is designed to be added to Telegram group chats. It monitors a Drupal site for updates and notifies users when new articles are published.

## Objective
- Monitor a Drupal website
- Detect when new articles are published
- Send notifications in a Telegram group chat

## Getting Started

### Prerequisites
- Go programming language installed (version 1.16 or later)
- Telegram bot token
- A running Telegram bot server

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
   RSS_URL=https://www.dennismorosoff.ru/rss.xml
   RSS_AUTH_USER=rss_user_if_needed
   RSS_AUTH_PASSWORD=rss_password_if_needed
   ```

3. **Build and run the bot:**
   ```sh
   go build -o drupal-reminder main.go
   ./drupal-reminder
   ```

### Configuration
- The bot will monitor the Drupal site specified in `DRUPAL_SITE_URL`
- The bot will send notifications to the Telegram group chat where it is added
- Default RSS URL: `https://www.dennismorosoff.ru/rss.xml`

### Bot Commands
- `/start` - Start working with the bot
- `/fetch` - Get website content
- `/check` - Check the latest article and send notification to all groups

## Deployment

### Automatic Deployment via GitHub Actions

The bot can be automatically deployed to a server when changes are pushed to the `master` branch.

#### Setup GitHub Secrets

Configure the following secrets in your GitHub repository settings (Settings → Secrets and variables → Actions):

- `DEPLOY_HOST` - Server IP address or domain name (e.g., `192.168.1.100` or `example.com`)
- `DEPLOY_USER` - SSH username for server access (e.g., `root` or your username)
- `DEPLOY_SSH_KEY` - Private SSH key for server access (copy the entire private key content)
- `DEPLOY_PATH` - Deployment path on the server (e.g., `/home/user/drupal-reminder` or `~/drupal-reminder`)

**Important:** For automatic deployment, it is recommended to use an SSH key without a passphrase. If your key is password-protected, create a separate key for deployment:

```sh
ssh-keygen -t ed25519 -C "github-actions-deploy" -f ~/.ssh/github_actions_deploy -N ""
```

#### How to Generate SSH Key

1. Generate an SSH key pair (if you don't have one):
   ```sh
   ssh-keygen -t ed25519 -C "github-actions"
   ```
   **Important:** When prompted for passphrase, press Enter to create a key without a password (for automation).

2. Copy the public key to your server:
   ```sh
   ssh-copy-id -i ~/.ssh/id_ed25519.pub user@your-server
   ```

3. Copy the private key content and add it to GitHub Secrets as `DEPLOY_SSH_KEY`:
   ```sh
   cat ~/.ssh/id_ed25519
   ```
   Copy the entire output, including the `-----BEGIN OPENSSH PRIVATE KEY-----` and `-----END OPENSSH PRIVATE KEY-----` lines.

#### Where to Configure GitHub Secrets

1. Open your repository on GitHub
2. Go to **Settings** (tab at the top of the repository page)
3. In the left menu, select **Secrets and variables** → **Actions**
4. Click **New repository secret**
5. For each secret, specify:
   - **Name** - secret name (e.g., `DEPLOY_HOST`)
   - **Secret** - secret value
   - Click **Add secret**

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

## Language
- [English](README.md)
- [Русский](README.ru.md)
