package gmailoauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// listHistoryMessageIDs walks users.history.list starting from the given
// historyId and returns the set of message IDs added since. The second
// return value is the cursor to persist for the next pass — Gmail hands
// it back as `historyId` on each page; we keep the highest.
//
// Returns an empty cursor on the special 404 "history cursor expired"
// response so the caller can fall back to the date-bounded query.
func listHistoryMessageIDs(ctx context.Context, client *http.Client, startHistoryID string) ([]string, string, error) {
	var (
		ids       []string
		seen      = map[string]bool{}
		pageToken string
		latest    = startHistoryID
	)
	// Cap iterations defensively — a runaway pagination loop on an account
	// with millions of history records would block the scheduler tick.
	for range 25 {
		u := fmt.Sprintf(
			"https://gmail.googleapis.com/gmail/v1/users/me/history?startHistoryId=%s&historyTypes=messageAdded&labelId=INBOX",
			url.QueryEscape(startHistoryID),
		)
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := client.Do(req)
		if err != nil {
			return ids, latest, fmt.Errorf("history.list: %w", err)
		}
		// Cursor expired — Gmail's docs say to start over with a full sync.
		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			return nil, "", nil
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return ids, latest, fmt.Errorf("history.list: %s", resp.Status)
		}
		var body struct {
			History []struct {
				MessagesAdded []struct {
					Message struct {
						ID string `json:"id"`
					} `json:"message"`
				} `json:"messagesAdded"`
			} `json:"history"`
			HistoryID     string `json:"historyId"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			_ = resp.Body.Close()
			return ids, latest, err
		}
		_ = resp.Body.Close()
		if body.HistoryID != "" {
			latest = body.HistoryID
		}
		for _, h := range body.History {
			for _, m := range h.MessagesAdded {
				if m.Message.ID == "" || seen[m.Message.ID] {
					continue
				}
				seen[m.Message.ID] = true
				ids = append(ids, m.Message.ID)
			}
		}
		if body.NextPageToken == "" {
			break
		}
		pageToken = body.NextPageToken
	}
	return ids, latest, nil
}

// fetchProfileHistoryID returns the mailbox's current historyId. Used to
// seed the cursor after a date-bounded fallback pass.
func fetchProfileHistoryID(ctx context.Context, client *http.Client) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://gmail.googleapis.com/gmail/v1/users/me/profile", nil)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("profile: %s", resp.Status)
	}
	var body struct {
		HistoryID string `json:"historyId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.HistoryID, nil
}
