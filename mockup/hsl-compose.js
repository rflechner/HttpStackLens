/* HttpStackLens — request composer (.http files).
   Self-contained module. Styles use CSS variables so it follows the
   light/dark theme without re-rendering. Exposes window.HSLCompose. */
(function () {
  'use strict';
  const $ = (s, r = document) => r.querySelector(s);
  const $$ = (s, r = document) => Array.from(r.querySelectorAll(s));
  const esc = (s) => String(s == null ? '' : s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  const uid = () => Math.random().toString(36).slice(2, 9);
  const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'];

  /* ── .http parsing / serialising ───────────────────────── */
  function parseHttp(text, name) {
    const vars = [], reqs = [];
    let cur = null, section = 'none';
    for (const line of String(text).split(/\r?\n/)) {
      if (/^###/.test(line)) {
        cur = { id: uid(), name: line.replace(/^#+\s*/, '').trim() || 'Untitled', method: 'GET', url: '', headers: [], body: '' };
        reqs.push(cur); section = 'reqline'; continue;
      }
      const vm = line.match(/^@([\w.-]+)\s*=\s*(.*)$/);
      if (vm && (section === 'none' || section === 'reqline')) { vars.push({ id: uid(), key: vm[1], value: vm[2].trim(), on: true }); continue; }
      if (!cur) continue;
      if (section === 'reqline') {
        if (!line.trim() || /^(#|\/\/)/.test(line)) continue;
        const m = line.match(/^([A-Za-z]+)\s+(\S+)(?:\s+HTTP\/[\d.]+)?\s*$/);
        if (m && METHODS.includes(m[1].toUpperCase())) { cur.method = m[1].toUpperCase(); cur.url = m[2]; }
        else cur.url = line.trim();
        section = 'headers'; continue;
      }
      if (section === 'headers') {
        if (!line.trim()) { section = 'body'; continue; }
        const m = line.match(/^([\w.-]+)\s*:\s*(.*)$/);
        if (m) cur.headers.push({ id: uid(), key: m[1], value: m[2].trim(), on: true });
        continue;
      }
      cur.body += (cur.body ? '\n' : '') + line;
    }
    reqs.forEach((r) => { r.body = r.body.replace(/\s+$/, ''); });
    return { id: uid(), name: name || 'untitled.http', vars, reqs, open: true };
  }

  function toHttp(file) {
    const vs = file.vars.filter((v) => v.on && v.key);
    let out = vs.map((v) => `@${v.key} = ${v.value}`).join('\n');
    if (out) out += '\n\n';
    out += file.reqs.map((r) => {
      const hs = r.headers.filter((h) => h.on && h.key);
      let s = `### ${r.name}\n${r.method} ${r.url}\n`;
      if (hs.length) s += hs.map((h) => `${h.key}: ${h.value}`).join('\n') + '\n';
      if (r.body.trim()) s += '\n' + r.body + '\n';
      return s;
    }).join('\n');
    return out;
  }

  /* ── seed data ─────────────────────────────────────────── */
  const SEED = [
    parseHttp(`@baseUrl = https://api.github.com
@token = ghp_exampletoken

### Current user
GET {{baseUrl}}/user
Authorization: Bearer {{token}}
Accept: application/vnd.github+json

### Open pull requests
GET {{baseUrl}}/repos/golang/go/pulls?state=open&per_page=5
Accept: application/vnd.github+json

### Create issue comment
POST {{baseUrl}}/repos/golang/go/issues/68412/comments
Authorization: Bearer {{token}}
Content-Type: application/json

{
  "body": "Reproduced on go1.23.2 — trace attached."
}
`, 'github.http'),
    parseHttp(`@auth = https://auth.corp.local

### Client credentials token
POST {{auth}}/oauth2/token
Content-Type: application/x-www-form-urlencoded

grant_type=client_credentials&client_id=hsl-cli&client_secret=s3cret

### JWKS
GET {{auth}}/.well-known/jwks.json
`, 'corp-auth.http'),
  ];

  /* ── state ─────────────────────────────────────────────── */
  const S = { files: [], fileId: null, reqId: null, tab: 'body', resTab: 'body', dirty: {}, response: null, sending: false, resMode: 'pretty', rawDraft: null, lastFormTab: 'body' };
  const STORE = 'hsl-http-files';
  const DEF_STORE = { folder: '~/HttpStackLens/requests', autosave: true, ext: '.http' };
  function storeCfg() {
    if (window.HSL && window.HSL.store) return window.HSL.store();
    try { return Object.assign({}, DEF_STORE, JSON.parse(localStorage.getItem('hsl-store') || '{}')); } catch (e) { return DEF_STORE; }
  }

  function load() {
    try {
      const raw = localStorage.getItem(STORE);
      if (raw) { const f = JSON.parse(raw); if (Array.isArray(f) && f.length) { S.files = f; return; } }
    } catch (e) {}
    S.files = SEED;
  }
  function save() { try { localStorage.setItem(STORE, JSON.stringify(S.files)); } catch (e) {} }
  const file = () => S.files.find((f) => f.id === S.fileId);
  const req = () => { const f = file(); return f && f.reqs.find((r) => r.id === S.reqId); };

  /* ── url / query helpers ───────────────────────────────── */
  function splitUrl(url) {
    const i = url.indexOf('?');
    if (i < 0) return { base: url, params: [] };
    const params = url.slice(i + 1).split('&').filter(Boolean).map((p) => {
      const j = p.indexOf('=');
      return { id: uid(), key: j < 0 ? p : p.slice(0, j), value: j < 0 ? '' : p.slice(j + 1), on: true };
    });
    return { base: url.slice(0, i), params };
  }
  function joinUrl(base, params) {
    const q = params.filter((p) => p.on && p.key).map((p) => `${p.key}=${p.value}`).join('&');
    return q ? `${base}?${q}` : base;
  }
  function interpolate(str, vars) {
    return String(str).replace(/\{\{\s*([\w.-]+)\s*\}\}/g, (m, k) => {
      const v = vars.find((x) => x.on && x.key === k);
      return v ? v.value : m;
    });
  }
  function methodColor(m) {
    return ({ GET: 'var(--mint)', POST: 'var(--warn)', PUT: 'var(--info)', PATCH: 'var(--pink)', DELETE: 'var(--danger)' })[m] || 'var(--dim)';
  }
  function statusVar(s) { return s >= 500 ? 'var(--danger)' : s >= 400 ? 'var(--warn)' : s >= 300 ? 'var(--info)' : s >= 200 ? 'var(--mint)' : 'var(--dim)'; }
  function fmtBytes(b) { return b < 1024 ? b + ' B' : b < 1048576 ? (b / 1024).toFixed(1) + ' KB' : (b / 1048576).toFixed(2) + ' MB'; }

  /* ── collections sidebar ───────────────────────────────── */
  function renderFiles() {
    const el = $('#cx-files');
    if (!el) return;
      el.innerHTML = S.files.map((f) => {
      const active = f.id === S.fileId;
      const rows = f.open ? f.reqs.map((r) => `<button class="cx-req${r.id === S.reqId ? ' on' : ''}" data-cx-req="${r.id}" data-cx-file="${f.id}" title="${esc(r.method)} ${esc(r.url)}">
        <span class="cx-m" style="color:${methodColor(r.method)}">${r.method}</span>
        <span class="cx-req-name">${esc(r.name)}</span></button>`).join('')
        + `<button class="cx-add-req" data-cx="new-req" data-cx-file="${f.id}">+ New request</button>` : '';
      return `<div class="cx-file${active ? ' active' : ''}">
        <button class="cx-file-head" data-cx="toggle-file" data-cx-file="${f.id}">
          <span class="cx-caret">${f.open ? '▾' : '▸'}</span>
          <svg class="cx-file-ico" width="13" height="14" viewBox="0 0 13 14" fill="none"><path d="M2 1.5h5l4 3.5v7.5H2z" stroke="currentColor" stroke-width="1.1" stroke-linejoin="round"/><path d="M7 1.5V5h4" stroke="currentColor" stroke-width="1.1" stroke-linejoin="round"/></svg>
          <span class="cx-file-name">${esc(f.name)}</span>
          ${S.dirty[f.id] ? '<span class="cx-dot-dirty" title="unsaved changes"></span>' : ''}
          <span class="cx-file-count">${f.reqs.length}</span></button>
        ${rows}</div>`;
      }).join('') || `<div class="cx-empty">No .http file open yet.<button class="cx-btn" data-cx="new-file" style="margin-top:10px">+ New .http file</button></div>`;
    const p = $('#cx-path');
    if (p) { p.textContent = storeCfg().folder; p.title = storeCfg().folder; }
  }

  /* ── key/value editor ──────────────────────────────────── */
  function kvEditor(list, kind, phk, phv) {
    const rows = list.map((it, i) => `<div class="cx-kv">
      <button class="cx-check${it.on ? ' on' : ''}" data-cx="kv-toggle" data-kind="${kind}" data-i="${i}">${it.on ? '✓' : ''}</button>
      <input class="cx-in mono" data-cx-field="kv" data-kind="${kind}" data-i="${i}" data-key="key" value="${esc(it.key)}" placeholder="${phk}" />
      <input class="cx-in mono" data-cx-field="kv" data-kind="${kind}" data-i="${i}" data-key="value" value="${esc(it.value)}" placeholder="${phv}" />
      <button class="cx-x" data-cx="kv-del" data-kind="${kind}" data-i="${i}">×</button></div>`).join('');
    return `<div class="cx-kv-list">${rows}<button class="cx-add" data-cx="kv-add" data-kind="${kind}">+ Add ${kind === 'vars' ? 'variable' : kind === 'params' ? 'parameter' : 'header'}</button></div>`;
  }

  /* ── editor pane ───────────────────────────────────────── */
  function renderEditor() {
    const wrap = $('#cx-editor');
    const r = req(), f = file();
    if (!r) {
      wrap.innerHTML = `<div class="cx-blank"><div class="cx-blank-t">No request selected</div>
        <div class="cx-blank-s">Pick one on the left, create a new request, or import a <span class="mono">.http</span> file.</div>
        <div class="cx-blank-a">${['new-req', 'new-file', 'import'].map((a) => `<button class="cx-btn" data-cx="${a}">${a === 'import' ? 'Import .http…' : a === 'new-file' ? '+ New .http file' : '+ New request'}</button>`).join('')}</div></div>`;
      return;
    }
    const { base, params } = splitUrl(r.url);
    const tabs = [['body', 'Body'], ['headers', `Headers${r.headers.length ? ' · ' + r.headers.length : ''}`], ['params', `Params${params.length ? ' · ' + params.length : ''}`], ['vars', `Variables${f.vars.length ? ' · ' + f.vars.length : ''}`]];
    const fileIco = '<svg width="11" height="12" viewBox="0 0 13 14" fill="none"><path d="M2 1.5h5l4 3.5v7.5H2z" stroke="currentColor" stroke-width="1.1" stroke-linejoin="round"/><path d="M7 1.5V5h4" stroke="currentColor" stroke-width="1.1" stroke-linejoin="round"/></svg>';
    const viewSeg = `<div class="cx-view-seg" title="Switch between the form editor and the raw ${esc(f.name)} text">
      <span class="cx-view-l">Edit as</span>
      <button class="cx-vbtn${S.tab === 'raw' ? '' : ' on'}" data-cx-tab="${S.lastFormTab || 'body'}">Form</button>
      <button class="cx-vbtn${S.tab === 'raw' ? ' on' : ''}" data-cx-tab="raw">${fileIco}<span class="mono">${esc(f.name)}</span></button></div>`;
    let pane = '';
    if (S.tab === 'body') {
      pane = `<div class="cx-body-pane">
        <div class="cx-body-head">
          <span class="cx-hint">Raw body · <span class="mono">{{variables}}</span> are substituted on send</span>
          <button class="cx-mini" data-cx="format-json">Format JSON</button></div>
        <textarea class="cx-ta mono" data-cx-field="body" spellcheck="false" placeholder='{ "key": "value" }'>${esc(r.body)}</textarea></div>`;
    } else if (S.tab === 'raw') {
      const text = S.rawDraft != null ? S.rawDraft : toHttp(f);
      pane = `<div class="cx-raw-pane">
        <div class="cx-body-head"><span class="cx-hint">Editing <span class="mono">${esc(f.name)}</span> directly — <span class="mono">@vars</span> at the top, one request per <span class="mono">###</span> block. Changes sync to the list on the left as you type.</span>
        <span id="cx-raw-stat" class="cx-raw-stat">${f.reqs.length} request${f.reqs.length === 1 ? '' : 's'}</span>
        <button class="cx-mini" data-cx="export">Save .http</button></div>
        <div class="cx-raw-wrap"><div class="cx-gutter mono" id="cx-gutter">${text.split('\n').map((_, i) => i + 1).join('<br>')}</div><textarea class="cx-ta raw mono" data-cx-field="raw" spellcheck="false" placeholder="### My request&#10;GET https://api.example.com">${esc(text)}</textarea></div></div>`;
    } else if (S.tab === 'headers') pane = kvEditor(r.headers, 'headers', 'Header name', 'Value');
    else if (S.tab === 'params') pane = kvEditor(params, 'params', 'Parameter', 'Value');
    else pane = `<div class="cx-vars"><div class="cx-hint" style="padding:10px 14px 2px">File-level variables for <span class="mono">${esc(f.name)}</span> — written as <span class="mono">@name = value</span> at the top of the file.</div>${kvEditor(f.vars, 'vars', 'name', 'value')}</div>`;

    wrap.innerHTML = `
      <div class="cx-namebar">
        <input class="cx-name" data-cx-field="name" value="${esc(r.name)}" placeholder="Request name" />
        <button class="cx-file-tag mono" data-cx-tab="raw" title="Open the raw ${esc(f.name)} text">${fileIco}${esc(f.name)}</button>
        ${S.dirty[f.id] ? '<span class="cx-dirty">unsaved</span>' : ''}
        <button class="cx-mini" data-cx="dup-req">Duplicate</button>
        <button class="cx-mini danger" data-cx="del-req">Delete</button>
      </div>
      <div class="cx-urlbar">
        <select class="cx-method" data-cx-field="method" style="color:${methodColor(r.method)}">${METHODS.map((m) => `<option ${m === r.method ? 'selected' : ''}>${m}</option>`).join('')}</select>
        <input class="cx-url mono" data-cx-field="url" value="${esc(r.url)}" placeholder="https://api.example.com/path" spellcheck="false" />
        <button class="cx-btn primary" data-cx="send" ${S.sending ? 'disabled' : ''}>${S.sending ? 'Sending…' : 'Send'}<span class="cx-kbd">⌘↵</span></button>
      </div>
      <div class="cx-tabs">${S.tab === 'raw' ? '<span class="cx-tabs-off">Form fields are hidden while you edit the file directly</span>' : tabs.map(([id, l]) => `<button class="cx-tab${S.tab === id ? ' on' : ''}" data-cx-tab="${id}">${l}</button>`).join('')}<span class="cx-grow"></span>${viewSeg}</div>
      <div class="cx-pane hsl-scroll${S.tab === 'raw' ? ' noscroll' : ''}">${pane}</div>`;
    if (S.tab === 'raw') {
      const ta = $('[data-cx-field="raw"]'), g = $('#cx-gutter');
      if (ta && g) ta.addEventListener('scroll', () => { g.scrollTop = ta.scrollTop; });
    }
  }

  /* ── response pane ─────────────────────────────────────── */
  function renderResponse() {
    const wrap = $('#cx-response');
    const res = S.response;
    if (S.sending) {
      wrap.innerHTML = `<div class="cx-blank"><div class="cx-spin"></div><div class="cx-blank-s" style="margin-top:14px">Waiting for response…</div></div>`;
      return;
    }
    if (!res) {
      wrap.innerHTML = `<div class="cx-blank"><div class="cx-blank-t">Response</div><div class="cx-blank-s">Send the request to see status, headers and body here.</div></div>`;
      return;
    }
    let body = '';
    if (S.resTab === 'headers') {
      body = `<div class="cx-hdrs">${res.headers.map(([k, v]) => `<div class="cx-hdr"><span class="cx-hk mono">${esc(k)}</span><span class="cx-hv mono">${esc(v)}</span></div>`).join('')}</div>`;
    } else {
      let text = res.body;
      if (S.resMode === 'pretty') { try { text = JSON.stringify(JSON.parse(res.body), null, 2); } catch (e) {} }
      body = `<pre class="cx-pre mono">${esc(text)}</pre>`;
    }
    wrap.innerHTML = `
      <div class="cx-res-head">
        <span class="cx-status" style="color:${statusVar(res.status)};background:color-mix(in srgb, ${statusVar(res.status)} 14%, transparent)">${res.status} ${esc(res.statusText)}</span>
        <span class="cx-meta">${res.ms} ms</span><span class="cx-meta">${fmtBytes(res.size)}</span>
        <span class="cx-src ${res.live ? 'live' : ''}">${res.live ? 'live' : 'simulated'}</span>
        <span class="cx-grow"></span>
        <button class="cx-mini" data-cx="copy-res">Copy</button>
      </div>
      <div class="cx-tabs sub">
        <button class="cx-tab${S.resTab === 'body' ? ' on' : ''}" data-cx-restab="body">Body</button>
        <button class="cx-tab${S.resTab === 'headers' ? ' on' : ''}" data-cx-restab="headers">Headers · ${res.headers.length}</button>
        <span class="cx-grow"></span>
        ${S.resTab === 'body' ? ['pretty', 'raw'].map((m) => `<button class="cx-seg${S.resMode === m ? ' on' : ''}" data-cx-resmode="${m}">${m}</button>`).join('') : ''}
      </div>
      <div class="cx-pane hsl-scroll">${body}</div>`;
  }

  function render() { renderFiles(); renderEditor(); renderResponse(); }

  /* ── sending ───────────────────────────────────────────── */
  function simulate(method, url, ms) {
    const path = (() => { try { return new URL(url).pathname; } catch (e) { return url; } })();
    const map = {
      POST: [201, 'Created'], PUT: [200, 'OK'], PATCH: [200, 'OK'], DELETE: [204, 'No Content'],
    };
    const [status, statusText] = map[method] || [200, 'OK'];
    const body = status === 204 ? '' : JSON.stringify({
      ok: true, method, path, note: 'Simulated response — the browser could not reach this host directly (CORS or private network).',
      requested_at: new Date().toISOString(),
    }, null, 2);
    return {
      status, statusText, ms, live: false, size: body.length,
      headers: [['content-type', 'application/json; charset=utf-8'], ['content-length', String(body.length)], ['x-hsl-simulated', 'true'], ['date', new Date().toUTCString()]],
      body,
    };
  }

  async function send() {
    const r = req(), f = file();
    if (!r || S.sending) return;
    S.sending = true; S.response = null; renderEditor(); renderResponse();
    const url = interpolate(r.url, f.vars);
    const headers = {};
    r.headers.filter((h) => h.on && h.key).forEach((h) => { headers[h.key] = interpolate(h.value, f.vars); });
    const body = ['GET', 'HEAD'].includes(r.method) ? undefined : interpolate(r.body, f.vars) || undefined;
    const t0 = performance.now();
    let res;
    try {
      const resp = await fetch(url, { method: r.method, headers, body, mode: 'cors' });
      const text = await resp.text();
      res = {
        status: resp.status, statusText: resp.statusText || '', ms: Math.round(performance.now() - t0), live: true,
        size: new Blob([text]).size, headers: Array.from(resp.headers.entries()), body: text,
      };
    } catch (e) {
      await new Promise((k) => setTimeout(k, 260 + Math.random() * 340));
      res = simulate(r.method, url, Math.round(performance.now() - t0));
    }
    S.sending = false; S.response = res; S.resTab = 'body';
    renderEditor(); renderResponse();
    if (window.HSL && window.HSL.pushRequest) {
      try {
        const u = new URL(url);
        window.HSL.pushRequest({
          method: r.method, scheme: u.protocol.replace(':', ''), host: u.host, path: u.pathname + u.search,
          status: res.status, mime: (res.headers.find((h) => h[0].toLowerCase() === 'content-type') || [, 'application/json'])[1].split(';')[0],
          size: res.size, ms: res.ms, origin: 'composer',
        });
      } catch (e) {}
    }
  }

  /* ── actions ───────────────────────────────────────────── */
  function markDirty() { const f = file(); if (f) S.dirty[f.id] = true; save(); }

  function selectReq(fileId, reqId) {
    S.fileId = fileId; S.reqId = reqId; S.response = null; S.rawDraft = null;
    if (S.tab === 'raw') S.tab = 'body';
    render();
  }
  function newReq(fileId) {
    const f = S.files.find((x) => x.id === fileId) || file() || S.files[0];
    if (!f) return;
    const r = { id: uid(), name: 'New request', method: 'GET', url: '', headers: [{ id: uid(), key: 'Accept', value: 'application/json', on: true }], body: '' };
    f.reqs.push(r); f.open = true; S.dirty[f.id] = true; save();
    selectReq(f.id, r.id);
  }
  function newFile() { openNewFileDialog(); }

  /* ── new-file dialog ──────────────────────────────────── */
  const NF = { name: '', tpl: 'vars' };
  const TPLS = [
    ['vars', 'API scratchpad', 'A @baseUrl variable and a first GET request'],
    ['blank', 'Empty file', 'Just the file — add requests yourself'],
    ['clone', 'Same variables as current file', 'Copies the @variables, no requests'],
  ];
  function nfRoot() {
    let el = $('#cx-modal-root');
    if (!el) { el = document.createElement('div'); el.id = 'cx-modal-root'; document.body.appendChild(el); }
    return el;
  }
  function openNewFileDialog() {
    NF.name = ''; NF.tpl = 'vars';
    renderNewFileDialog();
    setTimeout(() => { const i = $('#nf-name'); if (i) i.focus(); }, 30);
  }
  function closeNF() { nfRoot().innerHTML = ''; }
  function renderNewFileDialog() {
    const folder = storeCfg().folder;
    const name = (NF.name || 'requests') .replace(/\.http$/, '');
    nfRoot().innerHTML = `<div class="cx-ov" data-cx="nf-backdrop"><div class="cx-modal" data-cx="nf-card">
      <div class="cx-modal-head">
        <div><div class="cx-modal-t">New .http file</div>
        <div class="cx-modal-s">A plain text file holding one or more requests — editable here or in your editor.</div></div>
        <button class="cx-x" data-cx="nf-cancel">×</button></div>
      <div class="cx-modal-body">
        <div class="cx-f"><label class="cx-f-l">File name</label>
          <div class="cx-name-row"><input id="nf-name" class="cx-in mono" data-cx-nf="name" value="${esc(NF.name)}" placeholder="requests" /><span class="cx-ext mono">.http</span></div></div>
        <div class="cx-f"><label class="cx-f-l">Starting point</label>
          <div class="cx-tpls">${TPLS.map(([id, t, s]) => `<button class="cx-tpl${NF.tpl === id ? ' on' : ''}" data-cx-nf-tpl="${id}">
            <span class="cx-radio"></span><span><span class="cx-tpl-t">${t}</span><span class="cx-tpl-s">${s}</span></span></button>`).join('')}</div></div>
        <div class="cx-dest">
          <span class="cx-hint">Saved to</span>
          <span class="mono cx-dest-p">${esc(folder)}/${esc(name)}.http</span>
          <button class="cx-mini" data-cx="open-store-settings">Change folder…</button></div>
      </div>
      <div class="cx-modal-foot">
        <button class="cx-btn" data-cx="nf-cancel">Cancel</button>
        <button class="cx-btn primary" data-cx="nf-create">Create file</button></div></div></div>`;
  }
  function createFromDialog() {
    const base = (NF.name || 'requests').replace(/\.http$/, '').trim() || 'requests';
    const f = { id: uid(), name: base + '.http', vars: [], reqs: [], open: true };
    if (NF.tpl === 'vars') {
      f.vars = [{ id: uid(), key: 'baseUrl', value: 'https://api.example.com', on: true }];
      f.reqs = [{ id: uid(), name: 'Health check', method: 'GET', url: '{{baseUrl}}/health', headers: [{ id: uid(), key: 'Accept', value: 'application/json', on: true }], body: '' }];
    } else if (NF.tpl === 'clone') {
      const cur = file();
      if (cur) f.vars = cur.vars.map((v) => ({ ...v, id: uid() }));
    }
    S.files.push(f); S.dirty[f.id] = true; save(); closeNF();
    if (f.reqs.length) selectReq(f.id, f.reqs[0].id);
    else { S.fileId = f.id; S.reqId = null; render(); }
  }
  function exportFile() {
    const f = file() || S.files[0];
    if (!f) return;
    const blob = new Blob([toHttp(f)], { type: 'text/plain' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob); a.download = f.name; a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 2000);
    S.dirty[f.id] = false; renderEditor();
  }
  function importFiles(fileList) {
    const files = Array.from(fileList || []);
    let pending = files.length;
    files.forEach((file0) => {
      const rd = new FileReader();
      rd.onload = () => {
        const parsed = parseHttp(rd.result, file0.name);
        S.files.push(parsed);
        if (--pending === 0) { save(); S.fileId = parsed.id; S.reqId = parsed.reqs[0] && parsed.reqs[0].id; render(); }
      };
      rd.readAsText(file0);
    });
  }

  function fromCaptured(c) {
    let f = S.files.find((x) => x.name === 'captured.http');
    if (!f) { f = { id: uid(), name: 'captured.http', vars: [], reqs: [], open: true }; S.files.push(f); }
    const r = {
      id: uid(), name: `${c.method} ${c.path.split('?')[0].split('/').filter(Boolean).pop() || c.host}`,
      method: c.method, url: `${c.scheme}://${c.host}${c.path}`,
      headers: (c.headers || [['Accept', 'application/json']]).map(([k, v]) => ({ id: uid(), key: k, value: v, on: true })),
      body: c.body || '',
    };
    f.reqs.push(r); S.dirty[f.id] = true; save();
    setMode('compose');
    selectReq(f.id, r.id);
  }

  /* ── mode switch ───────────────────────────────────────── */
  function applyRaw(text) {
    const f = file(); if (!f) return;
    S.rawDraft = text;
    const parsed = parseHttp(text, f.name);
    const prevIdx = f.reqs.findIndex((x) => x.id === S.reqId);
    f.vars = parsed.vars; f.reqs = parsed.reqs;
    const keep = f.reqs[prevIdx >= 0 ? Math.min(prevIdx, f.reqs.length - 1) : 0];
    S.reqId = keep ? keep.id : null;
    markDirty(); renderFiles();
    const g = $('#cx-gutter'); if (g) g.innerHTML = text.split('\n').map((_, i) => i + 1).join('<br>');
    const st = $('#cx-raw-stat'); if (st) st.textContent = `${f.reqs.length} request${f.reqs.length === 1 ? '' : 's'}`;
  }

  function wireResize() {
    const drag = (grip, target, key, def, min, max, invert) => {
      if (!grip || !target) return;
      const stored = Number(localStorage.getItem(key));
      if (stored >= min) target.style.width = stored + 'px';
      let startX = 0, startW = 0;
      const move = (e) => {
        const d = invert ? startX - e.clientX : e.clientX - startX;
        target.style.width = Math.max(min, Math.min(typeof max === 'function' ? max() : max, startW + d)) + 'px';
      };
      const up = () => {
        document.removeEventListener('mousemove', move); document.removeEventListener('mouseup', up);
        document.body.style.cursor = ''; grip.classList.remove('on');
        localStorage.setItem(key, String(parseInt(target.style.width, 10) || def));
      };
      grip.addEventListener('mousedown', (e) => {
        e.preventDefault(); startX = e.clientX; startW = target.offsetWidth;
        grip.classList.add('on'); document.body.style.cursor = 'col-resize';
        document.addEventListener('mousemove', move); document.addEventListener('mouseup', up);
      });
      grip.addEventListener('dblclick', () => { target.style.width = def + 'px'; localStorage.setItem(key, String(def)); });
    };
    drag($('#cx-grip'), $('.cx-side'), 'hsl-side-w', 250, 190, 560, false);
    const main = $('.cx-main');
    drag($('#cx-grip2'), $('#cx-response'), 'hsl-res-w', 520, 280, () => Math.max(320, main.offsetWidth - 380), true);
  }

  function setMode(m) {
    $('#capture-view').style.display = m === 'capture' ? 'flex' : 'none';
    $('#capture-toolbar').style.display = m === 'capture' ? 'flex' : 'none';
    $('#compose-view').style.display = m === 'compose' ? 'flex' : 'none';
    $$('[data-mode]').forEach((b) => b.classList.toggle('on', b.dataset.mode === m));
    if (m === 'compose' && !S.reqId) {
      const f = S.files[0];
      if (f) { S.fileId = f.id; S.reqId = f.reqs[0] && f.reqs[0].id; }
      render();
    }
  }

  /* ── wiring ────────────────────────────────────────────── */
  function currentParams() { return splitUrl(req().url).params; }
  function listFor(kind) {
    if (kind === 'headers') return req().headers;
    if (kind === 'vars') return file().vars;
    return currentParams();
  }
  function writeParams(params) { const r = req(); r.url = joinUrl(splitUrl(r.url).base, params); }

  function wire() {
    document.addEventListener('click', (e) => {
      const mode = e.target.closest('[data-mode]');
      if (mode) { setMode(mode.dataset.mode); return; }
      const rq = e.target.closest('[data-cx-req]');
      if (rq) { selectReq(rq.dataset.cxFile, rq.dataset.cxReq); return; }
      const tb = e.target.closest('[data-cx-tab]');
      if (tb) { if (tb.dataset.cxTab !== 'raw') S.lastFormTab = tb.dataset.cxTab; S.tab = tb.dataset.cxTab; if (S.tab !== 'raw') S.rawDraft = null; renderEditor(); return; }
      const rt = e.target.closest('[data-cx-restab]');
      if (rt) { S.resTab = rt.dataset.cxRestab; renderResponse(); return; }
      const rm = e.target.closest('[data-cx-resmode]');
      if (rm) { S.resMode = rm.dataset.cxResmode; renderResponse(); return; }
      const tp = e.target.closest('[data-cx-nf-tpl]');
      if (tp) { NF.tpl = tp.dataset.cxNfTpl; renderNewFileDialog(); return; }
      const a = e.target.closest('[data-cx]');
      if (!a) return;
      const kind = a.dataset.kind, i = Number(a.dataset.i);
      switch (a.dataset.cx) {
        case 'toggle-file': {
          const f = S.files.find((x) => x.id === a.dataset.cxFile);
          if (f) { f.open = !f.open; S.fileId = f.id; }
          renderFiles(); break;
        }
        case 'new-req': newReq(a.dataset.cxFile); break;
        case 'new-file': newFile(); break;
        case 'nf-cancel': closeNF(); break;
        case 'nf-create': createFromDialog(); break;
        case 'nf-backdrop': if (e.target.closest('[data-cx="nf-card"]') === null) closeNF(); break;
        case 'open-store-settings': if (window.HSL && window.HSL.openFileSettings) window.HSL.openFileSettings(); break;
        case 'import': $('#cx-import').click(); break;
        case 'export': exportFile(); break;
        case 'send': send(); break;
        case 'del-req': {
          const f = file(); if (!f) break;
          f.reqs = f.reqs.filter((x) => x.id !== S.reqId);
          S.reqId = f.reqs[0] && f.reqs[0].id; S.dirty[f.id] = true; save(); render(); break;
        }
        case 'dup-req': {
          const f = file(), r = req(); if (!r) break;
          const c = JSON.parse(JSON.stringify(r)); c.id = uid(); c.name = r.name + ' copy';
          f.reqs.splice(f.reqs.indexOf(r) + 1, 0, c); S.dirty[f.id] = true; save(); selectReq(f.id, c.id); break;
        }
        case 'del-file': {
          const f = file(); if (!f || !confirm(`Remove ${f.name} from the workspace?`)) break;
          S.files = S.files.filter((x) => x !== f); S.fileId = S.files[0] && S.files[0].id;
          S.reqId = S.files[0] && S.files[0].reqs[0] && S.files[0].reqs[0].id; save(); render(); break;
        }
        case 'kv-add': {
          listFor(kind).push({ id: uid(), key: '', value: '', on: true });
          if (kind === 'params') writeParams(currentParams());
          markDirty(); renderEditor(); break;
        }
        case 'kv-del': {
          if (kind === 'params') { const p = currentParams(); p.splice(i, 1); writeParams(p); }
          else listFor(kind).splice(i, 1);
          markDirty(); renderEditor(); break;
        }
        case 'kv-toggle': {
          if (kind === 'params') { const p = currentParams(); p[i].on = !p[i].on; writeParams(p); }
          else { const l = listFor(kind); l[i].on = !l[i].on; }
          markDirty(); renderEditor(); break;
        }
        case 'format-json': {
          const r = req();
          try { r.body = JSON.stringify(JSON.parse(r.body), null, 2); markDirty(); renderEditor(); } catch (err) {}
          break;
        }
        case 'copy-res': if (S.response) navigator.clipboard && navigator.clipboard.writeText(S.response.body); break;
        default: break;
      }
    });

    document.addEventListener('input', (e) => {
      const nf = e.target.closest('[data-cx-nf]');
      if (nf) { NF.name = nf.value; const d = $('.cx-dest-p'); if (d) d.textContent = `${storeCfg().folder}/${(NF.name || 'requests').replace(/\.http$/, '') || 'requests'}.http`; return; }
      const el = e.target.closest('[data-cx-field]');
      if (!el) return;
      const r = req(); if (!r) return;
      const f = el.dataset.cxField;
      if (f === 'name') { r.name = el.value; renderFiles(); }
      else if (f === 'raw') { applyRaw(el.value); return; }
      else if (f === 'url') r.url = el.value;
      else if (f === 'body') r.body = el.value;
      else if (f === 'kv') {
        const kind = el.dataset.kind, i = Number(el.dataset.i), key = el.dataset.key;
        if (kind === 'params') { const p = currentParams(); p[i][key] = el.value; writeParams(p); const u = $('.cx-url'); if (u) u.value = r.url; }
        else listFor(kind)[i][key] = el.value;
      }
      markDirty();
    });

    document.addEventListener('change', (e) => {
      const el = e.target.closest('[data-cx-field="method"]');
      if (el) { const r = req(); r.method = el.value; markDirty(); renderEditor(); renderFiles(); return; }
      const imp = e.target.closest('#cx-import');
      if (imp) { importFiles(imp.files); imp.value = ''; }
    });

    document.addEventListener('keydown', (e) => {
      const rawTa = e.target.closest('[data-cx-field="raw"]');
      if (rawTa && e.key === 'Tab') {
        e.preventDefault();
        const s = rawTa.selectionStart, en = rawTa.selectionEnd;
        rawTa.value = rawTa.value.slice(0, s) + '  ' + rawTa.value.slice(en);
        rawTa.selectionStart = rawTa.selectionEnd = s + 2; applyRaw(rawTa.value); return;
      }
      if (e.key === 'Escape' && $('#cx-modal-root') && $('#cx-modal-root').innerHTML) { closeNF(); return; }
      if (e.key === 'Enter' && $('#nf-name')) { e.preventDefault(); createFromDialog(); return; }
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter' && $('#compose-view').style.display !== 'none') { e.preventDefault(); send(); }
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's' && $('#compose-view').style.display !== 'none') { e.preventDefault(); exportFile(); }
    });

    const view = $('#compose-view');
    view.addEventListener('dragover', (e) => { e.preventDefault(); view.classList.add('drop'); });
    view.addEventListener('dragleave', () => view.classList.remove('drop'));
    view.addEventListener('drop', (e) => {
      e.preventDefault(); view.classList.remove('drop');
      if (e.dataTransfer.files.length) importFiles(e.dataTransfer.files);
    });
  }

  function boot() {
    load();
    const f = S.files[0];
    if (f) { S.fileId = f.id; S.reqId = f.reqs[0] && f.reqs[0].id; }
    wire(); wireResize(); render();
    document.addEventListener('hsl-store-change', () => { renderFiles(); if ($('#cx-modal-root') && $('#cx-modal-root').innerHTML) renderNewFileDialog(); });
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
  else boot();

  window.HSLCompose = { fromCaptured, setMode, parseHttp, toHttp };
})();
