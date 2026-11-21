package monitor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/uniextra/Local_DNS-DRP/internal/config"
	"github.com/uniextra/Local_DNS-DRP/internal/dns"
	"github.com/uniextra/Local_DNS-DRP/internal/recovery"
)

type stubChecker struct {
	results map[string]bool
}

func (s *stubChecker) Check(_ context.Context, target dns.Target, _ string, _ time.Duration) (bool, string) {
	if s.results[target.Addr] {
		return true, "ok"
	}
	return false, "timeout"
}

type stubNotifier struct {
	messages []string
}

func (s *stubNotifier) Send(_ context.Context, message string) error {
	s.messages = append(s.messages, message)
	return nil
}

type stubRecoverer struct {
	calls int
	err   error
}

func (s *stubRecoverer) Recover(_ context.Context) error {
	s.calls++
	return s.err
}

func testConfig() config.MonitorConfig {
	return config.MonitorConfig{
		Targets: []dns.Target{
			{Name: "primary", Addr: "192.168.1.2:53"},
			{Name: "backup", Addr: "192.168.1.3:53"},
		},
		Interval:               10 * time.Millisecond,
		DNSTimeout:             time.Second,
		TelegramTimeout:        time.Second,
		TestDomain:             "google.com",
		RecoveryEnabled:        true,
		TargetFailureThreshold: 2,
		Router: recovery.RouterConfig{
			Model:            recovery.RouterModelAskeyRTF8316VW,
			URL:              "http://192.168.1.1",
			DNSPrimary:       "1.1.1.1",
			DNSSecondary:     "8.8.8.8",
			WiFiOff:          time.Second,
			FailureThreshold: 3,
			Cooldown:         15 * time.Minute,
		},
	}
}

func TestMonitorAlertsFallbackSkipAndRecovery(t *testing.T) {
	cfg := testConfig()
	checker := &stubChecker{results: map[string]bool{
		"192.168.1.2:53": true,
		"192.168.1.3:53": true,
		"1.1.1.1:53":     false,
		"8.8.8.8:53":     false,
	}}
	notifier := &stubNotifier{}
	router := &stubRecoverer{}
	now := time.Unix(1000, 0)

	mon := New(cfg, checker, notifier, router)
	mon.now = func() time.Time { return now }

	// Both targets down 3 times while fallback DNS are ALSO down -> recovery skipped!
	checker.results["192.168.1.2:53"] = false
	checker.results["192.168.1.3:53"] = false
	mon.CheckOnce(context.Background())
	mon.CheckOnce(context.Background())
	mon.CheckOnce(context.Background())
	if router.calls != 0 {
		t.Fatalf("expected 0 recovery calls when fallback DNS fail, got %d", router.calls)
	}
	last := notifier.messages[len(notifier.messages)-1]
	if !strings.Contains(last, "Posible caída de internet") {
		t.Fatalf("expected fallback skip warning, got %q", last)
	}
	for _, msg := range notifier.messages {
		if strings.Contains(strings.ToLower(msg), "adguard") {
			t.Fatalf("unexpected AdGuard branding in Telegram message: %q", msg)
		}
	}

	// Fallback DNS comes back -> recovery triggers immediately on next cycle
	checker.results["1.1.1.1:53"] = true
	mon.CheckOnce(context.Background())
	if router.calls != 1 {
		t.Fatalf("expected 1 recovery call when fallback DNS answers, got %d", router.calls)
	}

	// Within cooldown -> skipped
	now = now.Add(5 * time.Minute)
	mon.CheckOnce(context.Background())
	if router.calls != 1 {
		t.Fatalf("expected cooldown to block recovery, got %d", router.calls)
	}

	// After cooldown -> runs and reports error
	now = now.Add(11 * time.Minute)
	router.err = errors.New("router busy")
	mon.CheckOnce(context.Background())
	if router.calls != 2 {
		t.Fatalf("expected second recovery after cooldown, got %d", router.calls)
	}
}

func TestMonitorDebouncesPerTargetFailures(t *testing.T) {
	cfg := testConfig()
	cfg.Targets = []dns.Target{{Name: "primary", Addr: "192.168.1.2:53"}}
	cfg.TargetFailureThreshold = 2
	checker := &stubChecker{results: map[string]bool{"192.168.1.2:53": true}}
	notifier := &stubNotifier{}
	mon := New(cfg, checker, notifier, nil)

	mon.CheckOnce(context.Background()) // healthy baseline
	checker.results["192.168.1.2:53"] = false
	mon.CheckOnce(context.Background()) // 1/2 failures, still no alert
	if len(notifier.messages) != 0 {
		t.Fatalf("expected no alert after a single failure, got %v", notifier.messages)
	}

	mon.CheckOnce(context.Background()) // 2/2 failures, down alert
	if len(notifier.messages) != 1 || !strings.Contains(notifier.messages[0], "DNS caído") {
		t.Fatalf("expected one down alert, got %v", notifier.messages)
	}

	mon.CheckOnce(context.Background()) // still down, no duplicate alert
	if len(notifier.messages) != 1 {
		t.Fatalf("expected no duplicate down alert, got %v", notifier.messages)
	}

	checker.results["192.168.1.2:53"] = true
	mon.CheckOnce(context.Background()) // recovered alert
	if len(notifier.messages) != 2 || !strings.Contains(notifier.messages[1], "DNS recuperado") {
		t.Fatalf("expected one recovery alert, got %v", notifier.messages)
	}
}

func TestMonitorFailureThresholdOneAlertsImmediately(t *testing.T) {
	cfg := testConfig()
	cfg.Targets = []dns.Target{{Name: "primary", Addr: "192.168.1.2:53"}}
	cfg.TargetFailureThreshold = 1
	checker := &stubChecker{results: map[string]bool{"192.168.1.2:53": false}}
	notifier := &stubNotifier{}
	mon := New(cfg, checker, notifier, nil)

	mon.CheckOnce(context.Background())
	if len(notifier.messages) != 1 || !strings.Contains(notifier.messages[0], "DNS caído") {
		t.Fatalf("expected an immediate down alert, got %v", notifier.messages)
	}
}

func TestMonitorRunLifecycle(t *testing.T) {
	cfg := testConfig()
	checker := &stubChecker{results: map[string]bool{
		"192.168.1.2:53": true,
		"192.168.1.3:53": true,
	}}
	notifier := &stubNotifier{}
	mon := New(cfg, checker, notifier, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		mon.Run(ctx)
		close(done)
	}()

	time.Sleep(25 * time.Millisecond)
	cancel()
	<-done

	if len(notifier.messages) < 2 {
		t.Fatalf("expected start and stop notifications, got %v", notifier.messages)
	}
	if !strings.Contains(notifier.messages[0], "iniciado") {
		t.Fatalf("expected start notification, got %q", notifier.messages[0])
	}
	if !strings.Contains(notifier.messages[len(notifier.messages)-1], "detenido") {
		t.Fatalf("expected stop notification, got %q", notifier.messages[len(notifier.messages)-1])
	}
}
