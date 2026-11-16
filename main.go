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
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Target struct { Name string; Addr string }
type MonitorConfig struct {
	Targets []Target
	BotToken string
	ChatID string
	Interval time.Duration
	DNSTimeout time.Duration
	TelegramTimeout time.Duration
	TestDomain string
	Router RouterConfig
	RecoveryEnabled bool
}
type State struct { Known bool; Healthy bool }

func envDuration(name, fallback string) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" { raw = fallback }
	seconds, err := strconv.ParseFloat(raw, 64)
	const maxSeconds = float64(math.MaxInt64) / float64(time.Second)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > maxSeconds {
		return 0, fmt.Errorf("%s must be a finite positive number of seconds within the time.Duration range", name)
	}
	duration := time.Duration(seconds * float64(time.Second))
	if duration <= 0 { return 0, fmt.Errorf("%s is below the supported duration resolution", name) }
	return duration, nil
}

func loadConfig() (MonitorConfig, error) {
	var cfg MonitorConfig
	rawTargets := strings.TrimSpace(os.Getenv("TARGETS"))
	if rawTargets == "" { return cfg, errors.New("TARGETS is required (example: home=192.168.1.137,backup=192.168.1.46)") }
	seen := make(map[string]bool)
	for _, item := range strings.Split(rawTargets, ",") {
		item = strings.TrimSpace(item)
		if item == "" { continue }
		name, endpoint, found := strings.Cut(item, "=")
		if !found { endpoint, name = item, item }
		name, endpoint = strings.TrimSpace(name), strings.TrimSpace(endpoint)
		if name == "" || endpoint == "" { return cfg, fmt.Errorf("invalid TARGETS entry %q", item) }
		host, port, err := net.SplitHostPort(endpoint)
		if err != nil {
			if ip := net.ParseIP(endpoint); ip != nil { host, port = endpoint, "53"
			} else if strings.HasPrefix(endpoint, "[") && strings.HasSuffix(endpoint, "]") { host, port = strings.Trim(endpoint, "[]"), "53"
			} else { return cfg, fmt.Errorf("TARGETS must contain IP addresses, not hostnames: %q", endpoint) }
		}
		if net.ParseIP(host) == nil { return cfg, fmt.Errorf("TARGETS must contain IP addresses, not hostnames: %q", host) }
		portNum, err := strconv.Atoi(port)
		if err != nil || portNum < 1 || portNum > 65535 { return cfg, fmt.Errorf("invalid port in target %q", endpoint) }
		addr := net.JoinHostPort(host, strconv.Itoa(portNum))
		if seen[addr] { return cfg, fmt.Errorf("duplicate DNS endpoint %q", addr) }
		seen[addr] = true
		cfg.Targets = append(cfg.Targets, Target{Name:name, Addr:addr})
	}
	if len(cfg.Targets) == 0 { return cfg, errors.New("TARGETS contains no valid endpoints") }
	cfg.BotToken = strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	cfg.ChatID = strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID"))
	if cfg.BotToken == "" || cfg.ChatID == "" { return cfg, errors.New("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID are required") }
	var err error
	cfg.Interval, err = envDuration("CHECK_INTERVAL", "60"); if err != nil { return cfg, err }
	cfg.DNSTimeout, err = envDuration("DNS_TIMEOUT", "3"); if err != nil { return cfg, err }
	cfg.TelegramTimeout, err = envDuration("TELEGRAM_TIMEOUT", "10"); if err != nil { return cfg, err }
	cfg.TestDomain = strings.TrimSuffix(strings.TrimSpace(os.Getenv("DNS_TEST_DOMAIN")), ".")
	if cfg.TestDomain == "" { cfg.TestDomain = "google.com" }
	if _, err := makeDNSQuery(cfg.TestDomain, 1); err != nil { return cfg, fmt.Errorf("invalid DNS_TEST_DOMAIN: %w", err) }
	routerPassword := strings.TrimSpace(os.Getenv("ROUTER_PASSWORD"))
	cfg.RecoveryEnabled = routerPassword != ""
	if cfg.RecoveryEnabled {
		cfg.Router = RouterConfig{
			URL: strings.TrimSpace(os.Getenv("ROUTER_URL")), Username: strings.TrimSpace(os.Getenv("ROUTER_USERNAME")),
			Password: routerPassword, DNSPrimary: strings.TrimSpace(os.Getenv("DNS_PRIMARY")),
			DNSSecondary: strings.TrimSpace(os.Getenv("DNS_SECONDARY")),
		}
		if cfg.Router.URL == "" { cfg.Router.URL = "http://192.168.1.1" }
		if cfg.Router.Username == "" { cfg.Router.Username = "1234" }
		if cfg.Router.DNSPrimary == "" { cfg.Router.DNSPrimary = "1.1.1.1" }
		if cfg.Router.DNSSecondary == "" { cfg.Router.DNSSecondary = "8.8.8.8" }
		cfg.Router.WiFiOff, err = envDuration("WIFI_OFF_SECONDS", "10"); if err != nil { return cfg, err }
		cfg.Router.Cooldown, err = envDuration("RECOVERY_COOLDOWN", "900"); if err != nil { return cfg, err }
		cfg.Router.FailureThreshold = 3
	}
	return cfg, nil
}

func makeDNSQuery(domain string, id uint16) ([]byte, error) {
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" || len(domain) > 253 { return nil, fmt.Errorf("invalid DNS name length: %d bytes", len(domain)) }
	packet := make([]byte, 12)
	binary.BigEndian.PutUint16(packet[0:2], id)
	binary.BigEndian.PutUint16(packet[2:4], 0x0100)
	binary.BigEndian.PutUint16(packet[4:6], 1)
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 { return nil, fmt.Errorf("invalid DNS label %q", label) }
		packet = append(packet, byte(len(label)))
		packet = append(packet, label...)
	}
	packet = append(packet, 0, 0, 1, 0, 1)
	return packet, nil
}

// skipDNSName advances over a possibly compressed DNS name and returns its
// end offset. It rejects malformed compression pointers and out-of-range data.
func skipDNSName(packet []byte, offset int) (int, error) {
	if offset >= len(packet) { return 0, errors.New("DNS name exceeds packet") }
	cursor, end, jumps := offset, -1, 0
	for {
		if cursor >= len(packet) { return 0, errors.New("DNS name exceeds packet") }
		length := int(packet[cursor])
		switch length & 0xC0 {
		case 0x00:
			cursor++
			if length == 0 {
				if end < 0 { end = cursor }
				return end, nil
			}
			if length > 63 || cursor+length > len(packet) { return 0, errors.New("invalid DNS label length") }
			cursor += length
		case 0xC0:
			if cursor+1 >= len(packet) { return 0, errors.New("truncated DNS compression pointer") }
			pointer := ((length & 0x3F) << 8) | int(packet[cursor+1])
			if pointer >= len(packet) { return 0, errors.New("DNS compression pointer out of range") }
			if end < 0 { end = cursor + 2 }
			cursor = pointer
			jumps++
			if jumps > len(packet) { return 0, errors.New("DNS compression pointer loop") }
		default:
			return 0, errors.New("reserved DNS label encoding")
		}
	}
}

func normalizeDNSName(packet []byte, offset int) (string, int, error) {
	cursor := offset
	parts := make([]string, 0, 4)
	jumps := 0
	for {
		if cursor >= len(packet) { return "", 0, errors.New("DNS name exceeds packet") }
		length := int(packet[cursor])
		switch length & 0xC0 {
		case 0x00:
			cursor++
			if length == 0 { return strings.ToLower(strings.Join(parts, ".")), cursor, nil }
			if length > 63 || cursor+length > len(packet) { return "", 0, errors.New("invalid DNS label length") }
			parts = append(parts, string(packet[cursor:cursor+length]))
			cursor += length
		case 0xC0:
			if cursor+1 >= len(packet) { return "", 0, errors.New("truncated DNS compression pointer") }
			pointer := ((length & 0x3F) << 8) | int(packet[cursor+1])
			if pointer >= len(packet) { return "", 0, errors.New("DNS compression pointer out of range") }
			cursor = pointer
			jumps++
			if jumps > len(packet) { return "", 0, errors.New("DNS compression pointer loop") }
		default:
			return "", 0, errors.New("reserved DNS label encoding")
		}
	}
}

func validateDNSResponse(query, response []byte) error {
	if len(query) < 17 || len(response) < 12 { return errors.New("DNS packet too short") }
	if binary.BigEndian.Uint16(response[0:2]) != binary.BigEndian.Uint16(query[0:2]) { return errors.New("DNS response ID mismatch") }
	flags := binary.BigEndian.Uint16(response[2:4])
	if flags&0x8000 == 0 { return errors.New("packet is not a DNS response") }
	if flags&0x7800 != 0 { return errors.New("DNS response has unexpected opcode") }
	if flags&0x0200 != 0 { return errors.New("truncated DNS response") }
	if rcode := flags & 0x000f; rcode != 0 { return fmt.Errorf("DNS response code: %d", rcode) }
	qdCount := int(binary.BigEndian.Uint16(response[4:6]))
	anCount := int(binary.BigEndian.Uint16(response[6:8]))
	if qdCount != 1 { return fmt.Errorf("DNS response question count = %d, want 1", qdCount) }
	queryName, _, err := normalizeDNSName(query, 12)
	if err != nil { return fmt.Errorf("invalid query name: %w", err) }
	responseName, offset, err := normalizeDNSName(response, 12)
	if err != nil { return fmt.Errorf("invalid response question: %w", err) }
	if responseName != queryName || offset+4 > len(response) { return errors.New("DNS response question does not match query") }
	qtype := binary.BigEndian.Uint16(response[offset:offset+2])
	qclass := binary.BigEndian.Uint16(response[offset+2:offset+4])
	if qtype != binary.BigEndian.Uint16(query[len(query)-4:len(query)-2]) || qclass != binary.BigEndian.Uint16(query[len(query)-2:]) { return errors.New("DNS response question type/class does not match query") }
	if anCount == 0 { return errors.New("DNS response contains no answer") }

	// Walk all answer records and require at least one valid A record for an A
	// query. CNAME-only responses are valid only when followed by a terminal A.
	cursor := offset + 4
	answerIPs := 0
	for i := 0; i < anCount; i++ {
		next, err := skipDNSName(response, cursor)
		if err != nil { return fmt.Errorf("invalid answer owner name: %w", err) }
		if next+10 > len(response) { return errors.New("truncated DNS answer header") }
		typ := binary.BigEndian.Uint16(response[next:next+2])
		class := binary.BigEndian.Uint16(response[next+2:next+4])
		rdLength := int(binary.BigEndian.Uint16(response[next+8:next+10]))
		rdata := next + 10
		if rdata+rdLength > len(response) { return errors.New("truncated DNS answer data") }
		if typ == 1 && class == 1 && rdLength == net.IPv4len { answerIPs++ }
		cursor = rdata + rdLength
	}
	if answerIPs == 0 { return errors.New("DNS response contains no valid A record") }
	return nil
}

func checkDNS(target Target, domain string, timeout time.Duration) (bool, string) {
	id := uint16(time.Now().UnixNano())
	query, err := makeDNSQuery(domain, id)
	if err != nil { return false, err.Error() }
	conn, err := net.DialTimeout("udp", target.Addr, timeout)
	if err != nil { return false, err.Error() }
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(query); err != nil { return false, err.Error() }
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil { return false, err.Error() }
	if err := validateDNSResponse(query, buf[:n]); err != nil { return false, err.Error() }
	return true, "DNS query successful"
}

type Telegram struct { token string; chatID string; client *http.Client }
func newTelegram(cfg MonitorConfig) *Telegram { return &Telegram{token:cfg.BotToken, chatID:cfg.ChatID, client:&http.Client{Timeout:cfg.TelegramTimeout}} }
func (t *Telegram) send(message string) error {
	body, err := json.Marshal(map[string]any{"chat_id":t.chatID, "text":message, "disable_web_page_preview":true})
	if err != nil { return err }
	req, err := http.NewRequest(http.MethodPost, "https://api.telegram.org/bot"+t.token+"/sendMessage", bytes.NewReader(body))
	if err != nil { return err }
	req.Header.Set("Content-Type","application/json")
	resp, err := t.client.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return fmt.Errorf("Telegram returned HTTP %s", resp.Status) }
	var result struct { OK bool `json:"ok"`; Description string `json:"description"` }
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil { return err }
	if !result.OK { return fmt.Errorf("Telegram API: %s", result.Description) }
	return nil
}
func notify(tg *Telegram, message string) { if err := tg.send(message); err != nil { log.Printf("ERROR Telegram notification failed: %v", err) } }

func run(ctx context.Context, cfg MonitorConfig) {
	tg := newTelegram(cfg)
	states := make(map[string]State, len(cfg.Targets))
	var lines []string
	var router *AskeyRouter
	if cfg.RecoveryEnabled { router = newAskeyRouter(cfg.Router); log.Printf("Askey router recovery enabled for %s", cfg.Router.URL) } else { log.Printf("Askey router recovery disabled: ROUTER_PASSWORD is not set") }
	consecutiveAllDown := 0
	lastRecovery := time.Time{}
	recoveryRunning := false
	for _, target := range cfg.Targets { lines = append(lines, fmt.Sprintf("• %s: %s", target.Name, target.Addr)) }
	label := "servidores DNS"
	if len(cfg.Targets) == 1 { label = "servidor DNS" }
	notify(tg, fmt.Sprintf("🟢 AdGuard DNS Monitor iniciado\nMonitorizando %d %s\n%s", len(cfg.Targets), label, strings.Join(lines,"\n")))
	check := func() {
		allDown := true
		for _, target := range cfg.Targets {
			healthy, detail := checkDNS(target, cfg.TestDomain, cfg.DNSTimeout)
			previous := states[target.Addr]
			states[target.Addr] = State{Known:true, Healthy:healthy}
			if healthy {
				allDown = false
				log.Printf("%s (%s): OK", target.Name, target.Addr)
				if previous.Known && !previous.Healthy { notify(tg, fmt.Sprintf("🟢 AdGuard DNS RECUPERADO\nInstancia: %s\nIP: %s\nPrueba: %s\nEstado: DNS operativo", target.Name,target.Addr,cfg.TestDomain)) }
			} else {
				log.Printf("WARN %s (%s): FAILED - %s", target.Name,target.Addr,detail)
				if !previous.Known || previous.Healthy { notify(tg,fmt.Sprintf("🔴 FALLO AdGuard DNS\nInstancia: %s\nIP: %s\nPrueba: %s\nError: %s",target.Name,target.Addr,cfg.TestDomain,detail)) }
			}
		}
		if router != nil {
			if allDown { consecutiveAllDown++ } else { consecutiveAllDown = 0 }
			if allDown && consecutiveAllDown >= cfg.Router.FailureThreshold && !recoveryRunning && time.Since(lastRecovery) >= cfg.Router.Cooldown {
				recoveryRunning = true
				lastRecovery = time.Now()
				notify(tg, fmt.Sprintf("🚨 ADGuard: todos los DNS monitorizados están caídos. Iniciando recuperación del router Askey (%s).", cfg.Router.URL))
				recoveryCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
				err := router.Recover(recoveryCtx)
				cancel()
				recoveryRunning = false
				if err != nil {
					log.Printf("ERROR Askey router recovery failed: %v", err)
					notify(tg, fmt.Sprintf("🔴 ERROR recuperación router Askey\n%s", err))
				} else {
					log.Printf("Askey router recovery completed: DNS=%s,%s; Wi-Fi radios restarted", cfg.Router.DNSPrimary, cfg.Router.DNSSecondary)
					notify(tg, fmt.Sprintf("🟢 Recuperación router Askey completada\nDNS DHCP: %s / %s\nWi-Fi 2,4 y 5 GHz reiniciados", cfg.Router.DNSPrimary, cfg.Router.DNSSecondary))
				}
			}
		}
	}
	check()
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for { select { case <-ctx.Done(): notify(tg,"⚪ AdGuard DNS Monitor detenido"); return; case <-ticker.C: check() } }
}
func main() {
	log.SetFlags(log.Ldate|log.Ltime|log.LUTC)
	cfg, err := loadConfig()
	if err != nil { log.Fatalf("Configuration error: %v",err) }
	log.Printf("Starting AdGuard Home monitor: %d target(s), interval=%s, query=%s",len(cfg.Targets),cfg.Interval,cfg.TestDomain)
	for _, target := range cfg.Targets { log.Printf("Configured target: %s (%s)",target.Name,target.Addr) }
	ctx, stop := signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM)
	defer stop()
	run(ctx,cfg)
}
