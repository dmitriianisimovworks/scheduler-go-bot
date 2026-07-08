package usecase

import (
	"context"
	"time"

	"meeting-bot/internal/domain"
	"meeting-bot/internal/repository"
)

type ListMyMeetings struct {
	repo repository.MeetingRepository
}

func NewListMyMeetings(repo repository.MeetingRepository) ListMyMeetings {
	return ListMyMeetings{repo: repo}
}

func (uc ListMyMeetings) Execute(ctx context.Context, userID int64, from time.Time) ([]domain.Meeting, error) {
	return uc.repo.ListUpcomingForUser(ctx, userID, from, 20)
}
