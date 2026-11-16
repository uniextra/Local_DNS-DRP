package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newTestTelegram(token string, rt roundTripFunc) *Telegram {
	return &Telegram{token: token, chatID: "chat", client: &http.Client{Transport: rt}}
}

func okTelegramResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}
}

// net/http wraps transport errors in a *url.Error whose text embeds the full
// request URL, and the Telegram bot token is part of that URL. The error is
// logged by notify, so it must never carry the token.
func TestTelegramSendErrorsDoNotLeakBotToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		rt    roundTripFunc
	}{
		{
			name:  "transport error wrapped in url.Error",
			token: "123456:SECRET-token_value",
			rt: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("dial tcp: i/o timeout")
			},
		},
		{
			name:  "transport error that echoes the request URL",
			token: "123456:SECRET-token_value",
			rt: func(r *http.Request) (*http.Response, error) {
				return nil, errors.New("failed to reach " + r.URL.String())
			},
		},
		{
			name:  "request URL that fails to parse",
			token: "12%zzSECRET",
			rt: func(*http.Request) (*http.Response, error) {
				return okTelegramResponse(), nil
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := newTestTelegram(tc.token, tc.rt).send("hola")
			if err == nil {
				t.Fatal("send returned nil error")
			}
			if strings.Contains(err.Error(), tc.token) || strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("error leaks the bot token: %v", err)
			}
		})
	}
}

func TestTelegramSendKeepsUsefulErrorContext(t *testing.T) {
	tg := newTestTelegram("123456:token", func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: i/o timeout")
	})
	err := tg.send("hola")
	if err == nil || !strings.Contains(err.Error(), "i/o timeout") {
		t.Fatalf("error = %v, want the underlying cause to be preserved", err)
	}
}

func TestTelegramSendTruncatesLongMessages(t *testing.T) {
	const telegramLimit = 4096
	var got struct {
		Text string `json:"text"`
	}
	tg := newTestTelegram("token", func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			return nil, err
		}
		return okTelegramResponse(), nil
	})

	if err := tg.send(strings.Repeat("ñ🔴", 5000)); err != nil {
		t.Fatalf("send: %v", err)
	}
	if n := utf8.RuneCountInString(got.Text); n > telegramLimit {
		t.Fatalf("message has %d characters, Telegram rejects anything above %d", n, telegramLimit)
	}
	if !utf8.ValidString(got.Text) {
		t.Fatal("truncation produced invalid UTF-8")
	}
	if !strings.HasSuffix(got.Text, "…") {
		t.Fatal("truncated message is not marked with an ellipsis")
	}

	if err := tg.send("hola"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got.Text != "hola" {
		t.Fatalf("short message was altered: %q", got.Text)
	}
}
