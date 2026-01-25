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
	ChatIDs             []int64  `json:"chat_ids"`
}

// Менеджер бота
type BotManager struct {
	bot              *tgbotapi.BotAPI
	rssURL           string
	rssBaseURL       string
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
	stateMu          sync.Mutex
	lastCheckTime    string
}

func (bm *BotManager) persistState() error {
	// Snapshot known articles
	bm.knownArticlesMu.RLock()
	articleList := make([]string, 0, len(bm.knownArticles))
	for guid := range bm.knownArticles {
		articleList = append(articleList, guid)
	}
	bm.knownArticlesMu.RUnlock()

	// Snapshot chats
	bm.chatsMu.RLock()
	chatIDs := make([]int64, 0, len(bm.chats))
	for chatID := range bm.chats {
		chatIDs = append(chatIDs, chatID)
	}
	bm.chatsMu.RUnlock()

	bm.stateMu.Lock()
	defer bm.stateMu.Unlock()

	state := &State{
		LastCheckedArticles: articleList,
		LastCheckTime:       bm.lastCheckTime,
		ChatIDs:             chatIDs,
	}

	return saveState(bm.stateFile, state)
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

func baseOriginFromURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("failed to parse URL %q: %w", rawURL, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("URL must include scheme and host, got: %q", rawURL)
	}
	origin := (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String()
	return origin, nil
}

func initAuthClient(rssURL string, rssBaseURL string, authMethod string, loginURL string, username string, password string) (*http.Client, error) {
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

		if strings.TrimSpace(rssBaseURL) == "" {
			return nil, fmt.Errorf("DRUPAL_AUTH_METHOD=cookie requires a valid base URL derived from RSS_URL")
		}
		if err := loginToDrupal(client, rssBaseURL, loginURL, username, password); err != nil {
			return nil, err
		}
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported DRUPAL_AUTH_METHOD: %s", authMethod)
	}
}

func loginToDrupal(client *http.Client, baseURL string, loginURL string, username string, password string) error {
	if username == "" || password == "" {
		return fmt.Errorf("DRUPAL_AUTH_METHOD=cookie requires RSS_AUTH_USER and RSS_AUTH_PASSWORD")
	}

	log.Printf("Attempting to login to Drupal at %s (login URL: %s)", baseURL, loginURL)
	loginPageURL, err := resolveURL(baseURL, loginURL)
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
	if err := loginToDrupal(newClient, bm.rssBaseURL, bm.loginURL, bm.rssAuthUser, bm.rssAuthPassword); err != nil {
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

// Вспомогательная функция для преобразования относительного URL в абсолютный
func resolveImageURL(baseURL string, imageURL string) string {
	if imageURL == "" {
		return ""
	}
	// Если URL уже абсолютный, возвращаем как есть
	if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		return imageURL
	}
	// Преобразуем относительный URL в абсолютный
	base, err := url.Parse(baseURL)
	if err != nil {
		return imageURL
	}
	relative, err := url.Parse(imageURL)
	if err != nil {
		return imageURL
	}
	return base.ResolveReference(relative).String()
}

// Извлечение заглавного изображения статьи из структуры Drupal
func (bm *BotManager) fetchArticleImage(targetURL string) (string, error) {
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

	var imageURL string
	var source string

	// 1. Поиск в поле изображения Drupal (field-image, field-featured-image)
	selectors := []string{
		"div.field-name-field-image img",
		"div.field-name-field-featured-image img",
		"div[class*='field-image'] img",
		"div[class*='field-featured-image'] img",
		"img[data-field-name*='image']",
		"img[data-field-name*='featured']",
	}

	for _, selector := range selectors {
		img := doc.Find(selector).First()
		if img.Length() > 0 {
			if src, exists := img.Attr("src"); exists && src != "" {
				imageURL = resolveImageURL(targetURL, src)
				source = "Drupal field"
				log.Printf("Found image in Drupal field structure: %s", imageURL)
				break
			}
		}
	}

	// 2. Если не найдено, ищем первое изображение в основном контенте статьи
	if imageURL == "" {
		contentSelectors := []string{
			"article img",
			"main img",
			".node-content img",
			".field-body img",
			".content img",
		}

		for _, selector := range contentSelectors {
			img := doc.Find(selector).First()
			if img.Length() > 0 {
				// Пропускаем маленькие изображения (иконки, аватары) по классу
				if class, _ := img.Attr("class"); strings.Contains(strings.ToLower(class), "icon") || strings.Contains(strings.ToLower(class), "avatar") {
					continue
				}
				// Пропускаем изображения с маленькими размерами в src (обычно иконки)
				if src, exists := img.Attr("src"); exists && src != "" {
					// Пропускаем data: URL и очень маленькие изображения
					if strings.HasPrefix(src, "data:") {
						continue
					}
					imageURL = resolveImageURL(targetURL, src)
					source = "article content"
					log.Printf("Found image in article content: %s", imageURL)
					break
				}
			}
		}
	}

	// 3. Fallback: ищем мета-тег og:image
	if imageURL == "" {
		ogImage := doc.Find("meta[property='og:image']").First()
		if ogImage.Length() > 0 {
			if content, exists := ogImage.Attr("content"); exists && content != "" {
				imageURL = resolveImageURL(targetURL, content)
				source = "og:image"
				log.Printf("Found image in og:image meta tag: %s", imageURL)
			}
		}
	}

	if imageURL != "" {
		log.Printf("✅ Image found from %s: %s", source, imageURL)
		return imageURL, nil
	}

	log.Printf("⚠️  No image found for article: %s", targetURL)
	return "", nil // Возвращаем пустую строку, если изображение не найдено
}

// Загрузка состояния из файла
func loadState(filename string) (*State, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{
				LastCheckedArticles: []string{},
				LastCheckTime:       "",
				ChatIDs:             []int64{},
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

	// Сохраняем состояние (включая чаты)
	bm.stateMu.Lock()
	bm.lastCheckTime = time.Now().Format(time.RFC3339)
	bm.stateMu.Unlock()
	if err := bm.persistState(); err != nil {
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

				// Получаем изображение статьи
				imageURL, err := bm.fetchArticleImage(article.Link)
				if err != nil {
					log.Printf("⚠️  Failed to fetch article image: %v", err)
				}

				// Получаем список всех чатов
				bm.chatsMu.RLock()
				allChatIDs := make([]int64, 0, len(bm.chats))
				for chatID := range bm.chats {
					allChatIDs = append(allChatIDs, chatID)
				}
				bm.chatsMu.RUnlock()

				if len(allChatIDs) == 0 {
					log.Printf("No chats registered, skipping notification for article: %s", article.Title)
					return
				}

				// Отправляем статью во все чаты (как в команде /check)
				log.Printf("Sending new article notification to %d chats: %s", len(allChatIDs), article.Title)
				for _, chatID := range allChatIDs {
					bm.sendLastArticleToChat(chatID, article, imageURL)
				}
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

func (bm *BotManager) sendLastArticleToChat(chatID int64, item RSSItem, imageURL string) {
	if imageURL != "" {
		caption := fmt.Sprintf("<a href=\"%s\">%s</a>", item.Link, item.Title)
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(imageURL))
		photo.Caption = caption
		photo.ParseMode = "HTML"

		if _, err := bm.bot.Send(photo); err != nil {
			log.Printf("❌ Failed to send photo to chat %d: %v", chatID, err)
			// Fallback: отправляем текстовое сообщение
			textMsg := fmt.Sprintf("<a href=\"%s\">%s</a>", item.Link, item.Title)
			msg := tgbotapi.NewMessage(chatID, textMsg)
			msg.ParseMode = "HTML"
			if _, msgErr := bm.bot.Send(msg); msgErr != nil {
				log.Printf("❌ Failed to send fallback text message to chat %d: %v", chatID, msgErr)
			} else {
				log.Printf("✅ Sent fallback text message to chat %d", chatID)
			}
		} else {
			log.Printf("✅ Sent photo to chat %d", chatID)
		}
		return
	}

	// Изображение не найдено, отправляем текстовое сообщение с ссылкой
	textMsg := fmt.Sprintf("<a href=\"%s\">%s</a>", item.Link, item.Title)
	msg := tgbotapi.NewMessage(chatID, textMsg)
	msg.ParseMode = "HTML"
	if _, err := bm.bot.Send(msg); err != nil {
		log.Printf("❌ Failed to send text message to chat %d: %v", chatID, err)
	} else {
		log.Printf("✅ Sent text message to chat %d", chatID)
	}
}

// Добавление чата в список
func (bm *BotManager) addChat(chatID int64) {
	isNew := false
	bm.chatsMu.Lock()
	if !bm.chats[chatID] {
		bm.chats[chatID] = true
		isNew = true
	}
	bm.chatsMu.Unlock()

	if isNew {
		log.Printf("Chat %d added to notification list", chatID)
		if err := bm.persistState(); err != nil {
			log.Printf("Failed to save state after adding chat %d: %v", chatID, err)
		}
	}
}

// Обработка обновлений Telegram
func (bm *BotManager) handleUpdates() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	// Явно запрашиваем нужные типы апдейтов, чтобы стабильно получать события
	// о добавлении/удалении бота из групп (my_chat_member) и сообщения (message).
	u.AllowedUpdates = []string{"message", "my_chat_member"}

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

			// Регистрация группового чата через пересланное сообщение в личку боту.
			// Это помогает, если бот по каким-то причинам не получает апдейты из группы.
			if update.Message.Chat.Type == "private" && update.Message.ForwardFromChat != nil {
				fwdChat := update.Message.ForwardFromChat
				if fwdChat.Type == "group" || fwdChat.Type == "supergroup" {
					bm.addChat(fwdChat.ID)
					reply := fmt.Sprintf(
						"Группа зарегистрирована.\n\nChat ID: %d\nТип чата: %s\n\nТеперь /check будет рассылать и сюда.",
						fwdChat.ID, fwdChat.Type,
					)
					bm.bot.Send(tgbotapi.NewMessage(chatID, reply))
				}
			}

			if update.Message.IsCommand() {
				switch update.Message.Command() {
				case "start":
					bm.addChat(chatID)
					msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
						"Привет! Я бот для уведомлений о новых статьях.\n\nChat ID: %d\nТип чата: %s\n\nКоманды: /check, /about, /status",
						chatID, update.Message.Chat.Type,
					))
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

					// Берем только последнюю (первую в списке) статью
					// ВАЖНО: последняя статья выводится в любом случае, даже если она уже выводилась в качестве уведомления
					item := feed.Channel.Items[0]
					log.Printf("Processing last article: %s (will be sent regardless of notification status)", item.Title)

					// Пытаемся получить изображение статьи
					imageURL, err := bm.fetchArticleImage(item.Link)
					if err != nil {
						log.Printf("⚠️  Failed to fetch article image: %v", err)
						imageURL = "" // Убеждаемся, что imageURL пустая при ошибке
					}
					if imageURL != "" {
						log.Printf("✅ Article image found: %s", imageURL)
					} else {
						log.Printf("ℹ️  No article image, will send text message")
					}

					// ВАЖНО: последняя статья выводится в любом случае, даже если она уже выводилась в качестве уведомления
					// Всегда отправляем статью в текущий чат
					bm.addChat(chatID)
					log.Printf("Sending article to current chat %d: %s", chatID, item.Title)
					bm.sendLastArticleToChat(chatID, item, imageURL)

					// Поведение /check:
					// - в личке: дополнительно разослать во все известные чаты (кроме текущего)
					// - в группе: только в текущей группе (без общей рассылки)
					if update.Message.Chat.Type != "private" {
						// Для групп/супергрупп не делаем broadcast
						continue
					}

					// Личка: делаем рассылку по всем известным чатам (кроме текущего)
					bm.chatsMu.RLock()
					allChatIDs := make([]int64, 0, len(bm.chats))
					for id := range bm.chats {
						if id != chatID { // Исключаем текущий чат, так как уже отправили
							allChatIDs = append(allChatIDs, id)
						}
					}
					bm.chatsMu.RUnlock()

					if len(allChatIDs) > 0 {
						log.Printf("Broadcasting /check (private) to %d additional chats: %v", len(allChatIDs), allChatIDs)
						for _, targetChatID := range allChatIDs {
							bm.sendLastArticleToChat(targetChatID, item, imageURL)
						}
					} else {
						log.Printf("No additional chats to broadcast to")
					}
				case "status":
					isRegistered := false
					bm.chatsMu.RLock()
					isRegistered = bm.chats[chatID]
					totalChats := len(bm.chats)
					bm.chatsMu.RUnlock()

					text := fmt.Sprintf(
						"Статус\n\nChat ID: %d\nТип чата: %s\nЗарегистрирован: %t\nВсего чатов в базе: %d",
						chatID, update.Message.Chat.Type, isRegistered, totalChats,
					)
					bm.bot.Send(tgbotapi.NewMessage(chatID, text))
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

	rawAuthMethod := strings.TrimSpace(os.Getenv("DRUPAL_AUTH_METHOD"))
	authMethod := strings.ToLower(rawAuthMethod)
	authMethodSource := "env"
	if authMethod == "" {
		// Если креды заданы, вероятнее всего нужен cookie-логин (Drupal-only).
		if rssAuthUser != "" && rssAuthPassword != "" {
			authMethod = "cookie"
			authMethodSource = "default(cookie_due_to_credentials)"
		} else {
			authMethod = "basic"
			authMethodSource = "default(basic)"
			if (rssAuthUser != "" && rssAuthPassword == "") || (rssAuthUser == "" && rssAuthPassword != "") {
				log.Printf("⚠️  Only one of RSS_AUTH_USER/RSS_AUTH_PASSWORD is set; defaulting to basic auth")
			}
		}
	}
	loginURL := strings.TrimSpace(os.Getenv("DRUPAL_LOGIN_URL"))
	if loginURL == "" {
		loginURL = "/user/login"
	}
	log.Printf("✅ DRUPAL_AUTH_METHOD: %s (source: %s)", authMethod, authMethodSource)
	log.Printf("✅ DRUPAL_LOGIN_URL: %s", loginURL)

	rssBaseURL := ""
	if authMethod == "cookie" {
		var err error
		rssBaseURL, err = baseOriginFromURL(rssURL)
		if err != nil {
			log.Fatalf("❌ ERROR: Failed to derive base URL from RSS_URL: %v", err)
		}
		log.Printf("✅ Derived base URL for cookie login: %s", rssBaseURL)
	}

	authClient, err := initAuthClient(rssURL, rssBaseURL, authMethod, loginURL, rssAuthUser, rssAuthPassword)
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
			ChatIDs:             []int64{},
		}
	} else {
		log.Printf("✅ State loaded: %d known articles, last check: %s", len(state.LastCheckedArticles), state.LastCheckTime)
	}

	// Инициализируем известные статьи
	knownArticles := make(map[string]bool)
	for _, guid := range state.LastCheckedArticles {
		knownArticles[guid] = true
	}

	// Инициализируем известные чаты из state.json (чтобы не терять группы после рестарта)
	chats := make(map[int64]bool)
	for _, chatID := range state.ChatIDs {
		chats[chatID] = true
	}

	bm := &BotManager{
		bot:              bot,
		rssURL:           rssURL,
		rssBaseURL:       rssBaseURL,
		rssAuthUser:      rssAuthUser,
		rssAuthPassword:  rssAuthPassword,
		authMethod:       authMethod,
		loginURL:         loginURL,
		httpClient:       authClient,
		stateFile:        stateFileName,
		knownArticles:    knownArticles,
		chats:            chats,
		notificationChan: make(chan RSSItem, 100),
		ctx:              ctx,
		cancel:           cancel,
		lastCheckTime:    state.LastCheckTime,
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
