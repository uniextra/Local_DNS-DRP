package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// RouterModelAsusGeneric is the experimental AsusWrt / AsusWrt-Merlin adapter.
	RouterModelAsusGeneric = "asus-generic"
	asusMaxWiFiUnits       = 8
	asusResponseLimit      = 2 << 20
)

var errAsusAPIResponse = errors.New("ASUS router returned an API error")

type asusHTTPError struct {
	status int
}

func (e *asusHTTPError) Error() string {
	return fmt.Sprintf("ASUS router HTTP status %d", e.status)
}

type AsusGenericRouter struct {
	cfg    RouterConfig
	base   *url.URL
	client *http.Client
	token  string
}

func newAsusGenericRouter(cfg RouterConfig) (*AsusGenericRouter, error) {
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(cfg.URL), "/"))
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, errors.New("ASUS router URL must be an absolute HTTP or HTTPS URL")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create ASUS session cookie jar: %w", err)
	}
	return &AsusGenericRouter{
		cfg:  cfg,
		base: base,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
	}, nil
}

func (r *AsusGenericRouter) endpoint(path string) string {
	u := *r.base
	u.Path = strings.TrimRight(r.base.Path, "/") + "/" + strings.TrimLeft(path, "/")
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (r *AsusGenericRouter) post(ctx context.Context, path, contentType string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint(path), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create ASUS request: %w", err)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "Local-DNS-DRP/1.0")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ASUS router request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, asusResponseLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read ASUS router response: %w", err)
	}
	if len(data) > asusResponseLimit {
		return nil, errors.New("ASUS router response exceeded the 2 MiB limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &asusHTTPError{status: resp.StatusCode}
	}
	return data, nil
}

func (r *AsusGenericRouter) login(ctx context.Context) error {
	// AsusWrt's local login endpoint receives a Base64 user:password value
	// in login_authorization. The returned asus_token is scoped to this host.
	r.token = ""
	r.client.Jar.SetCookies(r.base, []*http.Cookie{{Name: "asus_token", Path: "/", MaxAge: -1}})
	credentials := base64.StdEncoding.EncodeToString([]byte(r.cfg.Username + ":" + r.cfg.Password))
	body := []byte(url.QueryEscape("login_authorization=" + credentials))
	data, err := r.post(ctx, "login.cgi", "text/plain; charset=utf-8", body)
	if err != nil {
		return fmt.Errorf("ASUS login request failed: %w", err)
	}
	object, err := decodeAsusObject(data)
	if err != nil {
		return fmt.Errorf("ASUS login response was not valid: %w", err)
	}
	token := asusValueString(object["asus_token"])
	if token == "" {
		return errors.New("ASUS login response did not contain an authentication token")
	}
	r.token = token
	r.client.Jar.SetCookies(r.base, []*http.Cookie{{
		Name: "asus_token", Value: token, Path: "/",
	}})
	return nil
}

func decodeAsusObject(data []byte) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, errors.New("expected a JSON object")
	}
	if object == nil {
		return nil, errors.New("empty JSON object")
	}
	if raw, ok := object["error_status"]; ok {
		status := asusValueString(raw)
		if status != "" && status != "0" && !strings.EqualFold(status, "false") {
			return nil, fmt.Errorf("%w (error_status=%s)", errAsusAPIResponse, status)
		}
	}
	return object, nil
}

func asusValueString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	var generic any
	if json.Unmarshal(raw, &generic) == nil {
		switch v := generic.(type) {
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(v)
		case nil:
			return ""
		default:
			return strings.TrimSpace(fmt.Sprint(v))
		}
	}
	return ""
}

func isAsusAuthenticationError(err error) bool {
	if err == nil {
		return false
	}
	var httpErr *asusHTTPError
	return errors.As(err, &httpErr) && (httpErr.status == http.StatusUnauthorized || httpErr.status == http.StatusForbidden) ||
		errors.Is(err, errAsusAPIResponse)
}

func (r *AsusGenericRouter) readNVRAM(ctx context.Context, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return nil, errors.New("no ASUS NVRAM keys requested")
	}
	hooks := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" || strings.ContainsAny(key, "() ;") {
			return nil, fmt.Errorf("invalid ASUS NVRAM key %q", key)
		}
		hooks = append(hooks, "nvram_get("+key+")")
	}
	body := []byte(url.QueryEscape("hook=" + strings.Join(hooks, ";")))

	for attempt := 0; attempt < 2; attempt++ {
		if r.token == "" {
			if err := r.login(ctx); err != nil {
				return nil, err
			}
		}
		data, err := r.post(ctx, "appGet.cgi", "text/plain; charset=utf-8", body)
		var object map[string]json.RawMessage
		if err == nil {
			object, err = decodeAsusObject(data)
		}
		if err != nil && attempt == 0 && isAsusAuthenticationError(err) {
			r.token = ""
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read ASUS NVRAM: %w", err)
		}
		values := make(map[string]string, len(keys))
		for _, key := range keys {
			raw, ok := object[key]
			if !ok {
				continue
			}
			values[key] = asusValueString(raw)
		}
		return values, nil
	}
	return nil, errors.New("ASUS authentication failed after one refresh attempt")
}

func (r *AsusGenericRouter) pushAction(ctx context.Context, action map[string]any) error {
	if r.token == "" {
		if err := r.login(ctx); err != nil {
			return err
		}
	}
	if action == nil {
		return errors.New("empty ASUS action")
	}
	if _, exists := action["action_mode"]; !exists {
		action["action_mode"] = "apply"
	}
	body, err := json.Marshal(action)
	if err != nil {
		return fmt.Errorf("encode ASUS action: %w", err)
	}
	data, err := r.post(ctx, "applyapp.cgi", "text/plain; charset=utf-8", []byte(url.QueryEscape(string(body))))
	if err != nil {
		return fmt.Errorf("send ASUS action: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if _, err := decodeAsusObject(data); err != nil {
		return fmt.Errorf("ASUS action response: %w", err)
	}
	return nil
}

func (r *AsusGenericRouter) updateDHCPDNS(ctx context.Context) error {
	keys := []string{"dhcp_dns1_x", "dhcp_dns2_x"}
	current, err := r.readNVRAM(ctx, keys)
	if err != nil {
		return err
	}
	want1 := strings.TrimSpace(r.cfg.DNSPrimary)
	want2 := strings.TrimSpace(r.cfg.DNSSecondary)
	if want1 == "" || want2 == "" {
		return errors.New("ASUS DHCP DNS values must not be empty")
	}
	if current[keys[0]] == want1 && current[keys[1]] == want2 {
		return nil
	}

	// These are LAN DHCP DNS settings. Do not use wan*_dns* keys, which
	// configure the router's WAN resolver rather than DNS advertised to clients.
	writeErr := r.pushAction(ctx, map[string]any{
		"action_mode":  "apply",
		"dhcp_dns1_x": want1,
		"dhcp_dns2_x": want2,
	})
	// A timed-out/failed write may have reached the router. Never replay it
	// blindly: inspect the live NVRAM values first.
	after, readErr := r.readNVRAM(ctx, keys)
	if readErr != nil {
		if writeErr != nil {
			return fmt.Errorf("write ASUS DHCP DNS: %v; read-back failed: %w", writeErr, readErr)
		}
		return fmt.Errorf("verify ASUS DHCP DNS: %w", readErr)
	}
	if after[keys[0]] != want1 || after[keys[1]] != want2 {
		return fmt.Errorf("ASUS DHCP DNS read-back mismatch (primary=%q secondary=%q); write error=%v",
			after[keys[0]], after[keys[1]], writeErr)
	}

	// NVRAM values are only useful to DHCP clients after dnsmasq reloads.
	restartErr := r.pushAction(ctx, map[string]any{
		"action_mode": "apply",
		"rc_service":  "restart_dnsmasq",
	})
	// A service command can also lose its acknowledgement after dispatch.
	// Read the saved NVRAM values, but never replay the restart blindly.
	verified, err := r.readNVRAM(ctx, keys)
	if err != nil {
		return fmt.Errorf("verify ASUS DHCP DNS after dnsmasq restart: %w", err)
	}
	if verified[keys[0]] != want1 || verified[keys[1]] != want2 {
		return fmt.Errorf("ASUS DHCP DNS changed after dnsmasq restart (primary=%q secondary=%q; restart error=%v)",
			verified[keys[0]], verified[keys[1]], restartErr)
	}
	if restartErr != nil {
		return fmt.Errorf("ASUS DHCP DNS values are stored, but dnsmasq restart could not be confirmed: %w", restartErr)
	}
	return nil
}

func asusWiFiKeys() []string {
	keys := make([]string, 0, asusMaxWiFiUnits)
	for unit := 0; unit < asusMaxWiFiUnits; unit++ {
		keys = append(keys, fmt.Sprintf("wl%d_radio", unit))
	}
	return keys
}

func (r *AsusGenericRouter) readRadios(ctx context.Context) (map[int]string, error) {
	keys := asusWiFiKeys()
	values, err := r.readNVRAM(ctx, keys)
	if err != nil {
		return nil, err
	}
	radios := make(map[int]string)
	for unit := 0; unit < asusMaxWiFiUnits; unit++ {
		value, ok := values[keys[unit]]
		if !ok || value == "" {
			continue
		}
		if value != "0" && value != "1" {
			return nil, fmt.Errorf("ASUS Wi-Fi unit %d has unsupported radio state %q", unit, value)
		}
		radios[unit] = value
	}
	if len(radios) == 0 {
		return nil, errors.New("ASUS firmware did not expose any wlN_radio NVRAM values")
	}
	return radios, nil
}

func sortedRadioUnits(radios map[int]string) []int {
	units := make([]int, 0, len(radios))
	for unit := range radios {
		units = append(units, unit)
	}
	sort.Ints(units)
	return units
}

func (r *AsusGenericRouter) applyRadioStates(ctx context.Context, states map[int]string) error {
	if len(states) == 0 {
		return nil
	}
	action := map[string]any{
		"action_mode": "apply",
		"rc_service":  "restart_wireless",
	}
	for _, unit := range sortedRadioUnits(states) {
		state, err := strconv.Atoi(states[unit])
		if err != nil || (state != 0 && state != 1) {
			return fmt.Errorf("invalid target state %q for ASUS Wi-Fi unit %d", states[unit], unit)
		}
		action[fmt.Sprintf("wl%d_radio", unit)] = state
	}
	return r.pushAction(ctx, action)
}

func radiosMatch(actual, expected map[int]string) bool {
	for unit, state := range expected {
		if actual[unit] != state {
			return false
		}
	}
	return true
}

func (r *AsusGenericRouter) applyAndVerifyRadioStates(ctx context.Context, states map[int]string) error {
	writeErr := r.applyRadioStates(ctx, states)
	actual, readErr := r.readRadios(ctx)
	if readErr != nil {
		if writeErr != nil {
			return fmt.Errorf("write Wi-Fi radio state: %v; read-back failed: %w", writeErr, readErr)
		}
		return fmt.Errorf("verify ASUS Wi-Fi radio state: %w", readErr)
	}
	if !radiosMatch(actual, states) {
		return fmt.Errorf("ASUS Wi-Fi radio read-back mismatch (actual=%v expected=%v; write error=%v)", actual, states, writeErr)
	}
	// As with DNS, a lost acknowledgement is acceptable only when read-back
	// proves that the requested state is now active.
	return nil
}

func (r *AsusGenericRouter) cycleWiFi(ctx context.Context) error {
	original, err := r.readRadios(ctx)
	if err != nil {
		return err
	}
	enabled := make(map[int]string)
	disable := make(map[int]string)
	for unit, state := range original {
		if state == "1" {
			enabled[unit] = "1"
			disable[unit] = "0"
		}
	}
	// A router with all its radios intentionally disabled must stay that way.
	if len(enabled) == 0 {
		return nil
	}

	if err := r.applyAndVerifyRadioStates(ctx, disable); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		restoreErr := r.applyAndVerifyRadioStates(cleanupCtx, enabled)
		cancel()
		if restoreErr != nil {
			return fmt.Errorf("disable ASUS Wi-Fi failed: %v; restore failed: %w", err, restoreErr)
		}
		return err
	}

	timer := time.NewTimer(r.cfg.WiFiOff)
	select {
	case <-ctx.Done():
		timer.Stop()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		restoreErr := r.applyAndVerifyRadioStates(cleanupCtx, enabled)
		cancel()
		if restoreErr != nil {
			return fmt.Errorf("%w; restoring ASUS Wi-Fi failed: %v", ctx.Err(), restoreErr)
		}
		return ctx.Err()
	case <-timer.C:
	}

	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	restoreErr := r.applyAndVerifyRadioStates(cleanupCtx, enabled)
	cancel()
	if restoreErr != nil {
		return fmt.Errorf("restore ASUS Wi-Fi radios: %w", restoreErr)
	}

	// Confirm that radios which were originally off were not enabled as a side effect.
	finalState, err := r.readRadios(ctx)
	if err != nil {
		return fmt.Errorf("verify final ASUS Wi-Fi state: %w", err)
	}
	if !radiosMatch(finalState, original) {
		return fmt.Errorf("ASUS Wi-Fi state differs from its original state (actual=%v original=%v)", finalState, original)
	}
	return nil
}

func (r *AsusGenericRouter) Recover(ctx context.Context) error {
	// Establish a fresh session for each recovery cycle.
	r.token = ""
	if err := r.login(ctx); err != nil {
		return err
	}
	if err := r.updateDHCPDNS(ctx); err != nil {
		return err
	}
	if err := r.cycleWiFi(ctx); err != nil {
		return err
	}
	return nil
}
