---
title: HttpStackLens
---

{{< blocks/cover title="See through your HTTP stack." image_anchor="top" height="med" color="white" >}}
<div class="mx-auto">
  <p class="lead mt-5">
    A local HTTP/HTTPS debugging proxy with a live Web UI. Inspect traffic in real time,
    decrypt HTTPS on demand, and forward through an authenticated corporate proxy —
    all from a single Go binary.
  </p>
  <a class="btn btn-lg btn-primary me-3 mb-4" href="docs/getting-started/">
    Get started <i class="fas fa-arrow-alt-circle-right ms-2"></i>
  </a>
  <a class="btn btn-lg btn-outline-primary me-3 mb-4" href="docs/features/">
    Explore features
  </a>
</div>
{{< /blocks/cover >}}

{{% blocks/lead color="primary" %}}
**For local development only.** HttpStackLens is a debugging tool for your own
machine — Windows, macOS and Linux. It is not built for production use or shared
networks.
{{% /blocks/lead %}}

{{% blocks/section color="light" type="row" %}}

{{% blocks/feature icon="fa-random" title="Forward proxy" %}}
Listens on `localhost:3128`, tunnels HTTPS via `CONNECT`, and streams requests
and responses bidirectionally.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-unlock" title="HTTPS decryption" %}}
Opt-in local MITM. Generates per-domain certificates from a debug root CA so you
can read encrypted bodies.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-satellite-dish" title="Live Web UI" %}}
A WASM + Tailwind interface streams traffic over SSE: request list, timings, body
decoding and inline image previews.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-building" title="Upstream proxy auth" %}}
Forward to a corporate proxy with NTLM, Kerberos and Negotiate on Windows — plus
a 401/407 compatibility mode.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-save" title="Capture & replay" %}}
Record sessions to a binary `.capture` format, browse saved captures and query
traffic through a REST API.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-broom" title="Clean teardown" %}}
One click removes the debug CA and every leaf it signed from your OS trust store —
nothing left behind.
{{% /blocks/feature %}}

{{% /blocks/section %}}

{{% blocks/section color="dark" type="row" %}}

{{% blocks/feature icon="fa-building" title="Authenticate behind a corporate proxy" url="docs/tutorial-upstream-proxy/" %}}
Point your tools at HttpStackLens and let it handle the authenticated upstream
proxy — a drop-in local proxy for your dev box, with no separate authentication
helper.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-search" title="Debug HTTPS, then clean up" url="docs/tutorial-https-decrypt/" %}}
Turn on decryption, inspect encrypted traffic, then remove every certificate so
your OS trust store stays pristine.
{{% /blocks/feature %}}

{{% /blocks/section %}}
