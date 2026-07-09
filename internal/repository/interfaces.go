package repository

import (
	"context"
	"time"

	"meeting-bot/internal/domain"
)

type UserRepository interface {
	Upsert(ctx context.Context, user domain.User) (domain.User, error)
	GetByTelegramID(ctx context.Context, telegramID int64) (domain.User, error)
	ListRegistered(ctx context.Context) ([]domain.User, error)
	SetRegistrationStep(ctx context.Context, telegramID int64, step string) error
	UpdateDisplayName(ctx context.Context, telegramID int64, displayName string) error
	UpdateTeam(ctx context.Context, telegramID int64, team string) error
	UpdateRole(ctx context.Context, telegramID int64, role string) error
	UpdateTimezone(ctx context.Context, telegramID int64, timezone string) error
}

type SettingsRepository interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key string, value string) error
}

type MeetingRepository interface {
	// CreateIfNoConflict atomically checks for a scheduling conflict among
	// participantIDs and inserts the meeting, closing the check-then-insert
	// race between a separate conflict check and insert. The returned bool
	// reports whether a conflict was found (in which case no row is created).
	CreateIfNoConflict(ctx context.Context, meeting domain.Meeting, participantIDs []int64) (domain.Meeting, bool, error)
	UpdateGoogleEvent(ctx context.Context, meetingID int64, googleEventID string, meetLink string) error
	ListUpcomingForUser(ctx context.Context, userID int64, from time.Time, limit int) ([]domain.Meeting, error)
	ListByDateRange(ctx context.Context, from time.Time, to time.Time) ([]domain.Meeting, error)
	Cancel(ctx context.Context, meetingID int64, requesterID int64) (domain.Meeting, error)
	IsReminderSent(ctx context.Context, meetingID int64, threshold string) (bool, error)
	MarkReminderSent(ctx context.Context, meetingID int64, threshold string) error
}
