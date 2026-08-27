# Release Notes

All notable changes to this project are documented here, newest first.
Versions follow [Semantic Versioning](https://semver.org).

## v0.2.0-alpha — 2026-08-27

### Features

- **Requests Composer** — a new Composer view, switchable with the Traffic view,
  that lets you write, edit and replay HTTP requests directly from the UI. It
  ships with syntax highlighting, resizable panes, tab persistence, and a gutter
  action to run any individual request from the raw editor (#17).
- **`.http` file support** — a full parser for `.http` files covering request
  lines, headers, body, comments and implicit `HTTP/1.1`, plus file-based
  request management with folder visibility so your collections live on disk
  next to your project (`ab5555b`).
- **Headers kept as comments** — headers you untick in the Composer are
  preserved as comments in the `.http` file instead of being dropped, so nothing
  is lost on a round-trip (#18).
- **Raw protocol view** — inspect a request exactly as it goes on the wire, with
  the HTTP version and headers sorted for readability (`1db941c`).
- **Desktop application** — HttpStackLens is now packaged as a native desktop app
  built with Wails, on top of the existing Web UI (`7f306f3`).
- **Composer requests flow through the proxy** — requests sent from the Composer
  are handled in memory by the proxy pipeline, so they appear in the capture
  alongside the rest of the traffic (`59fc524`).
- **`QUERY` HTTP method** is now supported by the request model (`f22a2d3`).

### Bug fixes

- `.http` file content is now stable across editor round-trips — opening and
  saving a file no longer reformats or loses parts of it (`775dfcf`).
- Parent components no longer re-dispatch their children's events, which caused
  duplicated actions in the UI (`6561c5d`).
- Fixed the macOS build and debug runs, which were failing (`d612b55`, `6fb6a78`).

### Maintenance

- Removed the standalone WASM composer demo now that the Composer is integrated
  into the main UI.
- Reworked the Wails packaging flow and refreshed the branding assets.
- Build tooling and Go dependencies updated; JSON schema mapping configured for
  `package.json`.

## v0.1.1-beta — 2026-07-26

### Features

- **Certificate cleanup** — a one-click cleanup that disables HTTPS decryption
  and clears the generated per-domain certificate cache.
- **Split request/response panes** — inspect a request and its response side by
  side, with a focus toggle to expand either pane.
- **Automatic body decompression** — compressed request/response bodies are now
  decompressed for inspection.
- **Recording decoupled from disk** — live UI recording no longer forces disk
  persistence, so you can watch traffic live without writing captures to disk.
- **More robust config loading** — if the config file is missing, a default one
  is created automatically instead of failing.

### Bug fixes

- Config path is now resolved dynamically relative to the executable, so the
  proxy finds its configuration regardless of the working directory.

### Maintenance

- Centralized resolution of configuration, certificate and log file paths.
- CI: GitHub Actions workflow for automated release builds and draft publishing,
  with packaging decoupled from publishing and triggers on `develop`/`main`.
- README: project status and info badges; repo skill for generating these
  release notes.

## v0.1.1-alpha — 2026-07-14

First public alpha. HttpStackLens is a debugging HTTP/HTTPS proxy with a live
Web UI, HTTPS decryption, and upstream-proxy authentication support.

### Features

- **HTTP/HTTPS proxy** — forward proxy with `CONNECT` tunneling and
  bidirectional streaming; parses chunked and `Content-Length` response bodies.
- **HTTPS interception & decryption** — optional MITM through a built-in
  certificate manager: generates per-domain certificates, installs the root CA
  into the Windows store / macOS keychain, and lets you toggle decryption at
  runtime.
- **Proxy authentication** — upstream proxy auth with NTLM, Kerberos and
  Negotiate on Windows, plus a compatibility mode for 401-based upstream proxies.
- **Live Web UI** — WASM + Tailwind interface streaming traffic over SSE:
  request list (newest first), detailed request/response inspection with
  timings, base64 body decoding with inline image previews, a resizable and
  persistent detail pane, and light/dark themes.
- **Runtime control from the UI** — start/stop the proxy, manage recording,
  and edit upstream-proxy, body-capture and access-control settings without
  restarting.
- **Traffic capture & storage** — record sessions to a binary `.capture`
  format, browse saved captures, and query traffic through a REST API
  (`/api/requests/…`); a bounded in-memory buffer retains the most recent
  requests.
- **Access control** — remote connections are restricted by default, with
  loopback / LAN / allowlist modes for both the proxy and the Web UI.
- **Configuration** — YAML config with sensible defaults auto-generated on
  first run.
- **Update awareness** — reports the running version with a link to its commit,
  and can optionally check GitHub for a newer release (opt-in via
  `updates.check_enabled`).

### Notable fixes

- macOS build and HTTPS scheme detection.
- Preserve buffered connection data via a custom `NetworkStream` (`net.Conn`).
- Prefer NTLM over Negotiate for Windows authentication.
- Skip CA re-installation when the root certificate is already trusted.

### Tooling

- Go-based build tool (WASM + CSS + native binary) that injects
  version / commit / date into the binary.
