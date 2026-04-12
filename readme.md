# HttpStackLens

> **Work in progress** — this project is far from finished and evolves over time.

HttpStackLens is a local HTTP/HTTPS proxy designed **for local development only**. It allows inspecting and visualizing HTTP traffic passing between a client and a server, acting as a minimal network debugging tool.

## Motivation

This project is primarily a **Go** learning exercise. The goal is to get familiar with the language and explore its idioms by comparing them with what I usually do in **C#** and **F#** — error handling, concurrency, code structure, etc.

## What it does (so far)

- Listens for incoming connections on a local port (default `3128`)
- Handles HTTPS tunnels via the `CONNECT` method
- Forwards requests and responses bidirectionally
- Web UI (WASM-based) to inspect live HTTP traffic

## What it doesn't do (yet)

- Does not decrypt SSL/TLS traffic — HTTPS tunnels are forwarded as-is
- Not intended for production use or shared networks

## Prerequisites

- [Go](https://go.dev/dl/) 1.26.1 or later
- [Node.js](https://nodejs.org/) (for the Web UI build — Tailwind CSS)

## Build

All build commands live in `webui/` and are driven by a Go build tool (`build-tools/`) invoked via npm scripts.

### Install dependencies (once)

```sh
cd webui
npm install
```

### Build everything

Builds the Web UI (WASM + CSS) and the native application binary:

```sh
cd webui
npm run build
```

### Build targets individually

| Command | What it does |
|---|---|
| `npm run build` | Web UI + native binary |
| `npm run build:webui` | WASM + Tailwind CSS only |
| `npm run build:app` | Native binary only |
| `npm run dev:css` | Tailwind CSS in watch mode (dev) |

The build tool auto-detects the current platform and produces `httpStackLens.exe` on Windows or `httpStackLens` on macOS/Linux.

> You can also invoke the build tool directly from the project root:
> ```sh
> go run ./build-tools              # everything
> go run ./build-tools webui        # Web UI only
> go run ./build-tools app          # binary only
> go run ./build-tools --help       # usage
> ```

### Cross-compilation

You can produce a binary for another platform using Go's built-in cross-compilation. The `build-tools` target does not yet support cross-compilation, so use `go build` directly:

**macOS → Windows:**

```sh
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o httpStackLens.exe .
```

**Windows → macOS:**

```powershell
$env:GOOS = "darwin"
$env:GOARCH = "amd64"
go build -ldflags="-s -w" -o httpStackLens .
```

### Windows-specific features

Two flags are available on Windows only (compiled in automatically when targeting `GOOS=windows`):

- `--windows-auth-require-ntlm` — require NTLM/Negotiate authentication from connecting clients
- `--output-proxy-add-windows-auth` — inject Windows credentials when forwarding to an upstream proxy

These rely on the Windows SSPI API (`secur32.dll`) and will return an error if used on other platforms.

## Usage

```sh
go run .
```

The proxy listens on `localhost:3128`. You can test it with curl:

```sh
curl -x http://localhost:3128 http://example.com
```
