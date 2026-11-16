## Router support

- [ ] This PR adds or changes a router recovery adapter.
- [ ] The adapter is registered in `newRouterRecovery` and has a stable `ROUTER_MODEL` identifier.
- [ ] The README supported-router table and recovery documentation are updated.
- [ ] Automated tests use a fake router/server and sanitized fixtures; they do not contact a physical router.

## Device and firmware

- Manufacturer and exact model:
- Hardware revision:
- Firmware/build:
- ISP and region (if firmware is customized):
- `ROUTER_MODEL` identifier:

## Recovery behavior

- [ ] DHCP DNS update
- [ ] Wi-Fi/radio restart (if applicable)
- [ ] Read-back verification
- [ ] Safe restoration after errors/cancellation

Describe the operations implemented and any known limitations:

## Evidence

Describe the sanitized endpoints/request/response examples or firmware evidence included. Confirm that passwords, cookies, session IDs, tokens, serial numbers, MAC addresses and personal data have been removed.

## Validation

- [ ] `gofmt -w *.go`
- [ ] `go test -race -count=1 ./...`
- [ ] `go vet ./...`
- [ ] `go build ./...`
- [ ] Physical router test performed (optional; describe exact firmware and result):

## Security

- [ ] No router credentials, tokens, cookies, session IDs or unredacted captures are committed.
- [ ] The adapter does not log secrets or make unrelated configuration changes.
- [ ] Writes are verified by reading the resulting router state.
