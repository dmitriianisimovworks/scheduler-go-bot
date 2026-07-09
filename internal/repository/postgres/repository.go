package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"meeting-bot/internal/config"
	"meeting-bot/internal/domain"
)

type Repository struct {
	config config.Config
	db     *sql.DB
}

func New(ctx context.Context, cfg config.Config) (*Repository, error) {
	db, err := sql.Open("pgx", dsn(cfg))
	if err != nil {
		return nil, err
	}

	repo := &Repository{config: cfg, db: db}
	if err := repo.waitForDB(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := repo.ensureSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return repo, nil
}

func dsn(cfg config.Config) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.PostgresUser,
		cfg.PostgresPassword,
		cfg.PostgresHost,
		cfg.PostgresPort,
		cfg.PostgresDB,
	)
}

func (r *Repository) waitForDB(ctx context.Context) error {
	var lastErr error
	for range 20 {
		if err := r.db.PingContext(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}

	return fmt.Errorf("postgres is not ready: %w", lastErr)
}

func (r *Repository) ensureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id BIGSERIAL PRIMARY KEY,
			telegram_id BIGINT NOT NULL UNIQUE,
			username TEXT NOT NULL DEFAULT '',
			first_name TEXT NOT NULL DEFAULT '',
			last_name TEXT NOT NULL DEFAULT '',
			full_name TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			team TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT '',
			timezone TEXT NOT NULL DEFAULT 'Europe/Moscow',
			registration_step TEXT NOT NULL DEFAULT 'confirm_name',
			registered_at TIMESTAMPTZ,
			last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS first_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS last_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS team TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'Europe/Moscow'`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS registration_step TEXT NOT NULL DEFAULT 'confirm_name'`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS registered_at TIMESTAMPTZ`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
		`CREATE TABLE IF NOT EXISTS meetings (
			id BIGSERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			creator_id BIGINT NOT NULL REFERENCES users(id),
			starts_at TIMESTAMPTZ NOT NULL,
			ends_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CHECK (ends_at > starts_at)
		)`,
		`CREATE TABLE IF NOT EXISTS meeting_participants (
			meeting_id BIGINT NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			PRIMARY KEY (meeting_id, user_id)
		)`,
		`ALTER TABLE meetings ADD COLUMN IF NOT EXISTS google_event_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE meetings ADD COLUMN IF NOT EXISTS meet_link TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS meeting_reminders (
			meeting_id BIGINT NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
			threshold TEXT NOT NULL,
			sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (meeting_id, threshold)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_meetings_starts_at ON meetings (starts_at)`,
		`CREATE INDEX IF NOT EXISTS idx_meetings_ends_at ON meetings (ends_at)`,
		`CREATE INDEX IF NOT EXISTS idx_meeting_participants_user_id ON meeting_participants (user_id)`,
	}

	for _, statement := range statements {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}

	return nil
}

func (r *Repository) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

func (r *Repository) SetSetting(ctx context.Context, key string, value string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
	`, key, value)
	return err
}

func (r *Repository) Upsert(ctx context.Context, user domain.User) (domain.User, error) {
	fullName := strings.TrimSpace(strings.TrimSpace(user.FirstName) + " " + strings.TrimSpace(user.LastName))
	if fullName == "" {
		fullName = user.Username
	}
	displayName := strings.TrimSpace(user.DisplayName)
	if displayName == "" {
		displayName = fullName
	}
	timezone := strings.TrimSpace(user.Timezone)
	if timezone == "" {
		timezone = r.config.AppTimezone
	}

	row := r.db.QueryRowContext(ctx, `
		INSERT INTO users (
			telegram_id,
			username,
			first_name,
			last_name,
			full_name,
			display_name,
			timezone,
			registration_step,
			last_seen_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		ON CONFLICT (telegram_id) DO UPDATE SET
			username = EXCLUDED.username,
			first_name = EXCLUDED.first_name,
			last_name = EXCLUDED.last_name,
			full_name = EXCLUDED.full_name,
			last_seen_at = NOW(),
			updated_at = NOW()
		RETURNING id, telegram_id, username, first_name, last_name, full_name, display_name, team, role, timezone, registration_step, registered_at, last_seen_at
	`,
		user.TelegramID,
		user.Username,
		user.FirstName,
		user.LastName,
		fullName,
		displayName,
		timezone,
		domain.RegistrationStepConfirmName,
	)

	return scanUser(row)
}

func (r *Repository) GetByTelegramID(ctx context.Context, telegramID int64) (domain.User, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, telegram_id, username, first_name, last_name, full_name, display_name, team, role, timezone, registration_step, registered_at, last_seen_at
		FROM users
		WHERE telegram_id = $1
	`, telegramID)

	return scanUser(row)
}

func (r *Repository) ListRegistered(ctx context.Context) ([]domain.User, error) {
	// registered_at (not registration_step) is the durable "onboarding
	// finished" flag — registration_step doubles as transient state for
	// in-progress profile edits, so filtering on it here would drop a user
	// who abandoned an edit mid-flow.
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, telegram_id, username, first_name, last_name, full_name, display_name, team, role, timezone, registration_step, registered_at, last_seen_at
		FROM users
		WHERE registered_at IS NOT NULL
		ORDER BY display_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (r *Repository) SetRegistrationStep(ctx context.Context, telegramID int64, step string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET registration_step = $2,
			registered_at = CASE WHEN $2 = 'complete' AND registered_at IS NULL THEN NOW() ELSE registered_at END,
			updated_at = NOW()
		WHERE telegram_id = $1
	`, telegramID, step)
	return err
}

func (r *Repository) UpdateDisplayName(ctx context.Context, telegramID int64, displayName string) error {
	return r.updateUserField(ctx, telegramID, "display_name", displayName)
}

func (r *Repository) UpdateTeam(ctx context.Context, telegramID int64, team string) error {
	return r.updateUserField(ctx, telegramID, "team", team)
}

func (r *Repository) UpdateRole(ctx context.Context, telegramID int64, role string) error {
	return r.updateUserField(ctx, telegramID, "role", role)
}

func (r *Repository) UpdateTimezone(ctx context.Context, telegramID int64, timezone string) error {
	return r.updateUserField(ctx, telegramID, "timezone", timezone)
}

func (r *Repository) updateUserField(ctx context.Context, telegramID int64, field string, value string) error {
	_, err := r.db.ExecContext(ctx, fmt.Sprintf(`
		UPDATE users
		SET %s = $2, updated_at = NOW()
		WHERE telegram_id = $1
	`, field), telegramID, strings.TrimSpace(value))
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (domain.User, error) {
	var user domain.User
	var registeredAt sql.NullTime
	err := row.Scan(
		&user.ID,
		&user.TelegramID,
		&user.Username,
		&user.FirstName,
		&user.LastName,
		&user.FullName,
		&user.DisplayName,
		&user.Team,
		&user.Role,
		&user.Timezone,
		&user.RegistrationStep,
		&registeredAt,
		&user.LastSeenAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, err
	}
	if err != nil {
		return domain.User{}, err
	}
	if registeredAt.Valid {
		user.RegisteredAt = registeredAt.Time
	}

	return user, nil
}

// CreateIfNoConflict runs the conflict check and the insert inside one
// transaction, taking a Postgres advisory lock per involved user first. This
// closes the race where two concurrent creates for overlapping participants
// could both pass a standalone conflict check before either had inserted its
// row. Locks are acquired in sorted order to avoid deadlocking with another
// concurrent transaction locking the same set of users.
func (r *Repository) CreateIfNoConflict(ctx context.Context, meeting domain.Meeting, participantIDs []int64) (domain.Meeting, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Meeting{}, false, err
	}
	defer tx.Rollback()

	lockIDs := append([]int64{}, participantIDs...)
	sort.Slice(lockIDs, func(i, j int) bool { return lockIDs[i] < lockIDs[j] })
	for _, id := range lockIDs {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, id); err != nil {
			return domain.Meeting{}, false, err
		}
	}

	var conflict bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM meetings m
			WHERE m.starts_at < $2 AND m.ends_at > $1
			AND (
				m.creator_id = ANY($3)
				OR EXISTS (
					SELECT 1 FROM meeting_participants mp
					WHERE mp.meeting_id = m.id AND mp.user_id = ANY($3)
				)
			)
		)
	`, meeting.StartsAt, meeting.EndsAt, participantIDs).Scan(&conflict)
	if err != nil {
		return domain.Meeting{}, false, err
	}
	if conflict {
		return domain.Meeting{}, true, nil
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO meetings (title, creator_id, starts_at, ends_at, google_event_id, meet_link)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, meeting.Title, meeting.CreatorID, meeting.StartsAt, meeting.EndsAt, meeting.GoogleEventID, meeting.MeetLink).Scan(&meeting.ID)
	if err != nil {
		return domain.Meeting{}, false, err
	}

	for _, participantID := range meeting.ParticipantIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO meeting_participants (meeting_id, user_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, meeting.ID, participantID); err != nil {
			return domain.Meeting{}, false, err
		}
	}

	if err := tx.Commit(); err != nil {
		return domain.Meeting{}, false, err
	}

	return meeting, false, nil
}

// UpdateGoogleEvent persists a Google Calendar event id/Meet link onto an
// already-created meeting. Called after CreateIfNoConflict so a failure to
// reach Google never leaves an orphaned Calendar event un-tracked by the
// bot — the meeting row is the source of truth and always exists first.
func (r *Repository) UpdateGoogleEvent(ctx context.Context, meetingID int64, googleEventID string, meetLink string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE meetings SET google_event_id = $2, meet_link = $3 WHERE id = $1
	`, meetingID, googleEventID, meetLink)
	return err
}

func (r *Repository) ListUpcomingForUser(ctx context.Context, userID int64, from time.Time, limit int) ([]domain.Meeting, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, creator_id, starts_at, ends_at, google_event_id, meet_link
		FROM meetings
		WHERE starts_at >= $1
		AND (
			creator_id = $2
			OR EXISTS (
				SELECT 1 FROM meeting_participants mp
				WHERE mp.meeting_id = meetings.id AND mp.user_id = $2
			)
		)
		ORDER BY starts_at
		LIMIT $3
	`, from, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanMeetings(ctx, rows)
}

func (r *Repository) ListByDateRange(ctx context.Context, from time.Time, to time.Time) ([]domain.Meeting, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, creator_id, starts_at, ends_at, google_event_id, meet_link
		FROM meetings
		WHERE starts_at >= $1 AND starts_at < $2
		ORDER BY starts_at
	`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanMeetings(ctx, rows)
}

func (r *Repository) scanMeetings(ctx context.Context, rows *sql.Rows) ([]domain.Meeting, error) {
	var meetings []domain.Meeting
	for rows.Next() {
		var meeting domain.Meeting
		if err := rows.Scan(&meeting.ID, &meeting.Title, &meeting.CreatorID, &meeting.StartsAt, &meeting.EndsAt, &meeting.GoogleEventID, &meeting.MeetLink); err != nil {
			return nil, err
		}
		meetings = append(meetings, meeting)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range meetings {
		participantIDs, err := r.participantIDs(ctx, meetings[i].ID)
		if err != nil {
			return nil, err
		}
		meetings[i].ParticipantIDs = participantIDs
	}

	return meetings, nil
}

func (r *Repository) participantIDs(ctx context.Context, meetingID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT user_id FROM meeting_participants WHERE meeting_id = $1`, meetingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) Cancel(ctx context.Context, meetingID int64, requesterID int64) (domain.Meeting, error) {
	var meeting domain.Meeting
	err := r.db.QueryRowContext(ctx, `
		DELETE FROM meetings WHERE id = $1 AND creator_id = $2
		RETURNING id, title, creator_id, starts_at, ends_at, google_event_id, meet_link
	`, meetingID, requesterID).Scan(&meeting.ID, &meeting.Title, &meeting.CreatorID, &meeting.StartsAt, &meeting.EndsAt, &meeting.GoogleEventID, &meeting.MeetLink)
	if err != nil {
		return domain.Meeting{}, err
	}
	return meeting, nil
}

func (r *Repository) IsReminderSent(ctx context.Context, meetingID int64, threshold string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM meeting_reminders WHERE meeting_id = $1 AND threshold = $2)
	`, meetingID, threshold).Scan(&exists)
	return exists, err
}

func (r *Repository) MarkReminderSent(ctx context.Context, meetingID int64, threshold string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO meeting_reminders (meeting_id, threshold) VALUES ($1, $2)
		ON CONFLICT (meeting_id, threshold) DO NOTHING
	`, meetingID, threshold)
	return err
}
