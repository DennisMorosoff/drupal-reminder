package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	stateAwaitingManualSleep = "awaiting_manual_sleep"
	stateAwaitingEditLast    = "awaiting_edit_last"
	stateAwaitingChildName   = "awaiting_child_name"
	stateAwaitingTimezone    = "awaiting_timezone"
	stateAwaitingBirthDate   = "awaiting_birth_date"
	stateAwaitingReminder    = "awaiting_custom_reminder"

	// Онбординг нового пользователя/семьи (профиль): имя -> таймзона -> дата рождения.
	stateOnboardingChildName  = "onboarding_child_name"
	stateOnboardingTimezone   = "onboarding_timezone"
	stateOnboardingBirthDate  = "onboarding_birth_date"
)

// Кнопки меню: при нажатии в режиме ввода сбрасываем состояние и обрабатываем как обычное действие.
var menuButtonTexts = map[string]bool{
	"Сон начался": true, "Начался 5 минут назад": true, "Начался 10 минут назад": true,
	"Начался 15 минут назад": true, "Начался 30 минут назад": true,
	"Сон закончился": true, "Закончился 5 минут назад": true, "Закончился 10 минут назад": true,
	"Закончился 15 минут назад": true, "Закончился 30 минут назад": true,
	"Добавить сон": true, "Исправить последний сон": true,
	"Отчеты": true, "Напоминания": true, "Настройки": true,
	"Оценить": true,
}

func isOnboardingState(state string) bool {
	switch state {
	case stateOnboardingChildName, stateOnboardingTimezone, stateOnboardingBirthDate:
		return true
	default:
		return false
	}
}

type SleepBot struct {
	api   *tgbotapi.BotAPI
	store *Store
	cfg   Config
}

type pendingActionPayload struct {
	Note string `json:"note,omitempty"`
}

func NewSleepBot(api *tgbotapi.BotAPI, store *Store, cfg Config) *SleepBot {
	return &SleepBot{
		api:   api,
		store: store,
		cfg:   cfg,
	}
}

func (b *SleepBot) Run(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = b.cfg.PollTimeout
	u.AllowedUpdates = []string{"message"}

	updates := b.api.GetUpdatesChan(u)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-updates:
			if update.Message == nil {
				continue
			}
			if err := b.handleMessage(ctx, update.Message); err != nil {
				log.Printf("handle message error: %v", err)
				_ = b.sendText(update.Message.Chat.ID, "Не получилось обработать сообщение. Попробуйте еще раз.")
			}
		}
	}
}

func (b *SleepBot) RunReminders(ctx context.Context) {
	ticker := time.NewTicker(b.cfg.ReminderTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := b.processReminders(ctx); err != nil {
				log.Printf("reminders error: %v", err)
			}
		}
	}
}

func (b *SleepBot) handleMessage(ctx context.Context, msg *tgbotapi.Message) error {
	if msg.Chat == nil || msg.From == nil {
		return nil
	}
	if msg.Chat.Type != "private" {
		return b.sendText(msg.Chat.ID, "Используйте бота в личном чате, чтобы не смешивать семейные данные с группой.")
	}

	var (
		userCtx UserContext
		created bool
		err     error
	)

	_, err = b.store.GetUserContext(ctx, msg.From.ID)
	switch {
	case err == nil:
		userCtx, created, err = b.store.EnsureMember(ctx, msg.From.ID, msg.Chat.ID, fullName(msg.From))
		if err != nil {
			return err
		}
	case errors.Is(err, sql.ErrNoRows):
		if msg.IsCommand() && strings.EqualFold(msg.Command(), "join") {
			return b.handleJoinOnly(ctx, msg)
		}
		userCtx, created, err = b.store.EnsureMember(ctx, msg.From.ID, msg.Chat.ID, fullName(msg.From))
		if err != nil {
			return err
		}
	default:
		return err
	}

	if created && ((msg.IsCommand() && strings.EqualFold(msg.Command(), "start")) || !msg.IsCommand()) {
		// Для новой семьи сразу запускаем онбординг профиля, чтобы отчёты/таймзона/вехи работали корректно.
		if err := b.startOnboarding(ctx, userCtx, msg.Chat.ID); err != nil {
			return err
		}
		return nil
	}

	if state, err := b.store.GetUserState(ctx, msg.From.ID); err == nil && state != nil && !msg.IsCommand() {
		text := strings.TrimSpace(msg.Text)
		if menuButtonTexts[text] {
			// Во время онбординга меню-кнопки не сбрасывают состояние: сначала ответьте на вопрос анкеты.
			if isOnboardingState(state.State) {
				return b.sendText(msg.Chat.ID, "Сначала ответьте на вопрос анкеты. Потом можно отмечать сон кнопками.")
			}
			_ = b.store.ClearUserState(ctx, msg.From.ID)
		} else {
			handled, stateErr := b.handleState(ctx, userCtx, msg, state)
			if stateErr != nil {
				return stateErr
			}
			if handled {
				return nil
			}
		}
	}

	if msg.IsCommand() {
		_ = b.store.ClearUserState(ctx, msg.From.ID)
		return b.handleCommand(ctx, userCtx, msg)
	}
	return b.handleText(ctx, userCtx, msg)
}

func (b *SleepBot) startOnboarding(ctx context.Context, userCtx UserContext, chatID int64) error {
	if err := b.store.SetUserState(ctx, userCtx.Member.TelegramUserID, userCtx.Family.ID, stateOnboardingChildName, pendingActionPayload{}); err != nil {
		return err
	}

	intro := strings.Join([]string{
		fmt.Sprintf("Привет! Это бот учёта сна для `%s`.", escapeTelegramMarkdown(userCtx.Child.Name)),
		"",
		"Сейчас настроим профиль семьи за 3 шага:",
		"1) имя ребёнка",
		"2) таймзона семьи",
		"3) дата/время рождения (нужно для корректных отчётов и вех).",
		"",
		"Шаг 1/3. Как зовут ребёнка?",
	}, "\n")
	return b.sendTextWithKeyboard(chatID, intro, b.mainKeyboard(false))
}

func (b *SleepBot) handleJoinOnly(ctx context.Context, msg *tgbotapi.Message) error {
	code := strings.TrimSpace(msg.CommandArguments())
	if code == "" {
		return b.sendText(msg.Chat.ID, "Использование: `/join ABC123`")
	}
	joined, err := b.store.JoinFamily(ctx, strings.ToUpper(code), msg.From.ID, msg.Chat.ID, fullName(msg.From))
	if err != nil {
		return b.sendText(msg.Chat.ID, escapeTelegramMarkdown(err.Error()))
	}
	return b.sendTextWithKeyboard(msg.Chat.ID, fmt.Sprintf("Готово. Теперь вы привязаны к семье `%s`.", escapeTelegramMarkdown(joined.Family.Name)), b.mainKeyboard(false))
}

func (b *SleepBot) handleCommand(ctx context.Context, userCtx UserContext, msg *tgbotapi.Message) error {
	command := strings.ToLower(msg.Command())
	args := strings.TrimSpace(msg.CommandArguments())

	switch command {
	case "start", "help":
		return b.sendWelcome(userCtx, msg.Chat.ID)
	case "reset_service":
		// Команда глобального сброса данных бота: полностью очищает SQLite.
		// В качестве защиты от случайного запуска требуется аргумент `confirm`.
		if args != "confirm" && args != "yes" {
			return b.sendText(msg.Chat.ID, "Использование: `/reset_service confirm`")
		}
		if err := b.store.ResetService(ctx); err != nil {
			return err
		}
		return b.sendText(msg.Chat.ID, "Сервис сброшен: все данные очищены.")
	case "silent_mode":
		// Команда глобального молчаливого режима: выключает все автоматические уведомления.
		if args != "" {
			return b.sendText(msg.Chat.ID, "Использование: `/silent_mode`")
		}
		if err := b.store.SetSilentMode(ctx, true); err != nil {
			return err
		}
		return b.sendText(msg.Chat.ID, "Молчаливый режим включен: все уведомления выключены.")
	case "invite":
		code, expiresAt, err := b.store.CreateInviteCode(ctx, userCtx.Family.ID)
		if err != nil {
			return err
		}
		escCode := escapeTelegramMarkdown(code)
		return b.sendText(msg.Chat.ID, fmt.Sprintf("Код приглашения: `%s`\nДействует до %s.\n\nВторой родитель может выполнить `/join %s`.", escCode, expiresAt.In(b.mustLocation(userCtx.Family.Timezone)).Format("02.01 15:04"), escCode))
	case "join":
		if args == "" {
			return b.sendText(msg.Chat.ID, "Использование: `/join ABC123`")
		}
		joined, err := b.store.JoinFamily(ctx, strings.ToUpper(args), msg.From.ID, msg.Chat.ID, fullName(msg.From))
		if err != nil {
			return b.sendText(msg.Chat.ID, escapeTelegramMarkdown(err.Error()))
		}
		return b.sendTextWithKeyboard(msg.Chat.ID, fmt.Sprintf("Готово. Теперь вы привязаны к семье `%s`.", escapeTelegramMarkdown(joined.Family.Name)), b.mainKeyboard(false))
	case "status":
		return b.sendStatus(ctx, userCtx, msg.Chat.ID)
	case "report":
		return b.sendDashboard(ctx, userCtx, msg.Chat.ID)
	case "day":
		return b.sendDayReport(ctx, userCtx, msg.Chat.ID)
	case "week":
		return b.sendRangeReport(ctx, userCtx, msg.Chat.ID, 7)
	case "month":
		return b.sendRangeReport(ctx, userCtx, msg.Chat.ID, 30)
	case "settings":
		return b.sendSettings(ctx, userCtx, msg.Chat.ID)
	case "reminders":
		return b.sendReminders(ctx, userCtx, msg.Chat.ID)
	case "setchild":
		if args != "" {
			if err := b.store.SetChildName(ctx, userCtx.Family.ID, args); err != nil {
				return b.sendText(msg.Chat.ID, escapeTelegramMarkdown(err.Error()))
			}
			return b.sendText(msg.Chat.ID, "Имя ребенка обновлено.")
		}
		if err := b.store.SetUserState(ctx, msg.From.ID, userCtx.Family.ID, stateAwaitingChildName, pendingActionPayload{}); err != nil {
			return err
		}
		return b.sendText(msg.Chat.ID, "Отправьте новое имя ребенка одним сообщением.")
	case "settimezone":
		if args != "" {
			if err := b.store.SetFamilyTimezone(ctx, userCtx.Family.ID, args); err != nil {
				return b.sendText(msg.Chat.ID, escapeTelegramMarkdown(err.Error()))
			}
			return b.sendText(msg.Chat.ID, "Таймзона обновлена.")
		}
		if err := b.store.SetUserState(ctx, msg.From.ID, userCtx.Family.ID, stateAwaitingTimezone, pendingActionPayload{}); err != nil {
			return err
		}
		return b.sendText(msg.Chat.ID, "Отправьте таймзону в формате `Europe/Moscow`.")
	case "setbirthdate":
		if args != "" {
			return b.applyBirthDate(ctx, userCtx, msg.Chat.ID, args)
		}
		if err := b.store.SetUserState(ctx, msg.From.ID, userCtx.Family.ID, stateAwaitingBirthDate, pendingActionPayload{}); err != nil {
			return err
		}
		return b.sendText(msg.Chat.ID, "Отправьте дату и время рождения: `02.01.2006 15:04` или только дату: `02.01.2006` (время — в вашей таймзоне из настроек). Можно RFC3339.")
	case "setwake":
		return b.updateReminderThreshold(ctx, userCtx, msg.Chat.ID, "wake_window_minutes", args)
	case "setmaxsleep":
		return b.updateReminderThreshold(ctx, userCtx, msg.Chat.ID, "max_sleep_minutes", args)
	case "setinactive":
		return b.updateReminderThreshold(ctx, userCtx, msg.Chat.ID, "inactivity_minutes", args)
	case "reminders_on":
		if err := b.store.SetReminderEnabled(ctx, userCtx.Family.ID, true); err != nil {
			return err
		}
		return b.sendText(msg.Chat.ID, "Все автоматические напоминания включены.")
	case "reminders_off":
		if err := b.store.SetReminderEnabled(ctx, userCtx.Family.ID, false); err != nil {
			return err
		}
		return b.sendText(msg.Chat.ID, "Все автоматические напоминания выключены.")
	case "milestone_notify":
		return b.setMilestoneNotifyEach(ctx, userCtx, msg.Chat.ID, args)
	case "milestone_report":
		return b.setMilestoneReportToday(ctx, userCtx, msg.Chat.ID, args)
	case "addreminder":
		if args != "" {
			return b.applyCustomReminder(ctx, userCtx, msg.Chat.ID, args)
		}
		if err := b.store.SetUserState(ctx, msg.From.ID, userCtx.Family.ID, stateAwaitingReminder, pendingActionPayload{}); err != nil {
			return err
		}
		return b.sendText(msg.Chat.ID, "Отправьте напоминание в формате `19:30 Купание`.")
	case "deletereminder":
		id, err := strconv.ParseInt(args, 10, 64)
		if err != nil {
			return b.sendText(msg.Chat.ID, "Использование: `/deletereminder 3`")
		}
		if err := b.store.DeleteCustomReminder(ctx, userCtx.Family.ID, id); err != nil {
			return b.sendText(msg.Chat.ID, escapeTelegramMarkdown(err.Error()))
		}
		return b.sendText(msg.Chat.ID, "Напоминание удалено.")
	case "editlast":
		if err := b.store.SetUserState(ctx, msg.From.ID, userCtx.Family.ID, stateAwaitingEditLast, pendingActionPayload{}); err != nil {
			return err
		}
		editMsg, err := b.editLastSleepMessage(ctx, userCtx)
		if err != nil {
			return err
		}
		return b.sendText(msg.Chat.ID, editMsg)
	case "cancel":
		if err := b.store.ClearUserState(ctx, msg.From.ID); err != nil {
			return err
		}
		return b.sendText(msg.Chat.ID, "Текущее действие отменено.")
	default:
		return b.sendText(msg.Chat.ID, "Неизвестная команда. Используйте /help.")
	}
}

func (b *SleepBot) handleText(ctx context.Context, userCtx UserContext, msg *tgbotapi.Message) error {
	text := strings.TrimSpace(msg.Text)
	switch text {
	case "Сон начался":
		return b.startSleep(ctx, userCtx, msg.Chat.ID, time.Now(), sourceRealTime)
	case "Начался 5 минут назад":
		return b.startSleep(ctx, userCtx, msg.Chat.ID, time.Now().Add(-5*time.Minute), sourceQuickBackdate)
	case "Начался 10 минут назад":
		return b.startSleep(ctx, userCtx, msg.Chat.ID, time.Now().Add(-10*time.Minute), sourceQuickBackdate)
	case "Начался 15 минут назад":
		return b.startSleep(ctx, userCtx, msg.Chat.ID, time.Now().Add(-15*time.Minute), sourceQuickBackdate)
	case "Начался 30 минут назад":
		return b.startSleep(ctx, userCtx, msg.Chat.ID, time.Now().Add(-30*time.Minute), sourceQuickBackdate)
	case "Сон закончился":
		return b.endSleep(ctx, userCtx, msg.Chat.ID, time.Now(), sourceRealTime)
	case "Закончился 5 минут назад":
		return b.endSleep(ctx, userCtx, msg.Chat.ID, time.Now().Add(-5*time.Minute), sourceQuickBackdate)
	case "Закончился 10 минут назад":
		return b.endSleep(ctx, userCtx, msg.Chat.ID, time.Now().Add(-10*time.Minute), sourceQuickBackdate)
	case "Закончился 15 минут назад":
		return b.endSleep(ctx, userCtx, msg.Chat.ID, time.Now().Add(-15*time.Minute), sourceQuickBackdate)
	case "Закончился 30 минут назад":
		return b.endSleep(ctx, userCtx, msg.Chat.ID, time.Now().Add(-30*time.Minute), sourceQuickBackdate)
	case "Добавить сон":
		if err := b.store.SetUserState(ctx, msg.From.ID, userCtx.Family.ID, stateAwaitingManualSleep, pendingActionPayload{}); err != nil {
			return err
		}
		return b.sendText(msg.Chat.ID, "Отправьте интервал сна: `11:10 - 12:35` или `16.03 11:10 - 16.03 12:35`.\n"+b.localTimeHint(userCtx))
	case "Исправить последний сон":
		if err := b.store.SetUserState(ctx, msg.From.ID, userCtx.Family.ID, stateAwaitingEditLast, pendingActionPayload{}); err != nil {
			return err
		}
		editMsg, err := b.editLastSleepMessage(ctx, userCtx)
		if err != nil {
			return err
		}
		return b.sendText(msg.Chat.ID, editMsg)
	case "Отчеты":
		return b.sendDashboard(ctx, userCtx, msg.Chat.ID)
	case "Оценить":
		return b.sendEvaluation(ctx, userCtx, msg.Chat.ID)
	case "Напоминания":
		return b.sendReminders(ctx, userCtx, msg.Chat.ID)
	case "Настройки":
		return b.sendSettings(ctx, userCtx, msg.Chat.ID)
	default:
		return b.sendTextWithKeyboard(msg.Chat.ID, "Используйте кнопки ниже или команды `/help`, `/report`, `/reminders`, `/settings`.", b.mainKeyboard(false))
	}
}

func (b *SleepBot) handleState(ctx context.Context, userCtx UserContext, msg *tgbotapi.Message, state *UserState) (bool, error) {
	text := strings.TrimSpace(msg.Text)
	switch state.State {
	case stateOnboardingChildName:
		if err := b.store.SetChildName(ctx, userCtx.Family.ID, text); err != nil {
			return true, b.sendText(msg.Chat.ID, escapeTelegramMarkdown(err.Error()))
		}
		if err := b.store.SetUserState(ctx, msg.From.ID, userCtx.Family.ID, stateOnboardingTimezone, pendingActionPayload{}); err != nil {
			return true, err
		}
		return true, b.sendTextWithKeyboard(
			msg.Chat.ID,
			"Шаг 2/3. Пришлите таймзону семьи (например `Europe/Moscow`).",
			b.mainKeyboard(false),
		)

	case stateOnboardingTimezone:
		if err := b.store.SetFamilyTimezone(ctx, userCtx.Family.ID, text); err != nil {
			return true, b.sendText(msg.Chat.ID, escapeTelegramMarkdown(err.Error()))
		}
		if err := b.store.SetUserState(ctx, msg.From.ID, userCtx.Family.ID, stateOnboardingBirthDate, pendingActionPayload{}); err != nil {
			return true, err
		}
		return true, b.sendTextWithKeyboard(
			msg.Chat.ID,
			"Шаг 3/3. Пришлите дату и время рождения `16.03.2026 14:30` или только дату `16.03.2026` (в вашей таймзоне). Можно RFC3339.",
			b.mainKeyboard(false),
		)

	case stateOnboardingBirthDate:
		loc := b.mustLocation(userCtx.Family.Timezone)
		birthDate, err := ParseBirthDateInput(text, loc)
		if err != nil {
			return true, b.sendText(msg.Chat.ID, "Не удалось разобрать дату рождения. Пример: `16.03.2026 14:30` или `16.03.2026`.")
		}
		if err := b.store.SetChildBirthDate(ctx, userCtx.Family.ID, birthDate); err != nil {
			return true, err
		}
		if err := b.store.ClearUserState(ctx, msg.From.ID); err != nil {
			return true, err
		}

		finish := strings.Join([]string{
			"Готово. Профиль сохранён.",
			"",
			"Вести журнал сна можно кнопками:",
			"`Сон начался` / `Сон закончился`",
			"и ретро-кнопками `Начался/Закончился ... минут назад`.",
			"",
			"Автоматические напоминания по умолчанию выключены.",
			"Команды для порогов:",
			"`/setwake 90`, `/setmaxsleep 120`, `/setinactive 240`",
			"и включение/выключение:",
			"`/reminders_on`, `/reminders_off`",
			"",
			"Красивые даты (вехи) по умолчанию выключены. Включить можно:",
			"`/milestone_notify on` и `/milestone_report on`",
		}, "\n")

		active, _ := b.store.GetActiveSleep(ctx, userCtx.Child.ID)
		return true, b.sendTextWithKeyboard(msg.Chat.ID, finish, b.mainKeyboard(active != nil))

	case stateAwaitingManualSleep:
		startAt, endAt, err := parseSleepRange(text, time.Now(), b.mustLocation(userCtx.Family.Timezone))
		if err != nil {
			return true, b.sendText(msg.Chat.ID, "Не понял интервал. Пример: `11:10 - 12:35`.")
		}
		if _, err := b.store.AddManualSleep(ctx, userCtx.Child.ID, userCtx.Member.ID, startAt, endAt, "manual"); err != nil {
			return true, b.sendText(msg.Chat.ID, escapeTelegramMarkdown(err.Error()))
		}
		if err := b.store.ClearUserState(ctx, msg.From.ID); err != nil {
			return true, err
		}
		return true, b.sendTextWithKeyboard(msg.Chat.ID, fmt.Sprintf("Сон сохранен: %s - %s.", formatLocalDateTime(startAt, b.mustLocation(userCtx.Family.Timezone)), formatLocalDateTime(endAt, b.mustLocation(userCtx.Family.Timezone))), b.mainKeyboard(false))
	case stateAwaitingEditLast:
		startAt, endAt, err := parseSleepRange(text, time.Now(), b.mustLocation(userCtx.Family.Timezone))
		if err != nil {
			return true, b.sendText(msg.Chat.ID, "Не понял интервал. Пример: `11:10 - 12:35`.")
		}
		if _, err := b.store.UpdateLastCompletedSleep(ctx, userCtx.Child.ID, userCtx.Member.ID, startAt, endAt); err != nil {
			return true, b.sendText(msg.Chat.ID, escapeTelegramMarkdown(err.Error()))
		}
		if err := b.store.ClearUserState(ctx, msg.From.ID); err != nil {
			return true, err
		}
		return true, b.sendText(msg.Chat.ID, "Последний сон обновлен.")
	case stateAwaitingChildName:
		if err := b.store.SetChildName(ctx, userCtx.Family.ID, text); err != nil {
			return true, b.sendText(msg.Chat.ID, escapeTelegramMarkdown(err.Error()))
		}
		if err := b.store.ClearUserState(ctx, msg.From.ID); err != nil {
			return true, err
		}
		return true, b.sendText(msg.Chat.ID, "Имя ребенка обновлено.")
	case stateAwaitingTimezone:
		if err := b.store.SetFamilyTimezone(ctx, userCtx.Family.ID, text); err != nil {
			return true, b.sendText(msg.Chat.ID, escapeTelegramMarkdown(err.Error()))
		}
		if err := b.store.ClearUserState(ctx, msg.From.ID); err != nil {
			return true, err
		}
		return true, b.sendText(msg.Chat.ID, "Таймзона обновлена.")
	case stateAwaitingBirthDate:
		if err := b.applyBirthDate(ctx, userCtx, msg.Chat.ID, text); err != nil {
			return true, err
		}
		if err := b.store.ClearUserState(ctx, msg.From.ID); err != nil {
			return true, err
		}
		return true, nil
	case stateAwaitingReminder:
		if err := b.applyCustomReminder(ctx, userCtx, msg.Chat.ID, text); err != nil {
			return true, err
		}
		if err := b.store.ClearUserState(ctx, msg.From.ID); err != nil {
			return true, err
		}
		return true, nil
	default:
		return false, nil
	}
}

func (b *SleepBot) processReminders(ctx context.Context) error {
	targets, err := b.store.GetReminderTargets(ctx)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, target := range targets {
		if !target.Settings.RemindersEnabled || len(target.Members) == 0 {
			continue
		}
		loc := b.mustLocation(target.Family.Timezone)
		active, err := b.store.GetActiveSleep(ctx, target.Child.ID)
		if err != nil {
			return err
		}
		lastCompleted, err := b.store.GetLastCompletedSleep(ctx, target.Child.ID)
		if err != nil {
			return err
		}
		lastEvent, err := b.store.GetLatestEventTime(ctx, target.Child.ID)
		if err != nil {
			return err
		}

		if active == nil && lastCompleted != nil && target.Settings.WakeWindowEnabled {
			due := lastCompleted.EndAt.Add(time.Duration(target.Settings.WakeWindowMinutes) * time.Minute)
			if now.After(due) {
				key := fmt.Sprintf("wake-window:%d:%d", lastCompleted.ID, target.Settings.WakeWindowMinutes)
				if ok, err := b.store.TryMarkNotificationSent(ctx, target.Family.ID, key); err == nil && ok {
					message := fmt.Sprintf("Пора готовить %s ко сну: окно бодрствования %d мин уже прошло.", escapeTelegramMarkdown(target.Child.Name), target.Settings.WakeWindowMinutes)
					b.broadcast(target.Members, message)
				}
			}
		}

		if active != nil && target.Settings.MaxSleepEnabled {
			due := active.StartAt.Add(time.Duration(target.Settings.MaxSleepMinutes) * time.Minute)
			if now.After(due) {
				key := fmt.Sprintf("max-sleep:%d:%d", active.ID, target.Settings.MaxSleepMinutes)
				if ok, err := b.store.TryMarkNotificationSent(ctx, target.Family.ID, key); err == nil && ok {
					message := fmt.Sprintf("%s спит уже %s. Это больше порога %d мин.", escapeTelegramMarkdown(target.Child.Name), formatDurationRU(now.Sub(active.StartAt)), target.Settings.MaxSleepMinutes)
					b.broadcast(target.Members, message)
				}
			}
		}

		if lastEvent != nil && target.Settings.InactivityEnabled {
			due := lastEvent.Add(time.Duration(target.Settings.InactivityMinutes) * time.Minute)
			if now.After(due) {
				key := fmt.Sprintf("inactivity:%d:%d", lastEvent.Unix()/60, target.Settings.InactivityMinutes)
				if ok, err := b.store.TryMarkNotificationSent(ctx, target.Family.ID, key); err == nil && ok {
					message := fmt.Sprintf("Давно нет записей о сне %s. Последнее событие было %s.", escapeTelegramMarkdown(target.Child.Name), formatLocalDateTime(*lastEvent, loc))
					b.broadcast(target.Members, message)
				}
			}
		}

		reminders, err := b.store.ListCustomReminders(ctx, target.Family.ID)
		if err != nil {
			return err
		}
		currentLocal := now.In(loc)
		currentDate := currentLocal.Format("2006-01-02")
		currentTime := currentLocal.Format("15:04")
		currentWeekday := strconv.Itoa(int(currentLocal.Weekday()))
		for _, reminder := range reminders {
			if !reminder.Enabled || reminder.AtTime != currentTime || !weekdayIncluded(reminder.Weekdays, currentWeekday) || reminder.LastFiredOn == currentDate {
				continue
			}
			key := fmt.Sprintf("custom:%d:%s", reminder.ID, currentDate)
			if ok, err := b.store.TryMarkNotificationSent(ctx, target.Family.ID, key); err == nil && ok {
				b.broadcast(target.Members, fmt.Sprintf("Напоминание: %s", escapeTelegramMarkdown(reminder.Title)))
				_ = b.store.MarkCustomReminderFired(ctx, reminder.ID, currentDate)
			}
		}

		if target.Settings.RemindersEnabled && target.Settings.MilestoneNotifyEach && len(target.Members) > 0 && target.Child.BirthDate != nil {
			loc := b.mustLocation(target.Family.Timezone)
			anchor, ok := BirthAnchorLocal(target.Child.BirthDate, loc)
			if ok && !anchor.After(now) {
				ForEachMilestoneDueForNotify(anchor, now, loc, func(m Milestone) {
					key := fmt.Sprintf("milestone:%s", m.ID)
					if okSent, err := b.store.TryMarkNotificationSent(ctx, target.Family.ID, key); err == nil && okSent {
						b.broadcast(target.Members, FormatMilestonePushMessage(escapeTelegramMarkdown(target.Child.Name), m.Title))
					}
				})
			}
		}
	}

	return nil
}

func (b *SleepBot) sendWelcome(userCtx UserContext, chatID int64) error {
	text := strings.Join([]string{
		fmt.Sprintf("Бот учета сна для `%s`.", escapeTelegramMarkdown(userCtx.Child.Name)),
		"",
		"Журнал сна ведётся в один тап:",
		"`Сон начался`, `Сон закончился`",
		"`Начался 5/10/15/30 минут назад`",
		"`Закончился 5/10/15/30 минут назад`",
		"`Добавить сон`, `Исправить последний сон`, `Отчеты`, `Напоминания`, `Настройки`",
		"",
		"Настройка напоминаний:",
		"автоматические напоминания по умолчанию выключены.",
		"`/reminders`, `/reminders_on`, `/reminders_off`, `/setwake 90`, `/setmaxsleep 120`, `/setinactive 240`",
		"",
		"Вехи (красивые даты) по умолчанию выключены:",
		"`/milestone_notify on|off`, `/milestone_report on|off`",
		"",
		"Полезные команды:",
		"`/report`, `/day`, `/week`, `/month`, `/invite`, `/join CODE`, `/settings`, `/cancel`",
		"`/silent_mode` — выключить все уведомления",
		"`/reset_service confirm` — полная очистка данных",
	}, "\n")

	active, _ := b.store.GetActiveSleep(context.Background(), userCtx.Child.ID)
	return b.sendTextWithKeyboard(chatID, text, b.mainKeyboard(active != nil))
}

func (b *SleepBot) startSleep(ctx context.Context, userCtx UserContext, chatID int64, startAt time.Time, source string) error {
	session, err := b.store.StartSleep(ctx, userCtx.Child.ID, userCtx.Member.ID, startAt.UTC(), source)
	if err != nil {
		return b.sendText(chatID, escapeTelegramMarkdown(err.Error()))
	}
	loc := b.mustLocation(userCtx.Family.Timezone)
	return b.sendTextWithKeyboard(chatID, fmt.Sprintf("Сон начался в %s.", formatLocalDateTime(session.StartAt, loc)), b.mainKeyboard(true))
}

func (b *SleepBot) endSleep(ctx context.Context, userCtx UserContext, chatID int64, endAt time.Time, source string) error {
	session, err := b.store.EndSleep(ctx, userCtx.Child.ID, userCtx.Member.ID, endAt.UTC(), source)
	if err != nil {
		return b.sendText(chatID, escapeTelegramMarkdown(err.Error()))
	}
	loc := b.mustLocation(userCtx.Family.Timezone)
	text := fmt.Sprintf("Сон завершен в %s.\nДлительность: %s.", formatLocalDateTime(*session.EndAt, loc), formatDurationRU(session.EndAt.Sub(session.StartAt)))
	return b.sendTextWithKeyboard(chatID, text, b.mainKeyboard(false))
}

func (b *SleepBot) sendStatus(ctx context.Context, userCtx UserContext, chatID int64) error {
	active, err := b.store.GetActiveSleep(ctx, userCtx.Child.ID)
	if err != nil {
		return err
	}
	members, err := b.store.GetFamilyMembers(ctx, userCtx.Family.ID)
	if err != nil {
		return err
	}
	loc := b.mustLocation(userCtx.Family.Timezone)

	var lines []string
	lines = append(lines, fmt.Sprintf("Семья: %s", escapeTelegramMarkdown(userCtx.Family.Name)))
	lines = append(lines, fmt.Sprintf("Ребенок: %s", escapeTelegramMarkdown(userCtx.Child.Name)))
	lines = append(lines, fmt.Sprintf("Таймзона: %s", escapeTelegramMarkdown(userCtx.Family.Timezone)))
	lines = append(lines, fmt.Sprintf("Подключено родителей: %d", len(members)))
	if active != nil {
		lines = append(lines, fmt.Sprintf("Сейчас идет сон с %s.", formatLocalDateTime(active.StartAt, loc)))
	} else {
		lines = append(lines, "Сейчас активного сна нет.")
	}
	return b.sendText(chatID, strings.Join(lines, "\n"))
}

func (b *SleepBot) sendDashboard(ctx context.Context, userCtx UserContext, chatID int64) error {
	active, err := b.store.GetActiveSleep(ctx, userCtx.Child.ID)
	if err != nil {
		return err
	}
	sessions, err := b.store.ListCompletedSleepsSince(ctx, userCtx.Child.ID, time.Now().UTC().AddDate(0, 0, -40))
	if err != nil {
		return err
	}
	loc := b.mustLocation(userCtx.Family.Timezone)
	report := BuildDashboardReport(escapeTelegramMarkdown(userCtx.Child.Name), sessions, active, loc, time.Now())
	report = b.appendMilestoneReportBlock(userCtx, report, time.Now().In(loc))
	return b.sendText(chatID, report)
}

func (b *SleepBot) sendDayReport(ctx context.Context, userCtx UserContext, chatID int64) error {
	loc := b.mustLocation(userCtx.Family.Timezone)
	active, err := b.store.GetActiveSleep(ctx, userCtx.Child.ID)
	if err != nil {
		return err
	}
	sessions, err := b.store.ListCompletedSleepsSince(ctx, userCtx.Child.ID, time.Now().UTC().AddDate(0, 0, -9))
	if err != nil {
		return err
	}
	day := time.Now().In(loc)
	report := BuildDayReport(sessions, active, day, loc)
	report = b.appendMilestoneReportBlock(userCtx, report, day)
	return b.sendText(chatID, report)
}

func (b *SleepBot) sendRangeReport(ctx context.Context, userCtx UserContext, chatID int64, days int) error {
	active, err := b.store.GetActiveSleep(ctx, userCtx.Child.ID)
	if err != nil {
		return err
	}
	sessions, err := b.store.ListCompletedSleepsSince(ctx, userCtx.Child.ID, time.Now().UTC().AddDate(0, 0, -(days+2)))
	if err != nil {
		return err
	}
	report := BuildRangeReport(sessions, active, time.Now(), days, b.mustLocation(userCtx.Family.Timezone))
	return b.sendText(chatID, report)
}

func (b *SleepBot) sendSettings(ctx context.Context, userCtx UserContext, chatID int64) error {
	var lines []string
	lines = append(lines, "Настройки:")
	lines = append(lines, fmt.Sprintf("Ребенок: %s", escapeTelegramMarkdown(userCtx.Child.Name)))
	lines = append(lines, fmt.Sprintf("Таймзона: %s", escapeTelegramMarkdown(userCtx.Family.Timezone)))
	if userCtx.Child.BirthDate != nil {
		loc := b.mustLocation(userCtx.Family.Timezone)
		lines = append(lines, fmt.Sprintf("Дата и время рождения: %s", formatChildBirthForSettings(*userCtx.Child.BirthDate, loc)))
	}
	lines = append(lines, "")
	lines = append(lines, "Команды:")
	lines = append(lines, "`/invite`")
	lines = append(lines, "`/setchild Имя`")
	lines = append(lines, "`/settimezone Europe/Moscow`")
	lines = append(lines, "`/setbirthdate 16.03.2026 14:30` или `/setbirthdate 16.03.2026`")
	lines = append(lines, "`/editlast`")
	lines = append(lines, "")
	lines = append(lines, "Красивые даты (от полуночи дня рождения в вашей таймзоне):")
	lines = append(lines, fmt.Sprintf("Уведомления о каждой вехе: %s (`/milestone_notify on|off`)", milestoneOnOff(userCtx.Settings.MilestoneNotifyEach)))
	lines = append(lines, fmt.Sprintf("Список в отчётах: %s (`/milestone_report on|off`)", milestoneOnOff(userCtx.Settings.MilestoneReportToday)))
	return b.sendText(chatID, strings.Join(lines, "\n"))
}

func (b *SleepBot) sendReminders(ctx context.Context, userCtx UserContext, chatID int64) error {
	custom, err := b.store.ListCustomReminders(ctx, userCtx.Family.ID)
	if err != nil {
		return err
	}
	var lines []string
	lines = append(lines, "Напоминания:")
	lines = append(lines, fmt.Sprintf("Включены: %t", userCtx.Settings.RemindersEnabled))
	lines = append(lines, fmt.Sprintf("Красивые даты — уведомления: %s", milestoneOnOff(userCtx.Settings.MilestoneNotifyEach)))
	lines = append(lines, fmt.Sprintf("Красивые даты — в отчётах: %s", milestoneOnOff(userCtx.Settings.MilestoneReportToday)))
	lines = append(lines, fmt.Sprintf("Окно бодрствования: %d мин", userCtx.Settings.WakeWindowMinutes))
	lines = append(lines, fmt.Sprintf("Слишком долгий сон: %d мин", userCtx.Settings.MaxSleepMinutes))
	lines = append(lines, fmt.Sprintf("Нет записей: %d мин", userCtx.Settings.InactivityMinutes))
	lines = append(lines, "")
	lines = append(lines, "Команды:")
	lines = append(lines, "`/reminders_on`, `/reminders_off`")
	lines = append(lines, "`/milestone_notify on|off`, `/milestone_report on|off`")
	lines = append(lines, "`/setwake 90`, `/setmaxsleep 120`, `/setinactive 240`")
	lines = append(lines, "`/addreminder 19:30 Купание`")
	if len(custom) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Пользовательские напоминания:")
		for _, reminder := range custom {
			lines = append(lines, fmt.Sprintf("`%d` %s %s", reminder.ID, reminder.AtTime, escapeTelegramMarkdown(reminder.Title)))
		}
		lines = append(lines, "Удаление: `/deletereminder ID`")
	}
	return b.sendText(chatID, strings.Join(lines, "\n"))
}

func milestoneOnOff(on bool) string {
	if on {
		return "вкл"
	}
	return "выкл"
}

func parseOnOffArg(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "on", "1", "да", "вкл", "true":
		return true, true
	case "off", "0", "нет", "выкл", "false":
		return false, true
	default:
		return false, false
	}
}

func (b *SleepBot) appendMilestoneReportBlock(userCtx UserContext, base string, calendarDay time.Time) string {
	if !userCtx.Settings.MilestoneReportToday || userCtx.Child.BirthDate == nil {
		return base
	}
	loc := b.mustLocation(userCtx.Family.Timezone)
	anchor, ok := BirthAnchorLocal(userCtx.Child.BirthDate, loc)
	if !ok {
		return base
	}
	from := calendarDay.In(loc)
	ms := NextMilestonesShownInDailyReportAtOrAfter(anchor, from, loc, 3)
	block := FormatMilestoneReportBlock(escapeTelegramMarkdown(userCtx.Child.Name), ms, loc, anchor)
	if block == "" {
		return base
	}
	return base + "\n\n" + block
}

func (b *SleepBot) setMilestoneNotifyEach(ctx context.Context, userCtx UserContext, chatID int64, args string) error {
	on, ok := parseOnOffArg(args)
	if !ok {
		return b.sendText(chatID, "Использование: `/milestone_notify on` или `/milestone_notify off`")
	}
	if err := b.store.SetMilestoneNotifyEach(ctx, userCtx.Family.ID, on); err != nil {
		return err
	}
	if on {
		return b.sendText(chatID, "Уведомления о красивых датах включены. Отправка — только при включённых напоминаниях (`/reminders_on`). Нужна дата рождения (`/setbirthdate`).")
	}
	return b.sendText(chatID, "Уведомления о красивых датах выключены.")
}

func (b *SleepBot) setMilestoneReportToday(ctx context.Context, userCtx UserContext, chatID int64, args string) error {
	on, ok := parseOnOffArg(args)
	if !ok {
		return b.sendText(chatID, "Использование: `/milestone_report on` или `/milestone_report off`")
	}
	if err := b.store.SetMilestoneReportToday(ctx, userCtx.Family.ID, on); err != nil {
		return err
	}
	if on {
		return b.sendText(chatID, "В отчётах будет список ближайших 3 красивых дат. Нужна дата рождения (`/setbirthdate`).")
	}
	return b.sendText(chatID, "Список красивых дат в отчётах выключен.")
}

func (b *SleepBot) updateReminderThreshold(ctx context.Context, userCtx UserContext, chatID int64, field string, args string) error {
	minutes, err := strconv.Atoi(strings.TrimSpace(args))
	if err != nil {
		return b.sendText(chatID, "Нужно указать число минут.")
	}
	if err := b.store.UpdateReminderThreshold(ctx, userCtx.Family.ID, field, minutes); err != nil {
		return err
	}
	return b.sendText(chatID, "Настройка обновлена.")
}

func (b *SleepBot) applyBirthDate(ctx context.Context, userCtx UserContext, chatID int64, raw string) error {
	loc := b.mustLocation(userCtx.Family.Timezone)
	birthDate, err := ParseBirthDateInput(raw, loc)
	if err != nil {
		return b.sendText(chatID, "Укажите `02.01.2006 15:04` или `02.01.2006` (время в вашей таймзоне), либо RFC3339.")
	}
	if err := b.store.SetChildBirthDate(ctx, userCtx.Family.ID, birthDate); err != nil {
		return err
	}
	return b.sendText(chatID, "Дата и время рождения сохранены.")
}

func (b *SleepBot) applyCustomReminder(ctx context.Context, userCtx UserContext, chatID int64, raw string) error {
	parts := strings.SplitN(strings.TrimSpace(raw), " ", 2)
	if len(parts) < 2 {
		return b.sendText(chatID, "Формат: `19:30 Купание`.")
	}
	if err := b.store.AddCustomReminder(ctx, userCtx.Family.ID, parts[0], parts[1], "0,1,2,3,4,5,6"); err != nil {
		return b.sendText(chatID, escapeTelegramMarkdown(err.Error()))
	}
	return b.sendText(chatID, "Пользовательское напоминание добавлено.")
}

func (b *SleepBot) broadcast(members []Member, text string) {
	for _, member := range members {
		if member.TelegramChatID == 0 {
			continue
		}
		if err := b.sendText(member.TelegramChatID, text); err != nil {
			log.Printf("broadcast failed to %d: %v", member.TelegramChatID, err)
		}
	}
}

func (b *SleepBot) sendText(chatID int64, text string) error {
	for _, part := range splitTelegramMessage(text, telegramMaxMessageRunes) {
		msg := tgbotapi.NewMessage(chatID, part)
		msg.ParseMode = "Markdown"
		_, err := b.api.Send(msg)
		if err != nil && telegramSendPlainFallback(err) {
			msg.ParseMode = ""
			_, err = b.api.Send(msg)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *SleepBot) sendTextWithKeyboard(chatID int64, text string, keyboard tgbotapi.ReplyKeyboardMarkup) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	_, err := b.api.Send(msg)
	return err
}

func (b *SleepBot) mainKeyboard(active bool) tgbotapi.ReplyKeyboardMarkup {
	if active {
		return tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Сон закончился"),
				tgbotapi.NewKeyboardButton("Закончился 5 минут назад"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Закончился 10 минут назад"),
				tgbotapi.NewKeyboardButton("Закончился 15 минут назад"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Закончился 30 минут назад"),
				tgbotapi.NewKeyboardButton("Исправить последний сон"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Оценить"),
				tgbotapi.NewKeyboardButton("Отчеты"),
				tgbotapi.NewKeyboardButton("Напоминания"),
				tgbotapi.NewKeyboardButton("Настройки"),
			),
		)
	}

	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Сон начался"),
			tgbotapi.NewKeyboardButton("Начался 5 минут назад"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Начался 10 минут назад"),
			tgbotapi.NewKeyboardButton("Начался 15 минут назад"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Начался 30 минут назад"),
			tgbotapi.NewKeyboardButton("Добавить сон"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Исправить последний сон"),
			tgbotapi.NewKeyboardButton("Оценить"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Отчеты"),
			tgbotapi.NewKeyboardButton("Напоминания"),
			tgbotapi.NewKeyboardButton("Настройки"),
		),
	)
}

func (b *SleepBot) mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		loc, _ = time.LoadLocation(b.cfg.DefaultTimezone)
	}
	return loc
}

func (b *SleepBot) localTimeHint(userCtx UserContext) string {
	return "Время в вашей таймзоне: `" + escapeTelegramMarkdown(userCtx.Family.Timezone) + "`"
}

func (b *SleepBot) sendEvaluation(ctx context.Context, userCtx UserContext, chatID int64) error {
	loc := b.mustLocation(userCtx.Family.Timezone)
	active, err := b.store.GetActiveSleep(ctx, userCtx.Child.ID)
	if err != nil {
		return err
	}
	since := time.Now().UTC().Add(-48 * time.Hour)
	sessions, err := b.store.ListCompletedSleepsSince(ctx, userCtx.Child.ID, since)
	if err != nil {
		return err
	}
	merged := sessionsWithActive(sessions, active, time.Now())
	report := BuildNormsReport(userCtx.Child, merged, loc, time.Now())
	return b.sendText(chatID, report)
}

func (b *SleepBot) editLastSleepMessage(ctx context.Context, userCtx UserContext) (string, error) {
	last, err := b.store.GetLastCompletedSleep(ctx, userCtx.Child.ID)
	if err != nil {
		return "", err
	}
	loc := b.mustLocation(userCtx.Family.Timezone)
	if last == nil || last.EndAt == nil {
		return "Записей сна пока нет. Сначала добавьте сон через «Сон».", nil
	}
	msg := "Отправьте новый интервал для последнего сна."
	interval := formatLocalDateTime(last.StartAt, loc) + " - " + formatLocalDateTime(*last.EndAt, loc)
	msg += "\n\nИсправляемый интервал (можно скопировать и отредактировать):\n`" + escapeTelegramMarkdown(interval) + "`"
	msg += "\n\n" + b.localTimeHint(userCtx)
	return msg, nil
}

func fullName(user *tgbotapi.User) string {
	name := strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
	if name == "" {
		name = user.UserName
	}
	return name
}

func parseSleepRange(input string, now time.Time, loc *time.Location) (time.Time, time.Time, error) {
	chunks := strings.Split(input, "-")
	if len(chunks) != 2 {
		return time.Time{}, time.Time{}, fmt.Errorf("bad range format")
	}
	startRaw := strings.TrimSpace(chunks[0])
	endRaw := strings.TrimSpace(chunks[1])

	startAt, startHasDate, err := parseFlexibleDateTime(startRaw, now, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	endAt, endHasDate, err := parseFlexibleDateTime(endRaw, now, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	if !endHasDate && !startHasDate && !endAt.After(startAt) {
		// Если пользователь указал только время без дат, то "конец" идёт на следующий
		// календарный день в локали, а не на +24 часа.
		endAt = endAt.AddDate(0, 0, 1)
	}
	if startHasDate && !endHasDate && !endAt.After(startAt) {
		// Та же логика: конец интервала без даты должен попасть в следующий локальный
		// календарный день относительно начала.
		endAt = endAt.AddDate(0, 0, 1)
	}
	return startAt.UTC(), endAt.UTC(), nil
}

func parseFlexibleDateTime(raw string, now time.Time, loc *time.Location) (time.Time, bool, error) {
	raw = strings.TrimSpace(raw)
	layoutsWithDate := []string{
		"02.01.2006 15:04",
		"02.01 15:04",
	}
	for _, layout := range layoutsWithDate {
		if parsed, err := time.ParseInLocation(layout, raw, loc); err == nil {
			if layout == "02.01 15:04" {
				parsed = time.Date(now.In(loc).Year(), parsed.Month(), parsed.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc)
			}
			return parsed, true, nil
		}
	}
	if parsed, err := time.ParseInLocation("15:04", raw, loc); err == nil {
		current := now.In(loc)
		return time.Date(current.Year(), current.Month(), current.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc), false, nil
	}
	return time.Time{}, false, fmt.Errorf("unsupported datetime")
}

func formatLocalDateTime(ts time.Time, loc *time.Location) string {
	return ts.In(loc).Format("02.01 15:04")
}

func weekdayIncluded(csv string, value string) bool {
	for _, part := range strings.Split(csv, ",") {
		if strings.TrimSpace(part) == value {
			return true
		}
	}
	return false
}

func decodePayload[T any](raw json.RawMessage) (T, error) {
	var payload T
	err := json.Unmarshal(raw, &payload)
	return payload, err
}
