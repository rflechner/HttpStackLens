---
title: Documentation
linkTitle: Documentation
weight: 20
---

Everything you need to build, run and use **HttpStackLens** — a local HTTP/HTTPS
debugging proxy with a live Web UI.

## Quick start

```sh
# Build everything (WASM + CSS + native binary)
go run ./build-tools/main.go

# Run the proxy + Web UI
go run .

# Send a request through it
curl -x http://localhost:3128 http://example.com
```

The proxy listens on `localhost:3128` and the Web UI on `localhost:9000`. See
[Getting started](getting-started/) for prerequisites and build details.
