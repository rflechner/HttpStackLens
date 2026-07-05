# AGENTS.md

## Project Intent

HttpStackLens is a local development proxy.

The long-term goal is to become an alternative to tools like PX Proxy or CNTLM,
with additional traffic inspection features inspired by Fiddler, Charles, or
mitmproxy.

The target use case is developers working behind corporate proxies that require
Windows authentication, while still wanting visibility into HTTP traffic from
tools such as curl, npm, NuGet, Git, IDEs, package managers, and local apps.

This project is for local development only, not production or shared-network
deployment.

## Product Direction

Prioritize:
- Reliable local proxy behavior.
- Corporate proxy compatibility.
- Windows authentication support.
- Clear observability of HTTP traffic.
- A simple Web UI for live inspection.
- Developer-tool compatibility.

Do not prioritize:
- Production hardening.
- Multi-user deployment.
- Cloud-hosted operation.
- Enterprise admin features.
- Complex plugin systems before the proxy core is solid.

## Current Capabilities

The app currently:
- Starts a local HTTP proxy.
- Supports HTTP requests.
- Supports HTTPS `CONNECT` tunneling.
- Supports opt-in HTTPS decryption through local MITM when configured.
- Can forward traffic to an upstream proxy.
- Has Windows-specific authentication support.
- Serves a Web UI using Go/WASM.
- Streams proxy events to the UI using SSE.
- Serves an OpenAPI contract for the Web UI API at `/openapi.yaml`.

## Important Design Constraints

HTTPS traffic can run in two modes:
- plain tunneling mode, where `CONNECT` traffic is forwarded as opaque TLS;
- MITM inspection mode, where `decrypt_https.enabled` uses a local CA to decrypt
  and inspect HTTPS traffic.

HTTPS interception must remain opt-in and must clearly separate plain tunneling
mode from MITM inspection mode.

Avoid silently weakening security.

## Architecture Notes

The main flow is:

1. `main.go` reads configuration.
2. OS-specific setup builds the proxy middleware pipeline.
3. The Web UI starts.
4. The TCP proxy server accepts browser/client connections.
5. The first proxy request is parsed.
6. The request is logged to the UI.
7. A middleware handles forwarding, tunneling, or authentication.

Key areas:
- `configuration/`: config loading and DTO conversion.
- `http/`: low-level HTTP parsing and stream helpers.
- `proxy/`: pipeline composition.
- `proxy/middlewares/`: actual proxy behavior.
- `security/`: Windows authentication helpers.
- `webui/`: embedded Web UI server.
- `webui/wasm/`: client-side UI logic.
- `webui/wwwroot/openapi.yaml`: OpenAPI contract for Web UI HTTP endpoints.

## Coding Guidance

Prefer simple Go code over clever abstractions.

This is also a Go learning project, so readability matters. When adding new
code:
- Keep error handling explicit.
- Keep concurrency understandable.
- Prefer small focused types.
- Avoid premature framework-style architecture.
- Add tests around parsing, proxy protocol behavior, and authentication edge
  cases.

## Networking Guidance

Be careful with:
- half-closed TCP connections
- `CONNECT` behavior
- `Proxy-Authorization` vs `Authorization`
- upstream proxy `407` responses
- connection reuse
- request-target forms:
  - origin-form: `/path`
  - absolute-form: `http://host/path`
  - authority-form: `host:port` for `CONNECT`

When changing forwarding behavior, test with at least:
- `curl`
- a plain HTTP URL
- an HTTPS URL through `CONNECT`
- an upstream proxy if possible

## UI Guidance

The Web UI should behave like a traffic inspector, not a marketing site.

Prioritize:
- dense readable session list
- request/response details
- headers
- status codes
- timing
- filtering/search
- export/replay later

Avoid decorative UI that makes inspection slower.

## API Documentation

The Web UI HTTP API is documented in `webui/wwwroot/openapi.yaml` and served by
the app at `/openapi.yaml`.

When adding, removing, or changing Web UI endpoints, request/response DTOs, query
parameters, status codes, or SSE event payloads, update `openapi.yaml` in the
same change. Keeping this contract current is required so the local API remains
usable from tools such as Swagger UI, Redoc, Bruno, Postman, and Insomnia.

## Build

Recommended build command:

```sh
go run .\build-tools\main.go
```

Useful variants:

```sh
go run .\build-tools\main.go webui
go run .\build-tools\main.go app
```

## Testing

Run Go tests before finalizing proxy/parser changes:

```sh
go test ./...
```

On Windows, authentication-related changes may require manual validation because
they depend on SSPI and corporate proxy behavior.

## Version Control

The maintainer handles all commits personally. Do not run `git commit`, `git push`,
or create branches/tags unless explicitly asked. Leave changes staged or unstaged
in the working tree and let the maintainer review and commit them.

## Security Notes

This tool may handle sensitive traffic.

Do not log secrets casually. Be especially careful with:
- `Authorization`
- `Proxy-Authorization`
- cookies
- tokens
- request/response bodies

If adding persistence, exports, HAR files, or debug logs, consider redaction
controls.

## Future Roadmap Ideas

Possible future features:
- better upstream proxy auth flow
- request/response capture
- HAR export
- filtering/search
- replay requests
- rules for header/body rewriting
- further HTTPS MITM controls and diagnostics
- per-tool profiles
- system proxy auto-configuration
- diagnostics page for corporate proxy issues
