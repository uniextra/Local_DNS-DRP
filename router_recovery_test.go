package main

import (
	"strings"
	"testing"
)

func TestXORDecodeRoundTrip(t *testing.T) {
	const input = "router-password"
	encoded := xorEncode(input)
	if encoded == input { t.Fatal("XOR encoding did not transform the input") }
	if xorEncode(encoded) != input { t.Fatal("XOR encoding is not reversible") }
}

func TestExtractJS(t *testing.T) {
	page := []byte(`<script>var lanHostDns = "1.1.1.1,8.8.8.8";</script>`)
	got, err := extractJS(page, "lanHostDns")
	if err != nil { t.Fatal(err) }
	if got != "1.1.1.1,8.8.8.8" { t.Fatalf("got %q", got) }
}

func TestExtractJSMissingField(t *testing.T) {
	_, err := extractJS([]byte(`var other = "value";`), "lanHostDns")
	if err == nil || !strings.Contains(err.Error(), "lanHostDns") { t.Fatalf("unexpected error: %v", err) }
}

func TestWiFiOffDefaultIsTenSeconds(t *testing.T) {
	t.Setenv("TARGETS", "home=192.168.1.2")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat")
	t.Setenv("ROUTER_PASSWORD", "secret")
	t.Setenv("WIFI_OFF_SECONDS", "")
	cfg, err := loadConfig()
	if err != nil { t.Fatal(err) }
	if cfg.Router.WiFiOff.Seconds() != 10 { t.Fatalf("WiFi off duration = %s, want 10s", cfg.Router.WiFiOff) }
	if !cfg.RecoveryEnabled { t.Fatal("router recovery should be enabled when ROUTER_PASSWORD is set") }
}

func TestRouterRecoveryDisabledWithoutPassword(t *testing.T) {
	t.Setenv("TARGETS", "home=192.168.1.2")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat")
	t.Setenv("ROUTER_PASSWORD", "")
	cfg, err := loadConfig()
	if err != nil { t.Fatal(err) }
	if cfg.RecoveryEnabled { t.Fatal("router recovery must be disabled without ROUTER_PASSWORD") }
}
