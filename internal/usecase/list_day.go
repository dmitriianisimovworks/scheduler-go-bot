package usecase

import "meeting-bot/internal/repository"

type ListDay struct {
	repo repository.MeetingRepository
}

func NewListDay(repo repository.MeetingRepository) ListDay {
	return ListDay{repo: repo}
}
