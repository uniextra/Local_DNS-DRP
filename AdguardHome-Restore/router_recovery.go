package main

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type RouterConfig struct {
	URL, Username, Password string
	DNSPrimary, DNSSecondary string
	WiFiOff time.Duration
	FailureThreshold int
	Cooldown time.Duration
}

type AskeyRouter struct {
	cfg RouterConfig
	client *http.Client
	sessionKey string
}

func newAskeyRouter(cfg RouterConfig) *AskeyRouter {
	jar, _ := cookiejar.New(nil)
	return &AskeyRouter{cfg: cfg, client: &http.Client{Timeout: 30*time.Second, Jar: jar}}
}

func xorEncode(s string) string {
	var b strings.Builder
	for _, r := range s { b.WriteRune(r ^ 0x1f) }
	return b.String()
}

func (r *AskeyRouter) endpoint(path string) string {
	return strings.TrimRight(r.cfg.URL, "/") + path
}

func (r *AskeyRouter) request(ctx context.Context, method, path string, form url.Values) ([]byte, error) {
	var body io.Reader
	if form != nil { body = strings.NewReader(form.Encode()) }
	req, err := http.NewRequestWithContext(ctx, method, r.endpoint(path), body)
	if err != nil { return nil, err }
	if form != nil { req.Header.Set("Content-Type", "application/x-www-form-urlencoded") }
	req.Header.Set("Referer", strings.TrimRight(r.cfg.URL, "/")+"/te_wifi.asp")
	resp, err := r.client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil { return nil, err }
	if resp.StatusCode < 200 || resp.StatusCode >= 400 { return nil, fmt.Errorf("router HTTP %s: %s", resp.Status, strings.TrimSpace(string(data))) }
	return data, nil
}

var sessionRE = regexp.MustCompile(`(?i)(?:sessionKey|session_key)\s*[:=]\s*["']([^"']+)["']`)

func (r *AskeyRouter) login(ctx context.Context) error {
	form := url.Values{
		"Password": {r.cfg.Password},
		"curWebPage": {"/te_wifi.asp"},
		"loginUsername": {xorEncode(r.cfg.Username)},
		"loginPassword": {xorEncode(r.cfg.Password)},
	}
	if _, err := r.request(ctx, http.MethodPost, "/cgi-bin/te_acceso_router.cgi", form); err != nil { return fmt.Errorf("router login: %w", err) }
	page, err := r.request(ctx, http.MethodGet, "/te_mapa_red_local.asp", nil)
	if err != nil { return fmt.Errorf("verify router session: %w", err) }
	match := sessionRE.FindSubmatch(page)
	if len(match) < 2 { return errors.New("router session established but sessionKey was not found on LAN settings page") }
	r.sessionKey = string(match[1])
	return nil
}

func (r *AskeyRouter) ensureLogin(ctx context.Context) error {
	if r.sessionKey != "" { return nil }
	return r.login(ctx)
}

var jsValue = func(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:var\s+)?`+regexp.QuoteMeta(name)+`\s*=\s*["']([^"']*)["']`)
}

func extractJS(page []byte, name string) (string, error) {
	m := jsValue(name).FindSubmatch(page)
	if len(m) < 2 { return "", fmt.Errorf("router page field %q not found", name) }
	return string(m[1]), nil
}

func (r *AskeyRouter) updateDNS(ctx context.Context) error {
	if err := r.ensureLogin(ctx); err != nil { return err }
	page, err := r.request(ctx, http.MethodGet, "/te_mapa_red_local.asp", nil)
	if err != nil { return err }
	fields := []string{"ethIpAddress","ethSubnetMask","DHCPActive","startIPAddress","endIPAddress","gatewayIPaddress"}
	values := make(map[string]string, len(fields))
	for _, name := range fields {
		values[name], err = extractJS(page, name)
		if err != nil { return err }
	}
	dnsForm := url.Values{
		"ethIpAddress": {values["ethIpAddress"]},
		"ethSubnetMask": {values["ethSubnetMask"]},
		"enblDhcpSrv": {values["DHCPActive"]},
		"dhcpEthStart": {values["startIPAddress"]},
		"dhcpEthEnd": {values["endIPAddress"]},
		"lanHostDns": {r.cfg.DNSPrimary+","+r.cfg.DNSSecondary},
		"lanHostDhcp": {"3^"+values["gatewayIPaddress"]},
		"sessionKey": {r.sessionKey},
		"curWebPage": {"/te_mapa_red_local.asp"},
	}
	if _, err := r.request(ctx, http.MethodPost, "/cgi-bin/te_mapa_red_local.cgi", dnsForm); err != nil { return fmt.Errorf("update DHCP DNS: %w", err) }
	verifyPage, err := r.request(ctx, http.MethodGet, "/te_mapa_red_local.asp", nil)
	if err != nil { return fmt.Errorf("verify DHCP DNS: %w", err) }
	got, err := extractJS(verifyPage, "lanHostDns")
	if err != nil { return err }
	want := r.cfg.DNSPrimary+","+r.cfg.DNSSecondary
	if strings.ReplaceAll(got, " ", "") != want { return fmt.Errorf("DNS verification mismatch: got %q, want %q", got, want) }
	return nil
}

type wifiXML struct {
	XMLName xml.Name `xml:"cbGetWifi"`
	Result string `xml:"Result"`
	Radio radioXML `xml:"dpm_cfg_wifi"`
}
type radioXML struct {
	ID string `xml:"dpm_cfg_wifi_id"`
	Admin string `xml:"radio_adm_state"`
	Operational string `xml:"radio_opr_status"`
	Channel string `xml:"adm_channel"`
	Country string `xml:"country_code"`
	Standard string `xml:"standard"`
	Bandwidth string `xml:"adm_bandwidth"`
	MCS string `xml:"mcs"`
	RTS string `xml:"rts_threshold"`
	Beacon string `xml:"beacon_interval"`
	DTIM string `xml:"dtim_interval"`
	TxPower string `xml:"adm_tx_pwr_ratio"`
	Preamble string `xml:"adm_tx_preamble"`
	Guard string `xml:"adm_guard_interval"`
	Beamforming string `xml:"beamforming"`
	MLD string `xml:"mld_group"`
	UniqueSSID string `xml:"wifi_unique_ssid_adm_state"`
	OBSS string `xml:"obss_coexistence"`
	ExtPos string `xml:"chn_40mhz_ext_pos"`
	Frag string `xml:"frag_threshold"`
	SCS string `xml:"scs"`
	TDWR string `xml:"tdwr_primary"`
	ZeroWaitDFS string `xml:"zerowaitDFS"`
}

func (r *AskeyRouter) getRadio(ctx context.Context, id int) (radioXML, error) {
	var radio radioXML
	data, err := r.request(ctx, http.MethodPost, "/cgi-bin/cbGetWifi.xml", url.Values{"dpm_cfg_wifi_0i": {strconv.Itoa(id)}})
	if err != nil { return radio, err }
	var result wifiXML
	if err := xml.Unmarshal(data, &result); err != nil { return radio, err }
	if result.Result != "0" { return radio, fmt.Errorf("router Wi-Fi read returned result %q", result.Result) }
	radio = result.Radio
	if radio.ID == "" { radio.ID = strconv.Itoa(id) }
	return radio, nil
}

func (r *AskeyRouter) setRadio(ctx context.Context, radio radioXML, admin string, easyMesh string) error {
	id, _ := strconv.Atoi(radio.ID)
	form := url.Values{
		"radio_adm_state": {admin}, "country_code": {radio.Country}, "standard": {radio.Standard},
		"adm_bandwidth": {radio.Bandwidth}, "adm_channel": {radio.Channel}, "mcs": {radio.MCS},
		"rts_threshold": {radio.RTS}, "beacon_interval": {radio.Beacon}, "dtim_interval": {radio.DTIM},
		"adm_tx_pwr_ratio": {radio.TxPower}, "adm_tx_preamble": {radio.Preamble},
		"adm_guard_interval": {radio.Guard}, "beamforming": {radio.Beamforming}, "mld_group": {radio.MLD},
		"wifi_unique_ssid_adm_state": {radio.UniqueSSID}, "dpm_cfg_wifi_id": {strconv.Itoa(id)},
		"sessionKey": {r.sessionKey},
	}
	if id == 0 {
		form.Set("obss_coexistence", radio.OBSS); form.Set("chn_40mhz_ext_pos", radio.ExtPos); form.Set("frag_threshold", radio.Frag)
	} else {
		form.Set("scs", radio.SCS); form.Set("tdwr_primary", radio.TDWR); form.Set("zerowaitDFS", radio.ZeroWaitDFS); form.Set("easymesh_adm_state", easyMesh)
	}
	_, err := r.request(ctx, http.MethodPost, "/cgi-bin/cbSetWifi.xml", form)
	return err
}

func (r *AskeyRouter) easyMeshState(ctx context.Context) (string, error) {
	page, err := r.request(ctx, http.MethodGet, "/wifi5g.asp", nil)
	if err != nil { return "", err }
	v, err := extractJS(page, "easymesh_adm_state")
	if err != nil { return "", err }
	return v, nil
}

func (r *AskeyRouter) restartWiFi(ctx context.Context) error {
	if err := r.ensureLogin(ctx); err != nil { return err }
	radios := make([]radioXML, 2)
	for id := 0; id < 2; id++ {
		radio, err := r.getRadio(ctx, id); if err != nil { return fmt.Errorf("read Wi-Fi radio %d: %w", id, err) }
		radios[id] = radio
	}
	easyMesh, err := r.easyMeshState(ctx)
	if err != nil { return fmt.Errorf("read 5 GHz EasyMesh state: %w", err) }
	wasEnabled := []bool{radios[0].Admin == "1", radios[1].Admin == "1"}
	for id := 0; id < 2; id++ {
		if err := r.setRadio(ctx, radios[id], "0", easyMesh); err != nil { return fmt.Errorf("disable Wi-Fi radio %d: %w", id, err) }
	}
	timer := time.NewTimer(r.cfg.WiFiOff); defer timer.Stop()
	select { case <-ctx.Done(): return ctx.Err(); case <-timer.C: }
	var errs []string
	for id := 0; id < 2; id++ {
		if !wasEnabled[id] { continue }
		if err := r.setRadio(ctx, radios[id], "1", easyMesh); err != nil {
			// A timed-out write has an ambiguous result; inspect actual state before retrying.
			checkCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			state, checkErr := r.getRadio(checkCtx, id); cancel()
			if checkErr != nil || state.Admin != "1" { errs = append(errs, fmt.Sprintf("radio %d restore: %v (verified state=%q, read error=%v)", id, err, state.Admin, checkErr)) }
		}
	}
	for id := 0; id < 2; id++ {
		if !wasEnabled[id] { continue }
		checkCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		state, checkErr := r.getRadio(checkCtx, id); cancel()
		if checkErr != nil || state.Admin != "1" || state.Operational != "up" {
			errs = append(errs, fmt.Sprintf("radio %d not operational (admin=%q operational=%q error=%v)", id, state.Admin, state.Operational, checkErr))
		}
	}
	if len(errs) > 0 { return errors.New(strings.Join(errs, "; ")) }
	return nil
}

func (r *AskeyRouter) Recover(ctx context.Context) error {
	if err := r.updateDNS(ctx); err != nil { return err }
	if err := r.restartWiFi(ctx); err != nil { return err }
	return nil
}

