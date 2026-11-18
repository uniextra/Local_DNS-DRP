## Router Support

- [ ] This PR adds or modifies a router recovery adapter.
- [ ] The adapter is registered in `NewRouterRecovery` (`internal/recovery/router.go`) with a stable `ROUTER_MODEL` identifier.
- [ ] The supported routers table in `README.md` has been updated.
- [ ] Tests use mocked HTTP servers (`httptest`) and sanitized fixtures; they do not contact a physical router.
- [ ] A completed [Router Discovery Report](docs/router-discovery-report-template.md) is attached, or an explanation is provided for why it does not apply.

## Device & Firmware

- Manufacturer and exact model:
- Hardware revision:
- Firmware / build:
- ISP / region (if customized firmware):
- `ROUTER_MODEL` identifier:
- AI agent used (optional: Claude, Codex, Antigravity, etc.):
- Browser / MCP tool used (optional):

## Recovery Behavior

- [ ] DHCP DNS update
- [ ] Wi-Fi radio restart/re-enable (if applicable)
- [ ] Post-write verification via read-back
- [ ] Safe restoration on errors or context cancellation

Describe the implemented operations and known limitations:

## Evidence

Describe the endpoints and sanitized request/response examples, or firmware evidence included. Distinguish between **observed**, **inferred**, and **pending verification**. Confirm that all passwords, cookies, session IDs, tokens, serial numbers, MAC addresses, and personal data have been removed.

## Validation

- [ ] `gofmt -w .`
- [ ] `go test -race -count=1 ./...`
- [ ] `go vet ./...`
- [ ] `go build ./...`
- [ ] Tested against a physical router (optional; state firmware and outcome):

## Security

- [ ] No real credentials, tokens, cookies, session IDs, or unsanitized captures are included.
- [ ] The adapter never logs secrets or modifies settings unrelated to recovery.
- [ ] Writes are verified by reading back the resulting router state.
