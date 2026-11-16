package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestXORDecodeRoundTrip(t *testing.T) {
	const input = "router-password"
	encoded := xorEncode(input)
	if encoded == input { t.Fatal("XOR encoding did not transform the input") }
	if xorEncode(encoded) != input { t.Fatal("XOR encoding is not reversible") }
}

func TestExtractJS(t *testing.T) {
	page := []byte(`<script>var lanHostDns = "1.1.1.1,8.8.8.8";</script>`)
	got, err := extractJS(page, "lanHostDns")
	if err != nil { t.Fatal(err) }
	if got != "1.1.1.1,8.8.8.8" { t.Fatalf("got %q", got) }
}

func TestExtractJSMissingField(t *testing.T) {
	_, err := extractJS([]byte(`var other = "value";`), "lanHostDns")
	if err == nil || !strings.Contains(err.Error(), "lanHostDns") { t.Fatalf("unexpected error: %v", err) }
}

func TestWiFiOffDefaultIsTenSeconds(t *testing.T) {
	t.Setenv("TARGETS", "home=192.168.1.2")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat")
	t.Setenv("ROUTER_PASSWORD", "secret")
	t.Setenv("WIFI_OFF_SECONDS", "")
	cfg, err := loadConfig()
	if err != nil { t.Fatal(err) }
	if cfg.Router.WiFiOff.Seconds() != 10 { t.Fatalf("WiFi off duration = %s, want 10s", cfg.Router.WiFiOff) }
	if !cfg.RecoveryEnabled { t.Fatal("router recovery should be enabled when ROUTER_PASSWORD is set") }
}

func TestRouterRecoveryDisabledWithoutPassword(t *testing.T) {
	t.Setenv("TARGETS", "home=192.168.1.2")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat")
	t.Setenv("ROUTER_PASSWORD", "")
	cfg, err := loadConfig()
	if err != nil { t.Fatal(err) }
	if cfg.RecoveryEnabled { t.Fatal("router recovery must be disabled without ROUTER_PASSWORD") }
}


func TestGetRadioRejectsUnexpectedRadioID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<cbGetWifi><Result>0</Result><dpm_cfg_wifi><dpm_cfg_wifi_id>1</dpm_cfg_wifi_id><radio_adm_state>1</radio_adm_state></dpm_cfg_wifi></cbGetWifi>`)
	}))
	defer server.Close()

	router := newAskeyRouter(RouterConfig{URL: server.URL})
	if _, err := router.getRadio(context.Background(), 0); err == nil {
		t.Fatal("expected unexpected radio ID to be rejected")
	}
}

func TestRestartWiFiRestoresRadiosWhenContextIsCancelled(t *testing.T) {
	var mu sync.Mutex
	states := map[int]string{0: "1", 1: "1"}
	var allOffOnce sync.Once
	allOff := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/te_acceso_router.cgi":
			w.WriteHeader(http.StatusOK)
		case "/te_mapa_red_local.asp":
			_, _ = fmt.Fprint(w, `<script>var sessionKey = "test-session";</script>`)
		case "/wifi5g.asp":
			_, _ = fmt.Fprint(w, `<script>var easymesh_adm_state = "0";</script>`)
		case "/cgi-bin/cbGetWifi.xml":
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			id, err := strconv.Atoi(r.Form.Get("dpm_cfg_wifi_0i"))
			if err != nil || id < 0 || id > 1 {
				http.Error(w, "bad radio id", http.StatusBadRequest)
				return
			}
			mu.Lock()
			admin := states[id]
			mu.Unlock()
			_, _ = fmt.Fprintf(w, `<cbGetWifi><Result>0</Result><dpm_cfg_wifi><dpm_cfg_wifi_id>%d</dpm_cfg_wifi_id><radio_adm_state>%s</radio_adm_state><radio_opr_status>up</radio_opr_status><country_code>US</country_code><standard>802.11</standard><adm_bandwidth>20</adm_bandwidth><adm_channel>1</adm_channel><mcs>0</mcs><rts_threshold>2347</rts_threshold><beacon_interval>100</beacon_interval><dtim_interval>1</dtim_interval><adm_tx_pwr_ratio>100</adm_tx_pwr_ratio><adm_tx_preamble>0</adm_tx_preamble><adm_guard_interval>0</adm_guard_interval><beamforming>0</beamforming><mld_group>0</mld_group><wifi_unique_ssid_adm_state>0</wifi_unique_ssid_adm_state></dpm_cfg_wifi></cbGetWifi>`, id, admin)
		case "/cgi-bin/cbSetWifi.xml":
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			id, err := strconv.Atoi(r.Form.Get("dpm_cfg_wifi_id"))
			if err != nil || id < 0 || id > 1 {
				http.Error(w, "bad radio id", http.StatusBadRequest)
				return
			}
			admin := r.Form.Get("radio_adm_state")
			mu.Lock()
			states[id] = admin
			off := states[0] == "0" && states[1] == "0"
			mu.Unlock()
			if off {
				allOffOnce.Do(func() { close(allOff) })
			}
			_, _ = fmt.Fprint(w, `<cbSetWifi><Result>0</Result></cbSetWifi>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	router := newAskeyRouter(RouterConfig{URL: server.URL, WiFiOff: 30 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-allOff
		cancel()
	}()

	err := router.restartWiFi(ctx)
	if err == nil || !strings.Contains(err.Error(), "cancel") {
		t.Fatalf("restartWiFi error = %v, want cancellation", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if states[0] != "1" || states[1] != "1" {
		t.Fatalf("radio states after cancellation = %v, want both enabled", states)
	}
}
