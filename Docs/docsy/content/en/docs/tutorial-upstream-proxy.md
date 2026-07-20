---
title: "Tutorial: authenticate behind a corporate proxy"
linkTitle: Corporate proxy
weight: 30
description: >
  Forward local traffic through an authenticated corporate proxy — no separate
  authentication helper needed.
---

Behind an authenticated corporate proxy, tools that can't do NTLM/Kerberos
themselves usually rely on a small local proxy that performs the authentication
for them. HttpStackLens can play that role — and let you watch the traffic while
it does.

{{% alert title="🎯 What you'll set up" color="info" %}}
Your apps → **HttpStackLens** (`localhost:3128`) → your company's authenticated
proxy → the internet. HttpStackLens adds the authentication your tools can't, so
they only ever see a plain, unauthenticated local proxy.
{{% /alert %}}

## 1. How it works

A corporate proxy typically demands NTLM, Kerberos or Negotiate authentication.
Many CLIs, package managers and SDKs can't perform that handshake — they just get a
`407 Proxy Authentication Required` and give up. A dedicated local proxy solves
this by authenticating upstream on their behalf.

{{% alert title="🧩 Who this is for" color="info" %}}
This mainly helps tools that speak a plain HTTP proxy but **don't implement** NTLM,
Kerberos or Negotiate themselves, so they fail with a `407` behind a
Windows-authenticated proxy. Common examples:

- **Node.js** & **npm / yarn / pnpm**
- **Python** — `pip`, `requests`, `conda`
- **Go modules** (`go get`), **Rust** `cargo`
- **Java** — Maven, Gradle; **Ruby** — `gem`, Bundler; **PHP** — Composer
- **Docker** & container package managers (`apt`, `apk`)
- Many cloud / CI CLIs and language-server / IDE extensions

Point any of them at `localhost:3128` and HttpStackLens performs the
authentication for them.
{{% /alert %}}

HttpStackLens does the same: it exposes an **unauthenticated** local proxy on
`localhost:3128`, forwards everything to your **upstream** corporate proxy, and
injects your Windows credentials on the way out. As a bonus, every request flows
through the live Web UI so you can see exactly what your tools are doing.

![HttpStackLens forwarding to an upstream proxy with Windows authentication.](/images/forward-proxy-server-with-windows-authentication.png)

## 2. Point HttpStackLens at the upstream proxy

Open `config.yaml` and set `output_proxy_uri` to your corporate proxy:

```yaml
proxy:
  port: 3128
  # The corporate proxy everything is forwarded to
  output_proxy_uri: http://proxy.corp.example.com:8080
  no_proxy:
    - "localhost"
    - "127.0.0.1"
    - ".local"
    - "host.docker.internal"
```

Hosts under `no_proxy` are reached directly, bypassing the upstream proxy — keep
your internal and loopback hosts there. You can also edit the upstream settings
live from the Web UI's settings panel, without restarting.

![Editing the upstream proxy from the Web UI.](/images/screenshots/upstream-settings.png)

## 3. Add Windows authentication

{{% alert title="🪟 Windows only" color="warning" %}}
Injecting the logged-in user's credentials relies on the Windows SSPI API
(`secur32.dll`). These options are compiled in only when targeting Windows and
return an error elsewhere.
{{% /alert %}}

Enable credential injection so HttpStackLens answers the upstream proxy's auth
challenge for you:

```yaml
proxy:
  output_proxy_uri: http://proxy.corp.example.com:8080
  # Inject the current Windows user's credentials toward the upstream proxy
  add_windows_authentication_to_output_proxy: true
```

HttpStackLens uses the current session's credentials over NTLM, Kerberos or
Negotiate — it prefers NTLM when both are offered. You never type or store a
password; the OS provides the token.

The same behaviour is available as a command-line flag when you run the binary
directly:

```sh
httpStackLens.exe --output-proxy-add-windows-auth
```

## 4. Handle 401/407 upstream proxies

Some upstream proxies challenge with a status the client doesn't expect, or expect
the auth on a specific step of the handshake. HttpStackLens ships a compatibility
mode for these `401`/`407`-based flows so the negotiation completes cleanly.

![The 401/407 compatibility handshake with the upstream proxy.](/images/upstream-proxy-401-compatibility-flow.png)

## 5. Point your tools at HttpStackLens

Now aim your tools at the local proxy — no credentials needed on their side:

```sh
# Shell (macOS / Linux)
export http_proxy=http://localhost:3128
export https_proxy=http://localhost:3128

# Shell (Windows PowerShell)
$env:HTTP_PROXY  = "http://localhost:3128"
$env:HTTPS_PROXY = "http://localhost:3128"

# git
git config --global http.proxy http://localhost:3128

# npm
npm config set proxy http://localhost:3128
npm config set https-proxy http://localhost:3128
```

{{% alert title="🐳 Containers & Docker" color="info" %}}
Point containers at `http://host.docker.internal:3128` and make sure that host is
in HttpStackLens's `no_proxy` list so it isn't forwarded back upstream.
{{% /alert %}}

## 6. Verify it works

1. **Send a request.**

   ```sh
   curl -x http://localhost:3128 https://example.com -I
   ```

2. **Watch it in the Web UI.** Open <http://localhost:9000>. The request should
   appear in the live list with a `2xx` status — proof it reached the internet
   through the authenticated upstream proxy.

3. **Retire the old helper.** Once traffic flows, you can stop whatever local
   authentication proxy you were running before. HttpStackLens now handles the
   authentication *and* gives you full visibility into the traffic.

{{% alert title="✅ Done" color="success" %}}
Your tools talk to a simple local proxy while HttpStackLens does the
corporate-proxy authentication — with every request visible in the UI.
{{% /alert %}}
