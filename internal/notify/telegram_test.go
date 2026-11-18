package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTruncateRunes(t *testing.T) {
	if got := TruncateRunes("hello", 10); got != "hello" {
		t.Fatalf("TruncateRunes = %q, want hello", got)
	}
	if got := TruncateRunes("abcdef", 4); got != "abc…" {
		t.Fatalf("TruncateRunes = %q, want abc…", got)
	}
}

func TestTelegramSendAndRedaction(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var gotBody map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer srv.Close()

		tg := NewTelegramWithBaseURL("secret-token", "999", srv.URL, 2*time.Second)
		if err := tg.Send(context.Background(), "hello"); err != nil {
			t.Fatalf("Send() error: %v", err)
		}
		if gotBody["chat_id"] != "999" || gotBody["text"] != "hello" {
			t.Fatalf("unexpected payload: %+v", gotBody)
		}
	})

	t.Run("http error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}))
		defer srv.Close()

		tg := NewTelegramWithBaseURL("tok", "1", srv.URL, time.Second)
		if err := tg.Send(context.Background(), "hi"); err == nil || !strings.Contains(err.Error(), "401") {
			t.Fatalf("expected 401 error, got %v", err)
		}
	})

	t.Run("api ok false", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"ok":false,"description":"chat not found"}`))
		}))
		defer srv.Close()

		tg := NewTelegramWithBaseURL("tok", "1", srv.URL, time.Second)
		if err := tg.Send(context.Background(), "hi"); err == nil || !strings.Contains(err.Error(), "chat not found") {
			t.Fatalf("expected chat not found error, got %v", err)
		}
	})

	t.Run("network error redacts token", func(t *testing.T) {
		const secret = "123456:ABC-DEF-SECRET"
		tg := NewTelegramWithBaseURL(secret, "1", "http://127.0.0.1:1", 200*time.Millisecond)
		err := tg.Send(context.Background(), "hi")
		if err == nil {
			t.Fatal("expected network error, got nil")
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked bot token: %v", err)
		}
	})
}
