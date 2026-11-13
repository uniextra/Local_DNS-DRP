package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
}
type State struct { Known bool; Healthy bool }

func envDuration(name, fallback string) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name)); if raw == "" { raw = fallback }
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || seconds <= 0 { return 0, fmt.Errorf("%s must be a positive number of seconds", name) }
	return time.Duration(seconds * float64(time.Second)), nil
}

func loadConfig() (MonitorConfig, error) {
	var cfg MonitorConfig
	rawTargets := strings.TrimSpace(os.Getenv("TARGETS"))
	if rawTargets == "" { return cfg, errors.New("TARGETS is required (example: home=192.168.1.137,backup=192.168.1.46)") }
	seen := make(map[string]bool)
	for _, item := range strings.Split(rawTargets, ",") {
		item = strings.TrimSpace(item); if item == "" { continue }
		name, endpoint, found := strings.Cut(item, "="); if !found { endpoint, name = item, item }
		name, endpoint = strings.TrimSpace(name), strings.TrimSpace(endpoint)
		if name == "" || endpoint == "" { return cfg, fmt.Errorf("invalid TARGETS entry %q", item) }
		host, port, err := net.SplitHostPort(endpoint)
		if err != nil {
			if ip := net.ParseIP(endpoint); ip != nil { host, port = endpoint, "53"
			} else if strings.HasPrefix(endpoint, "[") && strings.HasSuffix(endpoint, "]") { host, port = strings.Trim(endpoint, "[]"), "53"
			} else { return cfg, fmt.Errorf("invalid DNS endpoint %q; use IP or IP:port", endpoint) }
		}
		if net.ParseIP(host) == nil { return cfg, fmt.Errorf("TARGETS must contain IP addresses, not hostnames: %q", host) }
		portNum, err := strconv.Atoi(port); if err != nil || portNum < 1 || portNum > 65535 { return cfg, fmt.Errorf("invalid port in target %q", endpoint) }
		addr := net.JoinHostPort(host, strconv.Itoa(portNum)); if seen[addr] { return cfg, fmt.Errorf("duplicate DNS endpoint %q", addr) }; seen[addr] = true
		cfg.Targets = append(cfg.Targets, Target{Name:name, Addr:addr})
	}
	if len(cfg.Targets) == 0 { return cfg, errors.New("TARGETS contains no valid endpoints") }
	cfg.BotToken = strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")); cfg.ChatID = strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID"))
	if cfg.BotToken == "" || cfg.ChatID == "" { return cfg, errors.New("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID are required") }
	var err error
	cfg.Interval, err = envDuration("CHECK_INTERVAL", "60"); if err != nil { return cfg, err }
	cfg.DNSTimeout, err = envDuration("DNS_TIMEOUT", "3"); if err != nil { return cfg, err }
	cfg.TelegramTimeout, err = envDuration("TELEGRAM_TIMEOUT", "10"); if err != nil { return cfg, err }
	cfg.TestDomain = strings.TrimSuffix(strings.TrimSpace(os.Getenv("DNS_TEST_DOMAIN")), "."); if cfg.TestDomain == "" { cfg.TestDomain = "google.com" }
	if _, err := makeDNSQuery(cfg.TestDomain, 1); err != nil { return cfg, fmt.Errorf("invalid DNS_TEST_DOMAIN: %w", err) }
	return cfg, nil
}

func makeDNSQuery(domain string, id uint16) ([]byte, error) {
	packet := make([]byte, 12); binary.BigEndian.PutUint16(packet[0:2], id); binary.BigEndian.PutUint16(packet[2:4], 0x0100); binary.BigEndian.PutUint16(packet[4:6], 1)
	for _, label := range strings.Split(domain, ".") { if label == "" || len(label) > 63 { return nil, fmt.Errorf("invalid DNS label %q", label) }; packet = append(packet, byte(len(label))); packet = append(packet, label...) }
	packet = append(packet, 0, 0, 1, 0, 1)
	return packet, nil
}

func checkDNS(target Target, domain string, timeout time.Duration) (bool, string) {
	id := uint16(time.Now().UnixNano()); query, err := makeDNSQuery(domain, id); if err != nil { return false, err.Error() }
	conn, err := net.DialTimeout("udp", target.Addr, timeout); if err != nil { return false, err.Error() }; defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(query); err != nil { return false, err.Error() }
	buf := make([]byte, 4096); n, err := conn.Read(buf); if err != nil { return false, err.Error() }; if n < 12 { return false, "DNS response too short" }
	response := buf[:n]; if binary.BigEndian.Uint16(response[0:2]) != id { return false, "DNS response ID mismatch" }
	flags := binary.BigEndian.Uint16(response[2:4]); if flags&0x8000 == 0 { return false, "packet is not a DNS response" }; if flags&0x0200 != 0 { return false, "truncated DNS response" }
	if rcode := flags & 0x000f; rcode != 0 { return false, fmt.Sprintf("DNS response code: %d", rcode) }
	if binary.BigEndian.Uint16(response[6:8]) == 0 { return false, "DNS response contains no answer" }
	return true, "DNS query successful"
}

type Telegram struct { token string; chatID string; client *http.Client }
func newTelegram(cfg MonitorConfig) *Telegram { return &Telegram{token:cfg.BotToken, chatID:cfg.ChatID, client:&http.Client{Timeout:cfg.TelegramTimeout}} }
func (t *Telegram) send(message string) error {
	body, err := json.Marshal(map[string]any{"chat_id":t.chatID, "text":message, "disable_web_page_preview":true}); if err != nil { return err }
	req, err := http.NewRequest(http.MethodPost, "https://api.telegram.org/bot"+t.token+"/sendMessage", bytes.NewReader(body)); if err != nil { return err }; req.Header.Set("Content-Type","application/json")
	resp, err := t.client.Do(req); if err != nil { return err }; defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return fmt.Errorf("Telegram returned HTTP %s", resp.Status) }
	var result struct { OK bool `json:"ok"`; Description string `json:"description"` }
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil { return err }; if !result.OK { return fmt.Errorf("Telegram API: %s", result.Description) }; return nil
}
func notify(tg *Telegram, message string) { if err := tg.send(message); err != nil { log.Printf("ERROR Telegram notification failed: %v", err) } }

func run(ctx context.Context, cfg MonitorConfig) {
	tg := newTelegram(cfg); states := make(map[string]State, len(cfg.Targets)); var lines []string
	for _, target := range cfg.Targets { lines = append(lines, fmt.Sprintf("• %s: %s", target.Name, target.Addr)) }
	label := "servidores DNS"; if len(cfg.Targets) == 1 { label = "servidor DNS" }
	notify(tg, fmt.Sprintf("🟢 AdGuard DNS Monitor iniciado\nMonitorizando %d %s\n%s", len(cfg.Targets), label, strings.Join(lines,"\n")))
	check := func() {
		for _, target := range cfg.Targets {
			healthy, detail := checkDNS(target, cfg.TestDomain, cfg.DNSTimeout); previous := states[target.Addr]; states[target.Addr] = State{Known:true, Healthy:healthy}
			if healthy {
				log.Printf("%s (%s): OK", target.Name, target.Addr)
				if previous.Known && !previous.Healthy { notify(tg, fmt.Sprintf("🟢 AdGuard DNS RECUPERADO\nInstancia: %s\nIP: %s\nPrueba: %s\nEstado: DNS operativo", target.Name,target.Addr,cfg.TestDomain)) }
			} else {
				log.Printf("WARN %s (%s): FAILED - %s", target.Name,target.Addr,detail)
				if !previous.Known || previous.Healthy { notify(tg,fmt.Sprintf("🔴 FALLO AdGuard DNS\nInstancia: %s\nIP: %s\nPrueba: %s\nError: %s",target.Name,target.Addr,cfg.TestDomain,detail)) }
			}
		}
	}
	check(); ticker := time.NewTicker(cfg.Interval); defer ticker.Stop()
	for { select { case <-ctx.Done(): notify(tg,"⚪ AdGuard DNS Monitor detenido"); return; case <-ticker.C: check() } }
}
func main() {
	log.SetFlags(log.Ldate|log.Ltime|log.LUTC); cfg, err := loadConfig(); if err != nil { log.Fatalf("Configuration error: %v",err) }
	log.Printf("Starting AdGuard Home monitor: %d target(s), interval=%s, query=%s",len(cfg.Targets),cfg.Interval,cfg.TestDomain)
	for _, target := range cfg.Targets { log.Printf("Configured target: %s (%s)",target.Name,target.Addr) }
	ctx, stop := signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM); defer stop(); run(ctx,cfg)
}
