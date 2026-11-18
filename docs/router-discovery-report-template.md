# Router Discovery Report

> Protocol research report for adding a router recovery adapter to **Local_DNS-DRP**.
>
> Fill in only what you know. Use `Not observed` or `Pending verification` instead of guessing, and attach sanitized evidence only.

## 1. Device Identification

| Field | Value |
|---|---|
| Manufacturer | |
| Exact commercial model | |
| Hardware revision | |
| Firmware / build | |
| ISP / region | |
| Management URL / protocol (HTTP/HTTPS) | |
| Proposed `ROUTER_MODEL` slug | |
| Research date | |
| AI agent used (optional) | |
| Browser / MCP tool used (optional) | |

## 2. Scope & Environment

- Connection used (LAN, Wi-Fi, wired):
- Browser and dedicated profile (if applicable):
- Firmware version and relevant conditions:
- Operations investigated:
  - [ ] Read LAN/DHCP configuration
  - [ ] Update DHCP DNS
  - [ ] Read Wi-Fi radio state
  - [ ] Disable / re-enable Wi-Fi radios
  - [ ] Authentication & session lifecycle
  - [ ] Error & expired-session handling
- Were any writes performed on the router? List them and confirm explicit authorization:
- Was router access maintained throughout testing?:

## 3. Authentication Flow

Describe the flow without secret values.

| Step | Method | Path (no sensitive params) | Expected status/response | Notes |
|---|---|---|---|---|
| Initial load / login | | | | |
| Credential submission | | | | |
| Session / token acquisition | | | | |
| Session verification | | | | |
| Logout (if applicable) | | | | |

- Auth type (form, Basic, Digest, token, other):
- Session mechanism (cookie, bearer, CSRF token, other):
- How is an expired session detected?:
- Is automatic renewal supported?:
- Any ordering, nonce, challenge, or dynamic field dependencies?:

**Do NOT include** cookies, CSRF tokens, passwords, `Authorization` headers, session IDs, or login values.

## 4. Configuration Reads

### LAN / DHCP

| Data | Method & path | Response field | Sanitized example | Classification |
|---|---|---|---|---|
| LAN IP / mask | | | | |
| DHCP range | | | | |
| Primary DNS | | | | |
| Secondary DNS | | | | |
| DHCP enabled | | | | |

### Wi-Fi

| Radio / band | Method & path | Response field | State / field | Classification |
|---|---|---|---|---|
| 2.4 GHz | | | | |
| 5 GHz | | | | |
| Other bands (if any) | | | | |

## 5. Observed Write Operations

> Complete only if writes were explicitly authorized.

| Operation | Method & path | Content-Type | Payload fields (no secrets) | Response | Post-write verification |
|---|---|---|---|---|---|
| Update DHCP DNS | | | | | |
| Disable radio | | | | | |
| Re-enable radio | | | | | |

Document for each operation:

- Prior state and settings that must be preserved.
- Required request sequence, if any.
- Whether the response confirms acceptance or only receipt.
- Which follow-up read proves the change took effect.
- Side effects or temporary connectivity loss.
- Rollback steps if a step fails.

## 6. Errors & Edge Cases

| Case | Observed code/response | How to detect it | Expected adapter behavior |
|---|---|---|---|
| Invalid credentials | | | |
| Expired session | | | |
| Invalid CSRF / nonce | | | |
| Malformed request | | | |
| Rejected write | | | |
| HTTP 200 without effect | | | |
| Write timeout | | | |
| Router unreachable | | | |

## 7. Attached Evidence

| ID | File or reference | What it proves | How it was sanitized |
|---|---|---|---|
| E-01 | | | |
| E-02 | | | |
| E-03 | | | |

Attach minimal sanitized request/response fixtures under `testdata/routers/<router-model>/` when helpful. Never attach raw HAR files or full router backups.

## 8. Findings Classification

Mark each conclusion as **Observed**, **Inferred**, or **Pending verification**:

| Area | Status | Evidence | Risk / limitation |
|---|---|---|---|
| Login / session | | | |
| DHCP DNS read | | | |
| DHCP DNS write | | | |
| Wi-Fi state read | | | |
| Wi-Fi control | | | |
| Cross-firmware compatibility | | | |

## 9. Implementation Recommendation

- Viable to implement in Go without a browser?:
- Minimum required endpoints:
- Reads required before writing:
- Reads required to verify writes:
- Cleanup / restoration needed on error:
- Known risks:
- Exact firmware versions covered:
- `httptest` cases to include:

## 10. Security Checklist

- [ ] No real passwords or usernames.
- [ ] No cookies, CSRF tokens, session IDs, or `Authorization` headers.
- [ ] No public IPs, serial numbers, MAC addresses, or personal data.
- [ ] No private SSIDs or account details.
- [ ] All captures, logs, and fixtures manually reviewed.
- [ ] No unauthorized configuration changes performed.

## 11. Summary

Summarize what was discovered, what was tested, what remains pending, and whether you recommend implementing the adapter.
