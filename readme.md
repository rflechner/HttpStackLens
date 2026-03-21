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

## Usage

```sh
go run .
```

The proxy listens on `localhost:3128`. You can test it with curl:

```sh
curl -x http://localhost:3128 http://example.com
```
