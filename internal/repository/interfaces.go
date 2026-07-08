package repository

import (
	"context"
	"time"

	"meeting-bot/internal/domain"
)

type UserRepository interface {
	Upsert(ctx context.Context, user domain.User) (domain.User, error)
	GetByTelegramID(ctx context.Context, telegramID int64) (domain.User, error)
	SetRegistrationStep(ctx context.Context, telegramID int64, step string) error
	UpdateDisplayName(ctx context.Context, telegramID int64, displayName string) error
	UpdateTeam(ctx context.Context, telegramID int64, team string) error
	UpdateRole(ctx context.Context, telegramID int64, role string) error
	UpdateTimezone(ctx context.Context, telegramID int64, timezone string) error
}

type MeetingRepository interface {
	Create(ctx context.Context, meeting domain.Meeting) (domain.Meeting, error)
	ListByDay(ctx context.Context, day time.Time) ([]domain.Meeting, error)
	ListByWeek(ctx context.Context, day time.Time) ([]domain.Meeting, error)
	Cancel(ctx context.Context, meetingID int64, requesterID int64) error
	HasConflict(ctx context.Context, participantIDs []int64, startsAt time.Time, endsAt time.Time) (bool, error)
}
