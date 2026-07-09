# WebUI v2 — Task breakdown (implementing the Claude Design mockup)

> Planning document. Goal: turn the static mockup
> (`webui/wwwroot/mockup.html` + `mockup.js`, fake data) into a real UI wired to
> the proxy.
>
> Reference files:
> - Mockup: [`webui/wwwroot/mockup.html`](webui/wwwroot/mockup.html), [`webui/wwwroot/mockup.js`](webui/wwwroot/mockup.js)
> - Current UI (WASM): [`webui/wasm/main.go`](webui/wasm/main.go), [`webui/wwwroot/index.html`](webui/wwwroot/index.html)
> - UI server: [`webui/ui_server.go`](webui/ui_server.go)
> - Shared back/front contract: [`webui/wasm/shared/dto.go`](webui/wasm/shared/dto.go)
> - Pipeline / capture: [`proxy_server.go`](proxy_server.go), [`proxy/middlewares/https_interceptor.go`](proxy/middlewares/https_interceptor.go), [`storage/capture_record.go`](storage/capture_record.go)

---

## 1. The gap: mockup ↔ backend

The mockup is a **full** UI (request/response list, multi-tab detail, timings,
bodies, headers, config modals). The backend, however, currently streams **only
the request line**:

```go
// webui/wasm/shared/dto.go — everything that reaches the front today
type RequestEventDto struct {
    ID, Method, Host, Port, Path, Version
}
```

Published via the SSE `request_occurred` event (see [`logging/WebUiEventLogger.go`](logging/WebUiEventLogger.go)).
There is **no response event** — no status, size, duration, body or headers in
the real-time stream.

Good news: this data **already exists** in the capture layer
(`RequestRecord` / `ResponseRecord` in [`storage/capture_record.go`](storage/capture_record.go),
correlated by UUID, with body + headers + status) and is produced by the
`HttpsInterceptor`. **The bulk of the backend work = expose to the SSE stream / an
API what capture already knows, plus correlate and measure it.**

### Gap table (UI feature → backend state → work)

| Mockup feature | Data needed | Backend state | Backend work |
|---|---|---|---|
| Status column + pill | response status code | captured (`ResponseRecord`), **not streamed** | `response_occurred` event |
| Size / Content-Type columns | response headers | captured, not streamed | same |
| Dur column / waterfall | request duration | **not measured** | timer around RoundTrip |
| TLS lock (encrypted/decrypted) | scheme + MITM status | derivable (CONNECT + decrypt) | `tls`/`decrypted` flag in DTO |
| Overview tab (protocol, client, cache) | version, client IP, process | version OK; **IP/process not captured** | client attribution (best-effort) |
| Headers tab | req+resp headers | captured, not streamed | include in detail DTO / endpoint |
| Body tab (Pretty/Raw/Hex) | req+resp body | captured (with size cap) | `GET /requests/{id}/body` endpoint |
| Timing tab (dns/connect/tls/…) | timing breakdown | **not measured** | transport instrumentation |
| Pause/Capturing button | capture control | **absent** | pause/resume endpoint + state |
| Clear button | clear session | **absent** (front-only possible) | server-side reset if server state |
| HTTPS decryption toggle | enable/disable MITM at runtime | **config-only, at startup** | runtime toggle (big piece) |
| Certificate wizard | generate/install/export CA | logic exists (`certManager`), **no HTTP API** | cert endpoints |
| Upstream proxy modal | read/write outbound proxy + NTLM | config exists, **read-only via /config** | upstream read/write API |
| Access control modal | loopback/lan/allowlist/open mode + networks | **only `enable_remote_connection` bool** | access model + API |
| Body capture rules modal | MIME rules + sizes | config exists (`DecryptHttpsConfig`), **read-only** | rules read/write API |
| Replay / Edit & send | replay a request | **absent** | replay endpoint |
| Status bar (throughput ↓/↑) | real-time rate | **not measured** | bytes/s counter |
| Sidebar Hosts + counts | aggregates | derivable on the front | nothing (front) |

---

## 2. Architecture decision: **WASM** (settled)

The mockup is written in **vanilla JS** (`mockup.js` manipulates the DOM
directly), but the v2 UI **stays in Go/WASM** (`webui/wasm/main.go`), consistent
with the existing code and the AGENTS.md direction. The mockup serves as a
**visual and behavioral specification**; its rendering logic is **ported into
WASM**.

Consequences for the front end:
- The `mockup.js` logic (list rendering, detail, modals, waterfall, format
  helpers) is rewritten in Go under `webui/wasm/` — more verbose
  (`js.Global()...`), but a single language and **typed shared DTOs** via
  `shared/`.
- `mockup.js` and `mockup.html` stay as **reference** during the port (do not
  serve them in prod). The HTML/CSS (Tailwind, palette, shell structure) can be
  reused almost as-is in `index.html`; only the engine `<script>` changes (WASM
  instead of `mockup.js`).
- WASM caveat: generating large DOM blocks via HTML strings is easy to port
  (`innerHTML`), but the mockup's event delegation (a single `addEventListener`
  on `document`) must be replicated on the Go side to avoid multiplying
  `js.FuncOf` (memory/perf cost).

> **Decision locked.** All front-end tasks in §5 therefore mean "(re)write in
> WASM", building on the mockup's HTML/CSS and the DTOs in `shared/`.

---

## 3. Data contract (define first — blocks BOTH back and front)

This is the pivot of the effort. To be fixed in
[`webui/wasm/shared/dto.go`](webui/wasm/shared/dto.go) so it is shared between
server and WASM.

### 3.1 SSE events

- **`request_occurred`** (enrich the existing one): add `correlationId`
  (string/UUID stable, shared with the response), `scheme`, `tls`, `clientIp`,
  `process`, `startedAt`.
- **`response_occurred`** (new): `correlationId`, `status`, `statusText`,
  `contentType`, `size`, `durationMs`, `bodyAvailable`, `bodySkipped`, `stream`
  (SSE/WS).
- **`throughput`** (new, optional, ~1 Hz): `bytesIn`, `bytesOut` per second.
- **`capture_state`** (new): `capturing`, `decrypting`, `upstreamMode`,
  `accessMode` (to sync the toolbar/status bar on load and on change).

> Today `ID` is an incrementing `int` on the proxy side and a separate `UUID` on
> the interceptor side. **Cross-cutting task: unify the correlation identifier**
> so request↔response are linked all the way to the UI.

### 3.2 REST endpoints (detail on demand — large bodies are not streamed)

| Method | Route | Purpose |
|---|---|---|
| GET | `/api/requests/{id}` | full detail (req+resp headers, timings) |
| GET | `/api/requests/{id}/body?side=request\|response` | raw body (with content-type) |
| POST | `/api/capture/pause` · `/resume` | capture control |
| POST | `/api/capture/clear` | clear the session |
| GET/PUT | `/api/settings/body-rules` | MIME capture rules |
| GET/PUT | `/api/settings/upstream` | outbound proxy + NTLM |
| GET/PUT | `/api/settings/access` | access mode + networks |
| POST | `/api/tls/decryption` | enable/disable MITM at runtime |
| GET | `/api/tls/ca` | CA status (subject, fingerprint, expiry, installed?) |
| POST | `/api/tls/ca/generate` · `/install` · `/export` | certificate wizard |
| POST | `/api/requests/{id}/replay` | replay (with Edit & send option) |

---

## 4. BACKEND workstreams (Go)

### EPIC B1 — Response streaming (core, unblocks the whole right column)
- [x] B1.1 Unify the request/response correlation ID (int → UUID exposed to the UI).
      Added `UUID.String()` (canonical form), `RequestEventDto.CorrelationID`, and a
      single UUID generated per request in `proxy_server.go` — now shared by the
      capture record and the `request_occurred` event. `LogRequest` signature +
      both loggers updated.
- [x] B1.2 Extend `RequestEventDto` (`scheme`/`tls`/`decrypted`) + create
      `ResponseEventDto` in `shared/dto.go`.
- [x] B1.3 Measure duration (chrono around `transport.RoundTrip` + response write
      in [`https_interceptor.go`](proxy/middlewares/https_interceptor.go)).
      *Plain-HTTP timing not done — see B1.4 note.*
- [~] B1.4 Publish `request_occurred` + `response_occurred` from the interceptor
      via a new `EventSink` (satisfied by `WebUiEventLogger`); CONNECT is hidden in
      decrypt mode. **Plain-HTTP responses are NOT emitted**: `TunnelServer`/
      `ForwardProxyServer` just `io.Copy` the bytes without parsing the response —
      surfacing them needs response parsing on that path (deferred, own task).
- [x] B1.5 Flag `scheme`/`tls`/`decrypted` on requests and `stream` on responses
      (`text/event-stream`, HTTP 101 WebSocket upgrade).

### EPIC B2 — Detail on demand (headers + body)
- [x] B2.1 Keep a bounded in-memory buffer of the last N requests (records already
      produced; add an index by ID) — watch memory.
      Added `storage.RequestStore` (FIFO-bounded, thread-safe, keyed by correlation
      id; default 500). Fed by the HTTPS interceptor (req+resp records) and the
      plain-HTTP path (`proxy_server.go`), injected from `main.go`. Bodies are the
      already size-capped ones, so memory ≈ N × (req cap + resp cap). Not yet
      exposed over HTTP — that's B2.2.
- [x] B2.2 `/api/requests/{id}` endpoint (req+resp headers, metadata).
      Added a Web UI API handler backed by `storage.RequestStore`, plus shared
      detail DTOs containing request/response metadata, headers, and body
      availability flags. Bodies remain separate for B2.3.
- [x] B2.3 `/api/requests/{id}/body` endpoint (honor `BodySkipped`, return the real
      `Content-Type` for Pretty/Hex rendering).
      Added `side=request|response`, raw body responses, captured content-type
      propagation, `404` for absent bodies, and `413` + `X-Body-Skipped` for
      intentionally skipped bodies.
- [x] B2.4 Reuse [`storage/capture_session_reader.go`](storage/capture_session_reader.go)
      to optionally reopen a `.capture` file and replay it in the UI.
      Added capture-file APIs: `GET /api/captures` lists `.capture` files,
      `GET /api/captures/{name}/metadata` reads the file header, and
      `GET /api/captures/{name}/records?offset=&limit=` reads records in order
      through the existing binary reader, with bodies base64-encoded in JSON.

### EPIC B3 — Capture control
- [x] B3.1 Server capture state (`capturing` on/off) + gate in the pipeline.
      Added a shared `storage.CaptureController`; paused capture no longer
      publishes live request/response events or writes new records to the
      in-memory/file capture paths, while proxy forwarding continues.
- [x] B3.2 pause/resume/clear endpoints + `capture_state` event.
      Added `GET /api/capture/state`, `POST /api/capture/pause`,
      `POST /api/capture/resume`, and `POST /api/capture/clear`; mutating calls
      publish the `capture_state` SSE event.
- [x] B3.3 "Clear" wipes the server buffer.
      `POST /api/capture/clear` clears `storage.RequestStore`; it does not delete
      `.capture` files from disk.

### EPIC B4 — Detailed timings (Timing tab / real waterfall)
- [x] B4.1 Instrument the transport (`httptrace.ClientTrace`: DNS, connect, TLS,
      TTFB, download) instead of the fake ratios in `timingBar()`.
      The decrypted-HTTPS `forward()` in
      [`https_interceptor.go`](proxy/middlewares/https_interceptor.go) now wraps the
      upstream request with an `httptrace.ClientTrace` (`requestTrace`) capturing
      DNS/connect/TLS/wrote-request/first-byte timestamps around `transport.RoundTrip`,
      and derives the per-phase durations bounded by the overall start/end of the
      exchange (reused keep-alive connections yield zero for DNS/connect/TLS).
- [x] B4.2 Add the segments to the detail DTO.
      Added `storage.Timing` + `RequestStore.PutTiming`, kept in the in-memory
      `CapturedExchange` (not the binary capture format), and exposed through the new
      `shared.TimingDto` under `RequestDetailDto.timing` (`GET /api/requests/{id}`);
      `openapi.yaml` updated. Frontend waterfall/Timing tab wiring is F2.4.
  > Medium/high complexity; can ship in phase 2 (waterfall first based on total
  > duration only).

### EPIC B5 — Runtime settings
- [x] B5.1 Body-capture rules: expose/edit `DecryptHttpsConfig` (already modeled in
      [`configuration/config.go`](configuration/config.go)) via API; apply without restart.
      Added `GET`/`PUT /api/settings/body-capture` for `default_max_bytes` and
      MIME capture rules, backed by a thread-safe runtime settings store consumed
      by `HttpsInterceptor`; updates persist back to `config.yaml` and leave the
      HTTPS decryption toggle to B6.3.
- [x] B5.2 Upstream proxy: read/write API (`OutputProxyUri`, bypass, NTLM/domain).
      Added `GET`/`PUT /api/settings/upstream` for `output_proxy_uri`, `no_proxy`
      (bypass), and `add_windows_authentication` (NTLM/Negotiate), backed by a
      thread-safe `configuration.UpstreamSettingsStore`; updates are validated
      (URL scheme/host) and persisted back to `config.yaml` via
      `PersistUpstreamSettings`. `openapi.yaml` updated. Hot re-injection into the
      running pipeline remains out of scope: edits take effect on the next
      application start.
- [x] B5.3 Access control: **new model** (loopback/lan/allowlist/open + networks)
      replacing the plain `EnableRemoteConnection` bool; filtering at `Accept()` in
      [`proxy_server.go`](proxy_server.go) and the UI server.
      Added `GET`/`PUT /api/settings/access-control` for proxy + Web UI policies,
      backed by `configuration.AccessControlSettingsStore`; active filters update
      immediately and settings persist to `config.yaml` as `access_control`.
      Switching from loopback binding to a remote-capable binding may still require
      restart when the process was started on `127.0.0.1`.

### EPIC B6 — TLS / Certificate (wizard)
- [x] B6.1 CA status endpoint (subject, fingerprint, expiry, installed?) — extend
      `/certificates-infos` (`certManager.LoadCA`, cf. [`ui_server.go`](webui/ui_server.go)).
      The endpoint now returns CA availability, subject/issuer/serial, SHA-256
      fingerprint, validity dates, expired flag, install support, installed
      status, and load/check errors; macOS and Windows installers expose trust
      store checks through `CertInstaller`.
- [x] B6.2 Generate / (re)install / export the CA via `certManager` (install logic
      exists: `cert_install_*.go`) exposed over HTTP.
- [x] B6.3 Hot decryption toggle (insert/remove `HttpsInterceptor` from the
      pipeline). Added `GET`/`PUT /api/settings/decrypt-https`, backed by a
      switchable middleware pointer so new CONNECT requests use the current
      mode while existing tunnels keep their startup mode. Updates persist
      `decrypt_https.enabled` back to `config.yaml`.

### EPIC B7 (deferred) — Replay / Edit & send
- [ ] B7.1 Replay endpoint for a remembered request (re-emit through the pipeline).
- [ ] B7.2 "Edit & send": accept a modified body/headers. (Phase 2.)

### EPIC B8 (deferred) — Metrics
- [ ] B8.1 In/out byte counters (aggregated ~1 Hz) → `throughput` event.
- [ ] B8.2 (Optional) errors/avg already computable on the front from the stream.

---

## 5. FRONTEND workstreams (Go/WASM — cf. §2; "render" = port to WASM)

### EPIC F0 — Foundations
- [ ] F0.1 Integrate the `mockup.html` shell as v2 `index.html` (dedicated route in
      [`ui_server.go`](webui/ui_server.go), keep the old one during the switchover).
- [ ] F0.2 Replace the fake-data engine (`mockReq`, `startLive`, `BODIES`, `HOSTS`…)
      with the real SSE client (`request_occurred` + `response_occurred`, joined by
      `correlationId`).
- [ ] F0.3 Front-end state model (`rows`, selection, filters) fed by the stream;
      retention policy (the mockup caps at 220 rows).

### EPIC F1 — List + waterfall + sidebar
- [ ] F1.1 Render rows with real data; pending → completed when the response
      arrives (status/size/dur fill in).
- [ ] F1.2 Waterfall on real duration; sidebar counters (2xx/4xx/5xx/TLS) and hosts
      (front-end aggregates).
- [ ] F1.3 Text filter + sidebar filters + density (already present in the mockup,
      to be rewired onto the real model).

### EPIC F2 — Detail pane
- [x] F2.1 Overview wired to `/api/requests/{id}`.
- [x] F2.2 Real req+resp headers.
- [x] F2.3 Body (Pretty/Raw/Hex) via `/api/requests/{id}/body` (+ `bodySkipped` /
      `stream` states already visually designed in the mockup).
- [x] F2.4 Real timing (depends on B4; otherwise hide / "total only").
- [ ] F2.5 Replay / Edit & send buttons (depends on B7).

### EPIC F3 — Toolbar & status bar
- [ ] F3.1 Pause/Resume, Clear wired to B3.
- [ ] F3.2 Status bar: req/errors/avg/total (front) + throughput (B8) +
      decrypt/upstream/access states (via `capture_state`).

### EPIC F4 — Modals
- [ ] F4.1 "Body capture" settings ↔ B5.1.
- [ ] F4.2 "Upstream" settings ↔ B5.2.
- [ ] F4.3 "Access control" settings ↔ B5.3.
- [ ] F4.4 Certificate wizard ↔ B6 (replace the simulated progress in `renderCert`).
- [ ] F4.5 Shortcuts tab (static, OK as-is).

### EPIC F5 — Polish
- [ ] F5.1 Real keyboard shortcuts (Space/⌘K/⌘L/R/J/K — partly in the mockup).
- [ ] F5.2 SSE reconnection / stream-loss handling.
- [ ] F5.3 Cleanup: remove the fake data, delete the old UI.

---

## 6. Proposed phasing

- **Milestone 0 — Contract**: fix §3 (DTO/SSE/API). Architecture settled = WASM (§2).
  *The data contract blocks BOTH back and front — do it first.*
- **Milestone 1 — MVP "it's alive"**: B1 + B2 + F0 + F1 + F2.1/F2.2 → real-time
  request+response list with real Overview & Headers. **Highest value.**
- **Milestone 2 — Inspection**: B2.3 + F2.3 (body), B3 + F3.1 (pause/clear), B8 +
  status bar.
- **Milestone 3 — Settings**: B5 + F4.1/2/3 (body rules, upstream, access).
- **Milestone 4 — TLS & timings**: B6 + F4.4 (wizard), B4 + F2.4 (timings), B3.1
  hot decrypt toggle.
- **Milestone 5 — Advanced**: B7 replay/edit, capture files, F5 polish.

---

## 7. Watch-outs / risks

- **Hot HTTPS toggle (B6.3)**: implemented with an indirected pipeline
  (`SwitchableMiddleware`). Existing tunnels keep their current mode; new
  CONNECT requests use the latest setting.
- **Memory**: keeping headers+body for the last N requests for on-demand detail has
  a cost; reuse the `DecryptHttpsConfig` size cap and bound N.
- **Security**: the modals expose the CA/private key, the "Open" access mode, and
  the corporate proxy. Respect the AGENTS.md constraint ("do not silently weaken
  security"), default to loopback, explicit warnings (already present in the mockup).
- **Client attribution (IP/process)**: the IP is easy (`RemoteAddr`); the process
  name requires an OS lookup (best-effort, potentially out of scope for v2).
- **Plain HTTP vs decrypted HTTPS**: two code paths produce the events
  (proxy_server for plain, interceptor for MITM). The SSE contract must be emitted
  **on both sides** consistently.
- **Streaming (SSE/WS)**: body never captured (already handled in the capture
  layer) — the UI must show "frame log only", as planned in the mockup's
  `bodyRules`.
