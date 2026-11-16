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
	raw := strings.TrimSpace(os.Getenv(name)); if raw == "" { raw = fallback }
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > float64(1<<63-1)/float64(time.Second) {
		return 0, fmt.Errorf("%s must be a finite, positive duration within the time.Duration range", name)
	}
	duration := time.Duration(seconds * float64(time.Second))
	if duration <= 0 {
		return 0, fmt.Errorf("%s is too small to represent as a time.Duration", name)
	}
	return duration, nil
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
			} else { return cfg, fmt.Errorf("TARGETS must contain IP addresses, not hostnames: %q", endpoint) }
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
	packet := make([]byte, 12); binary.BigEndian.PutUint16(packet[0:2], id); binary.BigEndian.PutUint16(packet[2:4], 0x0100); binary.BigEndian.PutUint16(packet[4:6], 1)
	wireNameLength := 1 // terminating root label
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 {
			return nil, fmt.Errorf("invalid DNS label %q", label)
		}
		wireNameLength += 1 + len(label)
		if wireNameLength > 255 {
			return nil, fmt.Errorf("DNS name exceeds the 255-octet wire-format limit")
		}
		packet = append(packet, byte(len(label)))
		packet = append(packet, label...)
	}
	packet = append(packet, 0, 0, 1, 0, 1)
	return packet, nil
}

func readDNSName(packet []byte, offset int) (string, int, error) {
	var labels []string
	nextOffset := -1
	for steps := 0; steps < 128; steps++ {
		if offset >= len(packet) {
			return "", 0, errors.New("DNS name exceeds packet boundary")
		}
		length := int(packet[offset])
		if length&0xc0 == 0xc0 {
			if offset+1 >= len(packet) {
				return "", 0, errors.New("truncated DNS compression pointer")
			}
			pointer := ((length & 0x3f) << 8) | int(packet[offset+1])
			if pointer >= len(packet) {
				return "", 0, errors.New("DNS compression pointer out of range")
			}
			if nextOffset < 0 {
				nextOffset = offset + 2
			}
			offset = pointer
			continue
		}
		if length&0xc0 != 0 {
			return "", 0, errors.New("unsupported DNS label encoding")
		}
		offset++
		if length == 0 {
			if nextOffset < 0 {
				nextOffset = offset
			}
			return strings.Join(labels, "."), nextOffset, nil
		}
		if length > 63 || offset+length > len(packet) {
			return "", 0, errors.New("invalid or truncated DNS label")
		}
		labels = append(labels, string(packet[offset:offset+length]))
		offset += length
	}
	return "", 0, errors.New("DNS compression pointer loop or excessive indirection")
}

func validateDNSResponse(query, response []byte) error {
	if len(query) < 17 || len(response) < 12 {
		return errors.New("DNS query or response is too short")
	}
	if binary.BigEndian.Uint16(response[0:2]) != binary.BigEndian.Uint16(query[0:2]) {
		return errors.New("DNS response ID mismatch")
	}
	flags := binary.BigEndian.Uint16(response[2:4])
	if flags&0x8000 == 0 {
		return errors.New("packet is not a DNS response")
	}
	if flags&0x0200 != 0 {
		return errors.New("truncated DNS response")
	}
	if flags&0x7800 != 0 {
		return errors.New("DNS response opcode mismatch")
	}
	if rcode := flags & 0x000f; rcode != 0 {
		return fmt.Errorf("DNS response code: %d", rcode)
	}
	if binary.BigEndian.Uint16(query[4:6]) != 1 || binary.BigEndian.Uint16(response[4:6]) != 1 {
		return errors.New("DNS query/response must contain exactly one question")
	}

	queryName, queryEnd, err := readDNSName(query, 12)
	if err != nil || queryEnd+4 > len(query) {
		return errors.New("invalid DNS question in query")
	}
	queryType := binary.BigEndian.Uint16(query[queryEnd : queryEnd+2])
	queryClass := binary.BigEndian.Uint16(query[queryEnd+2 : queryEnd+4])

	responseName, responseEnd, err := readDNSName(response, 12)
	if err != nil || responseEnd+4 > len(response) {
		return errors.New("invalid DNS question in response")
	}
	responseType := binary.BigEndian.Uint16(response[responseEnd : responseEnd+2])
	responseClass := binary.BigEndian.Uint16(response[responseEnd+2 : responseEnd+4])
	if !strings.EqualFold(responseName, queryName) || responseType != queryType || responseClass != queryClass {
		return errors.New("DNS response question does not match the query")
	}

	answerCount := int(binary.BigEndian.Uint16(response[6:8]))
	if answerCount == 0 {
		return errors.New("DNS response contains no answer")
	}

	offset := responseEnd + 4
	type answerRecord struct {
		owner string
		recordType uint16
		recordClass uint16
		dataOffset int
		dataEnd int
	}
	records := make([]answerRecord, 0, answerCount)
	for i := 0; i < answerCount; i++ {
		owner, nameEnd, err := readDNSName(response, offset)
		if err != nil || nameEnd+10 > len(response) {
			return errors.New("malformed DNS answer record")
		}
		recordType := binary.BigEndian.Uint16(response[nameEnd : nameEnd+2])
		recordClass := binary.BigEndian.Uint16(response[nameEnd+2 : nameEnd+4])
		dataLength := int(binary.BigEndian.Uint16(response[nameEnd+8 : nameEnd+10]))
		dataOffset := nameEnd + 10
		dataEnd := dataOffset + dataLength
		if dataEnd > len(response) {
			return errors.New("truncated DNS answer data")
		}
		records = append(records, answerRecord{
			owner: strings.ToLower(owner), recordType: recordType, recordClass: recordClass,
			dataOffset: dataOffset, dataEnd: dataEnd,
		})
		offset = dataEnd
	}

	// Only accept records attached to the queried name or a CNAME target
	// reachable from it; unrelated answer-section data is not proof of health.
	reachable := map[string]bool{strings.ToLower(queryName): true}
	for pass := 0; pass < len(records); pass++ {
		changed := false
		for _, record := range records {
			if record.recordClass != 1 || record.recordType != 5 || !reachable[record.owner] {
				continue
			}
			target, targetEnd, err := readDNSName(response, record.dataOffset)
			if err == nil && targetEnd <= record.dataEnd {
				target = strings.ToLower(target)
				if !reachable[target] {
					reachable[target] = true
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}

	for _, record := range records {
		if !reachable[record.owner] || record.recordClass != queryClass || record.recordType != queryType {
			continue
		}
		switch record.recordType {
		case 1: // A
			if record.dataEnd-record.dataOffset == 4 {
				return nil
			}
		case 28: // AAAA
			if record.dataEnd-record.dataOffset == 16 {
				return nil
			}
		}
	}
	// A valid CNAME chain is also an authoritative response to the query,
	// even when the terminal address is not included in the answer section.
	for _, record := range records {
		if reachable[record.owner] && record.recordClass == 1 && record.recordType == 5 {
			_, targetEnd, err := readDNSName(response, record.dataOffset)
			if err == nil && targetEnd <= record.dataEnd {
				return nil
			}
		}
	}
	return errors.New("DNS response contains no usable answer for the requested record type")
}

func checkDNS(target Target, domain string, timeout time.Duration) (bool, string) {
	id := uint16(time.Now().UnixNano()); query, err := makeDNSQuery(domain, id); if err != nil { return false, err.Error() }
	conn, err := net.DialTimeout("udp", target.Addr, timeout); if err != nil { return false, err.Error() }; defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(query); err != nil { return false, err.Error() }
	buf := make([]byte, 4096); n, err := conn.Read(buf); if err != nil { return false, err.Error() }; if n < 12 { return false, "DNS response too short" }
	if err := validateDNSResponse(query, buf[:n]); err != nil {
		return false, err.Error()
	}
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
	var router *AskeyRouter
	if cfg.RecoveryEnabled { router = newAskeyRouter(cfg.Router); log.Printf("Askey router recovery enabled for %s", cfg.Router.URL) } else { log.Printf("Askey router recovery disabled: ROUTER_PASSWORD is not set") }
	consecutiveAllDown := 0
	lastRecovery := time.Time{}
	recoveryRunning := false
	for _, target := range cfg.Targets { lines = append(lines, fmt.Sprintf("• %s: %s", target.Name, target.Addr)) }
	label := "servidores DNS"; if len(cfg.Targets) == 1 { label = "servidor DNS" }
	notify(tg, fmt.Sprintf("🟢 AdGuard DNS Monitor iniciado\nMonitorizando %d %s\n%s", len(cfg.Targets), label, strings.Join(lines,"\n")))
	check := func() {
		allDown := true
		for _, target := range cfg.Targets {
			healthy, detail := checkDNS(target, cfg.TestDomain, cfg.DNSTimeout); previous := states[target.Addr]; states[target.Addr] = State{Known:true, Healthy:healthy}
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
	check(); ticker := time.NewTicker(cfg.Interval); defer ticker.Stop()
	for { select { case <-ctx.Done(): notify(tg,"⚪ AdGuard DNS Monitor detenido"); return; case <-ticker.C: check() } }
}
func main() {
	log.SetFlags(log.Ldate|log.Ltime|log.LUTC); cfg, err := loadConfig(); if err != nil { log.Fatalf("Configuration error: %v",err) }
	log.Printf("Starting AdGuard Home monitor: %d target(s), interval=%s, query=%s",len(cfg.Targets),cfg.Interval,cfg.TestDomain)
	for _, target := range cfg.Targets { log.Printf("Configured target: %s (%s)",target.Name,target.Addr) }
	ctx, stop := signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM); defer stop(); run(ctx,cfg)
}
