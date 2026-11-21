# Contributing Router Recovery Adapters

**Local_DNS-DRP** is designed to support multiple routers through independent adapters behind a common interface. Each vendor/firmware implementation lives in its own file under `internal/recovery/` so HTTP endpoints, authentication, and payloads remain isolated from the DNS monitor.

Check the [supported routers table](README.md#supported-routers) before starting.

## 1. Identify the Exact Device

Include in your PR description:

- Manufacturer and exact model (e.g. `Askey RTF8316VW`)
- Hardware revision and firmware/build version
- ISP / region (if running ISP-customized firmware)
- Supported recovery actions (DHCP DNS update, Wi-Fi restart/re-enable, post-write verification)

## 2. Discover the Router API Safely (Optional AI Workflow)

You can use an AI agent (Claude, Codex, Antigravity, etc.) with browser automation (such as Chrome DevTools MCP) or sanitized manual captures to produce a **Router Discovery Report**:

- **Discovery prompt:** [docs/router-discovery-agent-prompt.md](docs/router-discovery-agent-prompt.md)
- **Report template:** [docs/router-discovery-report-template.md](docs/router-discovery-report-template.md)

**Sanitization is mandatory:** never include passwords, cookies, `Set-Cookie`, `Authorization` headers, CSRF tokens, session IDs, public IPs, MAC addresses, serial numbers, or raw HAR dumps. Replace sensitive values with `<REDACTED>` and use documentation IPs (`192.0.2.10`).

## 3. Implement the Adapter

Choose a stable lower-case kebab-case `ROUTER_MODEL` slug (e.g. `askey-rtf8316vw`, `asus-generic`, `tplink-archer-ax55`), implement `RouterRecovery` in `internal/recovery/router_<vendor>_<model>.go`, and register it in both `ValidateRouterConfig` (pure validation, no client creation) and `NewRouterRecovery` inside [`internal/recovery/router.go`](internal/recovery/router.go):

```go
type RouterRecovery interface {
	Recover(context.Context) error
}
```

### Adapter Requirements

- Respect `ctx` and use bounded HTTP timeouts.
- Authenticate fresh during recovery; never hardcode observed cookies or session tokens.
- Read existing LAN/DHCP/Wi-Fi settings first and preserve everything unrelated to DNS recovery.
- Verify every write with a follow-up read (never trust HTTP 200 alone).
- Treat write timeouts as ambiguous: check actual router state before retrying.
- Restore any temporarily disabled Wi-Fi radios/services on error or context cancellation using a bounded background context.
- Never log secrets or fail silently when firmware responses do not match expectations.

## 4. Add Deterministic Tests & Validate

Add `httptest`-based unit tests in `internal/recovery/` covering login, auth failures, expired sessions, ambiguous write timeouts, preserved settings, and cancellation cleanup. Tests must never contact a physical router or the Internet.

Run before opening a PR:

```bash
gofmt -w .
go test -race -count=1 ./...
go vet ./...
go build ./...
```

## 5. Codex Co-author Attribution (Local Git Hook)

To automatically append `Co-authored-by: Codex <noreply@openai.com>` to local commits:

```bash
bash scripts/setup-codex-attribution.sh
```

The hook skips merge commits, avoids duplicates, and supports one-off opt-out with `SKIP_AI_COAUTHORS=1`.
