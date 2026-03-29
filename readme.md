# HttpStackLens

> **Work in progress** — this project is far from finished and evolves over time.

HttpStackLens is a local HTTP/HTTPS proxy designed **for local development only**. It allows inspecting and visualizing HTTP traffic passing between a client and a server, acting as a minimal network debugging tool.

## Motivation

This project is primarily a **Go** learning exercise. The goal is to get familiar with the language and explore its idioms by comparing them with what I usually do in **C#** and **F#** — error handling, concurrency, code structure, etc.

## What it does (so far)

- Listens for incoming connections on a local port (default `3128`)
- Handles HTTPS tunnels via the `CONNECT` method
- Forwards requests and responses bidirectionally

## What it doesn't do (yet)

- No traffic inspection UI
- Does not decrypt SSL/TLS traffic — HTTPS tunnels are forwarded as-is
- Not intended for production use or shared networks

## Build

### Prerequisites

- [Go](https://go.dev/dl/) 1.21 or later

### macOS

```sh
go build -ldflags="-s -w" -o httpStackLens .
./httpStackLens
```

> **Note:** Windows authentication (`--windows-auth-require-ntlm`, `--output-proxy-add-windows-auth`) is not supported on macOS and will return an error if used.

### Windows

```sh
go build -ldflags="-s -w" -o httpStackLens.exe .
.\httpStackLens.exe
```

Windows-specific features are available on this platform:

- `--windows-auth-require-ntlm` — require NTLM/Negotiate authentication from connecting clients
- `--output-proxy-add-windows-auth` — inject Windows authentication credentials when forwarding to an upstream proxy

These features rely on the Windows SSPI API (`secur32.dll`) and are compiled in automatically when targeting Windows.

### Cross-compilation

You can produce a Windows binary from macOS (and vice versa) using Go's built-in cross-compilation:

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

> The Windows-authentication features are compiled into the binary when `GOOS=windows` but will only function at runtime on a Windows host.

## Usage

```sh
go run .
```

The proxy listens on `localhost:3128`. You can test it with curl:

```sh
curl -x http://localhost:3128 http://example.com
```
