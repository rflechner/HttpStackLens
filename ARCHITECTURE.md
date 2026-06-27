# Architecture

HttpStackLens is a local development proxy that lets you inspect HTTP(S) traffic
from developer tools (curl, npm, NuGet, Git, IDEs…) while staying compatible with
corporate proxies that require Windows authentication. It can additionally
decrypt HTTPS traffic on the fly (opt-in MITM) and streams what it sees to a
local Web UI.

This document describes how the pieces fit together. For product intent and
contribution guidance, see [AGENTS.md](AGENTS.md).

## High-level picture

```
                         ┌───────────────────────────────────────────────┐
                         │                  main.go                      │
                         │  read config → build pipeline → start servers │
                         └───────────────┬───────────────┬───────────────┘
                                         │               │
                  ┌──────────────────────┘               └────────────────────┐
                  ▼                                                           ▼
        ┌────────────────────┐                                      ┌──────────────────────┐
        │  Proxy TCP server  │                                      │   Web UI HTTP server │
        │  (proxy_server.go) │                                      │   (webui/ui_server)  │
        └─────────┬──────────┘                                      └──────────┬───────────┘
                  │ one goroutine per connection                               │ SSE /events
                  ▼                                                            ▼
        ┌───────────────────────────────┐                          ┌─────────────────────┐
        │     Middleware pipeline       │   logs requests ───────► │   Hub (fan-out)     │
        │  (proxy/middlewares/*)        │   via EventLogger        └──────────┬──────────┘
        └───────────────────────────────┘                                     │ event stream
                  │ uses                                                      ▼
                  ▼                                             ┌──────────────────────────┐
        ┌───────────────────┐                                   │  Browser: Go/WASM client │
        │   CertStore /CA   │                                   │  (webui/wasm + wwwroot)  │
        │  (certManager/*)  │                                   └──────────────────────────┘
        └───────────────────┘
```

Two servers run in the same process:

- the **proxy** (raw TCP) that browsers / tools point at, and
- the **Web UI** (standard `net/http`) that serves the inspector and pushes live
  events over Server-Sent Events.

They are decoupled: the proxy emits events through a `ProxyEventLogger`
interface; the Web UI implementation of that interface publishes to a `Hub`.

## Startup flow (`main.go`)

1. `configuration.ReadConfiguration()` loads `config.yaml` (falls back to
   defaults on error).
2. `CreateOsSpecificProxyPipeline(config)` builds the middleware chain. This is
   an OS-specific function (see [Multi-OS strategy](#multi-os-strategy)) that
   also parses CLI flags overriding config.
3. Logging is initialized (`logging.Setup`).
4. The Web UI starts (`webui.ServeWebUi`), returning the `Hub`.
5. The CA is loaded/generated (`certManager.GetHttpsDebugRootCertificates`) and a
   `CertStore` is created.
6. If `proxy.decrypt_https` is enabled, the CA is installed into the OS trust
   store and an `HttpsInterceptor` is wrapped around the pipeline.
7. The proxy server is created with the pipeline + `CertStore` and `Run()` in a
   goroutine.
8. The main goroutine waits for `exit` on stdin, then shuts everything down via
   `stopChan`.

## Request lifecycle (the proxy)

`proxy_server.go` (package `main`) is the TCP front door:

1. `Run()` accepts a connection and spawns a goroutine per client.
2. The connection is wrapped in a `*http.NetworkStream` — a `net.Conn` with a
   single persistent buffered reader/writer, so bytes pulled off the socket
   alongside the request headers are never lost downstream.
3. `http.ReadProxyRequest(stream)` parses the request line + headers into a
   `models.ProxyRequest`.
4. The request is reported to the UI via `EventLogger.LogRequest`.
5. `pipeline.HandleProxyRequest(stream, request)` runs the middleware chain,
   which owns the rest of the conversation (forwarding/tunneling/decrypting).

## Middleware pipeline

All proxy behavior is expressed as middlewares implementing a single interface
(`proxy/middlewares/middleware.go`):

```go
type Middleware interface {
    HandleProxyRequest(browser net.Conn, request models.ProxyRequest) error
}
```

Middlewares either handle the connection themselves or delegate to a `Next`
middleware, so the chain is composed by nesting. Composition lives in
`proxy/proxy_server.go` (`ConfigureProxyPipelineBase`) and the OS-specific
`ConfigureOsSpecificProxyPipeline`.

| Middleware | File | Role |
|---|---|---|
| `TunnelServer` | `tunnel_server.go` | Default base. Dials the target; for `CONNECT` answers `200` then blind-pipes bytes (no decryption); for plain HTTP forwards the request. |
| `ForwardProxyServer` | `forward_proxy_server.go` | Base when an upstream proxy is configured. Forwards everything to the upstream gateway. |
| `WindowsAuthenticationServerMiddleware` | `authentication_server_windows.go` | Wraps the base. Authenticates the **browser → this proxy** via NTLM/Negotiate (407 challenge loop) before delegating. |
| `ForwardProxyServerWithWindowsAuthentication` | `forward_proxy_server_with_authentication_windows.go` | Adds Windows auth toward the **upstream** proxy. |
| `HttpsInterceptor` | `https_interceptor.go` | Opt-in MITM. On `CONNECT`, decrypts TLS instead of tunneling; otherwise delegates to `Next`. |

Typical chain (Windows, no upstream, decryption on):

```
HttpsInterceptor → WindowsAuthenticationServerMiddleware → TunnelServer
```

`HttpsInterceptor` is added in `main.go` (not in the OS pipeline builders) so the
OS-specific pipeline signatures stay untouched. Because it only acts on
`CONNECT` and delegates everything else, sitting at the outermost position is
acceptable for the common local-debug case.

### HTTPS interception (MITM)

When `proxy.decrypt_https` is true, `HttpsInterceptor.intercept` does:

1. Reply `200 Connection Established` to the browser.
2. `tls.Server` over the browser connection, selecting a certificate per SNI via
   `CertStore.GetCertificate(serverName)`.
3. Complete the TLS handshake → the browser stream is now readable in clear text.
4. Loop over keep-alive requests with `http.ReadRequest`.
5. Replay each request to the real server through an `http.Transport` (real TLS,
   real certificate verification). `Accept-Encoding` is stripped so the body
   comes back uncompressed.
6. If the response is HTML/JS, print it to the console (current milestone:
   proving decryption + re-encryption works).
7. Re-serialize the response back to the browser over the terminated TLS session
   with a `Content-Length` framing.

HTTP/2 is intentionally not advertised (no `h2` in ALPN), so clients fall back to
HTTP/1.1, which the `net/http` request/response (de)serialization handles.

## Certificate management (`certManager/`)

The MITM needs a local CA and a fresh leaf certificate per visited domain.

| File | Responsibility |
|---|---|
| `cert_helpers.go` | Generate/load the CA (`GenerateCA`, `LoadCA`), sign per-domain leaf certs (`signServerCert` with a **random 128-bit serial**), persist them as PEM (`saveDomainCert`), and create parent folders on demand (`ensureParentDir`). |
| `cert_store.go` | `CertStore`: thread-safe per-domain certificate cache (memory → disk → generate). Returns `*tls.Certificate` ready for the TLS handshake and optionally installs new domain certs into the OS personal store. |
| `cert_install.go` | OS-agnostic contract: the `CertInstaller` interface + a `noopInstaller`. |
| `cert_install_windows.go` | Native Windows implementation via `crypt32.dll` syscalls — adds the CA to the current user's `Root` store and domain certs to `MY`. No admin rights, no external process. |
| `cert_install_other.go` / `cert_install_darwin.go` | No-op installer on non-Windows OSes — the user installs manually. |

`CertStore` resolution order for a domain:

1. **memory cache** (`map` guarded by `sync.RWMutex`, double-checked locking);
2. **disk** (`tls.LoadX509KeyPair` of a previously issued pair);
3. **generate**, sign, persist, and (if a `CertInstaller` is wired) install.

Installation is best-effort: failures are logged, never fatal, because trusting
the CA is what actually makes interception work — a missing personal-store entry
must not break a request.

The serial number is randomized per leaf to avoid
`SEC_ERROR_REUSED_ISSUER_AND_SERIAL`: a CA must never reuse a serial across
certificates.

## Configuration (`configuration/`)

`config.go` defines `AppConfig` (proxy, webui, cert_manager, logging) and loads
it from `config.yaml` with `goccy/go-yaml`. Key flags:

- `proxy.decrypt_https` — enable HTTPS MITM.
- `proxy.output_proxy_uri` — upstream proxy.
- `proxy.require_windows_authentication` / `add_windows_authentication_to_output_proxy`.
- `cert_manager.ca_cert_file` / `ca_key_file` / `domain_certs_folder`.

`AppConfig.ToDto()` converts to a sanitized DTO (`webui/wasm/shared`) that is
safe to expose to the browser over `/config` — a deliberate duplicate type so
internal-only fields never leak to the UI.

## Web UI (`webui/`)

`ui_server.go` runs a standard `net/http` server with:

- `/` + `/css /js /images /wasm` — static assets (the UI is a Go program compiled
  to WebAssembly).
- `/events` — **Server-Sent Events** stream.
- `/config`, `/certificates-infos` — JSON endpoints consumed by the WASM client.

Live traffic uses a small pub/sub **`Hub`**: SSE clients `subscribe()` to a
buffered channel; `Publish(eventType, data)` fans messages out to all clients and
**drops messages for slow clients** (non-blocking send) so one stuck browser
can't stall the proxy.

### Client (`webui/wasm/`)

`webui/wasm/main.go` is compiled with `GOOS=js GOARCH=wasm` into
`wwwroot/wasm/app.wasm` and loaded by `wwwroot/index.html` via `wasm_exec.js`.
`webui/wasm/shared/dto.go` holds the DTOs shared between the Go backend and the
WASM frontend (same types on both sides of the wire).

### Asset embedding: dev vs prod

A build tag swaps the filesystem backing the UI:

- `fs_prod.go` (`!dev`) — `//go:embed wwwroot`, assets baked into the binary.
- `fs_dev.go` (`dev`) — `os.DirFS(".")`, edit HTML/CSS/JS without rebuilding.

## Logging (`logging/`)

`ProxyEventLogger` is the seam between the proxy and any sink:

- `WebUiEventLogger` — marshals events to JSON and `Hub.Publish`es them to the UI.
- `ConsoleEventLogger` — prints to stdout.
- `logger.go` — `slog`-based structured logging setup with an optional file sink.

## Multi-OS strategy

Platform differences are handled with Go build tags and per-OS files rather than
runtime branches:

- **Proxy pipeline**: `proxy/proxy_server_windows.go` adds the Windows auth
  middlewares; `proxy/proxy_server_darwin.go` builds only the base. App-level
  wiring mirrors this in `app_windows.go` / `app_darwin.go`.
- **Certificate install**: `cert_install_windows.go` (native crypt32) vs
  `cert_install_other.go` / `cert_install_darwin.go` (no-op). The shared
  `CertInstaller` contract means callers never branch on OS — an unimplemented
  platform simply does nothing.
- **Security/SSPI**: `security/auth_*_windows.go` vs `security/auth_server_darwin.go`.

The rule: a shared, OS-agnostic contract in a tag-free file, and one
implementation file per platform selected by `//go:build`.

## HTTP layer (`http/`)

A small hand-rolled HTTP toolkit tuned for proxy semantics:

- `network_stream.go` — `NetworkStream`, the buffered `net.Conn` wrapper that
  prevents byte loss across the pipeline.
- `http_request_reader.go` — `ReadProxyRequest` / `ReadHttpResponse` /
  `ReadHttpResponseBody` (Content-Length and chunked decoding).
- `models/` — request/response/endpoint types, including request-target forms
  (origin / absolute / authority for `CONNECT`).
- `parser/` — request-line/header/status parsers built on the
  `EasyParsingForGo` combinator library.

Note: the MITM path (`HttpsInterceptor`) deliberately uses the standard library
`net/http` for the decrypted inner requests, while the outer proxy protocol uses
this custom parser.

## Security / Windows authentication (`security/`)

`security/` wraps SSPI (via `alexbrainman/sspi`) to perform NTLM/Negotiate:

- `auth_server_windows.go` — server side (`ServerAuth`, token validation) used by
  the auth middleware's 407 challenge loop.
- `auth_client_windows.go` — client side, used when authenticating to an upstream
  proxy.
- `auth_server_darwin.go` — stubs so the project compiles on macOS.

Handle `Authorization` vs `Proxy-Authorization` and upstream `407`/`401` carefully
here — see the networking notes in [AGENTS.md](AGENTS.md).

## Build tooling (`build-tools/`)

`build-tools/main.go` is a tiny build orchestrator:

- `go run ./build-tools/main.go webui` — `npm install`, copy `wasm_exec.js`,
  build `app.wasm` (`GOOS=js GOARCH=wasm`), build Tailwind CSS.
- `go run ./build-tools/main.go app` — build the stripped native binary.
- no arg — both.

## Directory map

```
main.go, proxy_server.go, app_context.go, app_*.go   Process entry + TCP proxy front door
configuration/                                        Config loading + UI-safe DTO conversion
proxy/                                                Pipeline composition (per-OS)
proxy/middlewares/                                    Proxy behaviors (tunnel, forward, auth, MITM)
certManager/                                          CA, per-domain certs, cache, OS trust-store install
http/                                                 Custom HTTP parsing + buffered stream
http/models/, http/parser/                            HTTP types and combinator parsers
security/                                             SSPI / Windows authentication
webui/                                                Embedded Web UI server + SSE hub
webui/wasm/                                           Go/WASM client + shared DTOs
webui/wwwroot/                                        Static assets (html/css/js/wasm)
logging/                                              Event loggers + structured logging
build-tools/                                          WASM/CSS/native build orchestration
```
