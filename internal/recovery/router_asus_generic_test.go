package recovery

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockAsusWrt struct {
	mu                         sync.Mutex
	nvram                      map[string]string
	loginCount                 int
	dhcpDNSWrites              int
	dnsmasqRestarts            int
	wirelessRestarts           int
	failNextDNSWriteAfterApply bool
}

func newMockAsusWrt() *mockAsusWrt {
	return &mockAsusWrt{
		nvram: map[string]string{
			"dhcp_dns1_x": "9.9.9.9",
			"dhcp_dns2_x": "149.112.112.112",
			"wl0_radio":   "1",
			"wl1_radio":   "0",
			"wl2_radio":   "1",
		},
	}
}

func (m *mockAsusWrt) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/login.cgi":
		body, _ := io.ReadAll(req.Body)
		decoded, err := url.QueryUnescape(string(body))
		if err != nil {
			http.Error(w, "bad login request", http.StatusBadRequest)
			return
		}
		form, err := url.ParseQuery(decoded)
		if err != nil {
			http.Error(w, "bad login form", http.StatusBadRequest)
			return
		}
		want := base64.StdEncoding.EncodeToString([]byte("admin:router-secret"))
		if form.Get("login_authorization") != want {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		m.mu.Lock()
		m.loginCount++
		m.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "asus_token", Value: "test-token", Path: "/"})
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"asus_token":"test-token"}`)
	case "/appGet.cgi":
		if !hasAsusTestCookie(req) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(req.Body)
		decoded, err := url.QueryUnescape(string(body))
		if err != nil || !strings.HasPrefix(decoded, "hook=") {
			http.Error(w, "bad hook request", http.StatusBadRequest)
			return
		}
		values := make(map[string]string)
		m.mu.Lock()
		for _, hook := range strings.Split(strings.TrimPrefix(decoded, "hook="), ";") {
			key := strings.TrimSuffix(strings.TrimPrefix(hook, "nvram_get("), ")")
			if value, ok := m.nvram[key]; ok && key != hook {
				values[key] = value
			}
		}
		m.mu.Unlock()
		data, _ := json.Marshal(values)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	case "/applyapp.cgi":
		if !hasAsusTestCookie(req) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(req.Body)
		decoded, err := url.QueryUnescape(string(body))
		if err != nil {
			http.Error(w, "bad apply encoding", http.StatusBadRequest)
			return
		}
		action := make(map[string]any)
		if err := json.Unmarshal([]byte(decoded), &action); err != nil {
			http.Error(w, "bad apply JSON", http.StatusBadRequest)
			return
		}
		if action["action_mode"] != "apply" {
			http.Error(w, "missing action mode", http.StatusBadRequest)
			return
		}

		m.mu.Lock()
		defer m.mu.Unlock()
		dnsWrite := false
		for key, raw := range action {
			if key == "dhcp_dns1_x" || key == "dhcp_dns2_x" {
				value, ok := raw.(string)
				if !ok {
					http.Error(w, "DNS values must be strings", http.StatusBadRequest)
					return
				}
				m.nvram[key] = value
				dnsWrite = true
			}
			if strings.HasPrefix(key, "wl") && strings.HasSuffix(key, "_radio") {
				value, ok := raw.(float64)
				if !ok || (value != 0 && value != 1) {
					http.Error(w, "invalid radio state", http.StatusBadRequest)
					return
				}
				unit := strings.TrimSuffix(strings.TrimPrefix(key, "wl"), "_radio")
				if _, err := strconv.Atoi(unit); err != nil {
					http.Error(w, "invalid radio key", http.StatusBadRequest)
					return
				}
				m.nvram[key] = strconv.Itoa(int(value))
			}
		}
		if dnsWrite {
			m.dhcpDNSWrites++
			if m.failNextDNSWriteAfterApply {
				m.failNextDNSWriteAfterApply = false
				http.Error(w, "simulated lost acknowledgement", http.StatusInternalServerError)
				return
			}
		}
		switch action["rc_service"] {
		case "restart_dnsmasq":
			m.dnsmasqRestarts++
		case "restart_wireless":
			m.wirelessRestarts++
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"modify":"1"}`)
	default:
		http.NotFound(w, req)
	}
}

func hasAsusTestCookie(req *http.Request) bool {
	cookie, err := req.Cookie("asus_token")
	return err == nil && cookie.Value == "test-token"
}

func newAsusTestRouter(t *testing.T, mock *mockAsusWrt) (*AsusGenericRouter, func()) {
	t.Helper()
	server := httptest.NewServer(mock)
	router, err := newAsusGenericRouter(RouterConfig{
		Model:        RouterModelAsusGeneric,
		URL:          server.URL,
		Username:     "admin",
		Password:     "router-secret",
		DNSPrimary:   "1.1.1.1",
		DNSSecondary: "8.8.8.8",
		WiFiOff:      time.Millisecond,
	})
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	return router, server.Close
}

func TestNewAsusGenericRouterRejectsInvalidURL(t *testing.T) {
	for _, test := range []string{"", "router.local", "ftp://192.168.1.1"} {
		t.Run(fmt.Sprintf("url_%q", test), func(t *testing.T) {
			_, err := newAsusGenericRouter(RouterConfig{URL: test})
			if err == nil {
				t.Fatalf("newAsusGenericRouter(%q) succeeded, want invalid URL error", test)
			}
		})
	}
}

func TestAsusGenericRecoverUpdatesDHCPDNSAndRestoresRadioStates(t *testing.T) {
	mock := newMockAsusWrt()
	router, closeServer := newAsusTestRouter(t, mock)
	defer closeServer()

	if err := router.Recover(t.Context()); err != nil {
		t.Fatalf("Recover() error = %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.nvram["dhcp_dns1_x"] != "1.1.1.1" || mock.nvram["dhcp_dns2_x"] != "8.8.8.8" {
		t.Fatalf("DHCP DNS = %q/%q, want 1.1.1.1/8.8.8.8", mock.nvram["dhcp_dns1_x"], mock.nvram["dhcp_dns2_x"])
	}
	if mock.nvram["wl0_radio"] != "1" || mock.nvram["wl1_radio"] != "0" || mock.nvram["wl2_radio"] != "1" {
		t.Fatalf("final radio states = %v, want original 1/0/1", map[string]string{
			"wl0_radio": mock.nvram["wl0_radio"],
			"wl1_radio": mock.nvram["wl1_radio"],
			"wl2_radio": mock.nvram["wl2_radio"],
		})
	}
	if mock.loginCount != 1 {
		t.Fatalf("login count = %d, want 1", mock.loginCount)
	}
	if mock.dhcpDNSWrites != 1 || mock.dnsmasqRestarts != 1 {
		t.Fatalf("DHCP writes/restarts = %d/%d, want 1/1", mock.dhcpDNSWrites, mock.dnsmasqRestarts)
	}
	if mock.wirelessRestarts != 2 {
		t.Fatalf("wireless restarts = %d, want disable + restore", mock.wirelessRestarts)
	}
}

func TestAsusGenericVerifiesDHCPWriteAfterLostAcknowledgement(t *testing.T) {
	mock := newMockAsusWrt()
	mock.failNextDNSWriteAfterApply = true
	router, closeServer := newAsusTestRouter(t, mock)
	defer closeServer()

	if err := router.login(t.Context()); err != nil {
		t.Fatalf("login() error = %v", err)
	}
	if err := router.updateDHCPDNS(t.Context()); err != nil {
		t.Fatalf("updateDHCPDNS() should trust verified read-back after ambiguous acknowledgement: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.dhcpDNSWrites != 1 {
		t.Fatalf("DHCP write count = %d, want one (never replay an ambiguous write)", mock.dhcpDNSWrites)
	}
	if mock.dnsmasqRestarts != 1 {
		t.Fatalf("dnsmasq restarts = %d, want one", mock.dnsmasqRestarts)
	}
}

func TestAsusGenericCycleWiFiPreservesAllInitiallyDisabledRadios(t *testing.T) {
	mock := newMockAsusWrt()
	mock.nvram["wl0_radio"] = "0"
	mock.nvram["wl1_radio"] = "0"
	mock.nvram["wl2_radio"] = "0"
	router, closeServer := newAsusTestRouter(t, mock)
	defer closeServer()

	if err := router.cycleWiFi(t.Context()); err != nil {
		t.Fatalf("cycleWiFi() error = %v", err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.wirelessRestarts != 0 {
		t.Fatalf("wireless restart count = %d, want 0 when all radios were already disabled", mock.wirelessRestarts)
	}
}

func TestAsusGenericReadRadiosRejectsUnexpectedState(t *testing.T) {
	mock := newMockAsusWrt()
	mock.nvram["wl0_radio"] = "auto"
	router, closeServer := newAsusTestRouter(t, mock)
	defer closeServer()

	if err := router.login(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := router.readRadios(t.Context()); err == nil {
		t.Fatal("readRadios() accepted a non-boolean radio state")
	}
}
