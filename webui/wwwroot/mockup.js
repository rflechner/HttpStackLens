/* HttpStackLens — pure-vanilla engine. No framework.
   Renders into the static shell in the HTML file and drives every
   interaction with plain DOM + event delegation. This mirrors how a
   Go/WASM build would push markup into the DOM. */
(function () {
  'use strict';

  // ─── palette (mirrors Tailwind config) ───────────────────
  const C = {
    bg0: '#0e0f12', bg1: '#15171c', bg2: '#1c1f26', bg3: '#23272f',
    line: '#262a33', lineSoft: '#1f232a',
    ink: '#d8dce4', dim: '#8a93a3', faint: '#5a6173',
    mint: '#7fd4b4', warn: '#f1b45a', danger: '#e86a6a', info: '#7aa7ff', pink: '#d48ad6',
  };
  const METHOD_COLOR = {
    GET: '#7fd4b4', POST: '#f1b45a', PUT: '#7aa7ff', DELETE: '#e86a6a',
    PATCH: '#d48ad6', OPTIONS: '#8a93a3', HEAD: '#8a93a3', CONNECT: '#5a6173',
  };
  function statusColor(s) {
    if (s >= 500) return C.danger;
    if (s >= 400) return C.warn;
    if (s >= 300) return C.info;
    if (s >= 200) return C.mint;
    return C.dim;
  }

  // ─── mock data ───────────────────────────────────────────
  const HOSTS = [
    ['api.github.com', 'https'], ['raw.githubusercontent.com', 'https'],
    ['registry.npmjs.org', 'https'], ['auth.corp.local', 'https']
  ];
  const PATHS = {
    'api.github.com': ['/repos/anthropics/claude-cli/pulls?state=open', '/user', '/repos/golang/go/issues/68412/comments', '/search/code?q=ReverseProxy'],
    'raw.githubusercontent.com': ['/golang/go/master/src/net/http/server.go', '/anthropics/anthropic-sdk-go/main/client.go']
  };
  const MIMES = [
    ['application/json', 'json'], ['text/html', 'html'], ['text/css', 'css'],
    ['application/javascript', 'js'], ['image/png', 'img'], ['image/jpeg', 'img'],
    ['font/woff2', 'font'], ['text/event-stream', 'stream'], ['application/octet-stream', 'bin'],
  ];
  const METHODS = ['GET', 'GET', 'GET', 'POST', 'POST', 'PUT', 'DELETE', 'PATCH'];
  const PROCS = ['chrome.exe', 'node.exe', 'curl.exe', 'code.exe', 'msedge.exe'];
  const CLIENTS = ['127.0.0.1', '192.168.1.42'];

  function rnd(a) { return a[Math.floor(Math.random() * a.length)]; }
  function mockReq(id) {
    const [host, scheme] = rnd(HOSTS);
    const path = rnd(PATHS[host] || ['/']);
    const method = rnd(METHODS);
    const [mime, mimeColor] = rnd(MIMES);
    const roll = Math.random();
    let status;
    if (roll < 0.68) status = 200;
    else if (roll < 0.78) status = rnd([201, 204, 206]);
    else if (roll < 0.86) status = rnd([301, 302, 304]);
    else if (roll < 0.96) status = rnd([400, 401, 403, 404, 429]);
    else status = rnd([500, 502, 503, 504]);
    const tls = scheme === 'https';
    return {
      id, ts: Date.now(), method, scheme, host, path, status, mime, mimeColor,
      size: Math.floor(Math.random() * 900000) + 120,
      ms: Math.floor(Math.random() * 1400) + 8,
      tls, decrypted: tls && state.decryption,
      clientIp: rnd(CLIENTS), process: rnd(PROCS),
    };
  }

  const BODIES = {
    json: `{
  "login": "octocat",
  "id": 583231,
  "type": "User",
  "site_admin": false,
  "name": "The Octocat",
  "company": "@github",
  "public_repos": 8,
  "followers": 14832,
  "created_at": "2011-01-25T18:44:36Z"
}`,
    html: `<!doctype html>
<html lang="en">
  <head><meta charset="utf-8"><title>Index of /artifacts</title></head>
  <body>
    <h1>Index of /artifacts-prod/build-9182</h1>
    <ul><li><a href="bundle.tar.zst">bundle.tar.zst</a></li></ul>
  </body>
</html>`,
    stream: `event: tick
data: {"t":1732038291,"cpu":0.41}

event: tick
data: {"t":1732038292,"cpu":0.43}

[… body not captured · text/event-stream · streaming …]`,
    img: `[binary · image/png · 44.2 KB]
  89 50 4E 47 0D 0A 1A 0A 00 00 00 0D 49 48 44 52
  00 00 01 80 00 00 01 80 08 06 00 00 00 E0 77 3D
  [… preview only · body not captured …]`,
    bin: `[binary · application/octet-stream · 812 KB]
  1F 8B 08 00 00 00 00 00 00 03 ED 5A 4B 6F E3 46
  [… body not captured · exceeds 2 MB threshold …]`,
    js: `(function () {
  'use strict';
  var r = Object.freeze({ version: '18.3.1' });
  /* React production build — minified · truncated 89.4 KB */
})();`,
    css: `@font-face {
  font-family: 'JetBrains Mono';
  font-weight: 400;
  src: url(https://fonts.gstatic.com/s/jetbrainsmono/v20/…woff2) format('woff2');
}`,
    font: `[binary · font/woff2 · 26.1 KB]
  77 4F 46 32 00 01 00 00 00 00 66 A4 00 0B 00 00 […]`,
  };
  function bodyFor(r) { return BODIES[r.mimeColor] || `[body not captured · ${r.mime}]`; }

  function reqHeaders(r) {
    return [
      [':authority', r.host], [':method', r.method], [':path', r.path], [':scheme', r.scheme],
      ['accept', r.mime.startsWith('image') ? 'image/avif,image/webp,*/*' : 'application/json, */*'],
      ['accept-encoding', 'gzip, deflate, br, zstd'],
      ['authorization', 'Bearer ey…truncated…xA'],
      ['user-agent', 'HttpStackLens/0.8 (+go1.23)'],
      ['x-request-id', '01JDT4' + (r.id * 7).toString(36).toUpperCase().padStart(8, '0')],
    ];
  }
  function resHeaders(r) {
    return [
      [':status', String(r.status)],
      ['content-type', r.mime + (r.mime.startsWith('text') ? '; charset=utf-8' : '')],
      ['content-length', String(r.size)],
      ['cache-control', r.mime.startsWith('image') ? 'public, max-age=31536000' : 'private, no-cache'],
      ['server', 'nginx/1.25.3'], ['x-cache', ['MISS', 'HIT', '—'][r.id % 3]],
      ['date', new Date(r.ts).toUTCString()],
    ];
  }

  // ─── format helpers ──────────────────────────────────────
  function fmtBytes(b) {
    if (b == null) return '—';
    if (b < 1024) return b + ' B';
    if (b < 1048576) return (b / 1024).toFixed(1) + ' KB';
    return (b / 1048576).toFixed(2) + ' MB';
  }
  function fmtMs(ms) { return ms >= 1000 ? (ms / 1000).toFixed(2) + 's' : ms + 'ms'; }
  function fmtTime(ts) {
    const d = new Date(ts), p = (n) => String(n).padStart(2, '0');
    return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}.${String(d.getMilliseconds()).padStart(3, '0')}`;
  }
  const esc = (s) => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  // ─── state ───────────────────────────────────────────────
  const state = {
    rows: [], selId: null, capturing: true, decryption: true,
    filter: '', sidebar: 'all', detailTab: 'overview', bodyMode: 'pretty',
    density: 'normal',
    upstream: { on: true, ntlm: true, host: 'http://proxy.corp.local:8080', domain: 'CORP' },
    access: { mode: 'loopback', networks: ['192.168.1.0/24'] },
    bodyRules: [
      { id: 'json', label: 'JSON', mimes: 'application/json · application/*+json', on: true, max: 2097152 },
      { id: 'html', label: 'HTML', mimes: 'text/html · application/xhtml+xml', on: true, max: 1048576 },
      { id: 'text', label: 'Plain text / XML / CSS / JS', mimes: 'text/plain · text/css · application/xml', on: true, max: 1048576 },
      { id: 'form', label: 'Form data', mimes: 'x-www-form-urlencoded · multipart/form-data', on: true, max: 4194304 },
      { id: 'images', label: 'Images', mimes: 'image/*', on: false, max: 262144 },
      { id: 'fonts', label: 'Fonts', mimes: 'font/*', on: false, max: 262144 },
      { id: 'binary', label: 'Binary / octet-stream', mimes: 'application/octet-stream · zip · gzip', on: false, max: 0 },
      { id: 'sse', label: 'Server-Sent Events', mimes: 'text/event-stream', on: false, max: 0, locked: true, note: 'Streaming — body never captured, frame log only' },
      { id: 'ws', label: 'WebSocket frames', mimes: 'upgrade: websocket', on: false, max: 0, locked: true, note: 'Streaming — frame metadata only' },
    ],
  };

  // ─── tiny DOM helpers ────────────────────────────────────
  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

  function methodTag(m, sm) {
    const c = METHOD_COLOR[m] || C.dim;
    const pad = sm ? '1px 5px' : '2px 6px', fs = sm ? '10px' : '11px', mw = sm ? '40px' : '46px';
    return `<span class="font-mono font-semibold inline-block text-center rounded-[3px]" style="color:${c};background:${c}18;border:1px solid ${c}33;padding:${pad};font-size:${fs};min-width:${mw};letter-spacing:.3px">${m}</span>`;
  }
  function statusPill(s, sm) {
    const c = statusColor(s);
    const pad = sm ? '1px 5px' : '2px 7px', fs = sm ? '10px' : '11px';
    return `<span class="font-mono font-semibold inline-flex items-center gap-[5px] rounded-[3px]" style="color:${c};background:${c}14;border:1px solid ${c}33;padding:${pad};font-size:${fs}">
      <span style="width:5px;height:5px;border-radius:2px;background:${c}"></span>${s || '—'}</span>`;
  }
  function lock(on, warn) {
    const c = warn ? C.warn : on ? C.mint : C.faint;
    return `<svg width="10" height="12" viewBox="0 0 10 12" fill="none" style="display:block">
      <rect x="1" y="5" width="8" height="6" rx="1" stroke="${c}" stroke-width="1"/>
      <path d="${on ? 'M3 5V3.5a2 2 0 014 0V5' : 'M3 5V3.5a2 2 0 013-1.7'}" stroke="${c}" stroke-width="1" stroke-linecap="round"/></svg>`;
  }

  // ─── request list ────────────────────────────────────────
  const GRID = 'grid-template-columns:48px 54px 20px 56px 178px 1fr 132px 70px 76px 54px';
  function rowHeight() { return state.density === 'compact' ? 22 : state.density === 'comfy' ? 30 : 26; }

  function filteredRows() {
    let rows = state.rows;
    const sb = state.sidebar;
    if (sb !== 'all') {
      rows = rows.filter((r) => {
        if (sb === '2xx') return r.status < 300;
        if (sb === '4xx') return r.status >= 400 && r.status < 500;
        if (sb === '5xx') return r.status >= 500;
        if (sb === 'tls') return r.tls;
        return true;
      });
    }
    const q = state.filter.trim().toLowerCase();
    if (q) rows = rows.filter((r) =>
      r.host.toLowerCase().includes(q) || r.path.toLowerCase().includes(q) ||
      String(r.status).includes(q) || r.method.toLowerCase().includes(q));
    return rows;
  }

  function rowHTML(r) {
    const sel = r.id === state.selId;
    const stream = r.mimeColor === 'stream';
    const tooLarge = r.size > 512000 && !stream;
    return `<div data-row="${r.id}" class="grid items-center gap-[10px] cursor-pointer select-none" style="${GRID};height:${rowHeight()}px;padding:0 12px;background:${sel ? C.bg3 : 'transparent'};border-left:2px solid ${sel ? statusColor(r.status) : 'transparent'};border-bottom:1px solid ${C.lineSoft};font-family:'JetBrains Mono',monospace;font-size:11.5px;color:${C.ink}">
      <span style="color:${C.faint};text-align:right">${String(r.id).padStart(3, '0')}</span>
      ${methodTag(r.method, true)}
      <span title="${r.tls ? (r.decrypted ? 'decrypted' : 'encrypted') : 'cleartext'}">${r.tls ? lock(true, !r.decrypted) : `<span style="color:${C.faint};font-size:10px">◌</span>`}</span>
      ${statusPill(r.status, true)}
      <span class="truncate" style="color:${C.dim}">${r.host}</span>
      <span class="truncate" style="color:${C.ink}">${esc(r.path)}${stream ? `<span style="color:${C.warn};margin-left:8px;font-size:10px">· stream</span>` : ''}${tooLarge ? `<span style="color:${C.faint};margin-left:8px;font-size:10px">· body skipped</span>` : ''}</span>
      <span class="truncate" style="color:${C.faint}">${r.mime}</span>
      <span style="color:${C.dim};text-align:right;font-variant-numeric:tabular-nums">${fmtBytes(r.size)}</span>
      <span style="color:${C.faint};text-align:right;font-variant-numeric:tabular-nums">${fmtTime(r.ts)}</span>
      <span style="color:${C.dim};text-align:right;font-variant-numeric:tabular-nums">${fmtMs(r.ms)}</span>
    </div>`;
  }

  function renderList() {
    const rows = filteredRows();
    const list = $('#list');
    if (!rows.length) {
      list.innerHTML = `<div class="text-center" style="padding:40px;color:${C.faint};font-size:12px">no requests match <span style="color:${C.ink};font-family:'JetBrains Mono'">${esc(state.filter)}</span></div>`;
    } else {
      list.innerHTML = rows.map(rowHTML).join('');
    }
    renderWaterfall();
    renderSidebarCounts();
    renderStatusBar();
  }

  function appendRow(r) {
    const list = $('#list');
    const rows = filteredRows();
    // Only append if it passes the current filter and list isn't in empty-state.
    if (!rows.some((x) => x.id === r.id)) return;
    const wasBottom = list.scrollTop + list.clientHeight >= list.scrollHeight - 30;
    if ($('#list .text-center')) { renderList(); return; }
    list.insertAdjacentHTML('beforeend', rowHTML(r));
    const el = list.lastElementChild;
    el.classList.add('row-new');
    if (wasBottom) list.scrollTop = list.scrollHeight;
    renderWaterfall(); renderSidebarCounts(); renderStatusBar();
  }

  // ─── waterfall ───────────────────────────────────────────
  function renderWaterfall() {
    const wf = $('#waterfall');
    const rows = state.rows.slice(-130);
    const max = Math.max(1, ...rows.map((r) => r.ms));
    wf.innerHTML = rows.map((r) => {
      const h = Math.max(2, (r.ms / max) * 30);
      return `<div title="${r.method} ${esc(r.path)} · ${fmtMs(r.ms)}" style="width:3px;height:${h}px;background:${statusColor(r.status)};opacity:.85;flex-shrink:0;border-radius:1px"></div>`;
    }).join('');
  }

  // ─── sidebar ─────────────────────────────────────────────
  function renderSidebarCounts() {
    const rows = state.rows;
    const c = { all: rows.length, '2xx': 0, '4xx': 0, '5xx': 0, tls: 0 };
    for (const r of rows) {
      if (r.status < 300) c['2xx']++;
      else if (r.status >= 500) c['5xx']++;
      else if (r.status >= 400) c['4xx']++;
      if (r.tls) c.tls++;
    }
    $$('[data-side]').forEach((el) => {
      const k = el.dataset.side;
      const cnt = $('.side-count', el);
      if (cnt) cnt.textContent = c[k] ?? 0;
      const active = state.sidebar === k;
      el.style.background = active ? C.bg3 : 'transparent';
      el.style.color = active ? C.ink : C.dim;
    });
    // hosts
    const hostMap = {};
    for (const r of rows) hostMap[r.host] = (hostMap[r.host] || 0) + 1;
    const top = Object.entries(hostMap).sort((a, b) => b[1] - a[1]).slice(0, 6);
    $('#hosts').innerHTML = top.map(([h, n]) =>
      `<button data-host="${h}" class="flex items-center gap-2 w-full text-left rounded-[3px]" style="padding:5px 8px;background:transparent;border:none;cursor:pointer;color:${C.dim};font-family:'JetBrains Mono';font-size:11px">
        <span class="flex-1 truncate">${h}</span><span style="color:${C.faint};font-size:10.5px">${n}</span></button>`).join('');
  }

  // ─── detail pane ─────────────────────────────────────────
  function headersTable(rows) {
    return `<div style="font-family:'JetBrains Mono';font-size:11.5px;line-height:1.6">` + rows.map(([k, v]) =>
      `<div class="grid gap-4" style="grid-template-columns:200px 1fr;padding:3px 14px;border-bottom:1px solid ${C.lineSoft}">
        <span style="color:${k.startsWith(':') ? C.pink : C.info}">${esc(k)}</span>
        <span style="color:${C.ink};word-break:break-all">${esc(v)}</span></div>`).join('') + `</div>`;
  }

  function jsonHighlight(body) {
    let pretty;
    try { pretty = JSON.stringify(JSON.parse(body), null, 2); } catch { return esc(body); }
    return esc(pretty)
      .replace(/(&quot;(?:\\.|[^&]|&(?!quot;))*?&quot;)(\s*:)/g, `<span style="color:${C.pink}">$1</span>$2`)
      .replace(/: (&quot;(?:\\.|[^&]|&(?!quot;))*?&quot;)/g, `: <span style="color:${C.mint}">$1</span>`)
      .replace(/: (true|false|null)/g, `: <span style="color:${C.warn}">$1</span>`)
      .replace(/: (-?\d+\.?\d*)/g, `: <span style="color:${C.info}">$1</span>`);
  }

  function hexDump(body) {
    const src = body.slice(0, 512);
    let out = '';
    for (let i = 0; i < src.length; i += 16) {
      const chunk = src.slice(i, i + 16);
      const hex = Array.from(chunk).map((ch) => ch.charCodeAt(0).toString(16).padStart(2, '0')).join(' ');
      const ascii = Array.from(chunk).map((ch) => { const c = ch.charCodeAt(0); return c >= 32 && c < 127 ? ch : '.'; }).join('');
      out += `<div class="grid gap-3" style="grid-template-columns:80px 380px 1fr">
        <span style="color:${C.faint}">${i.toString(16).padStart(8, '0')}</span>
        <span style="color:${C.info}">${hex}</span>
        <span style="color:${C.dim}">${esc(ascii)}</span></div>`;
    }
    return `<div style="padding:12px 14px;font-family:'JetBrains Mono';font-size:11px;line-height:1.55">${out}</div>`;
  }

  function bodyPane(r) {
    const body = bodyFor(r);
    const mode = state.bodyMode;
    const tab = (id, label) => `<button data-bodymode="${id}" style="background:transparent;border:none;cursor:pointer;padding:8px 12px 7px;font-size:11.5px;font-family:Inter;font-weight:500;color:${mode === id ? C.ink : C.dim};border-bottom:2px solid ${mode === id ? C.mint : 'transparent'};margin-bottom:-1px">${label}</button>`;
    let content;
    if (mode === 'pretty' && r.mimeColor === 'json') content = `<pre style="margin:0;padding:12px 14px;font-family:'JetBrains Mono';font-size:11.5px;line-height:1.55;color:${C.ink};white-space:pre;overflow:auto">${jsonHighlight(body)}</pre>`;
    else if (mode === 'hex') content = hexDump(body);
    else content = `<pre style="margin:0;padding:12px 14px;font-family:'JetBrains Mono';font-size:11.5px;line-height:1.55;color:${C.dim};white-space:pre-wrap;word-break:break-all;overflow:auto">${esc(body)}</pre>`;
    return `<div class="flex flex-col h-full min-h-0">
      <div class="flex" style="border-bottom:1px solid ${C.line};background:${C.bg1};padding-left:6px;flex-shrink:0">${tab('pretty', 'Pretty')}${tab('raw', 'Raw')}${tab('hex', 'Hex')}</div>
      <div class="flex-1 min-h-0 overflow-auto hsl-scroll" style="background:${C.bg1}">${content}</div></div>`;
  }

  function timingBar(r) {
    const segs = [
      ['dns', Math.floor(r.ms * 0.06), C.pink], ['connect', Math.floor(r.ms * 0.12), C.info],
      ['tls', r.tls ? Math.floor(r.ms * 0.18) : 0, C.mint], ['send', Math.floor(r.ms * 0.04), C.warn],
      ['wait', Math.floor(r.ms * 0.48), C.dim], ['recv', Math.floor(r.ms * 0.12), C.ink],
    ];
    const total = segs.reduce((s, x) => s + x[1], 0) || 1;
    return `<div style="padding:10px 14px;border-bottom:1px solid ${C.lineSoft}">
      <div class="flex" style="height:8px;border-radius:2px;overflow:hidden;background:${C.bg2}">
        ${segs.map(([l, ms, c]) => ms > 0 ? `<div title="${l}: ${ms}ms" style="flex:${ms / total};background:${c};opacity:.85"></div>` : '').join('')}</div>
      <div class="flex flex-wrap gap-[14px]" style="margin-top:8px;font-family:'JetBrains Mono';font-size:10.5px;color:${C.dim}">
        ${segs.map(([l, ms, c]) => ms > 0 ? `<span class="inline-flex items-center gap-1"><span style="width:6px;height:6px;background:${c};border-radius:1px"></span>${l} <span style="color:${C.ink}">${ms}ms</span></span>` : '').join('')}</div></div>`;
  }

  function kv(k, v, color) {
    return `<div class="grid gap-[14px]" style="grid-template-columns:130px 1fr;padding:5px 0">
      <span style="color:${C.faint};font-size:11px;font-family:Inter">${k}</span>
      <span style="color:${color || C.ink};font-family:'JetBrains Mono';font-size:11.5px;word-break:break-all">${esc(v)}</span></div>`;
  }

  function renderDetail() {
    const wrap = $('#detail-wrap');
    const r = state.rows.find((x) => x.id === state.selId);
    if (!r) { wrap.style.display = 'none'; $('#list-region').style.flex = '1'; return; }
    wrap.style.display = 'flex';
    $('#list-region').style.flex = '0.55';

    const tab = (id, label, count) => `<button data-tab="${id}" style="background:transparent;border:none;cursor:pointer;padding:8px 12px 7px;font-size:11.5px;font-family:Inter;font-weight:500;color:${state.detailTab === id ? C.ink : C.dim};border-bottom:2px solid ${state.detailTab === id ? C.mint : 'transparent'};margin-bottom:-1px" class="inline-flex items-center gap-[6px]">${label}${count != null ? `<span style="color:${C.faint};font-family:'JetBrains Mono';font-size:10px">${count}</span>` : ''}</button>`;

    let body = '';
    const t = state.detailTab;
    if (t === 'overview') {
      body = `<div style="padding:12px 16px">
        <div class="grid gap-6" style="grid-template-columns:1fr 1fr">
          <div><div style="font-size:10.5px;color:${C.faint};text-transform:uppercase;letter-spacing:.7px;margin-bottom:4px">General</div>
            ${kv('URL', `${r.scheme}://${r.host}${r.path}`)}${kv('Method', r.method)}${kv('Status', String(r.status), statusColor(r.status))}${kv('Protocol', r.tls ? 'HTTP/2 over TLS 1.3' : 'HTTP/1.1')}${kv('Client', `${r.clientIp} · ${r.process}`)}</div>
          <div><div style="font-size:10.5px;color:${C.faint};text-transform:uppercase;letter-spacing:.7px;margin-bottom:4px">Transfer</div>
            ${kv('Content-Type', r.mime)}${kv('Size', fmtBytes(r.size))}${kv('Duration', fmtMs(r.ms))}${kv('TLS', r.tls ? (r.decrypted ? 'decrypted · HSL root CA' : 'passthrough · no key') : '—', r.tls ? (r.decrypted ? C.mint : C.warn) : C.faint)}${kv('Cache', r.id % 3 === 1 ? 'HIT' : 'MISS')}</div>
        </div>
        <div style="margin-top:18px"><div style="font-size:10.5px;color:${C.faint};text-transform:uppercase;letter-spacing:.7px;margin-bottom:6px">Timing</div>${timingBar(r)}</div></div>`;
    } else if (t === 'headers') {
      const sectionHead = (l, n) => `<div style="padding:6px 14px;font-size:10.5px;color:${C.faint};text-transform:uppercase;letter-spacing:.7px;font-weight:600;background:${C.bg2};border-bottom:1px solid ${C.line}">${l} headers · ${n}</div>`;
      body = sectionHead('Request', reqHeaders(r).length) + headersTable(reqHeaders(r)) + sectionHead('Response', resHeaders(r).length) + headersTable(resHeaders(r));
    } else if (t === 'body') {
      body = bodyPane(r);
    } else if (t === 'timing') {
      body = timingBar(r);
    } else if (t === 'raw') {
      body = `<pre style="margin:0;padding:12px 14px;font-family:'JetBrains Mono';font-size:11.5px;line-height:1.55;color:${C.dim};white-space:pre-wrap;word-break:break-all">${esc(`${r.method} ${r.path} HTTP/2\nhost: ${r.host}\n\n` + bodyFor(r))}</pre>`;
    }

    wrap.innerHTML = `
      <div style="padding:10px 14px;border-bottom:1px solid ${C.line};background:${C.bg1};display:flex;align-items:center;gap:10px;flex-shrink:0">
        ${methodTag(r.method)}${statusPill(r.status)}
        <div class="flex-1 overflow-hidden">
          <div class="truncate" style="font-family:'JetBrains Mono';font-size:12px;color:${C.ink}"><span style="color:${C.faint}">${r.scheme}://</span><span style="color:${C.dim}">${r.host}</span>${esc(r.path)}</div>
          <div style="font-size:11px;color:${C.faint};margin-top:2px;font-family:'JetBrains Mono'">#${String(r.id).padStart(3, '0')} · ${r.mime} · ${fmtBytes(r.size)} · ${fmtMs(r.ms)} · ${r.tls ? (r.decrypted ? 'TLS · decrypted' : 'TLS · passthrough') : 'cleartext'}</div>
        </div>
        <button data-action="replay" class="hsl-btn">⟲ Replay</button>
        <button data-action="edit" class="hsl-btn">Edit &amp; send…</button>
        <button data-action="close-detail" style="background:transparent;border:none;color:${C.faint};cursor:pointer;font-size:18px;padding:4px 6px">×</button>
      </div>
      <div class="flex" style="border-bottom:1px solid ${C.line};background:${C.bg1};padding-left:6px;flex-shrink:0">
        ${tab('overview', 'Overview')}${tab('headers', 'Headers', reqHeaders(r).length + resHeaders(r).length)}${tab('body', 'Body')}${tab('timing', 'Timing')}${tab('raw', 'Raw')}</div>
      <div class="flex-1 min-h-0 overflow-auto hsl-scroll">${body}</div>`;
  }

  // ─── status bar ──────────────────────────────────────────
  function renderStatusBar() {
    const rows = state.rows;
    const totalBytes = rows.reduce((s, r) => s + r.size, 0);
    const avg = rows.length ? Math.round(rows.reduce((s, r) => s + r.ms, 0) / rows.length) : 0;
    const err = rows.filter((r) => r.status >= 400).length;
    $('#statusbar').innerHTML = `
      <span class="inline-flex items-center gap-[6px]" style="padding:0 10px 0 14px;color:${state.capturing ? C.mint : C.faint}"><span class="rec-dot ${state.capturing ? 'on' : ''}"></span>${state.capturing ? 'capturing' : 'paused'}</span>
      <span style="padding:0 10px;color:${C.dim}">${rows.length} req</span>
      <span style="padding:0 10px;color:${C.dim}">${err} errors</span>
      <span style="padding:0 10px;color:${C.dim}">avg ${avg}ms</span>
      <span style="padding:0 10px;color:${C.dim}">${fmtBytes(totalBytes)} total</span>
      <span class="flex-1"></span>
      <span class="inline-flex items-center gap-[6px]" style="padding:0 10px;color:${state.decryption ? C.mint : C.warn}">${lock(state.decryption, !state.decryption)} HTTPS ${state.decryption ? 'decrypted' : 'passthrough'}</span>
      <span style="padding:0 10px;color:${C.dim}">upstream ${state.upstream.on ? (state.upstream.ntlm ? 'NTLM' : 'direct') : 'off'}</span>
      <span style="padding:0 14px 0 10px;color:${C.dim}">access ${state.access.mode}</span>`;
  }

  // ─── toolbar state sync ──────────────────────────────────
  function renderToolbar() {
    const cap = $('#btn-capture');
    cap.innerHTML = `<span class="rec-dot ${state.capturing ? 'on' : ''}"></span>${state.capturing ? 'Capturing' : 'Paused'}`;
    cap.style.color = state.capturing ? C.mint : C.dim;
    cap.style.background = state.capturing ? C.bg3 : 'transparent';

    const dec = $('#btn-decrypt');
    dec.innerHTML = `${lock(state.decryption)} HTTPS decryption · ${state.decryption ? 'on' : 'off'}`;
    dec.style.color = state.decryption ? C.mint : C.dim;
    dec.style.background = state.decryption ? C.bg3 : 'transparent';

    const up = $('#btn-upstream');
    up.textContent = `⇢ upstream · ${state.upstream.on ? (state.upstream.ntlm ? 'NTLM' : 'direct') : 'off'}`;
    up.style.color = state.upstream.on ? C.mint : C.dim;

    $('#btn-access').textContent = `⊙ access · ${state.access.mode}`;
  }

  // ─── modals ──────────────────────────────────────────────
  function openModal(html, width) {
    const root = $('#modal-root');
    root.innerHTML = `<div class="modal-backdrop" style="position:absolute;inset:0;background:rgba(8,9,12,.62);backdrop-filter:blur(3px);z-index:50;display:flex;align-items:center;justify-content:center">
      <div class="modal-card" style="width:${width || 620}px;max-width:92%;max-height:88%;display:flex;flex-direction:column;background:${C.bg1};border:1px solid ${C.line};border-radius:6px;box-shadow:0 24px 60px rgba(0,0,0,.55);overflow:hidden;color:${C.ink};font-family:Inter">${html}</div></div>`;
    root.style.pointerEvents = 'auto';
  }
  function closeModal() { $('#modal-root').innerHTML = ''; $('#modal-root').style.pointerEvents = 'none'; }

  function modalHeader(title, subtitle) {
    return `<div style="padding:14px 18px 12px;border-bottom:1px solid ${C.line};display:flex;align-items:flex-start;gap:10px">
      <div class="flex-1"><div style="font-size:13.5px;font-weight:600">${title}</div>${subtitle ? `<div style="font-size:11.5px;color:${C.dim};margin-top:3px">${subtitle}</div>` : ''}</div>
      <button data-action="close-modal" style="background:transparent;border:none;color:${C.dim};cursor:pointer;font-size:16px;line-height:1;padding:4px;margin-top:-2px">×</button></div>`;
  }
  function modalFooter(buttons) {
    return `<div style="padding:12px 18px;border-top:1px solid ${C.line};background:${C.bg2};display:flex;gap:8px;justify-content:flex-end">${buttons}</div>`;
  }
  function btn(label, action, tone) {
    const tones = {
      primary: `background:${C.mint};color:${C.bg0};border:1px solid ${C.mint};font-weight:600`,
      danger: `background:transparent;color:${C.danger};border:1px solid ${C.danger}66`,
      ghost: `background:transparent;color:${C.dim};border:1px solid ${C.line}`,
      default: `background:${C.bg3};color:${C.ink};border:1px solid ${C.line}`,
    };
    return `<button data-action="${action}" style="${tones[tone] || tones.default};border-radius:4px;padding:6px 12px;font-size:12px;font-family:Inter;cursor:pointer;height:28px;display:inline-flex;align-items:center;gap:6px">${label}</button>`;
  }
  function toggle(on, action, sm, disabled) {
    const w = sm ? 28 : 34, h = sm ? 16 : 20, dot = sm ? 12 : 16;
    return `<button ${disabled ? '' : `data-action="${action}"`} style="width:${w}px;height:${h}px;border-radius:${h / 2}px;padding:2px;background:${on ? C.mint : C.bg3};border:1px solid ${on ? C.mint : C.line};cursor:${disabled ? 'not-allowed' : 'pointer'};display:flex;align-items:center;opacity:${disabled ? .5 : 1}">
      <span style="width:${dot}px;height:${dot}px;border-radius:${dot / 2}px;background:${on ? C.bg0 : C.dim};transform:translateX(${on ? w - dot - 6 : 0}px);transition:transform .15s"></span></button>`;
  }

  // cert wizard
  const cert = { step: 0, progress: 0, timer: null };
  function openCert() { cert.step = 0; cert.progress = 0; renderCert(); }
  function renderCert() {
    const stepper = ['Review', 'Generate', 'Trust'].map((s, i) => {
      const on = i === cert.step, done = i < cert.step;
      return `<div class="flex items-center gap-2 flex-1"><span style="width:20px;height:20px;border-radius:10px;display:inline-flex;align-items:center;justify-content:center;background:${done ? C.mint : on ? C.bg3 : C.bg2};border:1px solid ${done || on ? C.mint : C.line};color:${done ? C.bg0 : on ? C.mint : C.dim};font-size:11px;font-weight:600;font-family:'JetBrains Mono'">${done ? '✓' : i + 1}</span><span style="font-size:12px;color:${on || done ? C.ink : C.dim};font-weight:500">${s}</span></div>`;
    }).join(`<div style="height:1px;flex:.3;background:${C.line}"></div>`);

    let inner = '';
    if (cert.step === 0) {
      inner = `<div style="font-size:12.5px;line-height:1.65;color:${C.dim}">
        <p style="margin:0 0 10px">To decrypt HTTPS, HttpStackLens generates a self-signed root CA and signs a leaf certificate for every host you visit — acting as a man-in-the-middle on your own traffic, only on this machine.</p>
        <div style="background:${C.bg2};border:1px solid ${C.line};border-radius:4px;padding:10px 12px;font-family:'JetBrains Mono';font-size:11.5px;color:${C.ink};margin:12px 0"><div style="color:${C.faint};font-size:10.5px;margin-bottom:4px">will be created</div>CN=HttpStackLens Root CA · RSA 3072 · SHA-256 · valid 1825 days</div>
        <div style="background:${C.warn}12;border:1px solid ${C.warn}40;border-radius:4px;padding:10px 12px;color:${C.warn};font-size:11.5px;display:flex;gap:10px"><span style="font-size:14px;line-height:1">⚠</span><div style="color:${C.ink}">The root key lets anyone impersonate any site to this user. It is stored at <code style="color:${C.warn};font-family:'JetBrains Mono'">%APPDATA%\\HttpStackLens\\root.pfx</code> — never share it.</div></div></div>`;
    } else if (cert.step === 1) {
      inner = `<div style="text-align:center;padding:24px 20px">
        <div class="spin" style="margin:0 auto 16px;width:40px;height:40px;border-radius:20px;border:2px solid ${C.bg2};border-top-color:${C.mint}"></div>
        <div style="font-size:12.5px;color:${C.ink};margin-bottom:4px">Generating keypair…</div>
        <div style="font-size:11px;color:${C.dim};font-family:'JetBrains Mono'">${Math.min(100, Math.floor(cert.progress))}%</div>
        <div style="width:70%;margin:14px auto 0;height:3px;border-radius:2px;background:${C.bg2};overflow:hidden"><div style="width:${Math.min(100, cert.progress)}%;height:100%;background:${C.mint};transition:width .1s"></div></div></div>`;
    } else {
      inner = `<div>
        <div style="background:${C.mint}10;border:1px solid ${C.mint}40;border-radius:4px;padding:10px 12px;color:${C.mint};font-size:11.5px;margin-bottom:14px;display:flex;gap:10px"><span>✓</span><div style="color:${C.ink}">Certificate generated. <span style="color:${C.dim}">Fingerprint:</span> <span style="font-family:'JetBrains Mono'">74:3F:2C:9A:…:B1</span></div></div>
        <div style="font-size:12.5px;color:${C.dim};margin-bottom:10px">Install it into the OS trust store so browsers and tools accept it:</div>
        <div class="grid gap-2">${[['⊞', 'Windows — Current User', 'Trusted Root Certification Authorities', 'Install'], ['', 'Firefox trust store', 'Firefox uses its own NSS store', 'Install'], ['⏍', 'Export PFX', 'Copy to %APPDATA% or USB drive', 'Export…']].map(([ic, n, h, a]) =>
          `<div class="flex items-center gap-3" style="padding:10px 12px;background:${C.bg2};border:1px solid ${C.line};border-radius:4px"><span style="font-size:16px;color:${C.dim};width:20px;text-align:center">${ic}</span><div class="flex-1"><div style="font-size:12px;color:${C.ink}">${n}</div><div style="font-size:11px;color:${C.faint}">${h}</div></div>${btn(a, 'noop')}</div>`).join('')}</div></div>`;
    }

    let footer;
    if (cert.step === 0) footer = btn('Cancel', 'close-modal', 'ghost') + btn('Generate certificate', 'cert-generate', 'primary');
    else if (cert.step === 1) footer = btn('Cancel', 'close-modal', 'ghost') + btn('Generating…', 'noop', 'default');
    else footer = btn('Cancel', 'close-modal', 'ghost') + btn('Done — enable decryption', 'cert-done', 'primary');

    openModal(modalHeader('HTTPS decryption setup', 'Install a local root certificate so HttpStackLens can inspect TLS traffic. The key never leaves this machine.') +
      `<div style="padding:16px 20px 20px"><div class="flex items-center gap-2" style="margin-bottom:18px">${stepper}</div>${inner}</div>` +
      modalFooter(footer), 620);
  }

  // settings modal with tabs
  let settingsTab = 'body';
  function openSettings(tab) { settingsTab = tab || 'body'; renderSettings(); }
  function renderSettings() {
    const tabs = [['cert', 'TLS / Certificate'], ['body', 'Body capture'], ['upstream', 'Upstream proxy'], ['access', 'Access control'], ['hotkeys', 'Shortcuts']];
    const nav = tabs.map(([id, l]) => `<button data-settab="${id}" style="display:block;width:100%;text-align:left;background:${settingsTab === id ? C.bg3 : 'transparent'};border:none;cursor:pointer;padding:7px 10px;border-radius:3px;margin-bottom:2px;color:${settingsTab === id ? C.ink : C.dim};font-size:12px;font-family:Inter;font-weight:500">${l}</button>`).join('');
    openModal(modalHeader('Settings') +
      `<div class="grid" style="grid-template-columns:180px 1fr;min-height:380px">
        <div style="border-right:1px solid ${C.line};padding:10px 8px;background:${C.bg2}">${nav}</div>
        <div style="padding:18px;overflow:auto" id="settings-body">${settingsBody()}</div></div>` +
      modalFooter(btn('Done', 'close-modal', 'primary')), 820);
  }
  function settingsBody() {
    if (settingsTab === 'cert') {
      return `<div style="font-size:12px;color:${C.dim};line-height:1.7">
        <div style="padding:12px;background:${C.bg2};border:1px solid ${C.line};border-radius:4px;margin-bottom:12px">
          <div class="flex items-center gap-[10px]" style="margin-bottom:8px"><span style="color:${C.mint};font-size:16px">●</span><div class="flex-1"><div style="font-size:12.5px;color:${C.ink};font-weight:600">Root CA installed</div><div style="font-size:11px;color:${C.dim};font-family:'JetBrains Mono'">74:3F:2C:9A:…:B1 · expires 2029-11-14</div></div></div>
          <div class="flex gap-[6px]">${btn('Export PFX', 'noop')}${btn('Reinstall', 'noop')}${btn('Regenerate root', 'noop', 'danger')}</div></div>
        <p>Tools with their own trust store (Firefox, Node without NODE_EXTRA_CA_CERTS, the JVM) need the cert installed separately.</p></div>`;
    }
    if (settingsTab === 'body') return bodyRulesPanel();
    if (settingsTab === 'upstream') return upstreamPanel();
    if (settingsTab === 'access') return accessPanel();
    if (settingsTab === 'hotkeys') {
      const keys = [['Space', 'Pause / resume capture'], ['⌘/Ctrl K', 'Filter requests'], ['⌘/Ctrl L', 'Clear session'], ['R', 'Replay selected'], ['Enter', 'Toggle detail pane'], ['J / K', 'Prev / next request']];
      return keys.map(([k, l]) => `<div class="flex justify-between" style="padding:7px 2px;border-bottom:1px solid ${C.lineSoft};font-size:12px"><span style="color:${C.dim}">${l}</span><span style="font-family:'JetBrains Mono';color:${C.ink};background:${C.bg2};border:1px solid ${C.line};border-radius:3px;padding:1px 7px;font-size:11px">${k}</span></div>`).join('');
    }
    return '';
  }

  function sizeLabel(b) { return b === 0 ? 'off' : b < 1048576 ? (b / 1024) + ' KB' : (b / 1048576) + ' MB'; }
  function bodyRulesPanel() {
    const head = `<div class="grid" style="grid-template-columns:32px 1fr 140px 120px;padding:8px 12px;background:${C.bg2};border-bottom:1px solid ${C.line};font-size:10.5px;font-weight:600;letter-spacing:.6px;text-transform:uppercase;color:${C.faint};font-family:Inter"><span></span><span>Category / MIME</span><span>Max per body</span><span style="text-align:right">Captured</span></div>`;
    const rows = state.bodyRules.map((r, i) => `<div class="grid items-center gap-2" style="grid-template-columns:32px 1fr 140px 120px;padding:10px 12px;border-bottom:${i === state.bodyRules.length - 1 ? 'none' : `1px solid ${C.lineSoft}`};background:${r.on ? 'transparent' : C.bg1};opacity:${r.locked ? .7 : 1}">
      ${toggle(r.on, `rule-toggle:${r.id}`, true, r.locked)}
      <div><div style="font-size:12px;color:${C.ink};font-family:Inter;font-weight:500">${r.label}${r.locked ? `<span style="color:${C.warn};font-size:10px;margin-left:8px">streaming</span>` : ''}</div><div style="font-size:11px;color:${C.faint};font-family:'JetBrains Mono';margin-top:2px">${r.mimes}</div>${r.note ? `<div style="font-size:10.5px;color:${C.warn};margin-top:3px">${r.note}</div>` : ''}</div>
      <select ${!r.on || r.locked ? 'disabled' : ''} data-rulemax="${r.id}" style="background:${C.bg2};color:${C.ink};border:1px solid ${C.line};border-radius:3px;padding:4px 6px;font-family:'JetBrains Mono';font-size:11.5px">${[0, 65536, 262144, 524288, 1048576, 2097152, 8388608].map((b) => `<option value="${b}" ${b === r.max ? 'selected' : ''}>${sizeLabel(b)}</option>`).join('')}</select>
      <div style="text-align:right;font-family:'JetBrains Mono';font-size:11px;color:${C.dim}">${r.on && !r.locked ? `${(r.id.charCodeAt(0) * 3) % 120 + 4} bodies` : '—'}</div></div>`).join('');
    return `<div style="font-size:12px;color:${C.dim};margin-bottom:12px;line-height:1.6">Pick which response bodies get captured and kept in memory. Streaming types (SSE, WebSocket) are always metadata-only — the frame log stays, the body doesn't.</div>
      <div style="border:1px solid ${C.line};border-radius:4px;overflow:hidden">${head}${rows}</div>`;
  }

  function field(label, hint, control) {
    return `<div><div class="flex items-baseline gap-2" style="margin-bottom:5px"><span style="font-size:11.5px;color:${C.ink};font-family:Inter;font-weight:500">${label}</span>${hint ? `<span style="font-size:11px;color:${C.faint}">${hint}</span>` : ''}</div>${control}</div>`;
  }
  function input(val, ph, wide, disabled) {
    return `<input value="${val || ''}" placeholder="${ph || ''}" ${disabled ? 'disabled' : ''} style="background:${C.bg2};color:${C.ink};border:1px solid ${C.line};border-radius:3px;padding:6px 8px;font-family:'JetBrains Mono';font-size:11.5px;width:${wide ? '100%' : '220px'};outline:none">`;
  }
  function upstreamPanel() {
    const u = state.upstream;
    return `<div class="grid gap-[14px]">
      <div style="font-size:12px;color:${C.dim};line-height:1.6">Route outgoing traffic through a corporate proxy. HttpStackLens can handle NTLM / Negotiate auth on your behalf so apps that can't speak it still reach the outside world.</div>
      ${field('Upstream proxy', 'Leave empty to connect directly', `<div class="flex gap-[6px]">${input(u.host, 'http://proxy.corp.local:8080', true)}${toggle(u.on, 'upstream-toggle')}</div>`)}
      ${field('Bypass hosts', 'Comma-separated · globs allowed', input('*.corp.local, 10.*, localhost', '', true))}
      <div style="padding:14px;background:${C.bg2};border:1px solid ${C.line};border-radius:4px">
        <div class="flex items-center gap-[10px]" style="margin-bottom:10px"><span style="font-size:16px">⊞</span><div class="flex-1"><div style="font-size:12.5px;color:${C.ink};font-weight:600">NTLM / Negotiate auth</div><div style="font-size:11px;color:${C.dim};margin-top:2px">Windows only · use current Windows session to authenticate</div></div>${toggle(u.ntlm, 'ntlm-toggle', false, !u.on)}</div>
        <div class="grid gap-[10px]" style="grid-template-columns:1fr 1fr;opacity:${u.ntlm ? 1 : .5}">${field('Domain', '', input(u.domain, 'CORP', false, !u.ntlm))}${field('Username override', '', input('', '(current session)', false, !u.ntlm))}</div></div>
      ${field('PAC script', 'Optional — overrides the settings above when matched', input('', 'http://wpad/wpad.dat', true))}</div>`;
  }
  function accessPanel() {
    const a = state.access;
    const radio = (mode, title, sub, danger) => `<button data-access="${mode}" class="flex items-center gap-3 w-full text-left" style="padding:10px 12px;background:${a.mode === mode ? C.bg3 : C.bg2};border:1px solid ${a.mode === mode ? (danger ? C.danger : C.mint) : C.line};border-radius:4px;cursor:pointer;color:${C.ink};font-family:Inter">
      <span style="width:14px;height:14px;border-radius:7px;flex-shrink:0;border:1.5px solid ${a.mode === mode ? (danger ? C.danger : C.mint) : C.faint};display:inline-flex;align-items:center;justify-content:center">${a.mode === mode ? `<span style="width:6px;height:6px;border-radius:3px;background:${danger ? C.danger : C.mint}"></span>` : ''}</span>
      <div class="flex-1"><div style="font-size:12.5px;font-weight:500;color:${danger && a.mode === mode ? C.danger : C.ink}">${title}</div><div style="font-size:11px;color:${C.dim};margin-top:2px">${sub}</div></div></button>`;
    let networks = '';
    if (a.mode === 'allowlist') {
      networks = `<div><div style="font-size:11.5px;color:${C.dim};margin-bottom:6px;font-family:Inter">Allowed networks</div><div style="border:1px solid ${C.line};border-radius:4px;overflow:hidden">${a.networks.map((c) => `<div class="flex items-center gap-[10px]" style="padding:7px 10px;border-bottom:1px solid ${C.lineSoft};background:${C.bg2}"><span style="font-family:'JetBrains Mono';font-size:11.5px;color:${C.ink};flex:1">${c}</span></div>`).join('')}<div style="padding:8px;background:${C.bg1}">${btn('+ Add network', 'noop', 'ghost')}</div></div></div>`;
    }
    return `<div class="grid gap-[14px]">
      <div style="font-size:12px;color:${C.dim};line-height:1.6">Control which machines can connect to this proxy. By default only loopback (127.0.0.1) is accepted — safer when the machine is on an untrusted network.</div>
      <div class="grid gap-2">${radio('loopback', 'Loopback only', '127.0.0.1 and ::1 · recommended')}${radio('lan', 'Private LAN', 'RFC 1918 — 10/8 · 172.16/12 · 192.168/16')}${radio('allowlist', 'Explicit allowlist', 'Only the networks below')}${radio('open', 'Open — any source', 'Dangerous on untrusted networks', true)}</div>
      ${networks}
      <div style="font-size:11px;color:${C.dim};font-family:'JetBrains Mono';padding:8px 10px;background:${C.bg2};border-radius:3px;border:1px solid ${C.line}">listening on <span style="color:${C.mint}">0.0.0.0:8823</span> · <span style="color:${C.ink}">${a.mode}</span></div></div>`;
  }

  // ─── live traffic ────────────────────────────────────────
  let liveTimer = null;
  function scheduleNext() {
    if (!state.capturing) return;
    liveTimer = setTimeout(() => {
      const r = mockReq((state.rows[state.rows.length - 1]?.id || 0) + 1);
      state.rows.push(r);
      if (state.rows.length > 220) state.rows.shift();
      appendRow(r);
      scheduleNext();
    }, 700 + Math.random() * 900);
  }
  function startLive() { clearTimeout(liveTimer); scheduleNext(); }
  function stopLive() { clearTimeout(liveTimer); }

  // ─── event delegation ────────────────────────────────────
  function wire() {
    document.addEventListener('click', (e) => {
      const row = e.target.closest('[data-row]');
      if (row) { const id = Number(row.dataset.row); state.selId = state.selId === id ? null : id; renderList(); renderDetail(); return; }

      const host = e.target.closest('[data-host]');
      if (host) { $('#filter').value = host.dataset.host; state.filter = host.dataset.host; renderList(); return; }

      const side = e.target.closest('[data-side]');
      if (side) { state.sidebar = side.dataset.side; renderList(); return; }

      const tab = e.target.closest('[data-tab]');
      if (tab) { state.detailTab = tab.dataset.tab; renderDetail(); return; }

      const bm = e.target.closest('[data-bodymode]');
      if (bm) { state.bodyMode = bm.dataset.bodymode; renderDetail(); return; }

      const settab = e.target.closest('[data-settab]');
      if (settab) { settingsTab = settab.dataset.settab; renderSettings(); return; }

      const acc = e.target.closest('[data-access]');
      if (acc) { state.access.mode = acc.dataset.access; renderSettings(); renderToolbar(); renderStatusBar(); return; }

      const ruleT = e.target.closest('[data-action^="rule-toggle:"]');
      if (ruleT) { const id = ruleT.dataset.action.split(':')[1]; const r = state.bodyRules.find((x) => x.id === id); if (r && !r.locked) r.on = !r.on; renderSettings(); return; }

      const act = e.target.closest('[data-action]');
      if (act) { handleAction(act.dataset.action); return; }

      if (e.target.closest('.modal-backdrop') && !e.target.closest('.modal-card')) closeModal();
    });

    // selects inside modals
    document.addEventListener('change', (e) => {
      const rm = e.target.closest('[data-rulemax]');
      if (rm) { const r = state.bodyRules.find((x) => x.id === rm.dataset.rulemax); if (r) r.max = Number(rm.value); }
    });

    $('#filter').addEventListener('input', (e) => { state.filter = e.target.value; renderList(); });
    $('#filter-clear').addEventListener('click', () => { $('#filter').value = ''; state.filter = ''; renderList(); });

    $$('[data-density]').forEach((b) => b.addEventListener('click', () => {
      state.density = b.dataset.density;
      $$('[data-density]').forEach((x) => {
        const on = x.dataset.density === state.density;
        x.style.background = on ? C.mint + '22' : C.bg2;
        x.style.color = on ? C.mint : C.dim;
        x.style.borderColor = on ? C.mint + '66' : C.line;
      });
      renderList(); renderDetail();
    }));

    document.addEventListener('keydown', (e) => {
      if (e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT') return;
      if (e.code === 'Space') { e.preventDefault(); handleAction('toggle-capture'); }
      if (e.key === 'k' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); $('#filter').focus(); }
      if (e.key === 'l' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); handleAction('clear'); }
      if (e.key === 'Escape') closeModal();
    });
  }

  function handleAction(a) {
    switch (a) {
      case 'toggle-capture':
        state.capturing = !state.capturing;
        if (state.capturing) startLive(); else stopLive();
        renderToolbar(); renderStatusBar();
        break;
      case 'clear': state.rows = []; state.selId = null; renderList(); renderDetail(); break;
      case 'toggle-decrypt':
        if (state.decryption) { state.decryption = false; renderToolbar(); renderStatusBar(); }
        else openCert();
        break;
      case 'open-upstream': openSettings('upstream'); break;
      case 'open-access': openSettings('access'); break;
      case 'open-settings': openSettings('body'); break;
      case 'close-modal': closeModal(); break;
      case 'close-detail': state.selId = null; renderList(); renderDetail(); break;
      case 'cert-generate':
        cert.step = 1; cert.progress = 0; renderCert();
        cert.timer = setInterval(() => {
          cert.progress += 4 + Math.random() * 8;
          if (cert.progress >= 100) { clearInterval(cert.timer); cert.step = 2; }
          renderCert();
        }, 90);
        break;
      case 'cert-done': state.decryption = true; closeModal(); renderToolbar(); renderStatusBar(); break;
      case 'upstream-toggle': state.upstream.on = !state.upstream.on; renderSettings(); renderToolbar(); renderStatusBar(); break;
      case 'ntlm-toggle': if (state.upstream.on) { state.upstream.ntlm = !state.upstream.ntlm; renderSettings(); renderToolbar(); renderStatusBar(); } break;
      default: break;
    }
  }

  // ─── boot ────────────────────────────────────────────────
  function boot() {
    for (let i = 1; i <= 36; i++) state.rows.push(mockReq(i));
    wire();
    renderToolbar();
    renderList();
    renderDetail();
    startLive();
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
  else boot();
})();
