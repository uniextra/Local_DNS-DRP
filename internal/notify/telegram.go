// Package notify implements notification clients for monitor alerts.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.telegram.org"

	// MaxMessageRunes keeps messages safely below the 4096-character limit
	// of the Telegram Bot API, which rejects longer texts outright.
	MaxMessageRunes = 4000
)

// TruncateRunes shortens s to at most max runes, marking the cut with an ellipsis.
func TruncateRunes(s string, max int) string {
	runes := []rune(s)
	if max < 1 || len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// Telegram sends alerts to a Telegram chat via the Bot API.
type Telegram struct {
	token   string
	chatID  string
	baseURL string
	client  *http.Client
}

// NewTelegram creates a Telegram client with the given credentials and HTTP timeout.
func NewTelegram(token, chatID string, timeout time.Duration) *Telegram {
	return NewTelegramWithBaseURL(token, chatID, defaultBaseURL, timeout)
}

// NewTelegramWithBaseURL creates a Telegram client targeting a custom API base URL.
func NewTelegramWithBaseURL(token, chatID, baseURL string, timeout time.Duration) *Telegram {
	return &Telegram{
		token:   token,
		chatID:  chatID,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

// redact removes the bot token from an error. net/http wraps failures in a
// *url.Error whose text embeds the full request URL, and the token is part of
// that URL, so logging the raw error would leak the credential.
func (t *Telegram) redact(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	text := err.Error()
	if t.token != "" {
		text = strings.ReplaceAll(text, t.token, "[REDACTED]")
	}
	return errors.New("Telegram request failed: " + text)
}

// Send delivers a message to the configured Telegram chat.
func (t *Telegram) Send(ctx context.Context, message string) error {
	body, err := json.Marshal(map[string]any{
		"chat_id":                  t.chatID,
		"text":                     TruncateRunes(message, MaxMessageRunes),
		"disable_web_page_preview": true,
	})
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", t.baseURL, t.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return t.redact(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return t.redact(err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("read Telegram response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Telegram returned HTTP %s: %s", resp.Status, strings.TrimSpace(string(respBytes)))
	}

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return fmt.Errorf("decode Telegram response: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("Telegram API: %s", result.Description)
	}
	return nil
}
