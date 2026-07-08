package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"meeting-bot/internal/domain"
	"meeting-bot/internal/repository"
)

var ErrConflict = errors.New("meeting conflicts with existing schedule")
var ErrPastTime = errors.New("meeting start time is in the past")

type CreateMeeting struct {
	repo repository.MeetingRepository
}

func NewCreateMeeting(repo repository.MeetingRepository) CreateMeeting {
	return CreateMeeting{repo: repo}
}

type CreateMeetingInput struct {
	Title          string
	CreatorID      int64
	ParticipantIDs []int64
	StartsAt       time.Time
	EndsAt         time.Time
	Now            time.Time
}

func (uc CreateMeeting) Execute(ctx context.Context, input CreateMeetingInput) (domain.Meeting, error) {
	if strings.TrimSpace(input.Title) == "" {
		return domain.Meeting{}, errors.New("title is required")
	}
	if !input.EndsAt.After(input.StartsAt) {
		return domain.Meeting{}, errors.New("end time must be after start time")
	}
	if !input.Now.IsZero() && !input.StartsAt.After(input.Now) {
		return domain.Meeting{}, ErrPastTime
	}

	participantIDs := append([]int64{input.CreatorID}, input.ParticipantIDs...)
	hasConflict, err := uc.repo.HasConflict(ctx, participantIDs, input.StartsAt, input.EndsAt)
	if err != nil {
		return domain.Meeting{}, err
	}
	if hasConflict {
		return domain.Meeting{}, ErrConflict
	}

	meeting := domain.Meeting{
		Title:          input.Title,
		CreatorID:      input.CreatorID,
		ParticipantIDs: input.ParticipantIDs,
		StartsAt:       input.StartsAt,
		EndsAt:         input.EndsAt,
	}
	return uc.repo.Create(ctx, meeting)
}
