package usecase

import (
	"context"
	"time"

	"meeting-bot/internal/domain"
	"meeting-bot/internal/repository"
)

type ListUpcoming struct {
	repo repository.MeetingRepository
}

func NewListUpcoming(repo repository.MeetingRepository) ListUpcoming {
	return ListUpcoming{repo: repo}
}

func (uc ListUpcoming) Execute(ctx context.Context, creatorID int64, from time.Time) ([]domain.Meeting, error) {
	return uc.repo.ListUpcomingByCreator(ctx, creatorID, from, 20)
}
