package config

import (
	"testing"
	"time"
)

func setBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TARGETS", "home=192.168.1.2,backup=192.168.1.3:5353")
	t.Setenv("TELEGRAM_BOT_TOKEN", "bot-token")
	t.Setenv("TELEGRAM_CHAT_ID", "123456")
	t.Setenv("CHECK_INTERVAL", "")
	t.Setenv("DNS_TIMEOUT", "")
	t.Setenv("TELEGRAM_TIMEOUT", "")
	t.Setenv("TARGET_FAILURE_THRESHOLD", "")
	t.Setenv("DNS_TEST_DOMAIN", "")
	t.Setenv("ROUTER_MODEL", "")
	t.Setenv("ROUTER_PASSWORD", "")
	t.Setenv("ROUTER_URL", "")
	t.Setenv("ROUTER_USERNAME", "")
	t.Setenv("DNS_PRIMARY", "")
	t.Setenv("DNS_SECONDARY", "")
	t.Setenv("WIFI_OFF_SECONDS", "")
	t.Setenv("RECOVERY_COOLDOWN", "")
	t.Setenv("FAILURE_THRESHOLD", "")
	t.Setenv("HEARTBEAT_FILE", "")
}

func TestLoadRecoveryDisabledByDefault(t *testing.T) {
	setBaseEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.RecoveryEnabled {
		t.Fatal("RecoveryEnabled = true, want false")
	}
	if cfg.Router.Model != "askey-rtf8316vw" {
		t.Fatalf("Router.Model = %q, want askey-rtf8316vw", cfg.Router.Model)
	}
	if len(cfg.Targets) != 2 || cfg.Targets[0].Addr != "192.168.1.2:53" || cfg.Targets[1].Addr != "192.168.1.3:5353" {
		t.Fatalf("unexpected targets: %+v", cfg.Targets)
	}
	if cfg.Interval != 60*time.Second || cfg.TestDomain != "google.com" {
		t.Fatalf("unexpected defaults: interval=%s domain=%q", cfg.Interval, cfg.TestDomain)
	}
	if cfg.TargetFailureThreshold != 2 {
		t.Fatalf("TargetFailureThreshold = %d, want 2", cfg.TargetFailureThreshold)
	}
	if cfg.HeartbeatFile == "" {
		t.Fatal("HeartbeatFile is empty, want a default path")
	}
}

func TestLoadRecoveryDefaults(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("ROUTER_PASSWORD", "secret")
	t.Setenv("ROUTER_MODEL", "  Askey-RTF8316VW ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if !cfg.RecoveryEnabled || cfg.Router.Model != "askey-rtf8316vw" {
		t.Fatalf("unexpected router config: %+v", cfg.Router)
	}
	if cfg.Router.URL != "http://192.168.1.1" || cfg.Router.Username != "1234" {
		t.Fatalf("unexpected router defaults: %+v", cfg.Router)
	}
	if cfg.Router.DNSPrimary != "1.1.1.1" || cfg.Router.DNSSecondary != "8.8.8.8" {
		t.Fatalf("unexpected DNS defaults: %s / %s", cfg.Router.DNSPrimary, cfg.Router.DNSSecondary)
	}
	if cfg.Router.FailureThreshold != 3 {
		t.Fatalf("Router.FailureThreshold = %d, want 3", cfg.Router.FailureThreshold)
	}
}

func TestLoadCustomThresholds(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("ROUTER_PASSWORD", "secret")
	t.Setenv("TARGET_FAILURE_THRESHOLD", "4")
	t.Setenv("FAILURE_THRESHOLD", "5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.TargetFailureThreshold != 4 || cfg.Router.FailureThreshold != 5 {
		t.Fatalf("unexpected thresholds: target=%d recovery=%d", cfg.TargetFailureThreshold, cfg.Router.FailureThreshold)
	}
}

func TestLoadAsusGenericModel(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("ROUTER_PASSWORD", "secret")
	t.Setenv("ROUTER_MODEL", "asus-generic")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.Router.Model != "asus-generic" {
		t.Fatalf("Router.Model = %q, want asus-generic", cfg.Router.Model)
	}
}

func TestLoadValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{name: "missing targets", env: map[string]string{"TARGETS": ""}},
		{name: "hostname target", env: map[string]string{"TARGETS": "home=dns.local"}},
		{name: "invalid port", env: map[string]string{"TARGETS": "home=192.168.1.2:99999"}},
		{name: "duplicate ipv6", env: map[string]string{"TARGETS": "a=2001:db8:0:0::1,b=[2001:db8::1]:53"}},
		{name: "missing telegram token", env: map[string]string{"TELEGRAM_BOT_TOKEN": ""}},
		{name: "missing telegram chat", env: map[string]string{"TELEGRAM_CHAT_ID": ""}},
		{name: "nan interval", env: map[string]string{"CHECK_INTERVAL": "NaN"}},
		{name: "inf interval", env: map[string]string{"CHECK_INTERVAL": "Inf"}},
		{name: "zero interval", env: map[string]string{"CHECK_INTERVAL": "0"}},
		{name: "overflow interval", env: map[string]string{"CHECK_INTERVAL": "1e20"}},
		{name: "interval below one second", env: map[string]string{"CHECK_INTERVAL": "0.5"}},
		{name: "timeout not below interval", env: map[string]string{"CHECK_INTERVAL": "2", "DNS_TIMEOUT": "2"}},
		{name: "target failure threshold zero", env: map[string]string{"TARGET_FAILURE_THRESHOLD": "0"}},
		{name: "target failure threshold too high", env: map[string]string{"TARGET_FAILURE_THRESHOLD": "11"}},
		{name: "target failure threshold not a number", env: map[string]string{"TARGET_FAILURE_THRESHOLD": "two"}},
		{name: "invalid domain", env: map[string]string{"DNS_TEST_DOMAIN": "bad..domain"}},
		{name: "invalid router url", env: map[string]string{"ROUTER_PASSWORD": "pw", "ROUTER_URL": "ftp://192.168.1.1"}},
		{name: "router url embedded creds", env: map[string]string{"ROUTER_PASSWORD": "pw", "ROUTER_URL": "http://user:pass@192.168.1.1"}},
		{name: "router url with query", env: map[string]string{"ROUTER_PASSWORD": "pw", "ROUTER_URL": "http://192.168.1.1/?x=1"}},
		{name: "router url with fragment", env: map[string]string{"ROUTER_PASSWORD": "pw", "ROUTER_URL": "http://192.168.1.1/#frag"}},
		{name: "recovery threshold zero", env: map[string]string{"ROUTER_PASSWORD": "pw", "FAILURE_THRESHOLD": "0"}},
		{name: "recovery threshold too high", env: map[string]string{"ROUTER_PASSWORD": "pw", "FAILURE_THRESHOLD": "11"}},
		{name: "invalid primary dns", env: map[string]string{"ROUTER_PASSWORD": "pw", "DNS_PRIMARY": "not-an-ip"}},
		{name: "invalid secondary dns", env: map[string]string{"ROUTER_PASSWORD": "pw", "DNS_SECONDARY": "not-an-ip"}},
		{name: "unsupported router model", env: map[string]string{"ROUTER_PASSWORD": "pw", "ROUTER_MODEL": "unknown-router"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setBaseEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Fatalf("Load() succeeded for %s, expected error", tc.name)
			}
		})
	}
}
