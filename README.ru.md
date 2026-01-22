# Бот уведомлений о новых статьях Drupal

Этот бот предназначен для добавления в групповые чаты Telegram. Он отслеживает сайт Drupal и уведомляет пользователей о публикации новых статей.

## Цель
- Мониторинг сайта Drupal
- Обнаружение новых опубликованных статей
- Отправка уведомлений в групповой чат Telegram

## Начало работы

### Требования
- Язык программирования Go (версия 1.16 или выше)
- Токен Telegram бота
- Запущенный сервер для Telegram бота

### Установка

1. **Клонируйте репозиторий:**
   ```sh
   git clone https://github.com/DennisMorosoff/drupal-reminder.git
   cd drupal-reminder
   ```

2. **Настройте переменные окружения:**
   Создайте файл `.env` со следующим содержимым:
   ```env
   TELEGRAM_BOT_TOKEN=ваш_токен_telegram_бота
   DRUPAL_SITE_URL=https://example.com
   RSS_URL=https://www.dennismorosoff.ru/rss.xml
   DRUPAL_AUTH_METHOD=basic
   DRUPAL_LOGIN_URL=/user/login
   RSS_AUTH_USER=пользователь_для_rss_если_нужен
   RSS_AUTH_PASSWORD=пароль_для_rss_если_нужен
   ```

3. **Соберите и запустите бота:**
   
   **Вариант 1: Использование скрипта сборки (рекомендуется - автоматически устанавливает версию из git):**
   ```sh
   # Linux/macOS
   ./build.sh
   
   # Windows
   build.bat
   ```
   
   **Вариант 2: Ручная сборка:**
   ```sh
   go build -o drupal-reminder-bot main.go
   ```
   
   **Вариант 3: Сборка с информацией о версии:**
   ```sh
   VERSION=$(git describe --tags --always --dirty)
   BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
   COMMIT=$(git rev-parse --short HEAD)
   go build -ldflags "-X main.version=$VERSION -X main.buildTime=$BUILD_TIME -X main.commitHash=$COMMIT" -o drupal-reminder-bot main.go
   ```
   
   **Запуск бота:**
   ```sh
   ./drupal-reminder-bot
   ```

### Конфигурация
- Бот будет отслеживать сайт Drupal, указанный в `DRUPAL_SITE_URL`
- Бот будет отправлять уведомления в групповой чат Telegram, куда он добавлен
- RSS URL по умолчанию: `https://www.dennismorosoff.ru/rss.xml`
- `DRUPAL_AUTH_METHOD=basic` использует HTTP Basic Auth при запросе RSS
- `DRUPAL_AUTH_METHOD=cookie` логинится в Drupal через `DRUPAL_LOGIN_URL` и использует сессионные cookies
- Для закрытых статей убедитесь, что роль авторизованного пользователя имеет доступ к RSS View

### Команды бота
- `/start` - Начать работу с ботом
- `/fetch` - Получить содержимое сайта
- `/check` - Проверить последнюю статью и отправить уведомление во все группы
- `/about` - Показать версию бота и информацию о сборке

## Деплой

### Автоматический деплой через GitHub Actions

Бот может быть автоматически развернут на сервере при отправке изменений в ветку `master`.

#### Настройка GitHub Secrets

Настройте следующие секреты в настройках репозитория GitHub (Settings → Secrets and variables → Actions):

- `DEPLOY_HOST` - IP адрес или доменное имя сервера (например, `192.168.1.100` или `example.com`)
- `DEPLOY_USER` - Имя пользователя SSH для доступа к серверу (например, `root` или ваше имя пользователя)
- `DEPLOY_SSH_KEY` - Приватный SSH ключ для доступа к серверу (скопируйте полное содержимое приватного ключа)
- `DEPLOY_PATH` - Путь для деплоя на сервере (например, `/home/user/drupal-reminder` или `~/drupal-reminder`)

**Важно:** Для автоматического деплоя рекомендуется использовать SSH ключ без пароля (passphrase). Если ваш ключ защищен паролем, создайте отдельный ключ для деплоя:

```sh
ssh-keygen -t ed25519 -C "github-actions-deploy" -f ~/.ssh/github_actions_deploy -N ""
```

#### Как сгенерировать SSH ключ

1. Создайте пару SSH ключей (если у вас её нет):
   ```sh
   ssh-keygen -t ed25519 -C "github-actions"
   ```
   **Важно:** При запросе passphrase нажмите Enter, чтобы создать ключ без пароля (для автоматизации).

2. Скопируйте публичный ключ на сервер:
   ```sh
   ssh-copy-id -i ~/.ssh/id_ed25519.pub user@your-server
   ```

3. Скопируйте содержимое приватного ключа и добавьте его в GitHub Secrets как `DEPLOY_SSH_KEY`:
   ```sh
   cat ~/.ssh/id_ed25519
   ```
   Скопируйте весь вывод, включая строки `-----BEGIN OPENSSH PRIVATE KEY-----` и `-----END OPENSSH PRIVATE KEY-----`.

#### Где настроить GitHub Secrets

1. Откройте ваш репозиторий на GitHub
2. Перейдите в **Settings** (вкладка вверху страницы репозитория)
3. В левом меню выберите **Secrets and variables** → **Actions**
4. Нажмите **New repository secret**
5. Для каждого секрета укажите:
   - **Name** - имя секрета (например, `DEPLOY_HOST`)
   - **Secret** - значение секрета
   - Нажмите **Add secret**

#### Настройка сервера (Полный чеклист)

Перед настройкой GitHub Actions необходимо настроить сервер. Выполните эти команды **на вашем сервере** (подключитесь по SSH как root или пользователь с sudo правами):

**1. Создайте отдельного пользователя для деплоя (рекомендуется):**

```bash
# Создайте нового пользователя (замените 'github-runner' на ваше предпочтение)
sudo adduser github-runner

# При запросе установите пароль (позже можно отключить) и заполните дополнительную информацию

# Добавьте пользователя в группу sudo (опционально, если нужны права администратора)
sudo usermod -aG sudo github-runner

# Переключитесь на нового пользователя
su - github-runner
```

**2. Настройте SSH директорию и права для пользователя деплоя:**

```bash
# Убедитесь, что вы залогинены как пользователь деплоя
# Создайте директорию .ssh
mkdir -p ~/.ssh

# Установите правильные права для директории .ssh (700 = rwx------)
chmod 700 ~/.ssh

# Создайте файл authorized_keys
touch ~/.ssh/authorized_keys

# Установите правильные права для authorized_keys (600 = rw-------)
chmod 600 ~/.ssh/authorized_keys

# Проверьте права
ls -la ~/.ssh/
# Ожидаемый вывод:
# drwx------ 2 github-runner github-runner 4096 ... .
# -rw------- 1 github-runner github-runner   ... authorized_keys
```

**3. Добавьте SSH публичный ключ в authorized_keys:**

**На вашей локальной машине**, создайте пару SSH ключей, если её нет:

```bash
# Создайте SSH ключ (нажмите Enter при запросе passphrase, чтобы создать ключ без пароля)
ssh-keygen -t ed25519 -C "github-actions-deploy"

# Или создайте отдельный ключ для деплоя
ssh-keygen -t ed25519 -C "github-actions-deploy" -f ~/.ssh/github_actions_deploy -N ""
```

**На вашей локальной машине**, скопируйте публичный ключ на сервер:

```bash
# Метод 1: Использование ssh-copy-id (самый простой способ)
ssh-copy-id -i ~/.ssh/id_ed25519.pub github-runner@your-server

# Или если используете отдельный ключ:
ssh-copy-id -i ~/.ssh/github_actions_deploy.pub github-runner@your-server

# Метод 2: Ручное копирование (если ssh-copy-id не работает)
# Сначала, отобразите ваш публичный ключ:
cat ~/.ssh/id_ed25519.pub

# Затем на сервере, добавьте его в authorized_keys:
# (Выполните эту команду на сервере, заменив PUBLIC_KEY на вывод из команды выше)
echo "PUBLIC_KEY" >> ~/.ssh/authorized_keys
```

**На сервере**, проверьте, что ключ добавлен:

```bash
# Проверьте, что публичный ключ есть в authorized_keys
cat ~/.ssh/authorized_keys

# Проверьте права на файл
ls -la ~/.ssh/authorized_keys
# Должно показать: -rw------- (600)
```

**4. Настройте SSH сервер (sshd_config) для безопасности:**

```bash
# Переключитесь обратно на root или используйте sudo
sudo nano /etc/ssh/sshd_config

# Или используйте ваш предпочитаемый редактор
sudo vi /etc/ssh/sshd_config
```

**Добавьте или измените эти настройки в `/etc/ssh/sshd_config`:**

```
# Включить аутентификацию по публичному ключу (обычно уже включено по умолчанию)
PubkeyAuthentication yes

# Отключить аутентификацию по паролю (рекомендуется для безопасности)
PasswordAuthentication no

# Запретить вход от root (рекомендуется для безопасности)
PermitRootLogin no

# Запретить пустые пароли
PermitEmptyPasswords no

# Установить максимум попыток аутентификации
MaxAuthTries 3

# Установить уровень логирования
LogLevel INFO
```

**После редактирования, проверьте конфигурацию SSH:**

```bash
# Проверьте SSH конфигурацию на синтаксические ошибки
sudo sshd -t

# Если проверка прошла, перезапустите SSH сервис
sudo systemctl restart ssh

# Или на некоторых системах:
sudo systemctl restart sshd

# Проверьте, что SSH сервис запущен
sudo systemctl status ssh
```

**Важно:** Перед отключением аутентификации по паролю убедитесь, что вы можете войти с помощью SSH ключей! Сначала протестируйте вход по SSH ключу:

```bash
# На вашей локальной машине, протестируйте SSH подключение
ssh -i ~/.ssh/id_ed25519 github-runner@your-server

# Если всё работает, можно безопасно отключить аутентификацию по паролю
```

**5. (Опционально) Установите и настройте fail2ban для защиты от брутфорса:**

```bash
# Установите fail2ban
sudo apt update
sudo apt install fail2ban -y

# Включите и запустите fail2ban
sudo systemctl enable fail2ban
sudo systemctl start fail2ban

# Проверьте статус
sudo systemctl status fail2ban

# Просмотрите логи fail2ban
sudo tail -f /var/log/fail2ban.log
```

**6. Создайте директорию для деплоя:**

```bash
# Убедитесь, что вы залогинены как пользователь деплоя
# Создайте директорию для деплоя
mkdir -p ~/drupal-reminder

# Или используйте пользовательский путь (не забудьте обновить DEPLOY_PATH в GitHub Secrets)
mkdir -p /home/github-runner/drupal-reminder

# Установите владельца (если нужно)
sudo chown -R github-runner:github-runner ~/drupal-reminder

# Установите права
chmod 755 ~/drupal-reminder
```

**7. Создайте файл .env в директории деплоя:**

```bash
# Перейдите в директорию деплоя
cd ~/drupal-reminder

# Создайте файл .env
nano .env

# Добавьте вашу конфигурацию:
TELEGRAM_BOT_TOKEN=ваш_токен_telegram_бота
DRUPAL_SITE_URL=https://example.com
RSS_URL=https://www.dennismorosoff.ru/rss.xml
RSS_AUTH_USER=пользователь_для_rss_если_нужен
RSS_AUTH_PASSWORD=пароль_для_rss_если_нужен

# Сохраните и установите права
chmod 600 .env
```

**8. (Опционально) Настройте systemd сервис для бота (альтернатива nohup):**

```bash
# Создайте файл systemd сервиса
sudo nano /etc/systemd/system/drupal-reminder.service
```

**Добавьте это содержимое (настройте пути по необходимости):**

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

**Включите и запустите сервис:**

```bash
# Перезагрузите демон systemd
sudo systemctl daemon-reload

# Включите сервис для запуска при загрузке
sudo systemctl enable drupal-reminder

# Запустите сервис
sudo systemctl start drupal-reminder

# Проверьте статус
sudo systemctl status drupal-reminder

# Просмотрите логи
sudo journalctl -u drupal-reminder -f
```

**9. Проверьте, что всё настроено правильно:**

```bash
# Протестируйте вход по SSH ключу (с вашей локальной машины)
ssh -i ~/.ssh/id_ed25519 github-runner@your-server

# На сервере, проверьте:
# - Права на SSH директорию
ls -la ~/.ssh/

# - Файл authorized_keys существует и имеет правильные права
ls -la ~/.ssh/authorized_keys

# - Директория деплоя существует
ls -ld ~/drupal-reminder

# - Файл .env существует
ls -la ~/drupal-reminder/.env

# - SSH сервис запущен
sudo systemctl status ssh

# - Проверка SSH конфигурации проходит
sudo sshd -t
```

**Сводка необходимых прав:**

- Директория `~/.ssh`: `700` (drwx------)
- Файл `~/.ssh/authorized_keys`: `600` (-rw-------)
- Директория деплоя: `755` (drwxr-xr-x)
- Файл `.env`: `600` (-rw-------)
- Бинарный файл бота: `755` (исполняемый)

#### Процесс деплоя

При отправке изменений в ветку `master`:
1. GitHub Actions компилирует бота для Linux
2. Бинарный файл копируется на сервер через SCP
3. Старый процесс бота останавливается
4. Новый процесс бота запускается в фоновом режиме

Логи бота будут записываться в файл `bot.log` в директории деплоя (или в journal systemd, если используется systemd сервис).

## Вклад в проект
Вклад в проект приветствуется! Пожалуйста, откройте issue или отправьте pull request с вашими изменениями.

## Лицензия
Этот проект лицензирован под MIT License - см. файл [LICENSE](LICENSE) для деталей.

## Язык
- [English](README.md)
- [Русский](README.ru.md)
