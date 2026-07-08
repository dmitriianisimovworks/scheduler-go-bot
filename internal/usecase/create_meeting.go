package usecase

import "meeting-bot/internal/repository"

type CreateMeeting struct {
	repo repository.MeetingRepository
}

func NewCreateMeeting(repo repository.MeetingRepository) CreateMeeting {
	return CreateMeeting{repo: repo}
}
