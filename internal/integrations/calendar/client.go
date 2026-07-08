package calendar

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"meeting-bot/internal/config"
	"meeting-bot/internal/usecase"
)

type Client struct {
	calendarID  string
	tokenSource oauth2.TokenSource
	enabled     bool
}

func New(cfg config.Config) *Client {
	if cfg.GoogleOAuthClientID == "" || cfg.GoogleOAuthClientSecret == "" || cfg.GoogleOAuthRefreshToken == "" || cfg.GoogleCalendarID == "" {
		return &Client{}
	}

	oauthConfig := &oauth2.Config{
		ClientID:     cfg.GoogleOAuthClientID,
		ClientSecret: cfg.GoogleOAuthClientSecret,
		Endpoint:     google.Endpoint,
	}

	return &Client{
		calendarID:  cfg.GoogleCalendarID,
		tokenSource: oauthConfig.TokenSource(context.Background(), &oauth2.Token{RefreshToken: cfg.GoogleOAuthRefreshToken}),
		enabled:     true,
	}
}

func (c *Client) Enabled() bool {
	return c.enabled
}

func (c *Client) service(ctx context.Context) (*gcal.Service, error) {
	return gcal.NewService(ctx, option.WithTokenSource(c.tokenSource))
}

func (c *Client) CreateEvent(ctx context.Context, input usecase.CalendarEventInput) (usecase.CalendarEvent, error) {
	if !c.enabled {
		return usecase.CalendarEvent{}, nil
	}

	srv, err := c.service(ctx)
	if err != nil {
		return usecase.CalendarEvent{}, err
	}

	event := &gcal.Event{
		Summary: input.Title,
		Start:   &gcal.EventDateTime{DateTime: input.StartsAt.Format(time.RFC3339)},
		End:     &gcal.EventDateTime{DateTime: input.EndsAt.Format(time.RFC3339)},
		ConferenceData: &gcal.ConferenceData{
			CreateRequest: &gcal.CreateConferenceRequest{
				RequestId:             fmt.Sprintf("meeting-bot-%d", time.Now().UnixNano()),
				ConferenceSolutionKey: &gcal.ConferenceSolutionKey{Type: "hangoutsMeet"},
			},
		},
	}

	created, err := srv.Events.Insert(c.calendarID, event).ConferenceDataVersion(1).Do()
	if err != nil {
		return usecase.CalendarEvent{}, err
	}

	return usecase.CalendarEvent{EventID: created.Id, MeetLink: created.HangoutLink}, nil
}

func (c *Client) DeleteEvent(ctx context.Context, eventID string) error {
	if !c.enabled || eventID == "" {
		return nil
	}

	srv, err := c.service(ctx)
	if err != nil {
		return err
	}

	return srv.Events.Delete(c.calendarID, eventID).Do()
}
