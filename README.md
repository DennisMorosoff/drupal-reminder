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
   go build -o drupal-reminder-bot main.go
   ./drupal-reminder-bot
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

#### Server Setup (Complete Checklist)

Before configuring GitHub Actions, you need to set up the server. Execute these commands **on your server** (connect via SSH as root or a user with sudo privileges):

**1. Create a dedicated user for deployment (recommended):**

```bash
# Create new user (replace 'github-runner' with your preferred username)
sudo adduser github-runner

# When prompted, set a password (you can disable it later) and fill in optional information

# Add user to sudo group (optional, if needed for admin tasks)
sudo usermod -aG sudo github-runner

# Switch to the new user
su - github-runner
```

**2. Set up SSH directory and permissions for the deployment user:**

```bash
# Make sure you're logged in as the deployment user
# Create .ssh directory
mkdir -p ~/.ssh

# Set correct permissions for .ssh directory (700 = rwx------)
chmod 700 ~/.ssh

# Create authorized_keys file
touch ~/.ssh/authorized_keys

# Set correct permissions for authorized_keys (600 = rw-------)
chmod 600 ~/.ssh/authorized_keys

# Verify permissions
ls -la ~/.ssh/
# Expected output:
# drwx------ 2 github-runner github-runner 4096 ... .
# -rw------- 1 github-runner github-runner   ... authorized_keys
```

**3. Add SSH public key to authorized_keys:**

**On your local machine**, generate SSH key pair if you don't have one:

```bash
# Generate SSH key (press Enter when asked for passphrase to create key without password)
ssh-keygen -t ed25519 -C "github-actions-deploy"

# Or create a dedicated key for deployment
ssh-keygen -t ed25519 -C "github-actions-deploy" -f ~/.ssh/github_actions_deploy -N ""
```

**On your local machine**, copy the public key to the server:

```bash
# Method 1: Using ssh-copy-id (easiest)
ssh-copy-id -i ~/.ssh/id_ed25519.pub github-runner@your-server

# Or if using a dedicated key:
ssh-copy-id -i ~/.ssh/github_actions_deploy.pub github-runner@your-server

# Method 2: Manual copy (if ssh-copy-id doesn't work)
# First, display your public key:
cat ~/.ssh/id_ed25519.pub

# Then on the server, add it to authorized_keys:
# (Run this command on the server, replacing PUBLIC_KEY with the output from above)
echo "PUBLIC_KEY" >> ~/.ssh/authorized_keys
```

**On the server**, verify the key was added:

```bash
# Check that the public key is in authorized_keys
cat ~/.ssh/authorized_keys

# Verify file permissions are correct
ls -la ~/.ssh/authorized_keys
# Should show: -rw------- (600)
```

**4. Configure SSH server (sshd_config) for security:**

```bash
# Switch back to root or use sudo
sudo nano /etc/ssh/sshd_config

# Or use your preferred editor
sudo vi /etc/ssh/sshd_config
```

**Add or modify these settings in `/etc/ssh/sshd_config`:**

```
# Enable public key authentication (should already be enabled by default)
PubkeyAuthentication yes

# Disable password authentication (recommended for security)
PasswordAuthentication no

# Disable root login (recommended for security)
PermitRootLogin no

# Disable empty passwords
PermitEmptyPasswords no

# Set maximum authentication tries
MaxAuthTries 3

# Set logging level
LogLevel INFO
```

**After editing, test the SSH configuration:**

```bash
# Test SSH configuration for syntax errors
sudo sshd -t

# If test passes, restart SSH service
sudo systemctl restart ssh

# Or on some systems:
sudo systemctl restart sshd

# Verify SSH service is running
sudo systemctl status ssh
```

**Important:** Before disabling password authentication, make sure you can log in with SSH keys! Test your SSH key login first:

```bash
# On your local machine, test SSH connection
ssh -i ~/.ssh/id_ed25519 github-runner@your-server

# If it works, you can safely disable password authentication
```

**5. (Optional) Install and configure fail2ban for brute force protection:**

```bash
# Install fail2ban
sudo apt update
sudo apt install fail2ban -y

# Enable and start fail2ban
sudo systemctl enable fail2ban
sudo systemctl start fail2ban

# Check status
sudo systemctl status fail2ban

# View fail2ban logs
sudo tail -f /var/log/fail2ban.log
```

**6. Create deployment directory:**

```bash
# Make sure you're logged in as the deployment user
# Create deployment directory
mkdir -p ~/drupal-reminder

# Or use a custom path (remember to update DEPLOY_PATH in GitHub Secrets)
mkdir -p /home/github-runner/drupal-reminder

# Set ownership (if needed)
sudo chown -R github-runner:github-runner ~/drupal-reminder

# Set permissions
chmod 755 ~/drupal-reminder
```

**7. Create .env file in deployment directory:**

```bash
# Navigate to deployment directory
cd ~/drupal-reminder

# Create .env file
nano .env

# Add your configuration:
TELEGRAM_BOT_TOKEN=your_telegram_bot_token_here
DRUPAL_SITE_URL=https://example.com
RSS_URL=https://www.dennismorosoff.ru/rss.xml
RSS_AUTH_USER=rss_user_if_needed
RSS_AUTH_PASSWORD=rss_password_if_needed

# Save and set permissions
chmod 600 .env
```

**8. (Optional) Set up systemd service for the bot (alternative to nohup):**

```bash
# Create systemd service file
sudo nano /etc/systemd/system/drupal-reminder.service
```

**Add this content (adjust paths as needed):**

```ini
[Unit]
Description=Drupal Reminder Bot
After=network.target

[Service]
Type=simple
User=github-runner
WorkingDirectory=/home/github-runner/drupal-reminder
ExecStart=/home/github-runner/drupal-reminder/drupal-reminder-bot
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

**Enable and start the service:**

```bash
# Reload systemd daemon
sudo systemctl daemon-reload

# Enable service to start on boot
sudo systemctl enable drupal-reminder

# Start service
sudo systemctl start drupal-reminder

# Check status
sudo systemctl status drupal-reminder

# View logs
sudo journalctl -u drupal-reminder -f
```

**9. Verify everything is set up correctly:**

```bash
# Test SSH key login (from your local machine)
ssh -i ~/.ssh/id_ed25519 github-runner@your-server

# On the server, verify:
# - SSH directory permissions
ls -la ~/.ssh/

# - authorized_keys file exists and has correct permissions
ls -la ~/.ssh/authorized_keys

# - Deployment directory exists
ls -ld ~/drupal-reminder

# - .env file exists
ls -la ~/drupal-reminder/.env

# - SSH service is running
sudo systemctl status ssh

# - SSH config test passes
sudo sshd -t
```

**Summary of required permissions:**

- `~/.ssh` directory: `700` (drwx------)
- `~/.ssh/authorized_keys` file: `600` (-rw-------)
- Deployment directory: `755` (drwxr-xr-x)
- `.env` file: `600` (-rw-------)
- Bot binary: `755` (executable)

#### Deployment Process

When you push changes to the `master` branch:
1. GitHub Actions compiles the bot for Linux
2. The binary is copied to the server via SCP
3. The old bot process is stopped
4. The new bot process is started in the background

The bot logs will be written to `bot.log` in the deployment directory (or to systemd journal if using systemd service).

## Contributing
Contributions are welcome! Please open an issue or submit a pull request with your changes.

## License
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Language
- [English](README.md)
- [Русский](README.ru.md)
