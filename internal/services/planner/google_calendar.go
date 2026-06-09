// Package planner — Google Calendar push/pull. The OAuth token comes from
// gmailoauth; the same account that holds the Gmail refresh token also
// holds calendar.events scope after re-consent (see gmailoauth.Scopes).
package planner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/mandloideep/miniclaw/internal/services/gmailoauth"
)

// gcalEvent is the subset of the v3 Event resource we read/write. Times
// are RFC3339 with timezone offsets so Google doesn't reinterpret them.
type gcalEvent struct {
	ID          string         `json:"id,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Description string         `json:"description,omitempty"`
	Start       gcalEventTime  `json:"start"`
	End         gcalEventTime  `json:"end"`
	Status      string         `json:"status,omitempty"`
	Extended    *gcalExtended  `json:"extendedProperties,omitempty"`
	Reminders   *gcalReminders `json:"reminders,omitempty"`
	Created     string         `json:"created,omitempty"`
	HTMLLink    string         `json:"htmlLink,omitempty"`
	Organizer   *gcalAttendee  `json:"organizer,omitempty"`
	Attendees   []gcalAttendee `json:"attendees,omitempty"`
}

type gcalEventTime struct {
	DateTime string `json:"dateTime,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

type gcalExtended struct {
	Private map[string]string `json:"private,omitempty"`
}

type gcalReminders struct {
	UseDefault bool `json:"useDefault"`
}

type gcalAttendee struct {
	Email string `json:"email,omitempty"`
	Self  bool   `json:"self,omitempty"`
}

// insertGoogleEvent POSTs to events.insert on the primary calendar and
// returns the resource the API echoes back.
func insertGoogleEvent(ctx context.Context, client *http.Client, b Block) (gcalEvent, error) {
	body := gcalEvent{
		Summary:     b.Title,
		Description: b.Notes,
		Start:       gcalEventTime{DateTime: b.StartAt, TimeZone: "UTC"},
		End:         gcalEventTime{DateTime: b.EndAt, TimeZone: "UTC"},
		Extended: &gcalExtended{Private: map[string]string{
			"miniclawBlockId": fmt.Sprintf("%d", b.ID),
		}},
		Reminders: &gcalReminders{UseDefault: true},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return gcalEvent{}, fmt.Errorf("marshal event: %w", err)
	}
	req, _ := http.NewRequestWithContext(
		ctx, http.MethodPost,
		"https://www.googleapis.com/calendar/v3/calendars/primary/events",
		bytes.NewReader(raw),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return gcalEvent{}, fmt.Errorf("calendar insert: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return gcalEvent{}, fmt.Errorf("calendar insert: %s", resp.Status)
	}
	var out gcalEvent
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return gcalEvent{}, fmt.Errorf("decode event: %w", err)
	}
	return out, nil
}

// deleteGoogleEvent removes a previously-promoted event. 404s are treated
// as success — the caller's local row is what owns the truth.
func deleteGoogleEvent(ctx context.Context, client *http.Client, eventID string) error {
	u := "https://www.googleapis.com/calendar/v3/calendars/primary/events/" + url.PathEscape(eventID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("calendar delete: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("calendar delete: %s", resp.Status)
	}
	return nil
}

// listUpcomingGoogleEvents pulls events.list bounded by timeMin/timeMax.
// Pagination caps at one extra page; for a personal calendar that's
// thousands of events without paging burn.
func listUpcomingGoogleEvents(ctx context.Context, client *http.Client, lookbackDays, lookaheadDays int) ([]gcalEvent, error) {
	now := time.Now().UTC()
	timeMin := now.AddDate(0, 0, -lookbackDays).Format(time.RFC3339)
	timeMax := now.AddDate(0, 0, lookaheadDays).Format(time.RFC3339)

	var (
		out       []gcalEvent
		pageToken string
	)
	for range 3 {
		params := url.Values{}
		params.Set("singleEvents", "true")
		params.Set("orderBy", "startTime")
		params.Set("timeMin", timeMin)
		params.Set("timeMax", timeMax)
		params.Set("maxResults", "250")
		if pageToken != "" {
			params.Set("pageToken", pageToken)
		}
		u := "https://www.googleapis.com/calendar/v3/calendars/primary/events?" + params.Encode()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("calendar list: %w", err)
		}
		if resp.StatusCode/100 != 2 {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("calendar list: %s", resp.Status)
		}
		var body struct {
			Items         []gcalEvent `json:"items"`
			NextPageToken string      `json:"nextPageToken"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("decode list: %w", err)
		}
		_ = resp.Body.Close()
		out = append(out, body.Items...)
		if body.NextPageToken == "" {
			break
		}
		pageToken = body.NextPageToken
	}
	return out, nil
}

// authedCalendarClient builds the HTTP client for a Gmail-OAuth account's
// email address. Lives here so the planner doesn't need to know how the
// gmailoauth helper stitches refresh tokens together.
func authedCalendarClient(ctx context.Context, email string) (*http.Client, error) {
	src, err := gmailoauth.TokenSource(ctx, "gmail_oauth:"+email)
	if err != nil {
		return nil, fmt.Errorf("calendar token source: %w", err)
	}
	return gmailoauth.AuthedHTTPClient(ctx, src), nil
}
