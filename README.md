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

#### Server Setup (Complete Checklist)

Before configuring GitHub Actions, you need to set up the server. Execute these commands **on your server**:

**1. Create a dedicated user for deployment (recommended):**

```bash
# Create new user (replace 'github-runner' with your preferred username)
sudo adduser github-runner

# Add user to sudo group (optional, if needed for admin tasks)
sudo usermod -aG sudo github-runner

# Or if you prefer to use existing user, skip this step
```

**2. Create deployment directory:**

```bash
# Login as the deployment user
su - github-runner
# Or: sudo su - github-runner

# Create deployment directory
mkdir -p ~/drupal-reminder
cd ~/drupal-reminder
```

**3. Set up SSH directory and permissions:**

```bash
# Create .ssh directory if it doesn't exist
mkdir -p ~/.ssh

# Set correct permissions
chmod 700 ~/.ssh

# Create authorized_keys file if it doesn't exist
touch ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys
```

**4. Generate SSH key pair on your local machine (if not done yet):**

```bash
# On your LOCAL machine (not on server)
ssh-keygen -t ed25519 -C "github-actions-deploy" -f ~/.ssh/github_actions_deploy -N ""

# This creates two files:
# - ~/.ssh/github_actions_deploy (private key) - you'll add this to GitHub Secrets
# - ~/.ssh/github_actions_deploy.pub (public key) - you'll add this to server
```

**5. Add public key to server:**

```bash
# Option A: Using ssh-copy-id (on your LOCAL machine)
ssh-copy-id -i ~/.ssh/github_actions_deploy.pub github-runner@your-server

# Option B: Manual method (on SERVER, as github-runner user)
# First, copy the public key content from your local machine:
cat ~/.ssh/github_actions_deploy.pub
# Then on server, paste it into authorized_keys:
echo "PASTE_PUBLIC_KEY_HERE" >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys

# Verify the key was added
cat ~/.ssh/authorized_keys
```

**6. Configure SSH server (sshd_config):**

```bash
# On server, as root or with sudo
sudo nano /etc/ssh/sshd_config

# Add or modify these lines:
PubkeyAuthentication yes
PasswordAuthentication no
PermitRootLogin no
PermitEmptyPasswords no
AuthorizedKeysFile .ssh/authorized_keys

# Save and exit (Ctrl+X, then Y, then Enter)

# Test SSH configuration for syntax errors
sudo sshd -t

# If test passes, restart SSH service
sudo systemctl restart ssh
# Or on some systems:
sudo systemctl restart sshd

# Verify SSH service is running
sudo systemctl status ssh
```

**7. Install and configure fail2ban (recommended for security):**

```bash
# Install fail2ban
sudo apt update
sudo apt install fail2ban -y

# Enable and start fail2ban
sudo systemctl enable fail2ban
sudo systemctl start fail2ban

# Verify it's running
sudo systemctl status fail2ban

# Check fail2ban status
sudo fail2ban-client status sshd
```

**8. Create .env file on server:**

```bash
# As github-runner user
cd ~/drupal-reminder

# Create .env file
nano .env

# Add your configuration:
TELEGRAM_BOT_TOKEN=your_telegram_bot_token_here
DRUPAL_SITE_URL=https://example.com
RSS_URL=https://example.com/rss.xml
RSS_AUTH_USER=rss_user_if_needed
RSS_AUTH_PASSWORD=rss_password_if_needed

# Save and set permissions
chmod 600 .env
```

**9. Verify SSH connection from local machine:**

```bash
# On your LOCAL machine
ssh -i ~/.ssh/github_actions_deploy github-runner@your-server

# If connection succeeds, you're all set!
# Test that you can create files:
touch ~/test.txt && rm ~/test.txt && echo "✅ Write permissions OK"
```

**10. Verify all permissions are correct:**

```bash
# On server, as github-runner user
ls -la ~/
# Should show: drwxr-xr-x ... drupal-reminder

ls -la ~/.ssh/
# Should show:
# drwx------ ... .
# -rw------- ... authorized_keys

ls -la ~/drupal-reminder/
# Should show .env file with permissions -rw-------
```

**11. Test deployment directory path:**

```bash
# On server
pwd
# Note the full path (e.g., /home/github-runner/drupal-reminder)

# This path will be used in GitHub Secret DEPLOY_PATH
# You can use either:
# - Full path: /home/github-runner/drupal-reminder
# - Or relative: ~/drupal-reminder (will be expanded automatically)
```

**12. Optional: Test manual deployment:**

```bash
# On server, in deployment directory
cd ~/drupal-reminder

# Manually copy and test the binary (if you have it locally)
# From your local machine:
scp -i ~/.ssh/github_actions_deploy drupal-reminder github-runner@your-server:~/drupal-reminder/

# On server:
chmod +x drupal-reminder
./drupal-reminder
# Press Ctrl+C to stop
```

**13. Check firewall rules (if using UFW):**

```bash
# Check firewall status
sudo ufw status

# If SSH port is blocked, allow it:
sudo ufw allow 22/tcp
sudo ufw allow ssh

# Verify
sudo ufw status numbered
```

**14. Monitor SSH logs (optional, for debugging):**

```bash
# Real-time SSH authentication logs
sudo tail -f /var/log/auth.log | grep github-runner

# Or on systems with journald:
sudo journalctl -u ssh -f
```

**Quick Verification Checklist:**

- [ ] User created and has home directory
- [ ] `.ssh` directory exists with permissions 700
- [ ] `authorized_keys` file exists with permissions 600
- [ ] Public key is in `authorized_keys`
- [ ] Deployment directory exists
- [ ] `.env` file created with correct values
- [ ] SSH connection works without password
- [ ] `sshd_config` configured correctly
- [ ] SSH service restarted after config changes
- [ ] fail2ban installed and running (optional but recommended)
- [ ] Firewall allows SSH connections

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
