package recovery

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewRouterRecoverySelector(t *testing.T) {
	if got := NormalizeRouterModel("  ASKEY-RTF8316VW "); got != RouterModelAskeyRTF8316VW {
		t.Fatalf("NormalizeRouterModel = %q", got)
	}
	if got := NormalizeRouterModel(""); got != DefaultRouterModel {
		t.Fatalf("NormalizeRouterModel('') = %q", got)
	}
	r, err := NewRouterRecovery(RouterConfig{Model: ""})
	if err != nil || r == nil {
		t.Fatalf("NewRouterRecovery('') = %v, %v", r, err)
	}
	if _, err := NewRouterRecovery(RouterConfig{Model: "unknown-vendor"}); err == nil {
		t.Fatal("expected error for unsupported ROUTER_MODEL")
	}
}

func TestXorEncodeAndExtractJS(t *testing.T) {
	orig := "admin_pass!123"
	if roundTrip := xorEncode(xorEncode(orig)); roundTrip != orig {
		t.Fatalf("xorEncode round-trip = %q, want %q", roundTrip, orig)
	}
	page := []byte(`var ethIpAddress = "192.168.1.1"; lanHostDns='1.1.1.1,8.8.8.8';`)
	if ip, err := extractJS(page, "ethIpAddress"); err != nil || ip != "192.168.1.1" {
		t.Fatalf("extractJS = %q, %v", ip, err)
	}
	if _, err := extractJS(page, "missing"); err == nil {
		t.Fatal("expected error for missing field")
	}
}

type mockRouterState struct {
	mu       sync.Mutex
	dns      string
	radios   [2]string
	logins   int
	setCalls int
}

func newMockRouterServer(t *testing.T, state *mockRouterState) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()

		switch r.URL.Path {
		case "/cgi-bin/te_acceso_router.cgi":
			state.logins++
			w.WriteHeader(http.StatusOK)
		case "/te_mapa_red_local.asp":
			_, _ = fmt.Fprintf(w, `
				var sessionKey = "sess123";
				var ethIpAddress = "192.168.1.1";
				var ethSubnetMask = "255.255.255.0";
				var DHCPActive = "1";
				var startIPAddress = "192.168.1.10";
				var endIPAddress = "192.168.1.200";
				var gatewayIPaddress = "192.168.1.1";
				var lanHostDns = %q;
			`, state.dns)
		case "/cgi-bin/te_mapa_red_local.cgi":
			_ = r.ParseForm()
			state.dns = r.FormValue("lanHostDns")
			w.WriteHeader(http.StatusOK)
		case "/wifi5g.asp":
			_, _ = w.Write([]byte(`var easymesh_adm_state = "0";`))
		case "/cgi-bin/cbGetWifi.xml":
			_ = r.ParseForm()
			id := r.FormValue("dpm_cfg_wifi_0i")
			idx := 0
			if id == "1" {
				idx = 1
			}
			adm := state.radios[idx]
			opr := "down"
			if adm == "1" {
				opr = "up"
			}
			_, _ = fmt.Fprintf(w, `<cbGetWifi><Result>0</Result><dpm_cfg_wifi><dpm_cfg_wifi_id>%s</dpm_cfg_wifi_id><radio_adm_state>%s</radio_adm_state><radio_opr_status>%s</radio_opr_status></dpm_cfg_wifi></cbGetWifi>`, id, adm, opr)
		case "/cgi-bin/cbSetWifi.xml":
			_ = r.ParseForm()
			state.setCalls++
			id := r.FormValue("dpm_cfg_wifi_id")
			adm := r.FormValue("radio_adm_state")
			if id == "1" {
				state.radios[1] = adm
			} else {
				state.radios[0] = adm
			}
			_, _ = w.Write([]byte(`<cbSetWifi><Result>0</Result></cbSetWifi>`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestAskeyRouterRecoverAndCancelRestore(t *testing.T) {
	state := &mockRouterState{
		dns:    "192.168.1.2,192.168.1.3",
		radios: [2]string{"1", "1"},
	}
	srv := newMockRouterServer(t, state)
	defer srv.Close()

	router := NewAskeyRouter(RouterConfig{
		URL:          srv.URL,
		Username:     "1234",
		Password:     "pass",
		DNSPrimary:   "1.1.1.1",
		DNSSecondary: "8.8.8.8",
		WiFiOff:      10 * time.Millisecond,
	})

	if err := router.Recover(context.Background()); err != nil {
		t.Fatalf("Recover() error: %v", err)
	}

	state.mu.Lock()
	if state.dns != "1.1.1.1,8.8.8.8" || state.radios != [2]string{"1", "1"} || state.setCalls != 4 {
		state.mu.Unlock()
		t.Fatalf("unexpected state after Recover: %+v", state)
	}
	state.mu.Unlock()

	router.cfg.WiFiOff = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	err := router.restartWiFi(ctx)
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.radios != [2]string{"1", "1"} {
		t.Fatalf("radios should be restored on cancel, got %v", state.radios)
	}
}
