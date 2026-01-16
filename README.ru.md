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
   RSS_AUTH_USER=пользователь_для_rss_если_нужен
   RSS_AUTH_PASSWORD=пароль_для_rss_если_нужен
   ```

3. **Соберите и запустите бота:**
   ```sh
   go build -o drupal-reminder main.go
   ./drupal-reminder
   ```

### Конфигурация
- Бот будет отслеживать сайт Drupal, указанный в `DRUPAL_SITE_URL`
- Бот будет отправлять уведомления в групповой чат Telegram, куда он добавлен
- RSS URL по умолчанию: `https://www.dennismorosoff.ru/rss.xml`

### Команды бота
- `/start` - Начать работу с ботом
- `/fetch` - Получить содержимое сайта
- `/check` - Проверить последнюю статью и отправить уведомление во все группы

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

Перед настройкой GitHub Actions необходимо настроить сервер. Выполните эти команды **на вашем сервере**:

**1. Создайте отдельного пользователя для деплоя (рекомендуется):**

```bash
# Создайте нового пользователя (замените 'github-runner' на ваше предпочтение)
sudo adduser github-runner

# Добавьте пользователя в группу sudo (опционально, если нужны права администратора)
sudo usermod -aG sudo github-runner

# Или используйте существующего пользователя, пропустите этот шаг
```

**2. Создайте директорию для деплоя:**

```bash
# Войдите под пользователем деплоя
su - github-runner
# Или: sudo su - github-runner

# Создайте директорию для деплоя
mkdir -p ~/drupal-reminder
cd ~/drupal-reminder
```

**3. Настройте директорию SSH и права доступа:**

```bash
# Создайте директорию .ssh, если её нет
mkdir -p ~/.ssh

# Установите правильные права
chmod 700 ~/.ssh

# Создайте файл authorized_keys, если его нет
touch ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys
```

**4. Сгенерируйте пару SSH ключей на локальной машине (если еще не сделано):**

```bash
# На вашей ЛОКАЛЬНОЙ машине (не на сервере)
ssh-keygen -t ed25519 -C "github-actions-deploy" -f ~/.ssh/github_actions_deploy -N ""

# Это создаст два файла:
# - ~/.ssh/github_actions_deploy (приватный ключ) - добавите в GitHub Secrets
# - ~/.ssh/github_actions_deploy.pub (публичный ключ) - добавите на сервер
```

**5. Добавьте публичный ключ на сервер:**

```bash
# Вариант A: Используя ssh-copy-id (на вашей ЛОКАЛЬНОЙ машине)
ssh-copy-id -i ~/.ssh/github_actions_deploy.pub github-runner@your-server

# Вариант B: Ручной метод (на СЕРВЕРЕ, под пользователем github-runner)
# Сначала скопируйте содержимое публичного ключа с локальной машины:
cat ~/.ssh/github_actions_deploy.pub
# Затем на сервере вставьте его в authorized_keys:
echo "ВСТАВЬТЕ_ПУБЛИЧНЫЙ_КЛЮЧ_СЮДА" >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys

# Проверьте, что ключ добавлен
cat ~/.ssh/authorized_keys
```

**6. Настройте SSH сервер (sshd_config):**

```bash
# На сервере, от root или с sudo
sudo nano /etc/ssh/sshd_config

# Добавьте или измените эти строки:
PubkeyAuthentication yes
PasswordAuthentication no
PermitRootLogin no
PermitEmptyPasswords no
AuthorizedKeysFile .ssh/authorized_keys

# Сохраните и выйдите (Ctrl+X, затем Y, затем Enter)

# Проверьте конфигурацию SSH на ошибки
sudo sshd -t

# Если проверка прошла, перезапустите SSH сервис
sudo systemctl restart ssh
# Или на некоторых системах:
sudo systemctl restart sshd

# Проверьте, что SSH сервис запущен
sudo systemctl status ssh
```

**7. Установите и настройте fail2ban (рекомендуется для безопасности):**

```bash
# Установите fail2ban
sudo apt update
sudo apt install fail2ban -y

# Включите и запустите fail2ban
sudo systemctl enable fail2ban
sudo systemctl start fail2ban

# Проверьте, что он работает
sudo systemctl status fail2ban

# Проверьте статус fail2ban
sudo fail2ban-client status sshd
```

**8. Создайте файл .env на сервере:**

```bash
# Под пользователем github-runner
cd ~/drupal-reminder

# Создайте файл .env
nano .env

# Добавьте вашу конфигурацию:
TELEGRAM_BOT_TOKEN=ваш_токен_telegram_бота
DRUPAL_SITE_URL=https://example.com
RSS_URL=https://example.com/rss.xml
RSS_AUTH_USER=пользователь_для_rss_если_нужен
RSS_AUTH_PASSWORD=пароль_для_rss_если_нужен

# Сохраните и установите права
chmod 600 .env
```

**9. Проверьте SSH подключение с локальной машины:**

```bash
# На вашей ЛОКАЛЬНОЙ машине
ssh -i ~/.ssh/github_actions_deploy github-runner@your-server

# Если подключение успешно, всё настроено!
# Проверьте, что можете создавать файлы:
touch ~/test.txt && rm ~/test.txt && echo "✅ Права на запись в порядке"
```

**10. Проверьте, что все права настроены правильно:**

```bash
# На сервере, под пользователем github-runner
ls -la ~/
# Должно показать: drwxr-xr-x ... drupal-reminder

ls -la ~/.ssh/
# Должно показать:
# drwx------ ... .
# -rw------- ... authorized_keys

ls -la ~/drupal-reminder/
# Должен показать файл .env с правами -rw-------
```

**11. Проверьте путь директории деплоя:**

```bash
# На сервере
pwd
# Запомните полный путь (например, /home/github-runner/drupal-reminder)

# Этот путь будет использоваться в GitHub Secret DEPLOY_PATH
# Можно использовать:
# - Полный путь: /home/github-runner/drupal-reminder
# - Или относительный: ~/drupal-reminder (будет автоматически развернут)
```

**12. Опционально: Проверьте ручной деплой:**

```bash
# На сервере, в директории деплоя
cd ~/drupal-reminder

# Вручную скопируйте и протестируйте бинарник (если он у вас есть локально)
# С вашей локальной машины:
scp -i ~/.ssh/github_actions_deploy drupal-reminder github-runner@your-server:~/drupal-reminder/

# На сервере:
chmod +x drupal-reminder
./drupal-reminder
# Нажмите Ctrl+C для остановки
```

**13. Проверьте правила файрвола (если используете UFW):**

```bash
# Проверьте статус файрвола
sudo ufw status

# Если SSH порт заблокирован, разрешите его:
sudo ufw allow 22/tcp
sudo ufw allow ssh

# Проверьте
sudo ufw status numbered
```

**14. Мониторинг SSH логов (опционально, для отладки):**

```bash
# Логи аутентификации SSH в реальном времени
sudo tail -f /var/log/auth.log | grep github-runner

# Или на системах с journald:
sudo journalctl -u ssh -f
```

**Чеклист быстрой проверки:**

- [ ] Пользователь создан и имеет домашнюю директорию
- [ ] Директория `.ssh` существует с правами 700
- [ ] Файл `authorized_keys` существует с правами 600
- [ ] Публичный ключ добавлен в `authorized_keys`
- [ ] Директория деплоя существует
- [ ] Файл `.env` создан с правильными значениями
- [ ] SSH подключение работает без пароля
- [ ] `sshd_config` настроен правильно
- [ ] SSH сервис перезапущен после изменений конфигурации
- [ ] fail2ban установлен и работает (опционально, но рекомендуется)
- [ ] Файрвол разрешает SSH подключения

#### Процесс деплоя

При отправке изменений в ветку `master`:
1. GitHub Actions компилирует бота для Linux
2. Бинарный файл копируется на сервер через SCP
3. Старый процесс бота останавливается
4. Новый процесс бота запускается в фоновом режиме

Логи бота будут записываться в файл `bot.log` в директории деплоя.

## Вклад в проект
Вклад в проект приветствуется! Пожалуйста, откройте issue или отправьте pull request с вашими изменениями.

## Лицензия
Этот проект лицензирован под MIT License - см. файл [LICENSE](LICENSE) для деталей.

## Язык
- [English](README.md)
- [Русский](README.ru.md)
