package main

import (
	"encoding/binary"
	"math"
	"net"
	"strings"
	"testing"
	"time"
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

func TestMakeDNSQueryRejectsOverlongName(t *testing.T) {
	domain := strings.Join([]string{strings.Repeat("a", 63), strings.Repeat("b", 63), strings.Repeat("c", 63), strings.Repeat("d", 63), "e"}, ".")
	if _, err := makeDNSQuery(domain, 1); err == nil { t.Fatal("expected overlong DNS name to be rejected") }
}

func TestValidateDNSResponse(t *testing.T) {
	query, err := makeDNSQuery("example.com", 0x1234)
	if err != nil { t.Fatal(err) }
	// A standard response with one compressed-owner A record.
	good := append([]byte(nil), query...)
	binary.BigEndian.PutUint16(good[2:4], 0x8180)
	binary.BigEndian.PutUint16(good[6:8], 1)
	good = append(good, 0xC0, 0x0C, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 192, 0, 2, 1)
	if err := validateDNSResponse(query, good); err != nil { t.Fatalf("valid response rejected: %v", err) }

	tests := []struct{name string; mutate func([]byte)}{
		{"wrong transaction ID", func(p []byte) { p[1]++ }},
		{"not a response", func(p []byte) { p[2] &^= 0x80 }},
		{"wrong question", func(p []byte) { p[13] = 'x' }},
		{"no answer", func(p []byte) { binary.BigEndian.PutUint16(p[6:8], 0) }},
		{"truncated answer", func(p []byte) { p = p[:len(p)-1] }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := append([]byte(nil), good...)
			if tc.name == "truncated answer" { p = p[:len(p)-1] } else { tc.mutate(p) }
			if err := validateDNSResponse(query, p); err == nil { t.Fatal("expected response to be rejected") }
		})
	}
}

func TestCheckDNSUsesConfiguredTarget(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	defer pc.Close()
	go func() {
		buf := make([]byte, 512)
		n, addr, err := pc.ReadFrom(buf)
		if err != nil { return }
		q := buf[:n]
		resp := append([]byte(nil), q...)
		binary.BigEndian.PutUint16(resp[2:4], 0x8180)
		binary.BigEndian.PutUint16(resp[6:8], 1)
		resp = append(resp, 0xC0, 0x0C, 0, 1, 0, 1, 0, 0, 0, 10, 0, 4, 192, 0, 2, 10)
		_, _ = pc.WriteTo(resp, addr)
	}()
	ok, detail := checkDNS(Target{Name:"test",Addr:pc.LocalAddr().String()}, "example.com", time.Second)
	if !ok { t.Fatalf("checkDNS failed against configured target: %s", detail) }
}

func TestEnvDurationRejectsNonFiniteAndOverflow(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf", "1e100"} {
		t.Setenv("DURATION_TEST", value)
		if _, err := envDuration("DURATION_TEST", "1"); err == nil { t.Errorf("envDuration accepted %q", value) }
	}
	t.Setenv("DURATION_TEST", "0.0000000001")
	if _, err := envDuration("DURATION_TEST", "1"); err == nil { t.Error("envDuration accepted sub-nanosecond duration") }
	_ = math.MaxInt64
}

// Remaining config tests exercise target parsing and Telegram requirements.
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
	base := map[string]string{"TELEGRAM_BOT_TOKEN":"token","TELEGRAM_CHAT_ID":"chat","CHECK_INTERVAL":"60","DNS_TIMEOUT":"3","TELEGRAM_TIMEOUT":"10","DNS_TEST_DOMAIN":"example.com"}
	for _, tc := range []struct{name,targets,want string}{
		{"hostname","home=adguard.local","IP addresses"},
		{"duplicate","one=192.168.1.2,two=192.168.1.2","duplicate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range base { t.Setenv(key,value) }
			t.Setenv("TARGETS",tc.targets)
			_, err := loadConfig()
			if err == nil || !strings.Contains(strings.ToLower(err.Error()),strings.ToLower(tc.want)) { t.Fatalf("loadConfig error = %v, want text %q",err,tc.want) }
		})
	}
}

func TestLoadConfigRequiresTelegramCredentials(t *testing.T) {
	t.Setenv("TARGETS","home=192.168.1.2")
	t.Setenv("TELEGRAM_BOT_TOKEN","")
	t.Setenv("TELEGRAM_CHAT_ID","")
	_, err := loadConfig()
	if err == nil || !strings.Contains(err.Error(),"TELEGRAM_BOT_TOKEN") { t.Fatalf("unexpected error: %v",err) }
}
