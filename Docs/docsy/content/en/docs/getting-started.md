---
title: Getting started
linkTitle: Getting started
weight: 20
description: >
  Build the binary, launch the proxy and open the Web UI in a few minutes.
---

## Prerequisites

| Tool                                          | Why                                        |
|-----------------------------------------------|--------------------------------------------|
| [Go 1.26.1+](https://go.dev/dl/)              | Compiles the proxy and the WASM Web UI.    |
| [Node.js](https://nodejs.org/)                | Builds the Tailwind CSS for the Web UI.    |

## Build

A Go build tool in `build-tools/` runs the whole pipeline — npm install, WASM
compilation, CSS generation and the native binary. This is the recommended path.

```sh
# From the project root — build everything
go run ./build-tools/main.go

# Or individual targets
go run ./build-tools/main.go webui   # Web UI only (WASM + CSS)
go run ./build-tools/main.go app     # Native binary only
go run ./build-tools/main.go --help  # Usage
```

The build tool auto-detects your platform and produces `httpStackLens.exe` on
Windows or `httpStackLens` on macOS/Linux.

## Run

1. **Start HttpStackLens.** From the project root:

   ```sh
   go run .
   ```

   On first run a `config.yaml` is generated with sensible defaults.

2. **Open the Web UI.** Browse to <http://localhost:9000> to watch live traffic
   and control the proxy.

3. **Send traffic through the proxy.** Point any client at `localhost:3128`:

   ```sh
   curl -x http://localhost:3128 http://example.com
   ```

{{% alert title="🔑 Default ports" color="info" %}}
Proxy → `3128`, Web UI → `9000`. Both are restricted to loopback by default.
Change them under `proxy.port` / `webui.port` in `config.yaml`.
{{% /alert %}}

## Where to next

- [🏢 Corporate proxy](../tutorial-upstream-proxy/) — sit behind an authenticated
  corporate proxy without a separate authentication helper.
- [🔍 Debug HTTPS & clean up](../tutorial-https-decrypt/) — decrypt HTTPS, inspect
  it, then wipe every certificate from your OS.
