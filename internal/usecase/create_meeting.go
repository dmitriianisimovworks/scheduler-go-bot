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
	repo     repository.MeetingRepository
	calendar CalendarClient
}

func NewCreateMeeting(repo repository.MeetingRepository, calendar CalendarClient) CreateMeeting {
	return CreateMeeting{repo: repo, calendar: calendar}
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
	meeting := domain.Meeting{
		Title:          input.Title,
		CreatorID:      input.CreatorID,
		ParticipantIDs: input.ParticipantIDs,
		StartsAt:       input.StartsAt,
		EndsAt:         input.EndsAt,
	}

	created, conflict, err := uc.repo.CreateIfNoConflict(ctx, meeting, participantIDs)
	if err != nil {
		return domain.Meeting{}, err
	}
	if conflict {
		return domain.Meeting{}, ErrConflict
	}

	// Calendar sync happens after the row exists, and its failure is
	// non-fatal: the meeting is already the source of truth in the DB, so a
	// Google API hiccup here just means it's missing a Meet link, not an
	// orphaned Calendar event with no corresponding meeting.
	if uc.calendar != nil && uc.calendar.Enabled() {
		event, err := uc.calendar.CreateEvent(ctx, CalendarEventInput{
			Title:    input.Title,
			StartsAt: input.StartsAt,
			EndsAt:   input.EndsAt,
		})
		if err == nil {
			if updErr := uc.repo.UpdateGoogleEvent(ctx, created.ID, event.EventID, event.MeetLink); updErr == nil {
				created.GoogleEventID = event.EventID
				created.MeetLink = event.MeetLink
			}
		}
	}

	return created, nil
}
