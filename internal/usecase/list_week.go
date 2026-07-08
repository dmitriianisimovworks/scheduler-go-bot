package usecase

import (
	"context"
	"time"

	"meeting-bot/internal/domain"
	"meeting-bot/internal/repository"
)

type ListWeek struct {
	repo repository.MeetingRepository
}

func NewListWeek(repo repository.MeetingRepository) ListWeek {
	return ListWeek{repo: repo}
}

func (uc ListWeek) Execute(ctx context.Context, day time.Time) ([]domain.Meeting, error) {
	return uc.repo.ListByWeek(ctx, day)
}
