package main

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestMakeDNSQuery(t *testing.T) {
	packet, err := makeDNSQuery("example.com", 0x1234)
	if err != nil { t.Fatalf("makeDNSQuery returned error: %v", err) }
	if got := binary.BigEndian.Uint16(packet[:2]); got != 0x1234 { t.Fatalf("transaction ID = %#x, want %#x", got, 0x1234) }
	if got := binary.BigEndian.Uint16(packet[2:4]); got&0x0100 == 0 { t.Fatal("recursion desired flag is not set") }
	if got := binary.BigEndian.Uint16(packet[4:6]); got != 1 { t.Fatalf("question count = %d, want 1", got) }
	if !strings.HasSuffix(string(packet), "\x00\x00\x01\x00\x01") { t.Fatal("query does not end with root terminator, A type and IN class") }
}

func TestMakeDNSQueryRejectsInvalidLabels(t *testing.T) {
	for _, domain := range []string{"", "example..com", strings.Repeat("a", 64) + ".com"} {
		if _, err := makeDNSQuery(domain, 1); err == nil { t.Errorf("makeDNSQuery(%q) expected error", domain) }
	}
}

func TestLoadConfigParsesIPv4AndIPv6Targets(t *testing.T) {
	t.Setenv("TARGETS", "home=192.168.1.2,lab=192.168.1.10:5353,office=[2001:db8::53]:53")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat")
	t.Setenv("CHECK_INTERVAL", "60")
	t.Setenv("DNS_TIMEOUT", "3")
	t.Setenv("TELEGRAM_TIMEOUT", "10")
	t.Setenv("DNS_TEST_DOMAIN", "example.com")
	cfg, err := loadConfig()
	if err != nil { t.Fatalf("loadConfig returned error: %v", err) }
	if len(cfg.Targets) != 3 { t.Fatalf("got %d targets, want 3", len(cfg.Targets)) }
	if cfg.Targets[0].Addr != "192.168.1.2:53" { t.Errorf("first target = %q", cfg.Targets[0].Addr) }
	if cfg.Targets[1].Addr != "192.168.1.10:5353" { t.Errorf("second target = %q", cfg.Targets[1].Addr) }
	if cfg.Targets[2].Addr != "[2001:db8::53]:53" { t.Errorf("third target = %q", cfg.Targets[2].Addr) }
}

func TestLoadConfigRejectsHostnameAndDuplicateEndpoint(t *testing.T) {
	base := map[string]string{
		"TELEGRAM_BOT_TOKEN": "token", "TELEGRAM_CHAT_ID": "chat",
		"CHECK_INTERVAL": "60", "DNS_TIMEOUT": "3", "TELEGRAM_TIMEOUT": "10",
		"DNS_TEST_DOMAIN": "example.com",
	}
	for _, tc := range []struct { name, targets, want string }{
		{"hostname", "home=adguard.local", "IP addresses"},
		{"duplicate", "one=192.168.1.2,two=192.168.1.2", "duplicate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range base { t.Setenv(key, value) }
			t.Setenv("TARGETS", tc.targets)
			_, err := loadConfig()
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
				t.Fatalf("loadConfig error = %v, want text %q", err, tc.want)
			}
		})
	}
}

func TestLoadConfigRequiresTelegramCredentials(t *testing.T) {
	t.Setenv("TARGETS", "home=192.168.1.2")
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_CHAT_ID", "")
	_, err := loadConfig()
	if err == nil || !strings.Contains(err.Error(), "TELEGRAM_BOT_TOKEN") { t.Fatalf("unexpected error: %v", err) }
}
