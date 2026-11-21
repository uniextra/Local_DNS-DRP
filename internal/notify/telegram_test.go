package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestTelegramSendRetries(t *testing.T) {
	t.Run("429 then success", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":0.01}}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer srv.Close()

		tg := NewTelegramWithBaseURL("tok", "1", srv.URL, 2*time.Second)
		if err := tg.Send(context.Background(), "hi"); err != nil {
			t.Fatalf("Send() error: %v", err)
		}
		if got := calls.Load(); got != 2 {
			t.Fatalf("expected 2 attempts, got %d", got)
		}
	})

	t.Run("5xx then success", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				http.Error(w, "temporary", http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer srv.Close()

		tg := NewTelegramWithBaseURL("tok", "1", srv.URL, 2*time.Second)
		if err := tg.Send(context.Background(), "hi"); err != nil {
			t.Fatalf("Send() error: %v", err)
		}
		if got := calls.Load(); got != 2 {
			t.Fatalf("expected 2 attempts, got %d", got)
		}
	})

	t.Run("4xx is not retried", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			http.Error(w, "bad request", http.StatusBadRequest)
		}))
		defer srv.Close()

		tg := NewTelegramWithBaseURL("tok", "1", srv.URL, time.Second)
		if err := tg.Send(context.Background(), "hi"); err == nil {
			t.Fatal("expected error")
		}
		if got := calls.Load(); got != 1 {
			t.Fatalf("expected 1 attempt, got %d", got)
		}
	})

	t.Run("context cancellation stops retries", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"parameters":{"retry_after":5}}`))
		}))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		tg := NewTelegramWithBaseURL("tok", "1", srv.URL, time.Second)
		if err := tg.Send(ctx, "hi"); err == nil {
			t.Fatal("expected context error")
		}
		if got := calls.Load(); got != 1 {
			t.Fatalf("expected 1 attempt before cancellation, got %d", got)
		}
	})
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
