// Command dns-monitor periodically checks configured DNS servers, sends Telegram alerts,
// and optionally triggers automated router recovery on total DNS outage.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/uniextra/Local_DNS-DRP/internal/config"
	"github.com/uniextra/Local_DNS-DRP/internal/dns"
	"github.com/uniextra/Local_DNS-DRP/internal/health"
	"github.com/uniextra/Local_DNS-DRP/internal/monitor"
	"github.com/uniextra/Local_DNS-DRP/internal/notify"
	"github.com/uniextra/Local_DNS-DRP/internal/recovery"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "check the monitor heartbeat and exit")
	flag.Parse()
	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	log.Printf("Starting Local_DNS-DRP monitor: %d target(s), interval=%s, query=%s", len(cfg.Targets), cfg.Interval, cfg.TestDomain)
	for _, target := range cfg.Targets {
		log.Printf("Configured target: %s (%s)", target.Name, target.Addr)
	}

	var router recovery.RouterRecovery
	if cfg.RecoveryEnabled {
		router, err = recovery.NewRouterRecovery(cfg.Router)
		if err != nil {
			log.Fatalf("Router configuration error: %v", err)
		}
		log.Printf("Router recovery enabled: model=%s url=%s", cfg.Router.Model, cfg.Router.URL)
		if strings.HasPrefix(strings.ToLower(cfg.Router.URL), "http://") {
			log.Printf("WARN ROUTER_URL uses plain HTTP: router credentials and session data are not encrypted in transit")
		}
	} else {
		log.Printf("Router recovery disabled: ROUTER_PASSWORD is not set")
	}

	tg := notify.NewTelegram(cfg.BotToken, cfg.ChatID, cfg.TelegramTimeout)
	mon := monitor.New(cfg, dns.Checker{}, tg, router)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mon.Run(ctx)
}

// runHealthcheck verifies that the monitor loop is still writing heartbeats.
// It is used by the container HEALTHCHECK and does not load the full config.
func runHealthcheck() int {
	interval := 60 * time.Second
	if raw := strings.TrimSpace(os.Getenv("CHECK_INTERVAL")); raw != "" {
		if seconds, err := strconv.ParseFloat(raw, 64); err == nil && seconds > 0 && seconds <= 365*24*60*60 {
			interval = time.Duration(seconds * float64(time.Second))
		}
	}
	path := strings.TrimSpace(os.Getenv("HEARTBEAT_FILE"))
	if path == "" {
		path = health.DefaultPath()
	}
	maxAge := 2*interval + 30*time.Second
	if maxAge < time.Minute {
		maxAge = time.Minute
	}
	if err := health.Check(path, maxAge); err != nil {
		fmt.Fprintf(os.Stderr, "unhealthy: %v\n", err)
		return 1
	}
	return 0
}
