package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Odoo sends analytics to the kittens_report Odoo module instead of keeping
// them here. Each event is one POST to a webhook route of the module, which
// checks the token and creates the record. Nothing is stored on this side, so
// an event sent while Odoo is down is lost after a few tries.
type Odoo struct {
	baseURL string
	token   string
	client  *http.Client
	// retryWait is how long to wait before each new try; tests shorten it.
	retryWait []time.Duration
}

// NewOdoo talks to the Odoo server at baseURL (https://odoo.example.com). The
// token is sent as "Authorization: Bearer <token>" and must match the token
// set in Odoo under Settings > Board Games.
func NewOdoo(baseURL, token string) *Odoo {
	return &Odoo{
		baseURL:   strings.TrimRight(baseURL, "/"),
		token:     token,
		client:    &http.Client{Timeout: 10 * time.Second},
		retryWait: []time.Duration{time.Second, 5 * time.Second},
	}
}

// errNoRetry marks an answer that will not get better by asking again, such as
// a wrong token.
var errNoRetry = errors.New("odoo refused the request")

// post sends body as JSON to path and decodes the answer into out (if not nil).
func (o *Odoo) post(ctx context.Context, path string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.token)

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	answer, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode >= 400 {
		err := fmt.Errorf("odoo %s: %s: %s", path, resp.Status, bytes.TrimSpace(answer))
		if resp.StatusCode < 500 {
			return fmt.Errorf("%w: %w", errNoRetry, err)
		}
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(answer, out)
}

// postWithRetry is post for events nobody is waiting on: a restarting Odoo
// gets a few more chances before the event is given up.
func (o *Odoo) postWithRetry(ctx context.Context, path string, body any) error {
	err := o.post(ctx, path, body, nil)
	for _, wait := range o.retryWait {
		if err == nil || errors.Is(err, errNoRetry) {
			return err
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return err
		}
		err = o.post(ctx, path, body, nil)
	}
	return err
}

// RoomCreated sends a new room to Odoo. It makes Odoo a Sink for the Recorder.
func (o *Odoo) RoomCreated(ctx context.Context, ev RoomEvent) error {
	return o.postWithRetry(ctx, "/kittens/webhook/room", ev)
}

// PlayerJoined sends a join to Odoo. Odoo ignores a reconnect into the same
// seat.
func (o *Odoo) PlayerJoined(ctx context.Context, ev JoinEvent) error {
	return o.postWithRetry(ctx, "/kittens/webhook/player", ev)
}

// AddSuggestion sends a suggestion that has already been through Clean and
// returns the id Odoo gave it. It is tried only once: the player is waiting.
func (o *Odoo) AddSuggestion(ctx context.Context, in SuggestionInput, c Country) (int64, error) {
	body := struct {
		SuggestionInput
		CountryCode string `json:"country_code"`
		CountryName string `json:"country_name"`
	}{in, c.Code, c.Name}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := o.post(ctx, "/kittens/webhook/suggestion", body, &out); err != nil {
		return 0, fmt.Errorf("analytics: saving suggestion: %w", err)
	}
	return out.ID, nil
}
