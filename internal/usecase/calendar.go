package usecase

import (
	"context"
	"time"
)

type CalendarClient interface {
	Enabled() bool
	CreateEvent(ctx context.Context, input CalendarEventInput) (CalendarEvent, error)
	DeleteEvent(ctx context.Context, eventID string) error
}

type CalendarEventInput struct {
	Title    string
	StartsAt time.Time
	EndsAt   time.Time
}

type CalendarEvent struct {
	EventID  string
	MeetLink string
}
