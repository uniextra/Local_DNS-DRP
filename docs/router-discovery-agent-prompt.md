# Prompt: Research a Router for Local_DNS-DRP

Act as an HTTP protocol, browser automation, and Go engineer. Your task is to research, document, and prepare the implementation of a disaster-recovery adapter for **Local_DNS-DRP**.

This prompt is provider-agnostic (Claude, Codex, Antigravity, etc.). Use only the tools actually available to you (such as Chrome DevTools MCP if connected). Never invent endpoints, parameters, or behaviors that you did not observe or verify from an identifiable source.

## Objective

Research the target router to determine with verifiable evidence:

1. How the web UI authenticates and manages sessions/CSRF tokens.
2. How LAN/DHCP settings—especially DNS servers—are read.
3. How DHCP DNS servers are updated.
4. How Wi-Fi radio state is read and (only if explicitly authorized) toggled.
5. How every change is verified via a follow-up read.
6. How the router responds to invalid credentials, expired sessions, errors, and timeouts.

The primary deliverable is a completed **Router Discovery Report** following [`docs/router-discovery-report-template.md`](router-discovery-report-template.md).

## Router Details

Use the details provided by the user (mark missing items as pending; never guess):

- Manufacturer:
- Exact model:
- Hardware revision:
- Firmware / build:
- ISP / region:
- Local management URL:
- Specific goal or constraints:

## Mandatory Safety Rules

**Start in read-only mode.** Do not perform any write operation until the user has reviewed a concrete plan and explicitly authorized that operation.

Without explicit authorization, do **NOT**:

- Change DNS, DHCP, LAN, Wi-Fi, or any other setting.
- Disable or restart Wi-Fi radios.
- Reboot the router, factory-reset, upgrade firmware, or change passwords.
- Disable the firewall, access controls, or security features.
- Send `POST`/`PUT`/`PATCH`/`DELETE` requests that modify state, or blindly retry ambiguous writes.

Before any authorized write, explain the endpoint, method, payload fields, expected effect, potential connectivity loss, and rollback steps. Change one setting at a time, record the prior state, and read back the resulting state afterward.

Never store or output real credentials, cookies, `Set-Cookie`, `Authorization` headers, CSRF tokens, session IDs, API keys, public IPs, MACs, serial numbers, or private SSIDs. Replace sensitive values with `<REDACTED>` and use documentation IPs (`192.0.2.10`). Use a dedicated browser profile and never save raw HAR dumps.

## Workflow

1. **Phase 1 — Identify & Prepare:** Confirm exact model, hardware revision, firmware, and HTTP/HTTPS URL in read-only mode.
2. **Phase 2 — Observe Authentication:** Document login method, path, Content-Type, field names (never secret values), session/CSRF mechanism, and expiration behavior.
3. **Phase 3 — Read Configuration (Read-Only):** Document endpoints and sanitized responses for LAN IP/mask, DHCP enabled/range, DHCP DNS, and Wi-Fi radio state.
4. **Phase 4 — Analyze Writes:** Describe proposed write operations from observed HTML/JS/API without executing them unless the user authorizes a controlled single-step test.
5. **Phase 5 — Errors & Edge Cases:** Document invalid credentials, expired sessions, malformed responses, rejected writes, HTTP 200 without effect, and write timeouts (always treat timeouts as ambiguous and read back state first).
6. **Phase 6 — Generate Report:** Fill in `docs/router-discovery-report-template.md`, classifying every finding as **Observed**, **Inferido/Inferred**, or **Pending verification**, with sanitized evidence IDs (`E-01`, `E-02`, ...).
7. **Phase 7 — Prepare Go Implementation (Only After Report Review):** Design a `RouterRecovery` adapter under `internal/recovery/`, registered via `ROUTER_MODEL` in `NewRouterRecovery`, with `httptest` unit tests and zero browser/MCP runtime dependencies.

Be precise and conservative. Absence of evidence is not evidence of absence. Never present inferences as observed facts.
