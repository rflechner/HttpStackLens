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

## Capture file format (`.capture`)

A capture session is persisted to a single binary file with the `.capture`
extension: a fixed header followed by a flat, append-friendly stream of records,
each record being either an HTTP **request** or an HTTP **response**. Responses
reference their request by id, so a request and its response do not need to be
adjacent — the file can be written incrementally as traffic flows.

### Conventions

- **Byte order: little-endian** for every integer. This matches the default of
  C#'s `BinaryWriter` / `BinaryReader`, so an interop reader/writer is trivial.
- **`bool`** — 1 byte, `0x00` = false, `0x01` = true.
- **`uuid`** — 16 raw bytes (RFC 4122 layout), no string formatting.
- **`lpstring` (length-prefixed string)** — `uint32` byte length (little-endian)
  followed by that many UTF-8 bytes. No NUL terminator. An empty string is a
  `uint32` `0` with no payload.

  > Note: this is a plain `uint32` prefix, **not** the 7-bit-encoded length that
  > `BinaryWriter.Write(string)` emits. A C# interop layer must read/write the
  > `uint32` length explicitly rather than using `Write(string)` / `ReadString`.

- **`httpversion` (uint8 enum)** — the HTTP version packed into one byte:
  **high nibble = major, low nibble = minor**, so it maps directly onto the
  project's `Version{Major, Minor}` (`major = b >> 4`, `minor = b & 0x0F`).
  `0x00` means unknown/unspecified.

  ```
  0x00  Unknown
  0x10  HTTP/1.0
  0x11  HTTP/1.1
  0x20  HTTP/2.0
  0x30  HTTP/3.0
  ```

- **`headers` (header collection)** — `int32` header count, then that many
  `(name: lpstring, value: lpstring)` pairs, preserving on-the-wire order and
  duplicates.
- **`blob` (length-prefixed bytes)** — `int64` byte length followed by the raw
  bytes (`-1` means "absent", `0` means "present but empty"). Used for bodies,
  which may be large or binary.
- **`crc32c` (record checksum)** — `uint32` CRC32-C (Castagnoli polynomial,
  `hash/crc32` with `crc32.Castagnoli`) computed over **all preceding bytes of
  the record**, i.e. the `record_type` byte through the last body byte. Written
  as the record's trailer. It is non-cryptographic: it detects accidental
  corruption, not tampering.

### File layout

```
┌──────────────┐
│    Header    │   fixed size
├──────────────┤
│   Record 0   │ ┐
│   Record 1   │ │  records_count records,
│   Record 2   │ │  each tagged Request or Response
│     ...      │ ┘
└──────────────┘
```

### Header

```
Offset  Size  Type      Field            Notes
------  ----  --------  ---------------  ---------------------------------------
0       4     bytes     magic            ASCII "HSLC" (HttpStackLens Capture)
4       2     int16     version          format version, starts at 1
6       1     bool      https_decrypted  whether HTTPS bodies were MITM-decrypted
7       4     int32     records_count    total number of records that follow
------  ----  --------  ---------------  ---------------------------------------
                        = 11 bytes
```

`records_count` counts records (requests + responses), making the file
self-describing for a sequential reader. A writer that streams may backfill this
field on close, or set it to `-1` to mean "read until EOF".

### Record framing

Every record starts with a 1-byte **record type** discriminator. The reader
switches on it to know what to parse next:

```
RecordType (uint8)
  0x01  Request
  0x02  Response
```

Every record **ends** with a `crc32c` trailer covering the whole record (from the
`record_type` byte to the last body byte). A reader validates each record
independently: on a checksum mismatch it can report exactly which record is
corrupt and keep reading the next ones, so a single damaged record does not make
the rest of the file unreadable.

#### Request record

```
Offset  Size      Type      Field          Notes
------  --------  --------  -------------  --------------------------------------
0       1         uint8     record_type    = 0x01
1       16        uuid      request_id     unique id for this request
17      var       lpstring  method         "GET", "POST", "CONNECT", …
var     var       lpstring  url            request target (absolute or origin)
var     1         uint8     http_version   HttpVersion enum (major<<4 | minor)
var     var       headers   headers        request header collection
var     var       blob      body           request body (may be -1 / absent)
var     4         uint32    crc32c         CRC32-C of all preceding record bytes
```

#### Response record

```
Offset  Size      Type      Field            Notes
------  --------  --------  ---------------  ------------------------------------
0       1         uint8     record_type      = 0x02
1       16        uuid      request_id       links back to the Request record
17      1         uint8     http_version     HttpVersion enum (major<<4 | minor)
18      2         int16     status_code      e.g. 200, 404
var     var       lpstring  status_message   "OK", "Not Found", …
var     var       headers   headers          response header collection
var     var       blob      body             response body (may be -1 / absent)
var     4         uint32    crc32c           CRC32-C of all preceding record bytes
```

### Full-file datagram

All multi-byte integers are **little-endian** (see [Conventions](#conventions)).

```
 HEADER
┌────────┬─────────┬───────────────────┬─────────────────┐
│ "HSLC" │ version │  https_decrypted  │  records_count  │
│ 4 B    │ int16   │  bool (1 B)       │  int32          │
└────────┴─────────┴───────────────────┴─────────────────┘

 RECORD (Request)                                                          ┌─ trailer ─┐
┌──────┬───────────┬──────────┬──────────┬──────────────┬──────────┬────────┬────────┐
│ 0x01 │ request_id│  method  │   url    │ http_version │ headers  │  body  │ crc32c │
│ 1 B  │ 16 B uuid │ lpstring │ lpstring │  uint8       │ int32+…  │ int64+…│ uint32 │
└──────┴───────────┴──────────┴──────────┴──────────────┴──────────┴────────┴────────┘
   └──────────────── CRC32-C covers these bytes ────────────────────────────┘

 RECORD (Response)                                                                    ┌─ trailer ─┐
┌──────┬───────────┬──────────────┬─────────────┬──────────────┬──────────┬────────┬────────┐
│ 0x02 │ request_id│ http_version │ status_code │status_message│ headers  │  body  │ crc32c │
│ 1 B  │ 16 B uuid │  uint8       │   int16     │  lpstring    │ int32+…  │ int64+…│ uint32 │
└──────┴───────────┴──────────────┴─────────────┴──────────────┴──────────┴────────┴────────┘
   └──────────────────── CRC32-C covers these bytes ───────────────────────────────┘

 ... records repeat until records_count is reached (or EOF).
```

### Design notes

- **Streaming-friendly**: records are self-delimited, so the proxy can append a
  request as soon as it is parsed and the matching response later, without
  seeking. A reader loops `records_count` times (or to EOF), reads the 1-byte
  type, and dispatches to the request/response parser.
- **Correlation**: a `Response.request_id` equals the `Request.request_id` it
  answers. Orphan responses (request not captured) are allowed; readers should
  tolerate a missing match.
- **Integrity**: each record carries its own `crc32c` trailer, so corruption is
  localized — a reader can skip a bad record and keep going instead of discarding
  the whole capture. CRC32-C is hardware-accelerated via `hash/crc32` and
  detects accidental damage only (use a MAC for tamper-resistance).
- **Versioning**: `version` gates layout changes. Readers must reject a version
  they do not understand rather than guess.
- **Security**: bodies and headers may contain `Authorization`,
  `Proxy-Authorization`, cookies and tokens — see the redaction guidance in
  [AGENTS.md](AGENTS.md). Redaction (or opt-in body capture) should happen before
  records are written, not at read time.

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
