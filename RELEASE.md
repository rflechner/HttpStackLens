# Release Notes

All notable changes to this project are documented here, newest first.
Versions follow [Semantic Versioning](https://semver.org).

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
