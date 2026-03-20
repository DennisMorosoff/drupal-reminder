package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	sourceRealTime      = "real_time"
	sourceQuickBackdate = "quick_backdated"
	sourceManual        = "manual"
)

type Store struct {
	db     *sql.DB
	cfg    Config
	clock  func() time.Time
	random *rand.Rand
	mu     sync.Mutex
}

type Member struct {
	ID             int64
	FamilyID       int64
	TelegramUserID int64
	TelegramChatID int64
	DisplayName    string
	Role           string
}

type Family struct {
	ID       int64
	Name     string
	Timezone string
}

type Child struct {
	ID        int64
	FamilyID  int64
	Name      string
	BirthDate *time.Time
}

type SleepSession struct {
	ID          int64
	ChildID     int64
	StartAt     time.Time
	EndAt       *time.Time
	StartSource string
	EndSource   string
	Note        string
	CreatedBy   int64
	UpdatedBy   int64
}

type ReminderSettings struct {
	FamilyID             int64
	RemindersEnabled     bool
	WakeWindowEnabled    bool
	MaxSleepEnabled      bool
	InactivityEnabled    bool
	WakeWindowMinutes    int
	MaxSleepMinutes      int
	InactivityMinutes    int
	MilestoneNotifyEach  bool
	MilestoneReportToday bool
}

type CustomReminder struct {
	ID          int64
	FamilyID    int64
	Title       string
	AtTime      string
	Weekdays    string
	Enabled     bool
	LastFiredOn string
}

type UserState struct {
	State   string
	Payload json.RawMessage
}

type UserContext struct {
	Member   Member
	Family   Family
	Child    Child
	Settings ReminderSettings
}

type ReminderTarget struct {
	Family   Family
	Child    Child
	Settings ReminderSettings
	Members  []Member
}

func NewStore(db *sql.DB, cfg Config) (*Store, error) {
	store := &Store{
		db:     db,
		cfg:    cfg,
		clock:  time.Now,
		random: rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	if err := store.initSchema(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) initSchema() error {
	statements := []string{
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS families (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			timezone TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS family_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			family_id INTEGER NOT NULL,
			telegram_user_id INTEGER NOT NULL UNIQUE,
			telegram_chat_id INTEGER NOT NULL,
			display_name TEXT NOT NULL,
			role TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(family_id) REFERENCES families(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS children (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			family_id INTEGER NOT NULL UNIQUE,
			name TEXT NOT NULL,
			birth_date TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(family_id) REFERENCES families(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS sleep_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			child_id INTEGER NOT NULL,
			start_at TEXT NOT NULL,
			end_at TEXT,
			start_source TEXT NOT NULL,
			end_source TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			created_by INTEGER NOT NULL,
			updated_by INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(child_id) REFERENCES children(id) ON DELETE CASCADE,
			FOREIGN KEY(created_by) REFERENCES family_members(id) ON DELETE CASCADE,
			FOREIGN KEY(updated_by) REFERENCES family_members(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sleep_sessions_child_start ON sleep_sessions(child_id, start_at);`,
		`CREATE TABLE IF NOT EXISTS invite_codes (
			code TEXT PRIMARY KEY,
			family_id INTEGER NOT NULL,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(family_id) REFERENCES families(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS reminder_settings (
			family_id INTEGER PRIMARY KEY,
			reminders_enabled INTEGER NOT NULL,
			wake_window_enabled INTEGER NOT NULL,
			max_sleep_enabled INTEGER NOT NULL,
			inactivity_enabled INTEGER NOT NULL,
			wake_window_minutes INTEGER NOT NULL,
			max_sleep_minutes INTEGER NOT NULL,
			inactivity_minutes INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(family_id) REFERENCES families(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS custom_reminders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			family_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			at_time TEXT NOT NULL,
			weekdays TEXT NOT NULL,
			enabled INTEGER NOT NULL,
			last_fired_on TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(family_id) REFERENCES families(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS user_states (
			telegram_user_id INTEGER PRIMARY KEY,
			family_id INTEGER NOT NULL,
			state TEXT NOT NULL,
			payload TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(family_id) REFERENCES families(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS notification_log (
			family_id INTEGER NOT NULL,
			reminder_key TEXT NOT NULL,
			sent_at TEXT NOT NULL,
			PRIMARY KEY (family_id, reminder_key),
			FOREIGN KEY(family_id) REFERENCES families(id) ON DELETE CASCADE
		);`,
	}

	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("schema init failed: %w", err)
		}
	}

	return s.migrateMilestoneSettingsColumns()
}

func (s *Store) migrateMilestoneSettingsColumns() error {
	stmts := []string{
		`ALTER TABLE reminder_settings ADD COLUMN milestone_notify_each INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE reminder_settings ADD COLUMN milestone_report_today INTEGER NOT NULL DEFAULT 0`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				return fmt.Errorf("migrate reminder_settings: %w", err)
			}
		}
	}
	return nil
}

func (s *Store) EnsureMember(ctx context.Context, telegramUserID int64, telegramChatID int64, displayName string) (UserContext, bool, error) {
	if _, err := s.GetUserContext(ctx, telegramUserID); err == nil {
		if err := s.updateMemberPresence(ctx, telegramUserID, telegramChatID, displayName); err != nil {
			return UserContext{}, false, err
		}
		refreshed, refreshErr := s.GetUserContext(ctx, telegramUserID)
		return refreshed, false, refreshErr
	} else if !errors.Is(err, sql.ErrNoRows) {
		return UserContext{}, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.nowUTCString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UserContext{}, false, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx,
		`INSERT INTO families(name, timezone, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"Наша семья", s.cfg.DefaultTimezone, now, now,
	)
	if err != nil {
		return UserContext{}, false, err
	}
	familyID, _ := result.LastInsertId()

	childResult, err := tx.ExecContext(ctx,
		`INSERT INTO children(family_id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		familyID, "Малыш", now, now,
	)
	if err != nil {
		return UserContext{}, false, err
	}
	_, _ = childResult.LastInsertId()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO reminder_settings(
			family_id, reminders_enabled, wake_window_enabled, max_sleep_enabled, inactivity_enabled,
			wake_window_minutes, max_sleep_minutes, inactivity_minutes,
			milestone_notify_each, milestone_report_today,
			created_at, updated_at
		) VALUES (?, 1, 1, 1, 1, 90, 120, 240, 0, 0, ?, ?)`,
		familyID, now, now,
	); err != nil {
		return UserContext{}, false, err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO family_members(
			family_id, telegram_user_id, telegram_chat_id, display_name, role, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		familyID, telegramUserID, telegramChatID, sanitizeDisplayName(displayName), "owner", now, now,
	); err != nil {
		return UserContext{}, false, err
	}

	if err := tx.Commit(); err != nil {
		return UserContext{}, false, err
	}

	createdCtx, err := s.GetUserContext(ctx, telegramUserID)
	return createdCtx, true, err
}

func (s *Store) JoinFamily(ctx context.Context, code string, telegramUserID int64, telegramChatID int64, displayName string) (UserContext, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return UserContext{}, fmt.Errorf("код приглашения пустой")
	}

	if _, err := s.GetUserContext(ctx, telegramUserID); err == nil {
		return UserContext{}, fmt.Errorf("этот Telegram-аккаунт уже привязан к семье")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return UserContext{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UserContext{}, err
	}
	defer tx.Rollback()

	var familyID int64
	var expiresAtRaw string
	if err := tx.QueryRowContext(ctx,
		`SELECT family_id, expires_at FROM invite_codes WHERE code = ?`,
		code,
	).Scan(&familyID, &expiresAtRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserContext{}, fmt.Errorf("код приглашения не найден")
		}
		return UserContext{}, err
	}

	expiresAt, err := parseStoredTime(expiresAtRaw)
	if err != nil {
		return UserContext{}, err
	}
	if expiresAt.Before(s.clock().UTC()) {
		return UserContext{}, fmt.Errorf("код приглашения уже истек")
	}

	now := s.nowUTCString()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO family_members(
			family_id, telegram_user_id, telegram_chat_id, display_name, role, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		familyID, telegramUserID, telegramChatID, sanitizeDisplayName(displayName), "parent", now, now,
	); err != nil {
		return UserContext{}, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM invite_codes WHERE code = ?`, code); err != nil {
		return UserContext{}, err
	}

	if err := tx.Commit(); err != nil {
		return UserContext{}, err
	}

	return s.GetUserContext(ctx, telegramUserID)
}

func (s *Store) CreateInviteCode(ctx context.Context, familyID int64) (string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	code := s.generateInviteCode()
	expiresAt := s.clock().UTC().Add(s.cfg.InviteTTL)
	now := s.nowUTCString()

	if _, err := s.db.ExecContext(ctx, `DELETE FROM invite_codes WHERE family_id = ?`, familyID); err != nil {
		return "", time.Time{}, err
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO invite_codes(code, family_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		code, familyID, toStoredTime(expiresAt), now,
	); err != nil {
		return "", time.Time{}, err
	}

	return code, expiresAt, nil
}

func (s *Store) GetUserContext(ctx context.Context, telegramUserID int64) (UserContext, error) {
	query := `
		SELECT
			m.id, m.family_id, m.telegram_user_id, m.telegram_chat_id, m.display_name, m.role,
			f.id, f.name, f.timezone,
			c.id, c.family_id, c.name, c.birth_date,
			rs.family_id, rs.reminders_enabled, rs.wake_window_enabled, rs.max_sleep_enabled, rs.inactivity_enabled,
			rs.wake_window_minutes, rs.max_sleep_minutes, rs.inactivity_minutes,
			COALESCE(rs.milestone_notify_each, 0), COALESCE(rs.milestone_report_today, 0)
		FROM family_members m
		JOIN families f ON f.id = m.family_id
		JOIN children c ON c.family_id = f.id
		JOIN reminder_settings rs ON rs.family_id = f.id
		WHERE m.telegram_user_id = ?
	`

	var (
		member          Member
		family          Family
		child           Child
		settings        ReminderSettings
		birthDateString sql.NullString
		remindersOn     int
		wakeOn          int
		maxSleepOn      int
		inactivityOn    int
		milestonePush   int
		milestoneReport int
	)

	err := s.db.QueryRowContext(ctx, query, telegramUserID).Scan(
		&member.ID, &member.FamilyID, &member.TelegramUserID, &member.TelegramChatID, &member.DisplayName, &member.Role,
		&family.ID, &family.Name, &family.Timezone,
		&child.ID, &child.FamilyID, &child.Name, &birthDateString,
		&settings.FamilyID, &remindersOn, &wakeOn, &maxSleepOn, &inactivityOn,
		&settings.WakeWindowMinutes, &settings.MaxSleepMinutes, &settings.InactivityMinutes,
		&milestonePush, &milestoneReport,
	)
	if err != nil {
		return UserContext{}, err
	}

	settings.RemindersEnabled = remindersOn == 1
	settings.WakeWindowEnabled = wakeOn == 1
	settings.MaxSleepEnabled = maxSleepOn == 1
	settings.InactivityEnabled = inactivityOn == 1
	settings.MilestoneNotifyEach = milestonePush == 1
	settings.MilestoneReportToday = milestoneReport == 1

	if birthDateString.Valid && strings.TrimSpace(birthDateString.String) != "" {
		if parsed, ok := ParseBirthDateStored(birthDateString.String); ok {
			child.BirthDate = &parsed
		}
	}

	return UserContext{
		Member:   member,
		Family:   family,
		Child:    child,
		Settings: settings,
	}, nil
}

func (s *Store) GetReminderTargets(ctx context.Context) ([]ReminderTarget, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.id, f.name, f.timezone, c.id, c.name, c.birth_date,
			rs.reminders_enabled, rs.wake_window_enabled, rs.max_sleep_enabled, rs.inactivity_enabled,
			rs.wake_window_minutes, rs.max_sleep_minutes, rs.inactivity_minutes,
			COALESCE(rs.milestone_notify_each, 0), COALESCE(rs.milestone_report_today, 0)
		FROM families f
		JOIN children c ON c.family_id = f.id
		JOIN reminder_settings rs ON rs.family_id = f.id
		ORDER BY f.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []ReminderTarget
	for rows.Next() {
		var (
			target          ReminderTarget
			birthDateString sql.NullString
			remindersOn     int
			wakeOn          int
			maxSleepOn      int
			inactivityOn    int
			milestonePush   int
			milestoneReport int
		)
		if err := rows.Scan(
			&target.Family.ID, &target.Family.Name, &target.Family.Timezone,
			&target.Child.ID, &target.Child.Name, &birthDateString,
			&remindersOn, &wakeOn, &maxSleepOn, &inactivityOn,
			&target.Settings.WakeWindowMinutes, &target.Settings.MaxSleepMinutes, &target.Settings.InactivityMinutes,
			&milestonePush, &milestoneReport,
		); err != nil {
			return nil, err
		}
		target.Settings.FamilyID = target.Family.ID
		target.Settings.RemindersEnabled = remindersOn == 1
		target.Settings.WakeWindowEnabled = wakeOn == 1
		target.Settings.MaxSleepEnabled = maxSleepOn == 1
		target.Settings.InactivityEnabled = inactivityOn == 1
		target.Settings.MilestoneNotifyEach = milestonePush == 1
		target.Settings.MilestoneReportToday = milestoneReport == 1

		if birthDateString.Valid && strings.TrimSpace(birthDateString.String) != "" {
			if parsed, ok := ParseBirthDateStored(birthDateString.String); ok {
				target.Child.BirthDate = &parsed
			}
		}

		members, err := s.GetFamilyMembers(ctx, target.Family.ID)
		if err != nil {
			return nil, err
		}
		target.Members = members
		targets = append(targets, target)
	}

	return targets, rows.Err()
}

func (s *Store) GetFamilyMembers(ctx context.Context, familyID int64) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, family_id, telegram_user_id, telegram_chat_id, display_name, role
		FROM family_members
		WHERE family_id = ?
		ORDER BY id
	`, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []Member
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.ID, &member.FamilyID, &member.TelegramUserID, &member.TelegramChatID, &member.DisplayName, &member.Role); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (s *Store) GetActiveSleep(ctx context.Context, childID int64) (*SleepSession, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, child_id, start_at, end_at, start_source, end_source, note, created_by, updated_by
		FROM sleep_sessions
		WHERE child_id = ? AND end_at IS NULL
		ORDER BY start_at DESC
		LIMIT 1
	`, childID)
	session, err := scanSleepSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return session, err
}

func (s *Store) StartSleep(ctx context.Context, childID int64, memberID int64, startAt time.Time, source string) (*SleepSession, error) {
	if err := s.validateTimestamp(startAt); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if existing, err := s.getActiveSleepTx(ctx, tx, childID); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, fmt.Errorf("сон уже идет с %s", existing.StartAt.Format("15:04"))
	}

	if err := s.ensureNoOverlapTx(ctx, tx, childID, startAt, nil, 0); err != nil {
		return nil, err
	}

	now := s.nowUTCString()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO sleep_sessions(
			child_id, start_at, end_at, start_source, end_source, note, created_by, updated_by, created_at, updated_at
		) VALUES (?, ?, NULL, ?, '', '', ?, ?, ?, ?)
	`, childID, toStoredTime(startAt), source, memberID, memberID, now, now)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetSleepByID(ctx, id)
}

func (s *Store) EndSleep(ctx context.Context, childID int64, memberID int64, endAt time.Time, source string) (*SleepSession, error) {
	if err := s.validateTimestamp(endAt); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	active, err := s.getActiveSleepTx(ctx, tx, childID)
	if err != nil {
		return nil, err
	}
	if active == nil {
		return nil, fmt.Errorf("сейчас нет активного сна")
	}
	if !endAt.After(active.StartAt) {
		return nil, fmt.Errorf("время окончания должно быть позже начала сна")
	}

	now := s.nowUTCString()
	if _, err := tx.ExecContext(ctx, `
		UPDATE sleep_sessions
		SET end_at = ?, end_source = ?, updated_by = ?, updated_at = ?
		WHERE id = ?
	`, toStoredTime(endAt), source, memberID, now, active.ID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetSleepByID(ctx, active.ID)
}

func (s *Store) AddManualSleep(ctx context.Context, childID int64, memberID int64, startAt time.Time, endAt time.Time, note string) (*SleepSession, error) {
	if err := s.validateTimestamp(startAt); err != nil {
		return nil, err
	}
	if err := s.validateTimestamp(endAt); err != nil {
		return nil, err
	}
	if !endAt.After(startAt) {
		return nil, fmt.Errorf("окончание должно быть позже начала")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if active, err := s.getActiveSleepTx(ctx, tx, childID); err != nil {
		return nil, err
	} else if active != nil {
		return nil, fmt.Errorf("сначала завершите текущий активный сон")
	}

	if err := s.ensureNoOverlapTx(ctx, tx, childID, startAt, &endAt, 0); err != nil {
		return nil, err
	}

	now := s.nowUTCString()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO sleep_sessions(
			child_id, start_at, end_at, start_source, end_source, note, created_by, updated_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, childID, toStoredTime(startAt), toStoredTime(endAt), sourceManual, sourceManual, strings.TrimSpace(note), memberID, memberID, now, now)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetSleepByID(ctx, id)
}

func (s *Store) UpdateLastCompletedSleep(ctx context.Context, childID int64, memberID int64, startAt time.Time, endAt time.Time) (*SleepSession, error) {
	if !endAt.After(startAt) {
		return nil, fmt.Errorf("окончание должно быть позже начала")
	}
	if err := s.validateTimestamp(startAt); err != nil {
		return nil, err
	}
	if err := s.validateTimestamp(endAt); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	last, err := s.getLastCompletedSleepTx(ctx, tx, childID)
	if err != nil {
		return nil, err
	}
	if last == nil {
		return nil, fmt.Errorf("нет завершенных снов для редактирования")
	}

	if err := s.ensureNoOverlapTx(ctx, tx, childID, startAt, &endAt, last.ID); err != nil {
		return nil, err
	}

	now := s.nowUTCString()
	if _, err := tx.ExecContext(ctx, `
		UPDATE sleep_sessions
		SET start_at = ?, end_at = ?, start_source = ?, end_source = ?, updated_by = ?, updated_at = ?
		WHERE id = ?
	`, toStoredTime(startAt), toStoredTime(endAt), sourceManual, sourceManual, memberID, now, last.ID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetSleepByID(ctx, last.ID)
}

func (s *Store) GetSleepByID(ctx context.Context, id int64) (*SleepSession, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, child_id, start_at, end_at, start_source, end_source, note, created_by, updated_by
		FROM sleep_sessions
		WHERE id = ?
	`, id)
	return scanSleepSession(row)
}

func (s *Store) ListCompletedSleepsSince(ctx context.Context, childID int64, since time.Time) ([]SleepSession, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, child_id, start_at, end_at, start_source, end_source, note, created_by, updated_by
		FROM sleep_sessions
		WHERE child_id = ? AND end_at IS NOT NULL AND start_at >= ?
		ORDER BY start_at ASC
	`, childID, toStoredTime(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return collectSleepSessions(rows)
}

func (s *Store) GetLastCompletedSleep(ctx context.Context, childID int64) (*SleepSession, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, child_id, start_at, end_at, start_source, end_source, note, created_by, updated_by
		FROM sleep_sessions
		WHERE child_id = ? AND end_at IS NOT NULL
		ORDER BY start_at DESC
		LIMIT 1
	`, childID)
	session, err := scanSleepSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return session, err
}

func (s *Store) GetLatestEventTime(ctx context.Context, childID int64) (*time.Time, error) {
	var startAt sql.NullString
	var endAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT start_at, end_at
		FROM sleep_sessions
		WHERE child_id = ?
		ORDER BY COALESCE(end_at, start_at) DESC
		LIMIT 1
	`, childID).Scan(&startAt, &endAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	raw := startAt.String
	if endAt.Valid && endAt.String != "" {
		raw = endAt.String
	}
	parsed, err := parseStoredTime(raw)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (s *Store) SetFamilyTimezone(ctx context.Context, familyID int64, timezone string) error {
	if _, err := time.LoadLocation(timezone); err != nil {
		return fmt.Errorf("не удалось загрузить таймзону: %w", err)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE families SET timezone = ?, updated_at = ? WHERE id = ?`,
		timezone, s.nowUTCString(), familyID,
	)
	return err
}

func (s *Store) SetChildName(ctx context.Context, familyID int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("имя ребенка не может быть пустым")
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE children SET name = ?, updated_at = ? WHERE family_id = ?`,
		name, s.nowUTCString(), familyID,
	)
	return err
}

func (s *Store) SetChildBirthDate(ctx context.Context, familyID int64, birthDate time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE children SET birth_date = ?, updated_at = ? WHERE family_id = ?`,
		FormatBirthDateStored(birthDate), s.nowUTCString(), familyID,
	)
	return err
}

func (s *Store) SetReminderEnabled(ctx context.Context, familyID int64, enabled bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE reminder_settings SET reminders_enabled = ?, updated_at = ? WHERE family_id = ?`,
		boolToInt(enabled), s.nowUTCString(), familyID,
	)
	return err
}

func (s *Store) SetMilestoneNotifyEach(ctx context.Context, familyID int64, enabled bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE reminder_settings SET milestone_notify_each = ?, updated_at = ? WHERE family_id = ?`,
		boolToInt(enabled), s.nowUTCString(), familyID,
	)
	return err
}

func (s *Store) SetMilestoneReportToday(ctx context.Context, familyID int64, enabled bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE reminder_settings SET milestone_report_today = ?, updated_at = ? WHERE family_id = ?`,
		boolToInt(enabled), s.nowUTCString(), familyID,
	)
	return err
}

func (s *Store) UpdateReminderThreshold(ctx context.Context, familyID int64, field string, minutes int) error {
	if minutes <= 0 {
		return fmt.Errorf("значение должно быть больше 0")
	}
	allowed := map[string]bool{
		"wake_window_minutes": true,
		"max_sleep_minutes":   true,
		"inactivity_minutes":  true,
	}
	if !allowed[field] {
		return fmt.Errorf("неподдерживаемое поле настроек")
	}

	query := fmt.Sprintf("UPDATE reminder_settings SET %s = ?, updated_at = ? WHERE family_id = ?", field)
	_, err := s.db.ExecContext(ctx, query, minutes, s.nowUTCString(), familyID)
	return err
}

func (s *Store) AddCustomReminder(ctx context.Context, familyID int64, atTime string, title string, weekdays string) error {
	atTime = strings.TrimSpace(atTime)
	title = strings.TrimSpace(title)
	weekdays = strings.TrimSpace(weekdays)
	if _, err := time.Parse("15:04", atTime); err != nil {
		return fmt.Errorf("время должно быть в формате HH:MM")
	}
	if title == "" {
		return fmt.Errorf("заголовок напоминания пустой")
	}
	if weekdays == "" {
		weekdays = "0,1,2,3,4,5,6"
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO custom_reminders(family_id, title, at_time, weekdays, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?)
	`, familyID, title, atTime, weekdays, s.nowUTCString(), s.nowUTCString())
	return err
}

func (s *Store) DeleteCustomReminder(ctx context.Context, familyID int64, reminderID int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM custom_reminders WHERE id = ? AND family_id = ?`, reminderID, familyID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("напоминание не найдено")
	}
	return nil
}

func (s *Store) ListCustomReminders(ctx context.Context, familyID int64) ([]CustomReminder, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, family_id, title, at_time, weekdays, enabled, last_fired_on
		FROM custom_reminders
		WHERE family_id = ?
		ORDER BY at_time ASC, id ASC
	`, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders []CustomReminder
	for rows.Next() {
		var reminder CustomReminder
		var enabled int
		if err := rows.Scan(
			&reminder.ID, &reminder.FamilyID, &reminder.Title, &reminder.AtTime,
			&reminder.Weekdays, &enabled, &reminder.LastFiredOn,
		); err != nil {
			return nil, err
		}
		reminder.Enabled = enabled == 1
		reminders = append(reminders, reminder)
	}
	return reminders, rows.Err()
}

func (s *Store) SetUserState(ctx context.Context, telegramUserID int64, familyID int64, state string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO user_states(telegram_user_id, family_id, state, payload, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(telegram_user_id) DO UPDATE SET
			family_id = excluded.family_id,
			state = excluded.state,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`, telegramUserID, familyID, state, string(encoded), s.nowUTCString())
	return err
}

func (s *Store) GetUserState(ctx context.Context, telegramUserID int64) (*UserState, error) {
	var state string
	var payload string
	err := s.db.QueryRowContext(ctx, `
		SELECT state, payload
		FROM user_states
		WHERE telegram_user_id = ?
	`, telegramUserID).Scan(&state, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &UserState{State: state, Payload: json.RawMessage(payload)}, nil
}

func (s *Store) ClearUserState(ctx context.Context, telegramUserID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_states WHERE telegram_user_id = ?`, telegramUserID)
	return err
}

// ResetService performs a full wipe of all persistent bot data.
// After this call the bot will start fresh and recreate a family/profile on next use.
func (s *Store) ResetService(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Ensure ON for this connection (needed for cascades).
	if _, err := tx.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		return err
	}

	// One entry point is enough because all dependent rows use ON DELETE CASCADE.
	if _, err := tx.ExecContext(ctx, `DELETE FROM families`); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Keep DB file compact (best-effort; VACUUM can be costly on big DBs).
	_, _ = s.db.ExecContext(ctx, `VACUUM;`)
	return nil
}

// SetSilentMode disables all automatic notifications for the whole service.
// It also disables the optional milestone block inside generated reports.
func (s *Store) SetSilentMode(ctx context.Context, silent bool) error {
	if !silent {
		// There's no separate "restore defaults" mode requested here; the caller can
		// control settings via existing commands.
		return nil
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE reminder_settings
		SET
			reminders_enabled = 0,
			wake_window_enabled = 0,
			max_sleep_enabled = 0,
			inactivity_enabled = 0,
			milestone_notify_each = 0,
			milestone_report_today = 0,
			updated_at = ?
	`, s.nowUTCString())
	return err
}

func (s *Store) TryMarkNotificationSent(ctx context.Context, familyID int64, key string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO notification_log(family_id, reminder_key, sent_at)
		VALUES (?, ?, ?)
	`, familyID, key, s.nowUTCString())
	if err != nil {
		return false, err
	}
	rowsAffected, _ := result.RowsAffected()
	return rowsAffected == 1, nil
}

func (s *Store) MarkCustomReminderFired(ctx context.Context, reminderID int64, localDate string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE custom_reminders SET last_fired_on = ?, updated_at = ? WHERE id = ?`,
		localDate, s.nowUTCString(), reminderID,
	)
	return err
}

func (s *Store) getActiveSleepTx(ctx context.Context, tx *sql.Tx, childID int64) (*SleepSession, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, child_id, start_at, end_at, start_source, end_source, note, created_by, updated_by
		FROM sleep_sessions
		WHERE child_id = ? AND end_at IS NULL
		ORDER BY start_at DESC
		LIMIT 1
	`, childID)
	session, err := scanSleepSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return session, err
}

func (s *Store) getLastCompletedSleepTx(ctx context.Context, tx *sql.Tx, childID int64) (*SleepSession, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, child_id, start_at, end_at, start_source, end_source, note, created_by, updated_by
		FROM sleep_sessions
		WHERE child_id = ? AND end_at IS NOT NULL
		ORDER BY start_at DESC
		LIMIT 1
	`, childID)
	session, err := scanSleepSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return session, err
}

func (s *Store) ensureNoOverlapTx(ctx context.Context, tx *sql.Tx, childID int64, startAt time.Time, endAt *time.Time, excludeID int64) error {
	proposedEnd := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	if endAt != nil {
		proposedEnd = endAt.UTC()
	}

	row := tx.QueryRowContext(ctx, `
		SELECT id, child_id, start_at, end_at, start_source, end_source, note, created_by, updated_by
		FROM sleep_sessions
		WHERE child_id = ? AND id <> ? AND start_at < ? AND COALESCE(end_at, ?) > ?
		LIMIT 1
	`, childID, excludeID, toStoredTime(proposedEnd), toStoredTime(proposedEnd), toStoredTime(startAt))

	session, err := scanSleepSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if session != nil {
		return fmt.Errorf("новый интервал пересекается с уже сохраненным сном")
	}
	return nil
}

func (s *Store) updateMemberPresence(ctx context.Context, telegramUserID int64, telegramChatID int64, displayName string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE family_members
		SET telegram_chat_id = ?, display_name = ?, updated_at = ?
		WHERE telegram_user_id = ?
	`, telegramChatID, sanitizeDisplayName(displayName), s.nowUTCString(), telegramUserID)
	return err
}

func (s *Store) validateTimestamp(ts time.Time) error {
	now := s.clock().UTC()
	if ts.After(now.Add(1 * time.Minute)) {
		return fmt.Errorf("время не может быть в будущем")
	}
	if ts.Before(now.Add(-s.cfg.MaxBackdate)) {
		return fmt.Errorf("время слишком старое: доступно не более %s назад", s.cfg.MaxBackdate)
	}
	return nil
}

func (s *Store) generateInviteCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	var builder strings.Builder
	for i := 0; i < 6; i++ {
		builder.WriteByte(alphabet[s.random.Intn(len(alphabet))])
	}
	return builder.String()
}

func (s *Store) nowUTCString() string {
	return toStoredTime(s.clock().UTC())
}

func scanSleepSession(scanner interface{ Scan(dest ...any) error }) (*SleepSession, error) {
	var (
		session    SleepSession
		startAtRaw string
		endAtRaw   sql.NullString
	)
	if err := scanner.Scan(
		&session.ID, &session.ChildID, &startAtRaw, &endAtRaw, &session.StartSource, &session.EndSource,
		&session.Note, &session.CreatedBy, &session.UpdatedBy,
	); err != nil {
		return nil, err
	}

	startAt, err := parseStoredTime(startAtRaw)
	if err != nil {
		return nil, err
	}
	session.StartAt = startAt

	if endAtRaw.Valid && endAtRaw.String != "" {
		endAt, err := parseStoredTime(endAtRaw.String)
		if err != nil {
			return nil, err
		}
		session.EndAt = &endAt
	}

	return &session, nil
}

func collectSleepSessions(rows *sql.Rows) ([]SleepSession, error) {
	var sessions []SleepSession
	for rows.Next() {
		session, err := scanSleepSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *session)
	}
	return sessions, rows.Err()
}

func sanitizeDisplayName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "родитель"
	}
	return name
}

func toStoredTime(ts time.Time) string {
	return ts.UTC().Format(time.RFC3339Nano)
}

func parseStoredTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
