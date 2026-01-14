package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
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

func fetchWebsiteContent(url string) (string, error) {
	httpResp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", httpResp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	textContent := doc.Text()
	return textContent, nil
}

func fetchFirstParagraph(url string) (string, error) {
	httpResp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
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
	req, err := http.NewRequest("GET", bm.rssURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if bm.rssAuthUser != "" && bm.rssAuthPassword != "" {
		req.SetBasicAuth(bm.rssAuthUser, bm.rssAuthPassword)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch RSS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var feed RSSFeed
	decoder := xml.NewDecoder(resp.Body)
	if err := decoder.Decode(&feed); err != nil {
		return nil, fmt.Errorf("failed to parse RSS: %w", err)
	}

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

					content, err := fetchWebsiteContent(url)
					if err != nil {
						bm.bot.Send(tgbotapi.NewMessage(chatID, "Failed to fetch website content: "+err.Error()))
						continue
					}

					truncatedContent := truncateToTelegramLimit(content)
					bm.bot.Send(tgbotapi.NewMessage(chatID, truncatedContent))
				case "check":
					url := os.Getenv("DRUPAL_SITE_URL")
					if url == "" {
						bm.bot.Send(tgbotapi.NewMessage(chatID, "DRUPAL_SITE_URL is not set"))
						continue
					}

					firstParagraph, err := fetchFirstParagraph(url)
					if err != nil {
						bm.bot.Send(tgbotapi.NewMessage(chatID, "Failed to fetch first paragraph: "+err.Error()))
						continue
					}

					truncatedParagraph := truncateToTelegramLimit(firstParagraph)
					bm.bot.Send(tgbotapi.NewMessage(chatID, truncatedParagraph))
				default:
					msg := tgbotapi.NewMessage(chatID, "Unknown command. Try /start, /fetch or /check")
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
	if err := godotenv.Load(); err != nil {
		log.Print("No .env file found")
	}

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Panic("TELEGRAM_BOT_TOKEN is not set")
	}

	rssURL := os.Getenv("RSS_URL")
	if rssURL == "" {
		rssURL = "https://www.dennismorosoff.ru/rss.xml"
	}

	rssAuthUser := os.Getenv("RSS_AUTH_USER")
	rssAuthPassword := os.Getenv("RSS_AUTH_PASSWORD")

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Загружаем состояние
	state, err := loadState(stateFileName)
	if err != nil {
		log.Printf("Failed to load state: %v, starting with empty state", err)
		state = &State{
			LastCheckedArticles: []string{},
			LastCheckTime:       "",
		}
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
		stateFile:        stateFileName,
		knownArticles:    knownArticles,
		chats:            make(map[int64]bool),
		notificationChan: make(chan RSSItem, 100),
		ctx:              ctx,
		cancel:           cancel,
	}

	// Запускаем горутины
	go bm.startRSSMonitoring()
	go bm.startNotificationQueue()

	// Запускаем обработку обновлений Telegram (блокирующий вызов)
	bm.handleUpdates()
}
