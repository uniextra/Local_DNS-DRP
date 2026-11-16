# Contributing router recovery adapters

This project accepts contributions that add automatic recovery support for other router models and firmware versions. Router support is implemented as a dedicated adapter so that vendor-specific HTTP endpoints, authentication, payloads and state handling do not leak into the DNS monitor.

Before starting, check the [supported router list](README.md#añadir-soporte-para-otro-router) and open an issue or discussion if you are unsure whether the model/firmware is already covered. A pull request is welcome once you have enough information to implement and test the adapter.

## 1. Identify the exact router

Please include all of the following in the PR description:

- Manufacturer and exact commercial model (for example, `Askey RTF8316VW`).
- Hardware revision/version printed on the label or shown in the administration UI.
- Firmware version/build number, including ISP-customized firmware if applicable.
- Country/region and ISP, when the firmware is ISP-provided.
- The exact recovery actions the router supports and which ones your adapter implements:
  - Update the DNS servers distributed by the router's DHCP service.
  - Restart/re-enable Wi-Fi radios, if applicable.
  - Read back and verify each change.

Do not assume that two devices with the same retail model use the same API. Different hardware revisions, ISP builds and firmware versions can expose different endpoints or fields. If support is firmware-specific, state the tested firmware version explicitly.

## 2. Gather evidence safely

Useful evidence includes sanitized HTTP request/response examples, endpoint paths, relevant HTML/JavaScript fields, XML/JSON payloads, and before/after router state.

Remove or replace all sensitive information before attaching evidence:

- Passwords, usernames where they identify a person, API keys and tokens.
- Cookies, session IDs, CSRF tokens and authorization headers.
- Public IP addresses, serial numbers, MAC addresses and personally identifying account details.
- Wi-Fi SSIDs and any private hostnames you do not want to disclose.

Use placeholders such as `<REDACTED>`, `192.0.2.10` and `example.invalid`. Never commit an unredacted HAR, browser capture, packet capture, router backup or configuration export. Do not include router credentials in the PR, issue, screenshots, test fixtures or logs.

Document whether the administration interface uses HTTP or HTTPS, how authentication works, how sessions expire, and how the router reports rejected or partially applied writes. HTTP 200 alone is not proof that a setting changed.

## 3. Select a ROUTER_MODEL identifier

The Docker environment variable `ROUTER_MODEL` selects the adapter. Use a stable lower-case kebab-case identifier based on manufacturer and model, for example:

- `askey-rtf8316vw`
- `tplink-archer-ax55`
- `fritzbox-7530-ax`

Use the same identifier in the adapter factory, tests, README supported-router table and Compose example. Do not use a hostname, IP address, firmware version or user-defined alias as the identifier.

For backwards compatibility, omitting `ROUTER_MODEL` selects `askey-rtf8316vw`. When `ROUTER_PASSWORD` or `ROUTER_PASSWORD_FILE` enables recovery, an unknown identifier is a configuration error. It must never fall back to the Askey adapter.

## 4. Implement a dedicated adapter

The common contract is:

```go
type RouterRecovery interface {
    Recover(context.Context) error
}
```

Add the vendor/model implementation in its own file, preferably named `router_<vendor>_<model>.go`. Keep the vendor-specific login, session, HTTP requests, parsing and write/read-back logic inside that adapter.

A simplified outline:

```go
type TPLinkArcherAX55Router struct {
    cfg    RouterConfig
    client *http.Client
}

func newTPLinkArcherAX55Router(cfg RouterConfig) RouterRecovery {
    return &TPLinkArcherAX55Router{
        cfg:    cfg,
        client: &http.Client{Timeout: 30 * time.Second},
    }
}

func (r *TPLinkArcherAX55Router) Recover(ctx context.Context) error {
    // 1. Authenticate and establish a fresh session.
    // 2. Read the current DHCP/Wi-Fi configuration.
    // 3. Apply only the requested recovery changes.
    // 4. Read back the configuration and verify the changes.
    // 5. Restore any temporarily disabled radios, even on cancellation.
    return nil
}
```

This is an illustration, not a working implementation. Use the router's documented or observed API; do not copy the Askey endpoints or payloads unless they are genuinely the same protocol.

Register the adapter in `newRouterRecovery` in `router_recovery.go`:

```go
switch model {
case RouterModelAskeyRTF8316VW:
    return newAskeyRouter(cfg), nil
case "tplink-archer-ax55":
    return newTPLinkArcherAX55Router(cfg), nil
default:
    return nil, fmt.Errorf("unsupported ROUTER_MODEL %q", model)
}
```

Also add a named constant for the new model identifier. Keep the default model unchanged unless the project explicitly decides to change it.

### Adapter requirements

A production adapter should:

- Honour the supplied context and use bounded HTTP timeouts.
- Never log credentials, cookies, session IDs or authorization data.
- Validate the router response and reject malformed or unexpected state.
- Read the existing settings before changing them; preserve unrelated LAN, DHCP and Wi-Fi settings.
- Verify changes by reading the resulting state back. Do not trust HTTP 200 or an optimistic API response.
- Treat ambiguous write timeouts as unknown outcomes and verify before retrying.
- Handle expired sessions deliberately; do not loop indefinitely on login failures.
- Restore radios/services that it temporarily disabled, including when the caller's context is cancelled. Use a separate bounded cleanup context where necessary.
- Avoid rebooting the router or changing settings unrelated to the recovery objective.
- Return actionable errors without leaking secrets.
- Fail safely when the firmware/API is not the one tested by the adapter.

If the model does not have Wi-Fi radios or the requested operation is not applicable, document that limitation explicitly. Do not claim that an operation was performed when the hardware does not support it.

## 5. Add automated tests

Tests must run without a physical router and without Internet access. Use `net/http/httptest` or another deterministic fake router server and sanitized fixtures under `testdata/routers/<router-model>/` where appropriate.

At minimum, cover:

- Successful authentication and recovery.
- Rejected credentials and expired/invalid sessions.
- Missing, malformed or unexpected response fields.
- Successful writes followed by read-back verification.
- HTTP 200 where the requested state was not actually applied.
- Ambiguous write failures/timeouts, verifying state before retrying.
- Preservation of unrelated router settings.
- Cancellation/error handling and restoration of any temporarily disabled radios/services.
- Rejection of unsupported model identifiers by the adapter selector.

Do not make tests contact a real router, use real credentials or depend on a user's home network. A physical-router test is useful as additional evidence, but it does not replace deterministic CI tests.

Run these checks before submitting:

```bash
gofmt -w *.go
go test -race -count=1 ./...
go vet ./...
go build ./...
```

The repository CI also builds the Docker image for `linux/amd64` and `linux/arm64`.

## 6. Update the documentation and Compose example

A router-support PR must update:

1. The supported-router table in `README.md`, including the exact tested hardware/firmware revision.
2. The `ROUTER_MODEL` environment variable description if the model list or selection behavior changes.
3. The recovery sequence and any router-specific limitations.
4. The Compose example only if a new common configuration variable is genuinely required.

Do not add router credentials, cookies, session keys, or model-specific secrets to `docker-compose.yml`. Use the existing `ROUTER_PASSWORD_FILE` mechanism for the router password.

## 7. Open the pull request

Create a branch from the latest `main`, add the adapter and tests, and open a PR using the repository template. Include:

- Exact router model, hardware revision and firmware/build.
- ISP/region if firmware is customized.
- Sanitized evidence and a short explanation of the authentication/API flow.
- The recovery operations implemented and any limitations.
- Test coverage and the commands/results from the local test run.
- Whether the adapter has also been tested against the physical router.

A contribution is not considered supported merely because the router model appears in the PR title. It must have a registered adapter, deterministic tests, updated documentation and passing CI. Maintainers may ask for further firmware-specific evidence before listing it as supported.

Thank you for helping make router recovery useful beyond the original Askey device.
