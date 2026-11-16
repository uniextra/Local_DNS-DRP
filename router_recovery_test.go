package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockAskey struct {
	mu                  sync.Mutex
	admin               [2]string
	ignoreDisable       int
	dns                 string
	dnsPostReturnsError bool
	loginCount          int
}

func (m *mockAskey) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/cgi-bin/cbGetWifi.xml":
		_ = req.ParseForm()
		id := 0
		if req.Form.Get("dpm_cfg_wifi_0i") == "1" {
			id = 1
		}
		m.mu.Lock()
		admin := m.admin[id]
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<cbGetWifi><Result>0</Result><dpm_cfg_wifi><dpm_cfg_wifi_id>%d</dpm_cfg_wifi_id><radio_adm_state>%s</radio_adm_state><radio_opr_status>%s</radio_opr_status><country_code>ES</country_code><standard>11</standard><adm_bandwidth>20</adm_bandwidth><adm_channel>1</adm_channel></dpm_cfg_wifi></cbGetWifi>`, id, admin, map[bool]string{true: "up", false: "down"}[admin == "1"])
	case "/cgi-bin/cbSetWifi.xml":
		_ = req.ParseForm()
		id := 0
		if req.Form.Get("dpm_cfg_wifi_id") == "1" {
			id = 1
		}
		admin := req.Form.Get("radio_adm_state")
		if req.Form.Get("sessionKey") != "test-session" {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<cbSetWifi><Result>1</Result></cbSetWifi>`))
			return
		}
		m.mu.Lock()
		if !(id == m.ignoreDisable && admin == "0") {
			m.admin[id] = admin
		}
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<cbSetWifi><Result>0</Result></cbSetWifi>`))
	case "/wifi5g.asp":
		_, _ = w.Write([]byte(`<script>var easymesh_adm_state = "0";</script>`))
	case "/te_mapa_red_local.asp":
		m.mu.Lock()
		dns := m.dns
		m.mu.Unlock()
		_, _ = fmt.Fprintf(w, `<script>var sessionKey = "test-session"; var ethIpAddress = "192.168.1.1"; var ethSubnetMask = "255.255.255.0"; var DHCPActive = "1"; var startIPAddress = "192.168.1.100"; var endIPAddress = "192.168.1.200"; var gatewayIPaddress = "192.168.1.1"; var lanHostDns = "%s";</script>`, dns)
	case "/cgi-bin/te_acceso_router.cgi":
		m.mu.Lock()
		m.loginCount++
		m.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	case "/cgi-bin/te_mapa_red_local.cgi":
		_ = req.ParseForm()
		m.mu.Lock()
		m.dns = req.Form.Get("lanHostDns")
		fail := m.dnsPostReturnsError
		m.mu.Unlock()
		if fail {
			http.Error(w, "simulated lost write response", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, req)
	}
}

func newMockRouter(t *testing.T, mock *mockAskey, off time.Duration) (*AskeyRouter, func()) {
	t.Helper()
	server := httptest.NewServer(mock)
	router := newAskeyRouter(RouterConfig{URL: server.URL, Password: "secret", DNSPrimary: "1.1.1.1", DNSSecondary: "8.8.8.8", WiFiOff: off})
	router.client = server.Client()
	router.sessionKey = "test-session"
	return router, server.Close
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not reached before timeout")
}

func TestXORDecodeRoundTrip(t *testing.T) {
	const input = "router-password"
	encoded := xorEncode(input)
	if encoded == input {
		t.Fatal("XOR encoding did not transform the input")
	}
	if xorEncode(encoded) != input {
		t.Fatal("XOR encoding is not reversible")
	}
}

func TestExtractJS(t *testing.T) {
	page := []byte(`<script>var lanHostDns = "1.1.1.1,8.8.8.8";</script>`)
	got, err := extractJS(page, "lanHostDns")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.1.1.1,8.8.8.8" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractJSMissingField(t *testing.T) {
	_, err := extractJS([]byte(`var other = "value";`), "lanHostDns")
	if err == nil || !strings.Contains(err.Error(), "lanHostDns") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetRadioRejectsInvalidID(t *testing.T) {
	r := &AskeyRouter{cfg: RouterConfig{URL: "http://127.0.0.1"}}
	for _, id := range []string{"", "abc", "2", "-1"} {
		err := r.setRadio(nil, radioXML{ID: id}, "0", "")
		if err == nil {
			t.Errorf("setRadio accepted invalid radio ID %q", id)
		}
	}
}

func TestWiFiCancellationAlwaysRestoresEnabledRadios(t *testing.T) {
	mock := &mockAskey{admin: [2]string{"1", "1"}, ignoreDisable: -1}
	router, closeServer := newMockRouter(t, mock, time.Hour)
	defer closeServer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- router.restartWiFi(ctx) }()
	waitFor(t, func() bool {
		mock.mu.Lock()
		defer mock.mu.Unlock()
		return mock.admin[0] == "0" && mock.admin[1] == "0"
	})
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("restartWiFi error = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("restartWiFi did not return after cancellation")
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.admin != [2]string{"1", "1"} {
		t.Fatalf("radio states after cancellation = %v, want both enabled", mock.admin)
	}
}

func TestWiFiDisableHTTP200MustBeVerified(t *testing.T) {
	mock := &mockAskey{admin: [2]string{"1", "1"}, ignoreDisable: 1}
	router, closeServer := newMockRouter(t, mock, time.Millisecond)
	defer closeServer()
	err := router.restartWiFi(context.Background())
	if err == nil {
		t.Fatal("expected failed state verification to abort Wi-Fi restart")
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.admin != [2]string{"1", "1"} {
		t.Fatalf("radio states after failed disable = %v, want both restored", mock.admin)
	}
}

func TestUpdateDNSVerifiesAppliedChangeAfterHTTPError(t *testing.T) {
	mock := &mockAskey{admin: [2]string{"1", "1"}, ignoreDisable: -1, dns: "9.9.9.9,149.112.112.112", dnsPostReturnsError: true}
	router, closeServer := newMockRouter(t, mock, time.Second)
	defer closeServer()
	if err := router.updateDNS(context.Background()); err != nil {
		t.Fatalf("updateDNS should accept the applied setting after an ambiguous HTTP error: %v", err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.dns != "1.1.1.1,8.8.8.8" {
		t.Fatalf("router DNS = %q, want requested DNS", mock.dns)
	}
}

func TestWiFiWriteRefreshesExpiredSessionOnce(t *testing.T) {
	mock := &mockAskey{admin: [2]string{"1", "1"}, ignoreDisable: -1}
	router, closeServer := newMockRouter(t, mock, time.Millisecond)
	defer closeServer()
	router.sessionKey = "expired-session"
	if err := router.restartWiFi(context.Background()); err != nil {
		t.Fatalf("restartWiFi failed after session refresh: %v", err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.loginCount != 1 {
		t.Fatalf("login attempts = %d, want one session refresh", mock.loginCount)
	}
	if mock.admin != [2]string{"1", "1"} {
		t.Fatalf("radio states after restart = %v, want both enabled", mock.admin)
	}
}

func TestWiFiOffDefaultIsTenSeconds(t *testing.T) {
	t.Setenv("TARGETS", "home=192.168.1.2")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat")
	t.Setenv("ROUTER_PASSWORD", "secret")
	t.Setenv("WIFI_OFF_SECONDS", "")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Router.WiFiOff.Seconds() != 10 {
		t.Fatalf("WiFi off duration = %s, want 10s", cfg.Router.WiFiOff)
	}
	if !cfg.RecoveryEnabled {
		t.Fatal("router recovery should be enabled when ROUTER_PASSWORD is set")
	}
}

func TestRouterRecoveryDisabledWithoutPassword(t *testing.T) {
	t.Setenv("TARGETS", "home=192.168.1.2")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat")
	t.Setenv("ROUTER_PASSWORD", "")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RecoveryEnabled {
		t.Fatal("router recovery must be disabled without ROUTER_PASSWORD")
	}
}
