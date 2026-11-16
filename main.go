package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Target struct {
	Name string
	Addr string
}
type MonitorConfig struct {
	Targets         []Target
	BotToken        string
	ChatID          string
	Interval        time.Duration
	DNSTimeout      time.Duration
	TelegramTimeout time.Duration
	TestDomain      string
	Router          RouterConfig
	RecoveryEnabled bool
}
type State struct {
	Known   bool
	Healthy bool
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

func loadConfig() (MonitorConfig, error) {
	var cfg MonitorConfig
	rawTargets := strings.TrimSpace(os.Getenv("TARGETS"))
	if rawTargets == "" {
		return cfg, errors.New("TARGETS is required (example: home=192.168.1.137,backup=192.168.1.46)")
	}
	seen := make(map[string]bool)
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
			return cfg, fmt.Errorf("invalid TARGETS entry %q", item)
		}
		host, port, err := net.SplitHostPort(endpoint)
		if err != nil {
			if ip := net.ParseIP(endpoint); ip != nil {
				host, port = endpoint, "53"
			} else if strings.HasPrefix(endpoint, "[") && strings.HasSuffix(endpoint, "]") {
				host, port = strings.Trim(endpoint, "[]"), "53"
			} else {
				return cfg, fmt.Errorf("TARGETS must contain IP addresses, not hostnames: %q", endpoint)
			}
		}
		if net.ParseIP(host) == nil {
			return cfg, fmt.Errorf("TARGETS must contain IP addresses, not hostnames: %q", host)
		}
		portNum, err := strconv.Atoi(port)
		if err != nil || portNum < 1 || portNum > 65535 {
			return cfg, fmt.Errorf("invalid port in target %q", endpoint)
		}
		addr := net.JoinHostPort(host, strconv.Itoa(portNum))
		if seen[addr] {
			return cfg, fmt.Errorf("duplicate DNS endpoint %q", addr)
		}
		seen[addr] = true
		cfg.Targets = append(cfg.Targets, Target{Name: name, Addr: addr})
	}
	if len(cfg.Targets) == 0 {
		return cfg, errors.New("TARGETS contains no valid endpoints")
	}
	var err error
	cfg.BotToken = strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	cfg.ChatID = strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID"))
	if cfg.BotToken == "" || cfg.ChatID == "" {
		return cfg, errors.New("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID are required")
	}
	cfg.Interval, err = envDuration("CHECK_INTERVAL", "60")
	if err != nil {
		return cfg, err
	}
	cfg.DNSTimeout, err = envDuration("DNS_TIMEOUT", "3")
	if err != nil {
		return cfg, err
	}
	cfg.TelegramTimeout, err = envDuration("TELEGRAM_TIMEOUT", "10")
	if err != nil {
		return cfg, err
	}
	cfg.TestDomain = strings.TrimSuffix(strings.TrimSpace(os.Getenv("DNS_TEST_DOMAIN")), ".")
	if cfg.TestDomain == "" {
		cfg.TestDomain = "google.com"
	}
	if _, err := makeDNSQuery(cfg.TestDomain, 1); err != nil {
		return cfg, fmt.Errorf("invalid DNS_TEST_DOMAIN: %w", err)
	}
	routerPassword := strings.TrimSpace(os.Getenv("ROUTER_PASSWORD"))
	routerModel := normalizeRouterModel(os.Getenv("ROUTER_MODEL"))
	cfg.Router.Model = routerModel
	cfg.RecoveryEnabled = routerPassword != ""
	if cfg.RecoveryEnabled {
		cfg.Router = RouterConfig{
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
		// These values are written verbatim into the router's DHCP settings during
		// an outage, so reject anything that is not a single IP address now rather
		// than discovering the typo in the middle of an incident.
		for _, setting := range []struct{ name, value string }{
			{"DNS_PRIMARY", cfg.Router.DNSPrimary},
			{"DNS_SECONDARY", cfg.Router.DNSSecondary},
		} {
			if net.ParseIP(setting.value) == nil {
				return cfg, fmt.Errorf("%s must be a single IP address, got %q", setting.name, setting.value)
			}
		}
		cfg.Router.WiFiOff, err = envDuration("WIFI_OFF_SECONDS", "10")
		if err != nil {
			return cfg, err
		}
		cfg.Router.Cooldown, err = envDuration("RECOVERY_COOLDOWN", "900")
		if err != nil {
			return cfg, err
		}
		cfg.Router.FailureThreshold = 3
		// Validate model support before the monitor starts. Unknown models must
		// never fall through to another manufacturer's router implementation.
		if _, err := newRouterRecovery(cfg.Router); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func makeDNSQuery(domain string, id uint16) ([]byte, error) {
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" || len(domain) > 253 {
		return nil, fmt.Errorf("invalid DNS name length: %d bytes", len(domain))
	}
	packet := make([]byte, 12)
	binary.BigEndian.PutUint16(packet[0:2], id)
	binary.BigEndian.PutUint16(packet[2:4], 0x0100)
	binary.BigEndian.PutUint16(packet[4:6], 1)
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 {
			return nil, fmt.Errorf("invalid DNS label %q", label)
		}
		packet = append(packet, byte(len(label)))
		packet = append(packet, label...)
	}
	packet = append(packet, 0, 0, 1, 0, 1)
	return packet, nil
}

// parseDNSName decodes a DNS name, including compression pointers. nextOffset
// is the position immediately after the encoded name in the original stream.
func parseDNSName(packet []byte, offset int) (string, int, error) {
	if offset < 0 || offset >= len(packet) {
		return "", 0, errors.New("DNS name exceeds packet")
	}
	cursor, nextOffset := offset, -1
	labels := make([]string, 0, 4)
	visited := make(map[int]bool)
	expandedLength := 1
	for {
		if cursor >= len(packet) {
			return "", 0, errors.New("DNS name exceeds packet")
		}
		length := int(packet[cursor])
		switch length & 0xC0 {
		case 0x00:
			cursor++
			if length == 0 {
				if nextOffset < 0 {
					nextOffset = cursor
				}
				return strings.ToLower(strings.Join(labels, ".")), nextOffset, nil
			}
			if length > 63 || cursor+length > len(packet) {
				return "", 0, errors.New("invalid DNS label length")
			}
			expandedLength += length + 1
			if expandedLength > 255 {
				return "", 0, errors.New("expanded DNS name exceeds 255 bytes")
			}
			labels = append(labels, string(packet[cursor:cursor+length]))
			cursor += length
		case 0xC0:
			if cursor+1 >= len(packet) {
				return "", 0, errors.New("truncated DNS compression pointer")
			}
			pointer := ((length & 0x3F) << 8) | int(packet[cursor+1])
			if pointer >= len(packet) {
				return "", 0, errors.New("DNS compression pointer out of range")
			}
			if nextOffset < 0 {
				nextOffset = cursor + 2
			}
			if visited[pointer] {
				return "", 0, errors.New("DNS compression pointer loop")
			}
			visited[pointer] = true
			cursor = pointer
		default:
			return "", 0, errors.New("reserved DNS label encoding")
		}
	}
}

func validateDNSResponse(query, response []byte) error {
	if len(query) < 17 || len(response) < 12 {
		return errors.New("DNS packet too short")
	}
	if binary.BigEndian.Uint16(response[0:2]) != binary.BigEndian.Uint16(query[0:2]) {
		return errors.New("DNS response ID mismatch")
	}
	flags := binary.BigEndian.Uint16(response[2:4])
	if flags&0x8000 == 0 {
		return errors.New("packet is not a DNS response")
	}
	if flags&0x7800 != 0 {
		return errors.New("DNS response has unexpected opcode")
	}
	if flags&0x0200 != 0 {
		return errors.New("truncated DNS response")
	}
	if rcode := flags & 0x000f; rcode != 0 {
		return fmt.Errorf("DNS response code: %d", rcode)
	}
	if flags&0x0040 != 0 {
		return errors.New("DNS response has reserved Z flag set")
	}
	qdCount := int(binary.BigEndian.Uint16(response[4:6]))
	anCount := int(binary.BigEndian.Uint16(response[6:8]))
	if qdCount != 1 {
		return fmt.Errorf("DNS response question count = %d, want 1", qdCount)
	}
	queryName, _, err := parseDNSName(query, 12)
	if err != nil {
		return fmt.Errorf("invalid query name: %w", err)
	}
	responseName, offset, err := parseDNSName(response, 12)
	if err != nil {
		return fmt.Errorf("invalid response question: %w", err)
	}
	if responseName != queryName || offset+4 > len(response) {
		return errors.New("DNS response question does not match query")
	}
	qtype := binary.BigEndian.Uint16(response[offset : offset+2])
	qclass := binary.BigEndian.Uint16(response[offset+2 : offset+4])
	if qtype != binary.BigEndian.Uint16(query[len(query)-4:len(query)-2]) || qclass != binary.BigEndian.Uint16(query[len(query)-2:]) {
		return errors.New("DNS response question type/class does not match query")
	}
	if qtype != 1 || qclass != 1 {
		return errors.New("DNS response query is not an A/IN question")
	}
	if anCount == 0 {
		return errors.New("DNS response contains no answer")
	}

	// Follow the CNAME chain and require an A record for the queried name or
	// one of its aliases; unrelated A records do not make the query healthy.
	cursor := offset + 4
	aOwners := make(map[string]bool)
	cnameTargets := make(map[string]string)
	for i := 0; i < anCount; i++ {
		owner, next, err := parseDNSName(response, cursor)
		if err != nil {
			return fmt.Errorf("invalid answer owner name: %w", err)
		}
		if next+10 > len(response) {
			return errors.New("truncated DNS answer header")
		}
		typ := binary.BigEndian.Uint16(response[next : next+2])
		class := binary.BigEndian.Uint16(response[next+2 : next+4])
		rdLength := int(binary.BigEndian.Uint16(response[next+8 : next+10]))
		rdata := next + 10
		if rdata+rdLength > len(response) {
			return errors.New("truncated DNS answer data")
		}
		if class == 1 && typ == 1 {
			if rdLength != net.IPv4len {
				return errors.New("invalid A record length")
			}
			aOwners[owner] = true
		} else if class == 1 && typ == 5 {
			target, consumed, err := parseDNSName(response, rdata)
			if err != nil || consumed != rdata+rdLength {
				return errors.New("invalid CNAME answer data")
			}
			if previous, exists := cnameTargets[owner]; exists && previous != target {
				return errors.New("conflicting CNAME records for answer owner")
			}
			cnameTargets[owner] = target
		}
		cursor = rdata + rdLength
	}
	current := queryName
	visited := make(map[string]bool)
	for {
		if aOwners[current] {
			return nil
		}
		if visited[current] {
			return errors.New("DNS CNAME loop")
		}
		visited[current] = true
		target, ok := cnameTargets[current]
		if !ok {
			return errors.New("DNS response contains no A record for the queried name or its CNAME chain")
		}
		current = target
	}
}

func checkDNS(target Target, domain string, timeout time.Duration) (bool, string) {
	id := uint16(time.Now().UnixNano())
	query, err := makeDNSQuery(domain, id)
	if err != nil {
		return false, err.Error()
	}
	conn, err := net.DialTimeout("udp", target.Addr, timeout)
	if err != nil {
		return false, err.Error()
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(query); err != nil {
		return false, err.Error()
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return false, err.Error()
	}
	if err := validateDNSResponse(query, buf[:n]); err != nil {
		return false, err.Error()
	}
	return true, "DNS query successful"
}

// fallbackDNSAnswers reports whether at least one of the resolvers that a
// recovery would install in the router's DHCP settings answers the monitor's
// test query. When none does, the outage is upstream of the LAN (ISP/WAN):
// pointing DHCP at those resolvers and restarting Wi-Fi cannot fix that, and
// would only disconnect every Wi-Fi client for nothing.
func fallbackDNSAnswers(cfg MonitorConfig, probe func(Target, string, time.Duration) (bool, string)) (bool, string) {
	var failures []string
	var probed []string
	for _, ip := range []string{cfg.Router.DNSPrimary, cfg.Router.DNSSecondary} {
		if ip == "" || slices.Contains(probed, ip) {
			continue
		}
		probed = append(probed, ip)
		healthy, detail := probe(Target{Name: ip, Addr: net.JoinHostPort(ip, "53")}, cfg.TestDomain, cfg.DNSTimeout)
		if healthy {
			return true, ""
		}
		failures = append(failures, fmt.Sprintf("%s: %s", ip, detail))
	}
	return false, strings.Join(failures, "; ")
}

type Telegram struct {
	token  string
	chatID string
	client *http.Client
}

func newTelegram(cfg MonitorConfig) *Telegram {
	return &Telegram{token: cfg.BotToken, chatID: cfg.ChatID, client: &http.Client{Timeout: cfg.TelegramTimeout}}
}

// telegramMaxMessageRunes keeps messages safely below the 4096-character limit
// of the Telegram Bot API, which rejects longer texts outright.
const telegramMaxMessageRunes = 4000

// truncateRunes shortens s to at most max runes, marking the cut with an ellipsis.
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if max < 1 || len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// redact removes the bot token from an error. net/http wraps failures in a
// *url.Error whose text embeds the full request URL, and the token is part of
// that URL, so logging the raw error would leak the credential.
func (t *Telegram) redact(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	text := err.Error()
	if t.token != "" {
		text = strings.ReplaceAll(text, t.token, "[REDACTED]")
	}
	return errors.New("Telegram request failed: " + text)
}

func (t *Telegram) send(message string) error {
	body, err := json.Marshal(map[string]any{"chat_id": t.chatID, "text": truncateRunes(message, telegramMaxMessageRunes), "disable_web_page_preview": true})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.telegram.org/bot"+t.token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return t.redact(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return t.redact(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Telegram returned HTTP %s", resp.Status)
	}
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("Telegram API: %s", result.Description)
	}
	return nil
}
func notify(tg *Telegram, message string) {
	if err := tg.send(message); err != nil {
		log.Printf("ERROR Telegram notification failed: %v", err)
	}
}

func run(ctx context.Context, cfg MonitorConfig) {
	tg := newTelegram(cfg)
	states := make(map[string]State, len(cfg.Targets))
	var lines []string
	var router RouterRecovery
	if cfg.RecoveryEnabled {
		var err error
		router, err = newRouterRecovery(cfg.Router)
		if err != nil {
			// loadConfig validates this already; keep runtime fail-safe if a
			// caller constructs MonitorConfig directly in an embedded use.
			log.Printf("ERROR router recovery disabled: %v", err)
			notify(tg, fmt.Sprintf("🔴 Recuperación del router desactivada: %v", err))
		} else {
			log.Printf("Router recovery enabled: model=%s url=%s", cfg.Router.Model, cfg.Router.URL)
			if strings.HasPrefix(strings.ToLower(cfg.Router.URL), "http://") {
				log.Printf("WARN ROUTER_URL uses plain HTTP: router credentials and session data are not encrypted in transit")
			}
		}
	} else {
		log.Printf("Router recovery disabled: ROUTER_PASSWORD is not set")
	}
	consecutiveAllDown := 0
	lastRecovery := time.Time{}
	fallbackSkipNotified := false
	for _, target := range cfg.Targets {
		lines = append(lines, fmt.Sprintf("• %s: %s", target.Name, target.Addr))
	}
	label := "servidores DNS"
	if len(cfg.Targets) == 1 {
		label = "servidor DNS"
	}
	notify(tg, fmt.Sprintf("🟢 AdGuard DNS Monitor iniciado\nMonitorizando %d %s\n%s", len(cfg.Targets), label, strings.Join(lines, "\n")))
	check := func() {
		allDown := true
		for _, target := range cfg.Targets {
			healthy, detail := checkDNS(target, cfg.TestDomain, cfg.DNSTimeout)
			previous := states[target.Addr]
			states[target.Addr] = State{Known: true, Healthy: healthy}
			if healthy {
				allDown = false
				log.Printf("%s (%s): OK", target.Name, target.Addr)
				if previous.Known && !previous.Healthy {
					notify(tg, fmt.Sprintf("🟢 AdGuard DNS RECUPERADO\nInstancia: %s\nIP: %s\nPrueba: %s\nEstado: DNS operativo", target.Name, target.Addr, cfg.TestDomain))
				}
			} else {
				log.Printf("WARN %s (%s): FAILED - %s", target.Name, target.Addr, detail)
				if !previous.Known || previous.Healthy {
					notify(tg, fmt.Sprintf("🔴 FALLO AdGuard DNS\nInstancia: %s\nIP: %s\nPrueba: %s\nError: %s", target.Name, target.Addr, cfg.TestDomain, detail))
				}
			}
		}
		if router != nil {
			if allDown {
				consecutiveAllDown++
			} else {
				consecutiveAllDown = 0
				fallbackSkipNotified = false
			}
			if allDown && consecutiveAllDown >= cfg.Router.FailureThreshold && time.Since(lastRecovery) >= cfg.Router.Cooldown {
				if answers, detail := fallbackDNSAnswers(cfg, checkDNS); !answers {
					// Not an AdGuard problem: the public resolvers are unreachable too.
					// Leave the router alone and look again on the next cycle; the
					// cooldown is only consumed by a recovery that actually starts.
					log.Printf("WARN router recovery skipped (model=%s): fallback DNS %s / %s do not answer either (%s)", cfg.Router.Model, cfg.Router.DNSPrimary, cfg.Router.DNSSecondary, detail)
					if !fallbackSkipNotified {
						fallbackSkipNotified = true
						notify(tg, fmt.Sprintf("🟠 Todos los DNS monitorizados están caídos, pero los DNS de respaldo (%s / %s) tampoco responden: parece una caída de internet, no de AdGuard. No se modifica el router; se reintentará en cada comprobación.", cfg.Router.DNSPrimary, cfg.Router.DNSSecondary))
					}
				} else {
					fallbackSkipNotified = false
					lastRecovery = time.Now()
					notify(tg, fmt.Sprintf("🚨 AdGuard: todos los DNS monitorizados están caídos. Iniciando recuperación del router %s (%s).", cfg.Router.Model, cfg.Router.URL))
					recoveryCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
					err := router.Recover(recoveryCtx)
					cancel()
					if err != nil {
						log.Printf("ERROR router recovery failed (model=%s): %v", cfg.Router.Model, err)
						notify(tg, fmt.Sprintf("🔴 ERROR recuperación router %s\n%s", cfg.Router.Model, err))
					} else {
						log.Printf("Router recovery completed (model=%s): DNS=%s,%s", cfg.Router.Model, cfg.Router.DNSPrimary, cfg.Router.DNSSecondary)
						notify(tg, fmt.Sprintf("🟢 Recuperación del router %s completada\nDNS DHCP: %s / %s", cfg.Router.Model, cfg.Router.DNSPrimary, cfg.Router.DNSSecondary))
					}
				}
			}
		}
	}
	check()
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			notify(tg, "⚪ AdGuard DNS Monitor detenido")
			return
		case <-ticker.C:
			check()
		}
	}
}
func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}
	log.Printf("Starting AdGuard Home monitor: %d target(s), interval=%s, query=%s", len(cfg.Targets), cfg.Interval, cfg.TestDomain)
	for _, target := range cfg.Targets {
		log.Printf("Configured target: %s (%s)", target.Name, target.Addr)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	run(ctx, cfg)
}
