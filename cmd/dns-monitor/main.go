// Command dns-monitor periodically checks configured DNS servers, sends Telegram alerts,
// and optionally triggers automated router recovery on total DNS outage.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/uniextra/Adguardhome-DRP/internal/config"
	"github.com/uniextra/Adguardhome-DRP/internal/dns"
	"github.com/uniextra/Adguardhome-DRP/internal/monitor"
	"github.com/uniextra/Adguardhome-DRP/internal/notify"
	"github.com/uniextra/Adguardhome-DRP/internal/recovery"
)

func main() {
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
