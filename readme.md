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

A Go build tool in `build-tools/` handles the entire build pipeline (npm install, WASM compilation, CSS generation, native binary). **This is the recommended way to build the project.**

If you prefer to run each step manually, see [Manual steps](#manual-steps) below.

### Using the build tool

From the project root — **this is all you need:**

```sh
go run .\build-tools\main.go
```

Additional targets:

```sh
go run .\build-tools\main.go webui        # Web UI only (WASM + CSS)
go run .\build-tools\main.go app          # Native binary only
go run .\build-tools\main.go --help       # Usage
```

Or via npm scripts from `webui/`:

| Command | What it does |
|---|---|
| `npm run build` | Web UI + native binary |
| `npm run build:webui` | WASM + Tailwind CSS only |
| `npm run build:app` | Native binary only |
| `npm run dev:css` | Tailwind CSS in watch mode (dev) |

The build tool auto-detects the current platform and produces `httpStackLens.exe` on Windows or `httpStackLens` on macOS/Linux.

---

### Manual steps

<details>
<summary>Click to expand</summary>

#### 1. Install npm dependencies

```sh
cd webui
npm install
```

#### 2. Copy wasm_exec.js

```sh
# macOS / Linux
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" webui/wwwroot/js/wasm_exec.js

# Windows (PowerShell)
Copy-Item "$(go env GOROOT)\lib\wasm\wasm_exec.js" -Destination webui\wwwroot\js\wasm_exec.js
```

#### 3. Compile Go to WASM

```sh
# macOS / Linux
GOOS=js GOARCH=wasm go build -o webui/wwwroot/wasm/app.wasm ./webui/wasm

# Windows (PowerShell)
$env:GOOS = "js"; $env:GOARCH = "wasm"
go build -o webui\wwwroot\wasm\app.wasm .\webui\wasm
$env:GOOS = ""; $env:GOARCH = ""
```

#### 4. Build Tailwind CSS

```sh
cd webui
npx tailwindcss -i ./src/input.css -o ./wwwroot/css/output.css --minify
```

#### 5. Build the native binary

```sh
# macOS / Linux
go build -ldflags="-s -w" -o httpStackLens .

# Windows
go build -ldflags="-s -w" -o httpStackLens.exe .
```

</details>

---

### Cross-compilation

The `build-tools` target builds for the current platform only. For cross-compilation, use `go build` directly:

**macOS → Windows:**

```sh
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o httpStackLens.exe .
```

**Windows → macOS:**

```powershell
$env:GOOS = "darwin"; $env:GOARCH = "amd64"
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
