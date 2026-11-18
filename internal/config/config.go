// Package config loads and validates runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/uniextra/Adguardhome-DRP/internal/dns"
	"github.com/uniextra/Adguardhome-DRP/internal/recovery"
)

// MonitorConfig holds all validated configuration for the DNS monitor service.
type MonitorConfig struct {
	Targets         []dns.Target
	BotToken        string
	ChatID          string
	Interval        time.Duration
	DNSTimeout      time.Duration
	TelegramTimeout time.Duration
	TestDomain      string
	Router          recovery.RouterConfig
	RecoveryEnabled bool
}

func envDuration(name, fallback string) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		raw = fallback
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	const maxDurationNanos = float64(1 << 63)
	nanos := seconds * float64(time.Second)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || nanos >= maxDurationNanos {
		return 0, fmt.Errorf("%s must be a finite positive number of seconds within the time.Duration range", name)
	}
	duration := time.Duration(nanos)
	if duration <= 0 {
		return 0, fmt.Errorf("%s is below the supported duration resolution", name)
	}
	return duration, nil
}

func parseTargets(rawTargets string) ([]dns.Target, error) {
	if rawTargets == "" {
		return nil, errors.New("TARGETS is required (example: home=192.168.1.137,backup=192.168.1.46)")
	}
	seen := make(map[string]bool)
	var targets []dns.Target
	for _, item := range strings.Split(rawTargets, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		name, endpoint, found := strings.Cut(item, "=")
		if !found {
			endpoint, name = item, item
		}
		name, endpoint = strings.TrimSpace(name), strings.TrimSpace(endpoint)
		if name == "" || endpoint == "" {
			return nil, fmt.Errorf("invalid TARGETS entry %q", item)
		}
		host, port, err := net.SplitHostPort(endpoint)
		if err != nil {
			if ip := net.ParseIP(endpoint); ip != nil {
				host, port = endpoint, "53"
			} else if strings.HasPrefix(endpoint, "[") && strings.HasSuffix(endpoint, "]") {
				host, port = strings.Trim(endpoint, "[]"), "53"
			} else {
				return nil, fmt.Errorf("TARGETS must contain IP addresses, not hostnames: %q", endpoint)
			}
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return nil, fmt.Errorf("TARGETS must contain IP addresses, not hostnames: %q", host)
		}
		portNum, err := strconv.Atoi(port)
		if err != nil || portNum < 1 || portNum > 65535 {
			return nil, fmt.Errorf("invalid port in target %q", endpoint)
		}
		addr := net.JoinHostPort(ip.String(), strconv.Itoa(portNum))
		if seen[addr] {
			return nil, fmt.Errorf("duplicate DNS endpoint %q", addr)
		}
		seen[addr] = true
		targets = append(targets, dns.Target{Name: name, Addr: addr})
	}
	if len(targets) == 0 {
		return nil, errors.New("TARGETS contains no valid endpoints")
	}
	return targets, nil
}

// Load reads and validates MonitorConfig from environment variables.
func Load() (MonitorConfig, error) {
	var cfg MonitorConfig
	targets, err := parseTargets(strings.TrimSpace(os.Getenv("TARGETS")))
	if err != nil {
		return cfg, err
	}
	cfg.Targets = targets

	cfg.BotToken = strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	cfg.ChatID = strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID"))
	if cfg.BotToken == "" || cfg.ChatID == "" {
		return cfg, errors.New("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID are required")
	}
	if cfg.Interval, err = envDuration("CHECK_INTERVAL", "60"); err != nil {
		return cfg, err
	}
	if cfg.DNSTimeout, err = envDuration("DNS_TIMEOUT", "3"); err != nil {
		return cfg, err
	}
	if cfg.TelegramTimeout, err = envDuration("TELEGRAM_TIMEOUT", "10"); err != nil {
		return cfg, err
	}

	cfg.TestDomain = strings.TrimSuffix(strings.TrimSpace(os.Getenv("DNS_TEST_DOMAIN")), ".")
	if cfg.TestDomain == "" {
		cfg.TestDomain = "google.com"
	}
	if _, err := dns.MakeQuery(cfg.TestDomain, 1); err != nil {
		return cfg, fmt.Errorf("invalid DNS_TEST_DOMAIN: %w", err)
	}

	routerPassword := strings.TrimSpace(os.Getenv("ROUTER_PASSWORD"))
	routerModel := recovery.NormalizeRouterModel(os.Getenv("ROUTER_MODEL"))
	cfg.Router.Model = routerModel
	cfg.RecoveryEnabled = routerPassword != ""
	if cfg.RecoveryEnabled {
		cfg.Router = recovery.RouterConfig{
			Model:        routerModel,
			URL:          strings.TrimSpace(os.Getenv("ROUTER_URL")),
			Username:     strings.TrimSpace(os.Getenv("ROUTER_USERNAME")),
			Password:     routerPassword,
			DNSPrimary:   strings.TrimSpace(os.Getenv("DNS_PRIMARY")),
			DNSSecondary: strings.TrimSpace(os.Getenv("DNS_SECONDARY")),
		}
		if cfg.Router.URL == "" {
			cfg.Router.URL = "http://192.168.1.1"
		}
		parsedRouterURL, parseErr := url.Parse(cfg.Router.URL)
		if parseErr != nil || parsedRouterURL.Host == "" || (parsedRouterURL.Scheme != "http" && parsedRouterURL.Scheme != "https") || parsedRouterURL.User != nil {
			return cfg, errors.New("ROUTER_URL must be an absolute HTTP(S) URL without embedded credentials")
		}
		if cfg.Router.Username == "" {
			cfg.Router.Username = "1234"
		}
		if cfg.Router.DNSPrimary == "" {
			cfg.Router.DNSPrimary = "1.1.1.1"
		}
		if cfg.Router.DNSSecondary == "" {
			cfg.Router.DNSSecondary = "8.8.8.8"
		}
		for _, setting := range []struct{ name, value string }{
			{"DNS_PRIMARY", cfg.Router.DNSPrimary},
			{"DNS_SECONDARY", cfg.Router.DNSSecondary},
		} {
			if net.ParseIP(setting.value) == nil {
				return cfg, fmt.Errorf("%s must be a single IP address, got %q", setting.name, setting.value)
			}
		}
		if cfg.Router.WiFiOff, err = envDuration("WIFI_OFF_SECONDS", "10"); err != nil {
			return cfg, err
		}
		if cfg.Router.Cooldown, err = envDuration("RECOVERY_COOLDOWN", "900"); err != nil {
			return cfg, err
		}
		cfg.Router.FailureThreshold = 3
		if _, err := recovery.NewRouterRecovery(cfg.Router); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}
