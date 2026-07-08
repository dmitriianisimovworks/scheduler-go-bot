package repository

import (
	"context"
	"time"

	"meeting-bot/internal/domain"
)

type UserRepository interface {
	Upsert(ctx context.Context, user domain.User) (domain.User, error)
}

type MeetingRepository interface {
	Create(ctx context.Context, meeting domain.Meeting) (domain.Meeting, error)
	ListByDay(ctx context.Context, day time.Time) ([]domain.Meeting, error)
	ListByWeek(ctx context.Context, day time.Time) ([]domain.Meeting, error)
	Cancel(ctx context.Context, meetingID int64, requesterID int64) error
	HasConflict(ctx context.Context, participantIDs []int64, startsAt time.Time, endsAt time.Time) (bool, error)
}
