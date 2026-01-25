package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/PuerkitoBio/goquery"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

var (
	version    = "dev"
	buildTime  = "unknown"
	commitHash = "unknown"
)

const (
	telegramMessageLimit = 4096
	stateFileName        = "state.json"
	checkInterval        = 1 * time.Hour
	notificationDelay    = 15 * time.Minute
)

// RSS структуры
type RSSFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel RSSChannel `xml:"channel"`
}

type RSSChannel struct {
	Title string    `xml:"title"`
	Link  string    `xml:"link"`
	Items []RSSItem `xml:"item"`
}

type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
}

// Структура для хранения состояния
type State struct {
	LastCheckedArticles []string `json:"last_checked_articles"`
	LastCheckTime       string   `json:"last_check_time"`
}

// Менеджер бота
type BotManager struct {
	bot              *tgbotapi.BotAPI
	rssURL           string
	rssAuthUser      string
	rssAuthPassword  string
	authMethod       string
	loginURL         string
	httpClient       *http.Client
	stateFile        string
	knownArticles    map[string]bool
	knownArticlesMu  sync.RWMutex
	chats            map[int64]bool
	chatsMu          sync.RWMutex
	notificationChan chan RSSItem
	ctx              context.Context
	cancel           context.CancelFunc
}

func truncateToTelegramLimit(text string) string {
	if len(text) <= telegramMessageLimit {
		return text
	}
	return text[:telegramMessageLimit-3] + "..."
}

// Разбивает текст на части, не превышающие лимит Telegram
func splitToTelegramMessages(text string) []string {
	if len(text) <= telegramMessageLimit {
		return []string{text}
	}

	var messages []string
	lines := strings.Split(text, "\n")
	currentMessage := ""

	for _, line := range lines {
		// Если добавление текущей строки превысит лимит, сохраняем текущее сообщение и начинаем новое
		if len(currentMessage)+len(line)+1 > telegramMessageLimit {
			if currentMessage != "" {
				messages = append(messages, strings.TrimSpace(currentMessage))
				currentMessage = ""
			}
			// Если одна строка слишком длинная, обрезаем её
			if len(line) > telegramMessageLimit {
				line = truncateToTelegramLimit(line)
			}
		}
		if currentMessage != "" {
			currentMessage += "\n"
		}
		currentMessage += line
	}

	if currentMessage != "" {
		messages = append(messages, strings.TrimSpace(currentMessage))
	}

	return messages
}

func resolveURL(baseURL string, pathOrURL string) (string, error) {
	if strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://") {
		return pathOrURL, nil
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse base URL: %w", err)
	}

	ref, err := url.Parse(pathOrURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	return base.ResolveReference(ref).String(), nil
}

func (bm *BotManager) newRequest(method string, targetURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, targetURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if bm.authMethod == "basic" && bm.rssAuthUser != "" && bm.rssAuthPassword != "" {
		req.SetBasicAuth(bm.rssAuthUser, bm.rssAuthPassword)
		log.Printf("Using Basic Auth for request to: %s", targetURL)
	}

	return req, nil
}

func initAuthClient(rssURL string, authMethod string, loginURL string, username string, password string) (*http.Client, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	switch authMethod {
	case "basic":
		return client, nil
	case "cookie":
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create cookie jar: %w", err)
		}
		client.Jar = jar

		if err := loginToDrupal(client, rssURL, loginURL, username, password); err != nil {
			return nil, err
		}
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported DRUPAL_AUTH_METHOD: %s", authMethod)
	}
}

func loginToDrupal(client *http.Client, rssURL string, loginURL string, username string, password string) error {
	if username == "" || password == "" {
		return fmt.Errorf("DRUPAL_AUTH_METHOD=cookie requires RSS_AUTH_USER and RSS_AUTH_PASSWORD")
	}

	log.Printf("Attempting to login to Drupal at %s (login URL: %s)", rssURL, loginURL)
	loginPageURL, err := resolveURL(rssURL, loginURL)
	if err != nil {
		return err
	}

	log.Printf("Loading login page: %s", loginPageURL)
	resp, err := client.Get(loginPageURL)
	if err != nil {
		return fmt.Errorf("failed to load login page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("⚠️  Login page returned status: %d", resp.StatusCode)
		return fmt.Errorf("login page returned status: %d", resp.StatusCode)
	}
	log.Printf("✅ Login page loaded successfully")

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to parse login page: %w", err)
	}

	form := doc.Find("form#user-login-form").First()
	if form.Length() == 0 {
		form = doc.Find("form").First()
	}
	if form.Length() == 0 {
		return fmt.Errorf("login form not found on login page")
	}

	action, exists := form.Attr("action")
	if !exists || strings.TrimSpace(action) == "" {
		action = loginPageURL
	}
	actionURL, err := resolveURL(loginPageURL, action)
	if err != nil {
		return err
	}

	values := url.Values{}
	values.Set("name", username)
	values.Set("pass", password)

	form.Find("input").Each(func(_ int, input *goquery.Selection) {
		name, hasName := input.Attr("name")
		if !hasName || name == "" {
			return
		}
		if name == "name" || name == "pass" {
			return
		}
		if value, ok := input.Attr("value"); ok {
			values.Set(name, value)
		}
	})

	if values.Get("form_id") == "" {
		values.Set("form_id", "user_login_form")
	}
	if values.Get("op") == "" {
		values.Set("op", "Log in")
	}

	log.Printf("Submitting login form to: %s", actionURL)
	req, err := http.NewRequest("POST", actionURL, strings.NewReader(values.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	postResp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Failed to submit login form: %v", err)
		return fmt.Errorf("failed to submit login form: %w", err)
	}
	defer postResp.Body.Close()

	bodyBytes, _ := io.ReadAll(postResp.Body)
	bodyText := string(bodyBytes)

	if postResp.StatusCode >= http.StatusBadRequest {
		log.Printf("❌ Login failed with status: %d", postResp.StatusCode)
		return fmt.Errorf("login failed with status: %d", postResp.StatusCode)
	}

	if strings.Contains(bodyText, "user-login-form") && strings.Contains(postResp.Request.URL.Path, "user/login") {
		log.Printf("❌ Login failed: login form still present, check username/password")
		return fmt.Errorf("login failed: check username/password")
	}

	log.Printf("✅ Login successful")
	return nil
}

// Перелогинивание при истечении сессии (для метода cookie)
func (bm *BotManager) renewAuth() error {
	if bm.authMethod != "cookie" {
		return nil // Для basic auth перелогинивание не требуется
	}

	if bm.rssAuthUser == "" || bm.rssAuthPassword == "" {
		return fmt.Errorf("RSS_AUTH_USER and RSS_AUTH_PASSWORD required for cookie auth renewal")
	}

	log.Printf("Renewing authentication (cookie method)...")

	// Создаем новый cookie jar
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("failed to create cookie jar: %w", err)
	}

	// Создаем новый клиент с новым cookie jar
	newClient := &http.Client{
		Timeout: 30 * time.Second,
		Jar:     jar,
	}

	// Выполняем логин
	if err := loginToDrupal(newClient, bm.rssURL, bm.loginURL, bm.rssAuthUser, bm.rssAuthPassword); err != nil {
		return fmt.Errorf("failed to renew auth: %w", err)
	}

	// Обновляем httpClient
	bm.httpClient = newClient
	log.Printf("✅ Authentication renewed successfully")

	return nil
}

func (bm *BotManager) fetchWebsiteContent(targetURL string) (string, error) {
	req, err := bm.newRequest("GET", targetURL, nil)
	if err != nil {
		return "", err
	}

	httpResp, err := bm.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		if httpResp.StatusCode == http.StatusUnauthorized || httpResp.StatusCode == http.StatusForbidden {
			return "", fmt.Errorf("authentication failed (status code: %d). Check your credentials", httpResp.StatusCode)
		}
		return "", fmt.Errorf("unexpected status code: %d", httpResp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	textContent := doc.Text()
	return textContent, nil
}

func (bm *BotManager) fetchFirstParagraph(targetURL string) (string, error) {
	req, err := bm.newRequest("GET", targetURL, nil)
	if err != nil {
		return "", err
	}

	httpResp, err := bm.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		if httpResp.StatusCode == http.StatusUnauthorized || httpResp.StatusCode == http.StatusForbidden {
			return "", fmt.Errorf("authentication failed (status code: %d). Check your credentials", httpResp.StatusCode)
		}
		return "", fmt.Errorf("unexpected status code: %d", httpResp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	firstParagraph := doc.Find("p").First()
	if firstParagraph.Length() > 0 {
		text := firstParagraph.Text()
		if text != "" {
			return text, nil
		}
	}

	return "", fmt.Errorf("no paragraph found on the page")
}

// Загрузка состояния из файла
func loadState(filename string) (*State, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{
				LastCheckedArticles: []string{},
				LastCheckTime:       "",
			}, nil
		}
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// Сохранение состояния в файл
func saveState(filename string, state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// Получение RSS с HTTP Basic Auth
func (bm *BotManager) fetchRSSFeed() (*RSSFeed, error) {
	req, err := bm.newRequest("GET", bm.rssURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := bm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch RSS: %w", err)
	}
	defer resp.Body.Close()

	// Проверяем ошибки авторизации
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		log.Printf("⚠️  Authentication failed (status: %d), attempting to renew auth...", resp.StatusCode)
		
		// Пытаемся перелогиниться (только для метода cookie)
		if bm.authMethod == "cookie" {
			if err := bm.renewAuth(); err != nil {
				return nil, fmt.Errorf("authentication failed and renewal failed: %w", err)
			}
			
			// Повторяем запрос после перелогинивания
			req, err := bm.newRequest("GET", bm.rssURL, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request after auth renewal: %w", err)
			}
			
			resp, err = bm.httpClient.Do(req)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch RSS after auth renewal: %w", err)
			}
			defer resp.Body.Close()
			
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("authentication still failing after renewal (status: %d)", resp.StatusCode)
			}
		} else {
			// Для basic auth просто возвращаем ошибку
			return nil, fmt.Errorf("authentication failed (status: %d). Check your credentials", resp.StatusCode)
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var feed RSSFeed
	decoder := xml.NewDecoder(resp.Body)
	if err := decoder.Decode(&feed); err != nil {
		return nil, fmt.Errorf("failed to parse RSS: %w", err)
	}

	log.Printf("✅ Successfully fetched RSS feed: %d articles found", len(feed.Channel.Items))
	return &feed, nil
}

// Проверка RSS и поиск новых статей
func (bm *BotManager) checkRSSFeed() error {
	feed, err := bm.fetchRSSFeed()
	if err != nil {
		return fmt.Errorf("failed to fetch RSS feed: %w", err)
	}

	bm.knownArticlesMu.Lock()
	defer bm.knownArticlesMu.Unlock()

	newArticles := []RSSItem{}
	for _, item := range feed.Channel.Items {
		if !bm.knownArticles[item.GUID] {
			bm.knownArticles[item.GUID] = true
			newArticles = append(newArticles, item)
		}
	}

	// Добавляем новые статьи в очередь уведомлений
	for _, item := range newArticles {
		select {
		case bm.notificationChan <- item:
		case <-bm.ctx.Done():
			return bm.ctx.Err()
		default:
			log.Printf("Warning: notification channel full, skipping article: %s", item.Title)
		}
	}

	if len(newArticles) > 0 {
		log.Printf("Found %d new articles", len(newArticles))
	}

	// Сохраняем состояние
	articleList := make([]string, 0, len(bm.knownArticles))
	for guid := range bm.knownArticles {
		articleList = append(articleList, guid)
	}

	state := &State{
		LastCheckedArticles: articleList,
		LastCheckTime:       time.Now().Format(time.RFC3339),
	}

	if err := saveState(bm.stateFile, state); err != nil {
		log.Printf("Failed to save state: %v", err)
	}

	return nil
}

// Периодическая проверка RSS
func (bm *BotManager) startRSSMonitoring() {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// Первая проверка сразу при запуске
	if err := bm.checkRSSFeed(); err != nil {
		log.Printf("Error checking RSS feed: %v", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := bm.checkRSSFeed(); err != nil {
				log.Printf("Error checking RSS feed: %v", err)
			}
		case <-bm.ctx.Done():
			return
		}
	}
}

// Обработка очереди уведомлений с задержкой
func (bm *BotManager) startNotificationQueue() {
	for {
		select {
		case item := <-bm.notificationChan:
			// Запланировать отправку через 15 минут
			go func(article RSSItem) {
				time.Sleep(notificationDelay)

				// Проверяем, не был ли контекст отменен
				select {
				case <-bm.ctx.Done():
					return
				default:
				}

				bm.sendNotificationToAllChats(article)
			}(item)

		case <-bm.ctx.Done():
			return
		}
	}
}

// Отправка уведомления во все чаты
func (bm *BotManager) sendNotificationToAllChats(item RSSItem) {
	message := fmt.Sprintf("📰 Новая статья: %s\n\n🔗 %s", item.Title, item.Link)

	bm.chatsMu.RLock()
	chatIDs := make([]int64, 0, len(bm.chats))
	for chatID := range bm.chats {
		chatIDs = append(chatIDs, chatID)
	}
	bm.chatsMu.RUnlock()

	if len(chatIDs) == 0 {
		log.Printf("No chats registered, skipping notification for article: %s", item.Title)
		return
	}

	for _, chatID := range chatIDs {
		msg := tgbotapi.NewMessage(chatID, truncateToTelegramLimit(message))
		if _, err := bm.bot.Send(msg); err != nil {
			log.Printf("Failed to send notification to chat %d: %v", chatID, err)
		}
	}
}

// Отправка уведомления во все чаты с превью статьи
func (bm *BotManager) sendNotificationToAllChatsWithPreview(item RSSItem, preview string) {
	message := fmt.Sprintf("📰 Новая статья: %s\n\n🔗 %s", item.Title, item.Link)
	if preview != "" {
		message += preview
	}

	bm.chatsMu.RLock()
	chatIDs := make([]int64, 0, len(bm.chats))
	for chatID := range bm.chats {
		chatIDs = append(chatIDs, chatID)
	}
	bm.chatsMu.RUnlock()

	if len(chatIDs) == 0 {
		log.Printf("No chats registered, skipping notification for article: %s", item.Title)
		return
	}

	for _, chatID := range chatIDs {
		msg := tgbotapi.NewMessage(chatID, truncateToTelegramLimit(message))
		if _, err := bm.bot.Send(msg); err != nil {
			log.Printf("Failed to send notification to chat %d: %v", chatID, err)
		}
	}
}

// Добавление чата в список
func (bm *BotManager) addChat(chatID int64) {
	bm.chatsMu.Lock()
	defer bm.chatsMu.Unlock()
	bm.chats[chatID] = true
	log.Printf("Chat %d added to notification list", chatID)
}

// Обработка обновлений Telegram
func (bm *BotManager) handleUpdates() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updatesChan := bm.bot.GetUpdatesChan(u)

	for update := range updatesChan {
		select {
		case <-bm.ctx.Done():
			return
		default:
		}

		// Обработка добавления бота в группу
		if update.MyChatMember != nil {
			member := update.MyChatMember
			if member.NewChatMember.User != nil {
				if member.NewChatMember.User.ID == bm.bot.Self.ID {
					status := member.NewChatMember.Status
					if status == "member" || status == "administrator" {
						bm.addChat(member.Chat.ID)
					}
				}
			}
		}

		// Обработка сообщений в группах (для регистрации чата)
		if update.Message != nil {
			chatID := update.Message.Chat.ID
			if update.Message.Chat.Type == "group" || update.Message.Chat.Type == "supergroup" {
				bm.addChat(chatID)
			}

			if update.Message.IsCommand() {
				switch update.Message.Command() {
				case "start":
					bm.addChat(chatID)
					msg := tgbotapi.NewMessage(chatID, "Привет! Я бот для уведомлений о новых статьях на сайте Дениса Морозова.")
					bm.bot.Send(msg)
				case "fetch":
					url := os.Getenv("DRUPAL_SITE_URL")
					if url == "" {
						bm.bot.Send(tgbotapi.NewMessage(chatID, "DRUPAL_SITE_URL is not set"))
						continue
					}

					content, err := bm.fetchWebsiteContent(url)
					if err != nil {
						bm.bot.Send(tgbotapi.NewMessage(chatID, "Failed to fetch website content: "+err.Error()))
						continue
					}

					truncatedContent := truncateToTelegramLimit(content)
					bm.bot.Send(tgbotapi.NewMessage(chatID, truncatedContent))
				case "check":
					log.Printf("Command /check received from chat %d", chatID)
					log.Printf("Fetching RSS feed with auth method: %s", bm.authMethod)
					
					feed, err := bm.fetchRSSFeed()
					if err != nil {
						log.Printf("❌ Failed to fetch RSS feed: %v", err)
						errorMsg := fmt.Sprintf("❌ Ошибка при получении RSS-ленты: %s", err.Error())
						bm.bot.Send(tgbotapi.NewMessage(chatID, errorMsg))
						continue
					}

					log.Printf("✅ RSS feed fetched successfully: %d articles found", len(feed.Channel.Items))

					if len(feed.Channel.Items) == 0 {
						log.Printf("⚠️  No articles found in RSS feed")
						bm.bot.Send(tgbotapi.NewMessage(chatID, "Нет статей в RSS-ленте"))
						continue
					}

					// Формируем список всех статей
					var articlesList strings.Builder
					articlesList.WriteString("📰 Статьи, доступные только после авторизации:\n\n")

					for i, item := range feed.Channel.Items {
						articlesList.WriteString(fmt.Sprintf("%d. %s\n🔗 %s\n\n", i+1, item.Title, item.Link))
					}

					log.Printf("Sending %d articles to chat %d", len(feed.Channel.Items), chatID)

					// Разбиваем на части, если сообщение слишком длинное
					messages := splitToTelegramMessages(articlesList.String())
					for i, msg := range messages {
						if _, err := bm.bot.Send(tgbotapi.NewMessage(chatID, msg)); err != nil {
							log.Printf("❌ Failed to send message part %d/%d to chat %d: %v", i+1, len(messages), chatID, err)
						} else {
							log.Printf("✅ Sent message part %d/%d to chat %d", i+1, len(messages), chatID)
						}
					}
				case "about":
					versionInfo := fmt.Sprintf("🤖 Drupal Reminder Bot\n\n"+
						"Версия: %s\n"+
						"Сборка: %s\n"+
						"Коммит: %s",
						version, buildTime, commitHash)
					msg := tgbotapi.NewMessage(chatID, versionInfo)
					bm.bot.Send(msg)
				default:
					msg := tgbotapi.NewMessage(chatID, "Unknown command. Try /start, /fetch, /check or /about")
					bm.bot.Send(msg)
				}
			} else if update.Message.Text != "" {
				msg := tgbotapi.NewMessage(chatID, "Извините, я обрабатываю только команды.")
				bm.bot.Send(msg)
			}
		}
	}
}

func main() {
	// Логируем начало работы
	log.Printf("=== Starting Drupal Reminder Bot ===")
	log.Printf("Working directory: %s", func() string {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		return wd
	}())

	// Загружаем переменные окружения
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file found (this is OK if using environment variables): %v", err)
	} else {
		log.Printf("✅ .env file loaded successfully")
	}

	// Проверяем обязательные переменные окружения
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("❌ ERROR: TELEGRAM_BOT_TOKEN is not set. Please set it in .env file or environment variables.")
	}
	log.Printf("✅ TELEGRAM_BOT_TOKEN is set (length: %d)", len(token))

	rssURL := os.Getenv("RSS_URL")
	if rssURL == "" {
		rssURL = "https://www.dennismorosoff.ru/rss.xml"
		log.Printf("Using default RSS_URL: %s", rssURL)
	} else {
		log.Printf("✅ RSS_URL is set: %s", rssURL)
	}

	rssAuthUser := os.Getenv("RSS_AUTH_USER")
	rssAuthPassword := os.Getenv("RSS_AUTH_PASSWORD")
	if rssAuthUser != "" {
		log.Printf("✅ RSS_AUTH_USER is set")
	} else {
		log.Printf("RSS_AUTH_USER is not set (RSS feed may be public)")
	}

	authMethod := strings.ToLower(strings.TrimSpace(os.Getenv("DRUPAL_AUTH_METHOD")))
	if authMethod == "" {
		authMethod = "basic"
	}
	loginURL := strings.TrimSpace(os.Getenv("DRUPAL_LOGIN_URL"))
	if loginURL == "" {
		loginURL = "/user/login"
	}
	log.Printf("✅ DRUPAL_AUTH_METHOD: %s", authMethod)
	log.Printf("✅ DRUPAL_LOGIN_URL: %s", loginURL)

	authClient, err := initAuthClient(rssURL, authMethod, loginURL, rssAuthUser, rssAuthPassword)
	if err != nil {
		log.Fatalf("❌ ERROR: Failed to init auth client: %v", err)
	}

	// Создаем бота
	log.Printf("Connecting to Telegram API...")
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("❌ ERROR: Failed to create bot API: %v", err)
	}

	bot.Debug = true
	log.Printf("✅ Authorized on account %s (ID: %d)", bot.Self.UserName, bot.Self.ID)

	// Настраиваем обработку сигналов для корректного завершения
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Обработка сигналов для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v, shutting down gracefully...", sig)
		cancel()
	}()

	// Загружаем состояние
	log.Printf("Loading state from %s...", stateFileName)
	state, err := loadState(stateFileName)
	if err != nil {
		log.Printf("⚠️  Failed to load state: %v, starting with empty state", err)
		state = &State{
			LastCheckedArticles: []string{},
			LastCheckTime:       "",
		}
	} else {
		log.Printf("✅ State loaded: %d known articles, last check: %s", len(state.LastCheckedArticles), state.LastCheckTime)
	}

	// Инициализируем известные статьи
	knownArticles := make(map[string]bool)
	for _, guid := range state.LastCheckedArticles {
		knownArticles[guid] = true
	}

	bm := &BotManager{
		bot:              bot,
		rssURL:           rssURL,
		rssAuthUser:      rssAuthUser,
		rssAuthPassword:  rssAuthPassword,
		authMethod:       authMethod,
		loginURL:         loginURL,
		httpClient:       authClient,
		stateFile:        stateFileName,
		knownArticles:    knownArticles,
		chats:            make(map[int64]bool),
		notificationChan: make(chan RSSItem, 100),
		ctx:              ctx,
		cancel:           cancel,
	}

	// Запускаем горутины
	log.Printf("Starting RSS monitoring goroutine...")
	go bm.startRSSMonitoring()

	log.Printf("Starting notification queue goroutine...")
	go bm.startNotificationQueue()

	log.Printf("✅ Bot is ready and running!")
	log.Printf("Waiting for Telegram updates...")

	// Запускаем обработку обновлений Telegram (блокирующий вызов)
	bm.handleUpdates()

	log.Printf("Bot stopped")
}
