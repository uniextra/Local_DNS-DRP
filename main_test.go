package main

import (
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMakeDNSQuery(t *testing.T) {
	packet, err := makeDNSQuery("example.com", 0x1234)
	if err != nil {
		t.Fatalf("makeDNSQuery returned error: %v", err)
	}
	if got := binary.BigEndian.Uint16(packet[:2]); got != 0x1234 {
		t.Fatalf("transaction ID = %#x, want %#x", got, 0x1234)
	}
	if got := binary.BigEndian.Uint16(packet[2:4]); got&0x0100 == 0 {
		t.Fatal("recursion desired flag is not set")
	}
	if got := binary.BigEndian.Uint16(packet[4:6]); got != 1 {
		t.Fatalf("question count = %d, want 1", got)
	}
	if !strings.HasSuffix(string(packet), "\x00\x00\x01\x00\x01") {
		t.Fatal("query does not end with root terminator, A type and IN class")
	}
}

func TestMakeDNSQueryRejectsInvalidNames(t *testing.T) {
	overlong := strings.Join([]string{strings.Repeat("a", 63), strings.Repeat("b", 63), strings.Repeat("c", 63), strings.Repeat("d", 63), "e"}, ".")
	for _, domain := range []string{"", "example..com", strings.Repeat("a", 64) + ".com", overlong} {
		if _, err := makeDNSQuery(domain, 1); err == nil {
			t.Errorf("makeDNSQuery(%q) expected error", domain)
		}
	}
}

func dnsName(name string) []byte {
	var packet []byte
	for _, label := range strings.Split(name, ".") {
		packet = append(packet, byte(len(label)))
		packet = append(packet, label...)
	}
	return append(packet, 0)
}

func dnsResponseBase(query []byte, answers int) []byte {
	response := append([]byte(nil), query...)
	binary.BigEndian.PutUint16(response[2:4], 0x8180)
	binary.BigEndian.PutUint16(response[6:8], uint16(answers))
	return response
}

func appendARecord(response []byte, owner []byte, ip net.IP) []byte {
	response = append(response, owner...)
	response = append(response, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4)
	return append(response, ip.To4()...)
}

func TestValidateDNSResponseAcceptsARecord(t *testing.T) {
	query, err := makeDNSQuery("example.com", 0x1234)
	if err != nil {
		t.Fatal(err)
	}
	response := dnsResponseBase(query, 1)
	response = appendARecord(response, []byte{0xC0, 0x0C}, net.IPv4(192, 0, 2, 1))
	if err := validateDNSResponse(query, response); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}
}

func TestValidateDNSResponseFollowsCNAMEChain(t *testing.T) {
	query, err := makeDNSQuery("example.com", 0x1234)
	if err != nil {
		t.Fatal(err)
	}
	response := dnsResponseBase(query, 2)
	target := dnsName("edge.example.net")
	response = append(response, 0xC0, 0x0C, 0, 5, 0, 1, 0, 0, 0, 60, byte(len(target)>>8), byte(len(target)))
	response = append(response, target...)
	response = appendARecord(response, target, net.IPv4(198, 51, 100, 8))
	if err := validateDNSResponse(query, response); err != nil {
		t.Fatalf("valid CNAME response rejected: %v", err)
	}
}

func TestValidateDNSResponseRejectsUnrelatedOrMalformedAnswers(t *testing.T) {
	query, err := makeDNSQuery("example.com", 0x1234)
	if err != nil {
		t.Fatal(err)
	}
	unrelated := dnsResponseBase(query, 1)
	unrelated = appendARecord(unrelated, dnsName("unrelated.example"), net.IPv4(192, 0, 2, 1))
	truncated := dnsResponseBase(query, 1)
	truncated = appendARecord(truncated, []byte{0xC0, 0x0C}, net.IPv4(192, 0, 2, 1))
	truncated = truncated[:len(truncated)-1]

	tests := []struct {
		name   string
		packet []byte
	}{
		{"wrong transaction ID", func() []byte {
			p := dnsResponseBase(query, 1)
			p[1]++
			return appendARecord(p, []byte{0xC0, 0x0C}, net.IPv4(192, 0, 2, 1))
		}()},
		{"not a response", func() []byte {
			p := dnsResponseBase(query, 1)
			p[2] &^= 0x80
			return appendARecord(p, []byte{0xC0, 0x0C}, net.IPv4(192, 0, 2, 1))
		}()},
		{"wrong question", func() []byte {
			p := dnsResponseBase(query, 1)
			p[13] = 'x'
			return appendARecord(p, []byte{0xC0, 0x0C}, net.IPv4(192, 0, 2, 1))
		}()},
		{"zero answers", dnsResponseBase(query, 0)},
		{"unrelated A record", unrelated},
		{"truncated A record", truncated},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateDNSResponse(query, tc.packet); err == nil {
				t.Fatal("expected response to be rejected")
			}
		})
	}
}

func TestCheckDNSQueriesConfiguredTarget(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	go func() {
		buf := make([]byte, 512)
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		query := buf[:n]
		response := dnsResponseBase(query, 1)
		response = appendARecord(response, []byte{0xC0, 0x0C}, net.IPv4(192, 0, 2, 10))
		_, _ = pc.WriteTo(response, addr)
	}()
	ok, detail := checkDNS(Target{Name: "test", Addr: pc.LocalAddr().String()}, "example.com", time.Second)
	if !ok {
		t.Fatalf("checkDNS failed against configured target: %s", detail)
	}
}

func TestEnvDurationRejectsNonFiniteAndOverflow(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf", "1e100"} {
		t.Setenv("DURATION_TEST", value)
		if _, err := envDuration("DURATION_TEST", "1"); err == nil {
			t.Errorf("envDuration accepted %q", value)
		}
	}
	t.Setenv("DURATION_TEST", "0.0000000001")
	if _, err := envDuration("DURATION_TEST", "1"); err == nil {
		t.Error("envDuration accepted sub-nanosecond duration")
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
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}
	if len(cfg.Targets) != 3 {
		t.Fatalf("got %d targets, want 3", len(cfg.Targets))
	}
	if cfg.Targets[0].Addr != "192.168.1.2:53" {
		t.Errorf("first target = %q", cfg.Targets[0].Addr)
	}
	if cfg.Targets[1].Addr != "192.168.1.10:5353" {
		t.Errorf("second target = %q", cfg.Targets[1].Addr)
	}
	if cfg.Targets[2].Addr != "[2001:db8::53]:53" {
		t.Errorf("third target = %q", cfg.Targets[2].Addr)
	}
}

func TestLoadConfigRejectsHostnameAndDuplicateEndpoint(t *testing.T) {
	base := map[string]string{"TELEGRAM_BOT_TOKEN": "token", "TELEGRAM_CHAT_ID": "chat", "CHECK_INTERVAL": "60", "DNS_TIMEOUT": "3", "TELEGRAM_TIMEOUT": "10", "DNS_TEST_DOMAIN": "example.com"}
	for _, tc := range []struct{ name, targets, want string }{
		{"hostname", "home=adguard.local", "IP addresses"},
		{"duplicate", "one=192.168.1.2,two=192.168.1.2", "duplicate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range base {
				t.Setenv(key, value)
			}
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
	if err == nil || !strings.Contains(err.Error(), "TELEGRAM_BOT_TOKEN") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestLoadConfigValidatesRouterURL(t *testing.T) {
	t.Setenv("TARGETS", "home=192.168.1.2")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat")
	t.Setenv("ROUTER_PASSWORD", "secret")
	for _, value := range []string{"ftp://192.168.1.1", "router.local", "http://user:pass@192.168.1.1"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("ROUTER_URL", value)
			_, err := loadConfig()
			if err == nil || !strings.Contains(err.Error(), "ROUTER_URL") {
				t.Fatalf("loadConfig error = %v, want ROUTER_URL validation error", err)
			}
		})
	}
	t.Setenv("ROUTER_URL", "https://192.168.1.1")
	if _, err := loadConfig(); err != nil {
		t.Fatalf("valid HTTPS router URL rejected: %v", err)
	}
}

func TestLoadConfigValidatesFallbackDNS(t *testing.T) {
	t.Setenv("TARGETS", "home=192.168.1.2")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat")
	t.Setenv("ROUTER_PASSWORD", "secret")
	for _, name := range []string{"DNS_PRIMARY", "DNS_SECONDARY"} {
		for _, value := range []string{"dns.google", "1.1.1.1,8.8.8.8", "300.1.1.1", "not-an-ip"} {
			t.Run(name+"="+value, func(t *testing.T) {
				t.Setenv("DNS_PRIMARY", "1.1.1.1")
				t.Setenv("DNS_SECONDARY", "8.8.8.8")
				t.Setenv(name, value)
				_, err := loadConfig()
				if err == nil || !strings.Contains(err.Error(), name) {
					t.Fatalf("loadConfig error = %v, want %s validation error", err, name)
				}
			})
		}
	}
	t.Setenv("DNS_PRIMARY", "9.9.9.9")
	t.Setenv("DNS_SECONDARY", "2606:4700:4700::1111")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("valid fallback DNS addresses rejected: %v", err)
	}
	if cfg.Router.DNSPrimary != "9.9.9.9" || cfg.Router.DNSSecondary != "2606:4700:4700::1111" {
		t.Fatalf("fallback DNS = %q / %q", cfg.Router.DNSPrimary, cfg.Router.DNSSecondary)
	}
}

func TestFallbackDNSAnswers(t *testing.T) {
	const timeout = 2 * time.Second
	newCfg := func(primary, secondary string) MonitorConfig {
		return MonitorConfig{
			TestDomain: "example.com",
			DNSTimeout: timeout,
			Router:     RouterConfig{DNSPrimary: primary, DNSSecondary: secondary},
		}
	}
	tests := []struct {
		name       string
		cfg        MonitorConfig
		answering  map[string]bool
		wantOK     bool
		wantProbed []string
	}{
		{
			name:       "no fallback resolver answers: the outage is upstream",
			cfg:        newCfg("1.1.1.1", "2606:4700:4700::1111"),
			wantOK:     false,
			wantProbed: []string{"1.1.1.1:53", "[2606:4700:4700::1111]:53"},
		},
		{
			name:       "only the secondary answers",
			cfg:        newCfg("1.1.1.1", "8.8.8.8"),
			answering:  map[string]bool{"8.8.8.8:53": true},
			wantOK:     true,
			wantProbed: []string{"1.1.1.1:53", "8.8.8.8:53"},
		},
		{
			name:       "primary answers: the secondary is not probed",
			cfg:        newCfg("1.1.1.1", "8.8.8.8"),
			answering:  map[string]bool{"1.1.1.1:53": true},
			wantOK:     true,
			wantProbed: []string{"1.1.1.1:53"},
		},
		{
			name:       "identical primary and secondary are probed once",
			cfg:        newCfg("1.1.1.1", "1.1.1.1"),
			wantOK:     false,
			wantProbed: []string{"1.1.1.1:53"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var probed []string
			probe := func(target Target, domain string, got time.Duration) (bool, string) {
				probed = append(probed, target.Addr)
				if domain != "example.com" || got != timeout {
					t.Errorf("probe(%s) used domain %q and timeout %s", target.Addr, domain, got)
				}
				if tc.answering[target.Addr] {
					return true, "DNS query successful"
				}
				return false, "i/o timeout"
			}
			ok, detail := fallbackDNSAnswers(tc.cfg, probe)
			if ok != tc.wantOK {
				t.Fatalf("fallbackDNSAnswers = %v (%s), want %v", ok, detail, tc.wantOK)
			}
			if strings.Join(probed, ",") != strings.Join(tc.wantProbed, ",") {
				t.Fatalf("probed %v, want %v", probed, tc.wantProbed)
			}
			if !ok && !strings.Contains(detail, "i/o timeout") {
				t.Fatalf("detail %q does not explain why the fallback resolvers failed", detail)
			}
		})
	}
}

func TestSecretEnvPrefersFileAndTrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("  file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_SECRET", "environment-secret")
	t.Setenv("TEST_SECRET_FILE", path)
	got, err := secretEnv("TEST_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if got != "file-secret" {
		t.Fatalf("secretEnv = %q, want file value", got)
	}
}

func TestSecretEnvFallsBackToEnvironment(t *testing.T) {
	t.Setenv("TEST_SECRET", "  environment-secret\n")
	t.Setenv("TEST_SECRET_FILE", "")
	got, err := secretEnv("TEST_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if got != "environment-secret" {
		t.Fatalf("secretEnv = %q, want trimmed environment value", got)
	}
}

func TestSecretEnvReportsUnreadableFile(t *testing.T) {
	t.Setenv("TEST_SECRET", "environment-secret")
	t.Setenv("TEST_SECRET_FILE", filepath.Join(t.TempDir(), "missing"))
	if _, err := secretEnv("TEST_SECRET"); err == nil {
		t.Fatal("expected missing secret file to return an error")
	}
}

func TestLoadConfigReadsCredentialFiles(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"telegram": filepath.Join(dir, "telegram"),
		"chat":     filepath.Join(dir, "chat"),
		"router":   filepath.Join(dir, "router"),
	}
	values := map[string]string{
		"telegram": "bot-token",
		"chat":     "123456",
		"router":   "router-password",
	}
	for name, path := range files {
		if err := os.WriteFile(path, []byte(values[name]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TARGETS", "home=192.168.1.2")
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_CHAT_ID", "")
	t.Setenv("ROUTER_PASSWORD", "")
	t.Setenv("TELEGRAM_BOT_TOKEN_FILE", files["telegram"])
	t.Setenv("TELEGRAM_CHAT_ID_FILE", files["chat"])
	t.Setenv("ROUTER_PASSWORD_FILE", files["router"])
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BotToken != "bot-token" || cfg.ChatID != "123456" {
		t.Fatalf("credential files not loaded: token=%q chat=%q", cfg.BotToken, cfg.ChatID)
	}
	if !cfg.RecoveryEnabled || cfg.Router.Password != "router-password" {
		t.Fatal("router password file was not loaded")
	}
}

func TestLoadConfigDefaultsRouterModel(t *testing.T) {
	t.Setenv("TARGETS", "home=192.168.1.2")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat")
	t.Setenv("ROUTER_PASSWORD", "secret")
	t.Setenv("ROUTER_MODEL", "")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Router.Model != RouterModelAskeyRTF8316VW {
		t.Fatalf("router model = %q, want %q", cfg.Router.Model, RouterModelAskeyRTF8316VW)
	}
}

func TestLoadConfigUsesExplicitRouterModel(t *testing.T) {
	t.Setenv("TARGETS", "home=192.168.1.2")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat")
	t.Setenv("ROUTER_PASSWORD", "secret")
	t.Setenv("ROUTER_MODEL", "ASKEY-RTF8316VW")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Router.Model != RouterModelAskeyRTF8316VW {
		t.Fatalf("router model = %q, want normalized %q", cfg.Router.Model, RouterModelAskeyRTF8316VW)
	}
}

func TestLoadConfigRejectsUnsupportedRouterModelWhenRecoveryEnabled(t *testing.T) {
	t.Setenv("TARGETS", "home=192.168.1.2")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat")
	t.Setenv("ROUTER_PASSWORD", "secret")
	t.Setenv("ROUTER_MODEL", "unknown-router-123")
	_, err := loadConfig()
	if err == nil || !strings.Contains(err.Error(), "unsupported ROUTER_MODEL") {
		t.Fatalf("loadConfig error = %v, want unsupported ROUTER_MODEL error", err)
	}
}
