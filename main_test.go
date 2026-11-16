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


func buildAResponse(query []byte, questionOverride []byte, answerOwner []byte) []byte {
	question := query[12:]
	if questionOverride != nil {
		question = questionOverride
	}
	response := make([]byte, 12)
	binary.BigEndian.PutUint16(response[0:2], binary.BigEndian.Uint16(query[0:2]))
	binary.BigEndian.PutUint16(response[2:4], 0x8180)
	binary.BigEndian.PutUint16(response[4:6], 1)
	binary.BigEndian.PutUint16(response[6:8], 1)
	response = append(response, question...)
	if answerOwner == nil {
		answerOwner = []byte{0xc0, 0x0c}
	}
	response = append(response, answerOwner...)
	response = append(response, 0, 1, 0, 1) // A, IN
	response = append(response, 0, 0, 0, 60) // TTL
	response = append(response, 0, 4, 93, 184, 216, 34)
	return response
}

func TestValidateDNSResponse(t *testing.T) {
	query, err := makeDNSQuery("example.com", 0x1234)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("valid answer", func(t *testing.T) {
		if err := validateDNSResponse(query, buildAResponse(query, nil, nil)); err != nil {
			t.Fatalf("valid response rejected: %v", err)
		}
	})
	t.Run("mismatched question", func(t *testing.T) {
		question := append([]byte(nil), query[12:]...)
		question[2] = 'x'
		if err := validateDNSResponse(query, buildAResponse(query, question, nil)); err == nil {
			t.Fatal("expected mismatched question to be rejected")
		}
	})
	t.Run("unrelated answer owner", func(t *testing.T) {
		owner := []byte{9, 'u', 'n', 'r', 'e', 'l', 'a', 't', 'e', 'd', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 0}
		if err := validateDNSResponse(query, buildAResponse(query, nil, owner)); err == nil {
			t.Fatal("expected unrelated answer owner to be rejected")
		}
	})
	t.Run("mismatched transaction ID", func(t *testing.T) {
		response := buildAResponse(query, nil, nil)
		binary.BigEndian.PutUint16(response[0:2], 0xffff)
		if err := validateDNSResponse(query, response); err == nil {
			t.Fatal("expected mismatched transaction ID to be rejected")
		}
	})
}

func TestMakeDNSQueryRejectsNamesOver255WireOctets(t *testing.T) {
	domain := strings.Join([]string{
		strings.Repeat("a", 63), strings.Repeat("b", 63),
		strings.Repeat("c", 63), strings.Repeat("d", 60),
	}, ".")
	if _, err := makeDNSQuery(domain, 1); err == nil {
		t.Fatal("expected overlong DNS name to be rejected")
	}
}

func TestEnvDurationRejectsNonFiniteAndOutOfRangeValues(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf", "1e100", "1e-100", "0", "-1"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("TEST_DURATION", value)
			if _, err := envDuration("TEST_DURATION", "60"); err == nil {
				t.Fatalf("envDuration(%q) expected error", value)
			}
		})
	}
}
