// Package monitor implements the DNS polling loop, alert state machine, and recovery orchestration.
package monitor

import (
	"context"
	"fmt"
	"log"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/uniextra/Adguardhome-DRP/internal/config"
	"github.com/uniextra/Adguardhome-DRP/internal/dns"
	"github.com/uniextra/Adguardhome-DRP/internal/recovery"
)

// DNSChecker checks whether a DNS target resolves the test domain.
type DNSChecker interface {
	Check(ctx context.Context, target dns.Target, domain string, timeout time.Duration) (bool, string)
}

// Notifier delivers alert messages.
type Notifier interface {
	Send(ctx context.Context, message string) error
}

// State tracks the last known health status of a DNS target.
type State struct {
	Known   bool
	Healthy bool
}

// FallbackDNSAnswers reports whether at least one of the resolvers that a
// recovery would install in the router's DHCP settings answers the monitor's
// test query. When none does, the outage is upstream of the LAN (ISP/WAN).
func FallbackDNSAnswers(ctx context.Context, cfg config.MonitorConfig, checker DNSChecker) (bool, string) {
	var failures []string
	var probed []string
	for _, ip := range []string{cfg.Router.DNSPrimary, cfg.Router.DNSSecondary} {
		if ip == "" || slices.Contains(probed, ip) {
			continue
		}
		probed = append(probed, ip)
		healthy, detail := checker.Check(ctx, dns.Target{Name: ip, Addr: net.JoinHostPort(ip, "53")}, cfg.TestDomain, cfg.DNSTimeout)
		if healthy {
			return true, ""
		}
		failures = append(failures, fmt.Sprintf("%s: %s", ip, detail))
	}
	return false, strings.Join(failures, "; ")
}

// Monitor coordinates periodic DNS checks, notifications, and router recovery.
type Monitor struct {
	cfg                  config.MonitorConfig
	checker              DNSChecker
	notifier             Notifier
	router               recovery.RouterRecovery
	now                  func() time.Time
	states               map[string]State
	consecutiveAllDown   int
	lastRecovery         time.Time
	fallbackSkipNotified bool
}

// New creates a Monitor with the provided dependencies. router may be nil when recovery is disabled.
func New(cfg config.MonitorConfig, checker DNSChecker, notifier Notifier, router recovery.RouterRecovery) *Monitor {
	return &Monitor{
		cfg:      cfg,
		checker:  checker,
		notifier: notifier,
		router:   router,
		now:      time.Now,
		states:   make(map[string]State, len(cfg.Targets)),
	}
}

func (m *Monitor) notify(ctx context.Context, message string) {
	notifyCtx, cancel := context.WithTimeout(ctx, m.cfg.TelegramTimeout)
	defer cancel()
	if err := m.notifier.Send(notifyCtx, message); err != nil {
		log.Printf("ERROR Telegram notification failed: %v", err)
	}
}

// CheckOnce runs a single check cycle across all configured targets and triggers recovery if thresholds are met.
func (m *Monitor) CheckOnce(ctx context.Context) {
	allDown := true
	for _, target := range m.cfg.Targets {
		healthy, detail := m.checker.Check(ctx, target, m.cfg.TestDomain, m.cfg.DNSTimeout)
		previous := m.states[target.Addr]
		m.states[target.Addr] = State{Known: true, Healthy: healthy}
		if healthy {
			allDown = false
			log.Printf("%s (%s): OK", target.Name, target.Addr)
			if previous.Known && !previous.Healthy {
				m.notify(ctx, fmt.Sprintf("🟢 AdGuard DNS RECUPERADO\nInstancia: %s\nIP: %s\nPrueba: %s\nEstado: DNS operativo", target.Name, target.Addr, m.cfg.TestDomain))
			}
		} else {
			log.Printf("WARN %s (%s): FAILED - %s", target.Name, target.Addr, detail)
			if !previous.Known || previous.Healthy {
				m.notify(ctx, fmt.Sprintf("🔴 FALLO AdGuard DNS\nInstancia: %s\nIP: %s\nPrueba: %s\nError: %s", target.Name, target.Addr, m.cfg.TestDomain, detail))
			}
		}
	}

	if m.router == nil {
		return
	}
	if allDown {
		m.consecutiveAllDown++
	} else {
		m.consecutiveAllDown = 0
		m.fallbackSkipNotified = false
		return
	}
	if m.consecutiveAllDown < m.cfg.Router.FailureThreshold {
		return
	}
	if !m.lastRecovery.IsZero() && m.now().Sub(m.lastRecovery) < m.cfg.Router.Cooldown {
		return
	}

	if answers, detail := FallbackDNSAnswers(ctx, m.cfg, m.checker); !answers {
		log.Printf("WARN router recovery skipped (model=%s): fallback DNS %s / %s do not answer either (%s)", m.cfg.Router.Model, m.cfg.Router.DNSPrimary, m.cfg.Router.DNSSecondary, detail)
		if !m.fallbackSkipNotified {
			m.fallbackSkipNotified = true
			m.notify(ctx, fmt.Sprintf("🟠 Todos los DNS monitorizados están caídos, pero los DNS de respaldo (%s / %s) tampoco responden: parece una caída de internet, no de AdGuard. No se modifica el router; se reintentará en cada comprobación.", m.cfg.Router.DNSPrimary, m.cfg.Router.DNSSecondary))
		}
		return
	}

	m.fallbackSkipNotified = false
	m.notify(ctx, fmt.Sprintf("🚨 AdGuard: todos los DNS monitorizados están caídos. Iniciando recuperación del router %s (%s).", m.cfg.Router.Model, m.cfg.Router.URL))
	recoveryCtx, cancel := context.WithTimeout(ctx, 2*time.Minute+m.cfg.Router.WiFiOff)
	err := m.router.Recover(recoveryCtx)
	cancel()
	m.lastRecovery = m.now()

	if err != nil {
		log.Printf("ERROR router recovery failed (model=%s): %v", m.cfg.Router.Model, err)
		m.notify(ctx, fmt.Sprintf("🔴 ERROR recuperación router %s\n%s", m.cfg.Router.Model, err))
		return
	}
	log.Printf("Router recovery completed (model=%s): DNS=%s,%s", m.cfg.Router.Model, m.cfg.Router.DNSPrimary, m.cfg.Router.DNSSecondary)
	m.notify(ctx, fmt.Sprintf("🟢 Recuperación del router %s completada\nDNS DHCP: %s / %s", m.cfg.Router.Model, m.cfg.Router.DNSPrimary, m.cfg.Router.DNSSecondary))
}

// Run starts the monitoring loop and blocks until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	lines := make([]string, 0, len(m.cfg.Targets))
	for _, target := range m.cfg.Targets {
		lines = append(lines, fmt.Sprintf("• %s: %s", target.Name, target.Addr))
	}
	label := "servidores DNS"
	if len(m.cfg.Targets) == 1 {
		label = "servidor DNS"
	}
	m.notify(ctx, fmt.Sprintf("🟢 AdGuard DNS Monitor iniciado\nMonitorizando %d %s\n%s", len(m.cfg.Targets), label, strings.Join(lines, "\n")))

	m.CheckOnce(ctx)

	ticker := time.NewTicker(m.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.notify(context.Background(), "⚪ AdGuard DNS Monitor detenido")
			return
		case <-ticker.C:
			m.CheckOnce(ctx)
		}
	}
}
