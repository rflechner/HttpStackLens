/* HttpStackLens — pure-vanilla engine. No framework.
   Renders into the static shell in the HTML file and drives every
   interaction with plain DOM + event delegation. This mirrors how a
   Go/WASM build would push markup into the DOM. */
(function () {
  'use strict';

  // ─── palettes ────────────────────────────────────────────
  // Every colour family has a light and a dark variant. `mint` is the UI's
  // semantic accent slot; its actual hue depends on the selected theme.
  const LIGHT_BASE = {
    desk: '#e9e5de', sidebar: '#faf8f4', bg0: '#f1eee8', bg1: '#ffffff', bg2: '#f6f3ee', bg3: '#eae5db',
    line: '#e5e0d6', lineSoft: '#efebe3', ink: '#2b2926', dim: '#6f6b62', faint: '#a49f94',
    warn: '#c98a2b', danger: '#d1584f', info: '#4a7fc4', pink: '#9a6cc9', white: '#ffffff', onAccent: '#ffffff',
    success: '#3f9d6b', scroll: '#d8d2c6', scrollHover: '#c7c0b2',
  };
  const DARK_BASE = {
    desk: '#0a0b0d', sidebar: '#15171c', bg0: '#0e0f12', bg1: '#15171c', bg2: '#1c1f26', bg3: '#23272f',
    line: '#262a33', lineSoft: '#1f232a', ink: '#d8dce4', dim: '#8a93a3', faint: '#5a6173',
    warn: '#f1b45a', danger: '#e86a6a', info: '#7aa7ff', pink: '#d48ad6', white: '#ffffff', onAccent: '#0c1210',
    success: '#7fd4b4', scroll: '#23272f', scrollHover: '#2f343d',
  };
  const palette = (base, accent, overrides) => Object.assign({}, base, { mint: accent }, overrides || {});
  const PALETTES = {
    light: palette(LIGHT_BASE, '#2f9e8f', { onAccent: '#0c2a24' }),
    dark: palette(DARK_BASE, '#7fd4b4', { onAccent: '#0c2a24' }),
    'rose-light': palette(LIGHT_BASE, '#bd3f83', { desk: '#f0e6eb', sidebar: '#fff8fb', bg0: '#f8eff3', bg2: '#fdf5f8', bg3: '#f1dfe8', line: '#ead3de', lineSoft: '#f3e4eb', info: '#7766c5', pink: '#bd3f83' }),
    'rose-dark': palette(DARK_BASE, '#f08fbd', { desk: '#100a0e', sidebar: '#1b1218', bg0: '#120c10', bg1: '#1b1218', bg2: '#251820', bg3: '#30202a', line: '#392530', lineSoft: '#2c1d25', pink: '#f08fbd' }),
    'red-light': palette(LIGHT_BASE, '#b43b3b', { desk: '#eee3e1', sidebar: '#fff9f8', bg0: '#f7eeec', bg2: '#fcf5f3', bg3: '#eeddda', line: '#e3cfcb', lineSoft: '#f0e2df', danger: '#a51f2d', warn: '#9a6500' }),
    'red-dark': palette(DARK_BASE, '#ff7373', { desk: '#100909', sidebar: '#1b1111', bg0: '#120b0b', bg1: '#1b1111', bg2: '#251818', bg3: '#302020', line: '#3d2929', lineSoft: '#2d1e1e', danger: '#ff5c68', warn: '#ffc266', onAccent: '#2a0505' }),
    'yellow-light': palette(LIGHT_BASE, '#a96d00', { desk: '#eee9d8', sidebar: '#fffdf4', bg0: '#f7f3e5', bg2: '#fcf8ea', bg3: '#eee5c6', line: '#e4d9b7', lineSoft: '#f0e8ce', warn: '#a96d00', onAccent: '#ffffff' }),
    'yellow-dark': palette(DARK_BASE, '#ffd166', { desk: '#0e0d08', sidebar: '#19170f', bg0: '#111008', bg1: '#19170f', bg2: '#222017', bg3: '#2d291b', line: '#383321', lineSoft: '#292619', warn: '#ffd166', onAccent: '#2c2100' }),
    'green-light': palette(LIGHT_BASE, '#16855b', { desk: '#e2ebe5', sidebar: '#f7fcf8', bg0: '#edf5ef', bg2: '#f4faf6', bg3: '#dcecdf', line: '#cfe1d3', lineSoft: '#e3eee5', success: '#16855b' }),
    'green-dark': palette(DARK_BASE, '#63e6a6', { desk: '#070e0a', sidebar: '#101a14', bg0: '#09110c', bg1: '#101a14', bg2: '#17241c', bg3: '#1e3025', line: '#263b2e', lineSoft: '#1c2c22', success: '#63e6a6' }),
    'blue-light': palette(LIGHT_BASE, '#2869c7', { desk: '#e1e8f0', sidebar: '#f7faff', bg0: '#edf2f8', bg2: '#f4f7fc', bg3: '#dce6f3', line: '#cfdae8', lineSoft: '#e2e9f2', info: '#2869c7' }),
    'blue-dark': palette(DARK_BASE, '#65a7ff', { desk: '#080c12', sidebar: '#111823', bg0: '#0a1018', bg1: '#111823', bg2: '#182231', bg3: '#202d40', line: '#29394f', lineSoft: '#1d2939', info: '#65a7ff' }),
    'contrast-light': palette(LIGHT_BASE, '#005fcc', { desk: '#ffffff', sidebar: '#ffffff', bg0: '#ffffff', bg2: '#f2f2f2', bg3: '#d9e8ff', line: '#292929', lineSoft: '#b5b5b5', ink: '#000000', dim: '#292929', faint: '#555555', danger: '#b00020', warn: '#7a4b00', info: '#005fcc', pink: '#7b1fa2', success: '#006b35', scroll: '#555555', scrollHover: '#111111' }),
    'contrast-dark': palette(DARK_BASE, '#00ffff', { desk: '#000000', sidebar: '#000000', bg0: '#000000', bg1: '#000000', bg2: '#111111', bg3: '#202020', line: '#ffffff', lineSoft: '#666666', ink: '#ffffff', dim: '#eeeeee', faint: '#bdbdbd', danger: '#ff5c5c', warn: '#ffe066', info: '#66b3ff', pink: '#ff8cff', success: '#66ff99', scroll: '#aaaaaa', scrollHover: '#ffffff', onAccent: '#000000' }),
  };
  function methodColors(colors) {
    return { GET: colors.mint, POST: colors.warn, PUT: colors.info, DELETE: colors.danger, PATCH: colors.pink, OPTIONS: colors.dim, HEAD: colors.dim, CONNECT: colors.faint };
  }
  let themeName = (function () {
    try { const saved = localStorage.getItem('hsl-theme'); return PALETTES[saved] ? saved : 'light'; }
    catch (e) { return 'light'; }
  })();
  let C = PALETTES[themeName];
  let METHOD_COLOR = methodColors(C);
  let savedDetailHeight = null;
  let savedDensity = 'normal';
  try {
    const stored = Number(localStorage.getItem('hsl-detail-height'));
    if (Number.isFinite(stored) && stored > 0) savedDetailHeight = stored;
    const density = localStorage.getItem('hsl-density');
    if (density === 'compact' || density === 'normal' || density === 'comfy') savedDensity = density;
  } catch (e) {}
  function statusColor(s) {
    if (s >= 500) return C.danger;
    if (s >= 400) return C.warn;
    if (s >= 300) return C.info;
    if (s >= 200) return C.mint;
    return C.dim;
  }

  // ─── content-type → colour category ─────────────────────
  // Maps a response Content-Type onto the palette bucket used for the "Type"
  // column and the body pane. Kept in the JS layer with the row templates.
  function mimeCategory(mime, stream) {
    if (stream) return 'stream';
    const m = (mime || '').toLowerCase().split(';')[0].trim();
    if (!m) return 'bin';
    if (m === 'text/event-stream') return 'stream';
    if (m === 'application/json' || m.endsWith('+json')) return 'json';
    if (m === 'text/html' || m === 'application/xhtml+xml') return 'html';
    if (m === 'text/css') return 'css';
    if (m.includes('javascript') || m.endsWith('+javascript') || m === 'application/ecmascript') return 'js';
    if (m.startsWith('image/')) return 'img';
    if (m.startsWith('font/') || m.includes('woff') || m === 'application/font-sfnt') return 'font';
    return 'bin';
  }

  // ─── format helpers ──────────────────────────────────────
  function fmtBytes(b) {
    if (b == null) return '—';
    if (b < 1024) return b + ' B';
    if (b < 1048576) return (b / 1024).toFixed(1) + ' KB';
    return (b / 1048576).toFixed(2) + ' MB';
  }
  function fmtMs(ms) { if (ms == null) return '—'; return ms >= 1000 ? (ms / 1000).toFixed(2) + 's' : ms + 'ms'; }
  function fmtTime(ts) {
    if (ts == null) return '—';
    const d = new Date(ts), p = (n) => String(n).padStart(2, '0');
    return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}.${String(d.getMilliseconds()).padStart(3, '0')}`;
  }
  const esc = (s) => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  // ─── state ───────────────────────────────────────────────
  const state = {
    rows: [], selId: null, capturing: true, proxyRunning: true, proxyAddress: '', decryption: true,
    liveRows: [],
    source: { kind: 'live', name: '', metadata: null },
    captures: { files: [], loading: false, loaded: false, opening: false, error: null },
    runtime: { memoryBytes: null, loading: false },
    build: { version: null, commit: null, commitUrl: null },
    update: { available: false, latest: null, url: null },
    filter: '', sidebar: 'all', detailTab: 'overview', bodyMode: 'pretty',
    // Request/response split panes: null shows both, otherwise the named side is
    // expanded and the other collapses to a clickable rail.
    detailFocus: null,
    density: savedDensity,
    detailHeight: savedDetailHeight,
    upstream: { on: false, ntlm: false, host: '', noProxy: [], loaded: false, loading: false, saving: false, dirty: false, saved: false, error: null },
    access: { mode: 'loopback', networks: [], appliedMode: 'loopback', appliedNetworks: [], loaded: false, loading: false, saving: false, dirty: false, saved: false, error: null },
    certificate: { loaded: false, loading: false, busy: false, action: '', status: null, error: null, cleanup: null },
    // Real body-capture settings (B5.1), fed by WASM from /api/settings/body-capture.
    bodyCapture: { loaded: false, loading: false, defaultMaxBytes: null, mimeTypes: [], error: null, saving: false, saved: false, dirty: false },
  };

  // ─── tiny DOM helpers ────────────────────────────────────
  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

  function methodTag(m, sm) {
    const c = METHOD_COLOR[m] || C.dim;
    const pad = sm ? '2px 6px' : '3px 8px', fs = sm ? '10.5px' : '11.5px', mw = sm ? '44px' : '50px';
    return `<span class="inline-block text-center rounded-[6px]" style="color:${c};background:${c}1c;padding:${pad};font-size:${fs};min-width:${mw};letter-spacing:.2px;font-family:Inter;font-weight:600">${m}</span>`;
  }
  function statusPill(s, sm) {
    const c = statusColor(s);
    const pad = sm ? '2px 7px' : '3px 9px', fs = sm ? '10.5px' : '11.5px';
    return `<span class="inline-flex items-center gap-[5px] rounded-[6px]" style="color:${c};background:${c}18;padding:${pad};font-size:${fs};font-family:Inter;font-weight:600">
      <span style="width:5px;height:5px;border-radius:3px;background:${c}"></span>${s || '—'}</span>`;
  }
  function lock(on, warn) {
    const c = warn ? C.warn : on ? C.mint : C.faint;
    return `<svg width="10" height="12" viewBox="0 0 10 12" fill="none" style="display:block">
      <rect x="1" y="5" width="8" height="6" rx="1" stroke="${c}" stroke-width="1"/>
      <path d="${on ? 'M3 5V3.5a2 2 0 014 0V5' : 'M3 5V3.5a2 2 0 013-1.7'}" stroke="${c}" stroke-width="1" stroke-linecap="round"/></svg>`;
  }

  // ─── live / saved capture sources ───────────────────────
  function captureDate(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return 'unknown date';
    const today = new Date();
    const sameDay = date.toDateString() === today.toDateString();
    return new Intl.DateTimeFormat(undefined, sameDay
      ? { hour: '2-digit', minute: '2-digit' }
      : { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' }).format(date);
  }

  function renderCaptureSessions() {
    const live = $('#live-session');
    if (!live) return;
    const isLive = state.source.kind === 'live';
    live.style.background = isLive ? C.bg3 : 'transparent';
    live.style.color = isLive ? C.ink : C.dim;
    const dot = $('.rec-dot', live);
    if (dot) dot.classList.toggle('on', state.capturing);
    $('#live-session-count').textContent = state.liveRows.length;

    const root = $('#capture-sessions');
    const captures = state.captures;
    if (captures.loading && !captures.loaded) {
      root.innerHTML = `<div style="padding:8px 10px;color:${C.faint};font-size:11.5px">Loading captures…</div>`;
      return;
    }
    const error = captures.error
      ? `<div style="padding:8px 10px;color:${C.danger};font-size:11px;line-height:1.4">${esc(captures.error)}</div>`
      : '';
    if (!captures.files.length) {
      root.innerHTML = error || `<div style="padding:8px 10px;color:${C.faint};font-size:11.5px">No saved captures</div>`;
      return;
    }
    root.innerHTML = error + captures.files.map((file) => {
      const active = state.source.kind === 'capture' && state.source.name === file.name;
      const opening = captures.opening === file.name;
      return `<button data-open-capture="${encodeURIComponent(file.name)}" class="flex items-center gap-[8px] w-full text-left rounded-[8px]" style="padding:7px 10px;border:none;cursor:pointer;background:${active ? C.bg3 : 'transparent'};color:${active ? C.ink : C.dim};font-family:Inter">
        <span style="color:${active ? C.mint : C.faint};font-size:12px">${opening ? '◌' : '▣'}</span>
        <span class="flex-1 min-w-0"><span class="block truncate" style="font-size:11.5px;font-weight:500">${esc(file.name.replace(/^capture-/, '').replace(/\.capture$/, ''))}</span><span class="block" style="font-size:10.5px;color:${C.faint};margin-top:2px">${captureDate(file.modified_at)} · ${fmtBytes(file.size)}</span></span>
      </button>`;
    }).join('');
  }

  function renderSourceBanner() {
    const banner = $('#source-banner');
    const waterfall = $('#waterfall');
    const subtitle = $('#source-subtitle');
    if (state.source.kind === 'live') {
      if (subtitle) subtitle.textContent = 'Live session';
      banner.style.display = 'none';
      waterfall.style.display = 'flex';
      return;
    }
    const metadata = state.source.metadata || {};
    if (subtitle) subtitle.textContent = 'Saved capture';
    banner.style.display = 'flex';
    waterfall.style.display = 'none';
    banner.style.cssText = `display:flex;align-items:center;gap:12px;min-height:52px;padding:8px 16px;background:${C.bg2};border-bottom:1px solid ${C.line}`;
    banner.innerHTML = `<span style="width:8px;height:8px;border-radius:4px;background:${C.info}"></span>
      <div class="flex-1 min-w-0"><div class="truncate" style="font-size:12.5px;font-weight:600;color:${C.ink}">${esc(state.source.name)}</div>
        <div style="font-size:11px;color:${C.dim};margin-top:2px">Read-only capture · ${state.rows.length} requests · ${fmtBytes(metadata.size)} · ${metadata.https_decrypted ? 'HTTPS decrypted' : 'HTTPS passthrough'} · timestamps unavailable</div></div>
      <button data-action="back-to-live" style="display:inline-flex;align-items:center;gap:7px;height:34px;padding:0 14px;border-radius:8px;border:1px solid ${C.mint};background:${C.mint};color:${C.onAccent};font-family:Inter;font-size:12.5px;font-weight:700;cursor:pointer;white-space:nowrap;box-shadow:0 3px 10px ${C.mint}35">
        <span aria-hidden="true" style="font-size:14px;line-height:1">●</span> Back to live
      </button>`;
  }

  async function fetchJSON(url) {
    const response = await fetch(url, { headers: { Accept: 'application/json' } });
    if (!response.ok) {
      const message = (await response.text()).trim();
      throw new Error(message || `HTTP ${response.status}`);
    }
    return response.json();
  }

  async function loadCaptureFiles() {
    if (state.captures.loading) return;
    state.captures.loading = true; state.captures.error = null;
    renderCaptureSessions();
    try {
      const files = await fetchJSON('/api/captures');
      state.captures.files = Array.isArray(files) ? files : [];
      state.captures.loaded = true;
    } catch (error) {
      state.captures.error = `Could not list captures: ${error.message}`;
    } finally {
      state.captures.loading = false;
      renderCaptureSessions();
    }
  }

  async function loadRuntimeStats() {
    if (state.runtime.loading) return;
    state.runtime.loading = true;
    try {
      const stats = await fetchJSON('/api/runtime/stats');
      state.runtime.memoryBytes = typeof stats.memory_bytes === 'number' ? stats.memory_bytes : null;
      renderProxySummary();
    } catch (error) {
      // Keep the last successful value: a temporary UI/API interruption should
      // not make the footer flicker between a value and an error every 5s.
    } finally {
      state.runtime.loading = false;
    }
  }

  // Fetched once at boot: the running build never changes at runtime.
  async function loadBuildInfo() {
    try {
      const info = await fetchJSON('/api/version');
      state.build.version = info.version || null;
      state.build.commit = info.commit || null;
      state.build.commitUrl = info.commit_url || null;
      renderStatusBar();
    } catch (error) {
      // Non-critical: leave the version slot empty if it can't be fetched.
    }
  }

  // Checked once at boot. The backend caches the GitHub result, so this stays
  // well within the API rate limit even across reloads. Prefers the direct asset
  // download for this platform, falling back to the release page.
  async function loadUpdateCheck() {
    try {
      const info = await fetchJSON('/api/update-check');
      if (!info.checked || !info.update_available) return;
      state.update.available = true;
      state.update.latest = info.latest_version || null;
      state.update.url = info.asset_url || info.release_url || null;
      renderStatusBar();
    } catch (error) {
      // Non-critical: no badge if the check can't run.
    }
  }

  function captureHeaders(headers) {
    return Array.isArray(headers) ? headers.map((header) => [header.name || '', header.value || '']) : [];
  }

  function captureHeader(headers, name) {
    const match = (headers || []).find((header) => String(header.name || '').toLowerCase() === name.toLowerCase());
    return match ? match.value : '';
  }

  function capturedBody(record, contentType) {
    return {
      loaded: true,
      loading: false,
      available: !!record.body_available,
      skipped: !!record.body_skipped,
      contentType: contentType || '',
      bodyBase64: record.body_base64 || '',
    };
  }

  function captureRequestLocation(request) {
    const raw = request.url || '/';
    const hostHeader = captureHeader(request.headers, 'host');
    if (request.method === 'CONNECT') return { scheme: 'https', host: raw, path: raw };
    try {
      const parsed = new URL(raw);
      return { scheme: parsed.protocol.replace(':', ''), host: parsed.host, path: parsed.pathname + parsed.search };
    } catch (error) {
      return { scheme: 'http', host: hostHeader, path: raw.startsWith('/') ? raw : '/' + raw };
    }
  }

  function captureRows(records, metadata) {
    const byRequestID = new Map();
    const rows = [];
    for (const item of records) {
      if (item.type === 'request' && item.request) {
        const request = item.request;
        const location = captureRequestLocation(request);
        const requestContentType = captureHeader(request.headers, 'content-type');
        const row = {
          id: rows.length + 1, ts: null, method: request.method || 'GET',
          scheme: location.scheme, host: location.host, path: location.path,
          version: request.http_version || '', status: null, statusText: '', mime: '', mimeColor: 'bin',
          size: null, ms: null, stream: false, bodyAvailable: false, bodySkipped: false,
          tls: location.scheme === 'https' || request.method === 'CONNECT', decrypted: !!metadata.https_decrypted,
          correlationId: request.request_id || '',
          detail: { request: { httpVersion: request.http_version || '', headers: captureHeaders(request.headers) }, response: null },
          bodies: { request: capturedBody(request, requestContentType), response: { loaded: true, loading: false, available: false } },
        };
        rows.push(row);
        byRequestID.set(request.request_id, row);
      } else if (item.type === 'response' && item.response) {
        const response = item.response;
        const row = byRequestID.get(response.request_id);
        if (!row) continue;
        const contentType = captureHeader(response.headers, 'content-type');
        row.status = Number(response.status || 0) || null;
        row.statusText = response.status_text || '';
        row.mime = contentType;
        row.mimeColor = mimeCategory(contentType, contentType.toLowerCase().startsWith('text/event-stream'));
        row.stream = row.mimeColor === 'stream';
        row.size = Number(response.body_size || 0);
        row.bodyAvailable = !!response.body_available;
        row.bodySkipped = !!response.body_skipped;
        row.detail.response = { httpVersion: response.http_version || '', headers: captureHeaders(response.headers) };
        row.bodies.response = capturedBody(response, contentType);
      }
    }
    return rows;
  }

  async function openCapture(name) {
    const token = Symbol(name);
    state.captures.openToken = token;
    state.captures.opening = name; state.captures.error = null;
    renderCaptureSessions();
    try {
      const encoded = encodeURIComponent(name);
      const metadata = await fetchJSON(`/api/captures/${encoded}/metadata`);
      const records = [];
      let offset = 0;
      do {
        const page = await fetchJSON(`/api/captures/${encoded}/records?offset=${offset}&limit=500`);
        records.push(...(page.records || []));
        if (!page.has_more) break;
        if (page.next_offset <= offset) throw new Error('Capture pagination did not advance.');
        offset = page.next_offset;
      } while (state.captures.openToken === token);
      if (state.captures.openToken !== token) return;
      state.source = { kind: 'capture', name, metadata };
      state.rows = captureRows(records, metadata);
      state.selId = null; state.sidebar = 'all';
      renderToolbar(); renderList(); renderDetail(); renderSourceBanner();
    } catch (error) {
      if (state.captures.openToken === token) state.captures.error = `Could not open ${name}: ${error.message}`;
    } finally {
      if (state.captures.openToken === token) {
        state.captures.opening = false;
        renderCaptureSessions();
      }
    }
  }

  function backToLive() {
    state.captures.openToken = null;
    state.source = { kind: 'live', name: '', metadata: null };
    state.rows = state.liveRows;
    state.selId = null;
    renderToolbar(); renderList(); renderDetail(); renderSourceBanner();
  }

  // ─── request list ────────────────────────────────────────
  const GRID = 'grid-template-columns:44px 60px 22px 66px 190px 1fr 140px 74px 84px 58px';
  function rowHeight() { return state.density === 'compact' ? 28 : state.density === 'comfy' ? 42 : 34; }

  function renderDensityButtons() {
    $$('[data-density]').forEach((button) => {
      const active = button.dataset.density === state.density;
      button.style.background = active ? C.bg1 : 'transparent';
      button.style.color = active ? C.mint : C.dim;
      button.style.border = active ? `1px solid ${C.line}` : 'none';
      button.style.fontWeight = active ? '600' : '500';
      button.style.boxShadow = active ? '0 1px 2px rgba(0,0,0,.04)' : 'none';
    });
  }

  function filteredRows() {
    let rows = state.rows;
    const sb = state.sidebar;
    if (sb !== 'all') {
      rows = rows.filter((r) => {
        if (sb === '2xx') return r.status != null && r.status < 300;
        if (sb === '4xx') return r.status >= 400 && r.status < 500;
        if (sb === '5xx') return r.status >= 500;
        if (sb === 'tls') return r.tls;
        return true;
      });
    }
    const q = state.filter.trim().toLowerCase();
    if (q) rows = rows.filter((r) =>
      r.host.toLowerCase().includes(q) || r.path.toLowerCase().includes(q) ||
      (r.status != null && String(r.status).includes(q)) || r.method.toLowerCase().includes(q));
    return rows;
  }

  function rowHTML(r) {
    const sel = r.id === state.selId;
    const stream = r.mimeColor === 'stream' || r.stream;
    return `<div data-row="${r.id}" class="grid items-center gap-[12px] cursor-pointer select-none" style="${GRID};height:${rowHeight()}px;padding:0 16px;background:${sel ? C.bg3 : 'transparent'};border-left:3px solid ${sel ? statusColor(r.status) : 'transparent'};border-bottom:1px solid ${C.lineSoft};font-family:Inter;font-size:12.5px;color:${C.ink}">
      <span style="color:${C.faint};text-align:right;font-variant-numeric:tabular-nums">${String(r.id).padStart(3, '0')}</span>
      ${methodTag(r.method, true)}
      <span title="${r.tls ? (r.decrypted ? 'decrypted' : 'encrypted') : 'cleartext'}">${r.tls ? lock(true, !r.decrypted) : `<span style="color:${C.faint};font-size:11px">◌</span>`}</span>
      ${statusPill(r.status, true)}
      <span class="truncate" style="color:${C.dim}">${r.host}</span>
      <span class="truncate" style="color:${C.ink}">${esc(r.path)}${stream ? `<span style="color:${C.warn};margin-left:8px;font-size:11px">· stream</span>` : ''}${r.bodySkipped ? `<span style="color:${C.faint};margin-left:8px;font-size:11px">· body skipped</span>` : ''}</span>
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
      // Newest first: state.rows stays chronological, the list renders reversed.
      list.innerHTML = rows.slice().reverse().map(rowHTML).join('');
    }
    renderWaterfall();
    renderSidebarCounts();
    renderStatusBar();
    renderCaptureSessions();
    renderSourceBanner();
  }

  function appendRow(r) {
    const list = $('#list');
    const rows = filteredRows();
    // Only insert if it passes the current filter and list isn't in empty-state.
    if (!rows.some((x) => x.id === r.id)) return;
    if ($('#list .text-center')) { renderList(); return; }
    // Newest first: prepend at the top of the list.
    const atTop = list.scrollTop <= 30;
    list.insertAdjacentHTML('afterbegin', rowHTML(r));
    const el = list.firstElementChild;
    el.classList.add('row-new');
    // Keep the viewport stable: follow the top when already there, otherwise
    // offset the scroll so existing rows don't jump under the new one.
    if (atTop) list.scrollTop = 0;
    else list.scrollTop += el.offsetHeight;
    renderWaterfall(); renderSidebarCounts(); renderStatusBar();
  }

  // A row starts life from a `request_occurred` event (no response yet, so
  // status / size / duration are null) and is later completed in place by
  // updateExternalRow when the matching `response_occurred` arrives.
  function normalizeExternalRow(r) {
    return {
      id: Number(r.id || 0),
      ts: r.ts ? Number(r.ts) : Date.now(),
      method: r.method || 'GET',
      scheme: r.scheme || (r.tls ? 'https' : 'http'),
      host: r.host || '',
      path: r.path || '/',
      version: r.version || '',
      status: null,
      statusText: '',
      mime: '',
      mimeColor: 'bin',
      size: null,
      ms: null,
      stream: false,
      bodyAvailable: false,
      bodySkipped: false,
      tls: !!r.tls,
      decrypted: !!r.decrypted,
      correlationId: r.correlationId || '',
      detail: null,   // lazily fetched from /api/requests/{id}
      bodies: {},     // side → { text, skipped, available, contentType, loading }
    };
  }

  function appendExternalRow(r) {
    const row = normalizeExternalRow(r);
    state.liveRows.push(row);
    if (state.liveRows.length > 220) {
      const dropped = state.liveRows.shift();
      if (dropped && dropped.id === state.selId) { state.selId = null; renderDetail(); }
    }
    if (state.source.kind !== 'live') {
      renderCaptureSessions();
      return row.id;
    }
    state.rows = state.liveRows;
    appendRow(row);
    return row.id;
  }

  // Completes a row when its response is streamed in. Keyed by correlationId
  // (falls back to id) since the response event carries no sequence number.
  function updateExternalRow(r) {
    const cid = r.correlationId || '';
    const row = state.liveRows.find((x) => (cid && x.correlationId === cid) || (r.id != null && x.id === Number(r.id)));
    if (!row) return -1;
    if (r.status != null) row.status = Number(r.status);
    if (r.statusText != null) row.statusText = r.statusText;
    if (r.mime != null) row.mime = r.mime;
    row.stream = !!r.stream;
    row.mimeColor = mimeCategory(row.mime, row.stream);
    if (r.size != null) row.size = Number(r.size);
    if (r.ms != null) row.ms = Number(r.ms);
    row.bodyAvailable = !!r.bodyAvailable;
    row.bodySkipped = !!r.bodySkipped;
    if (state.source.kind !== 'live') return row.id;
    // Re-render just this row in the list (cheapest correct option is a
    // targeted DOM swap; fall back to a full list render when it isn't visible).
    const el = document.querySelector(`#list [data-row="${row.id}"]`);
    if (el) el.outerHTML = rowHTML(row);
    else renderList();
    renderWaterfall(); renderSidebarCounts(); renderStatusBar();
    if (row.id === state.selId) {
      // Detail was possibly fetched while the response was still pending —
      // drop the cache so headers / timing / body reload now they exist.
      row.detail = null;
      row.bodies = {};
      renderDetail();
    }
    return row.id;
  }

  // ─── waterfall ───────────────────────────────────────────
  function renderWaterfall() {
    const wf = $('#waterfall');
    const rows = state.rows.slice(-130);
    const max = Math.max(1, ...rows.map((r) => r.ms));
    wf.innerHTML = rows.map((r) => {
      const h = Math.max(2, (r.ms / max) * 34);
      return `<div title="${r.method} ${esc(r.path)} · ${fmtMs(r.ms)}" style="width:4px;height:${h}px;background:${statusColor(r.status)};opacity:.9;flex-shrink:0;border-radius:2px"></div>`;
    }).join('');
  }

  // ─── sidebar ─────────────────────────────────────────────
  function renderSidebarCounts() {
    const rows = state.rows;
    const c = { all: rows.length, '2xx': 0, '4xx': 0, '5xx': 0, tls: 0 };
    for (const r of rows) {
      if (r.status != null) {
        if (r.status < 300) c['2xx']++;
        else if (r.status >= 500) c['5xx']++;
        else if (r.status >= 400) c['4xx']++;
      }
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
      `<button data-host="${h}" class="flex items-center gap-2 w-full text-left rounded-[8px]" style="padding:7px 10px;background:transparent;border:none;cursor:pointer;color:${C.dim};font-family:Inter;font-size:12px;font-weight:500">
        <span class="flex-1 truncate">${h}</span><span style="color:${C.faint};font-size:11px">${n}</span></button>`).join('');
  }

  // ─── detail pane ─────────────────────────────────────────
  // The detail metadata (headers + timing) is fetched lazily from
  // /api/requests/{correlationId} through WASM; bodies come from the same id's
  // /body endpoint. renderDetail kicks the fetch and re-renders on arrival.
  function ensureDetail(r) {
    if (!r || r.detail) return; // already loaded / loading / errored
    if (!r.correlationId) { r.detail = { error: 'No correlation id for this request.' }; return; }
    r.detail = { loading: true };
    const fn = window.hslLoadDetail;
    if (typeof fn === 'function') fn(r.correlationId);
    else r.detail = { error: 'Detail view is not available.' };
  }
  function ensureBody(r, side) {
    const cur = r.bodies[side];
    if (cur && (cur.loading || cur.loaded)) return;
    r.bodies[side] = { loading: true };
    const fn = window.hslLoadBody;
    if (typeof fn === 'function') fn(r.correlationId, side);
    else r.bodies[side] = { loaded: true, available: false, error: 'Body fetch is not available.' };
  }

  function detailNote(msg) {
    return `<div style="padding:22px 16px;color:${C.faint};font-size:12px;font-family:Inter">${esc(msg)}</div>`;
  }

  function headersTable(rows) {
    if (!rows || !rows.length) return detailNote('No headers.');
    return `<div style="font-family:'JetBrains Mono';font-size:11.5px;line-height:1.6">` + rows.map(([k, v]) =>
      `<div class="grid gap-3" style="grid-template-columns:minmax(0,130px) minmax(0,1fr);padding:3px 14px;border-bottom:1px solid ${C.lineSoft}">
        <span style="color:${k.startsWith(':') ? C.pink : C.info};word-break:break-all">${esc(k)}</span>
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

  function bodyBytes(body) {
    if (!body || !body.bodyBase64) return null;
    try {
      const binary = atob(body.bodyBase64);
      return Uint8Array.from(binary, (ch) => ch.charCodeAt(0));
    } catch (e) { return null; }
  }

  function bodyText(body) {
    const bytes = bodyBytes(body);
    if (!bytes) return body && body.text ? body.text : '';
    try { return new TextDecoder('utf-8').decode(bytes); }
    catch (e) { return Array.from(bytes).map((b) => String.fromCharCode(b)).join(''); }
  }

  function imageContentType(contentType) {
    const mime = (contentType || '').split(';')[0].trim().toLowerCase();
    return /^image\/[a-z0-9.+-]+$/.test(mime) ? mime : '';
  }

  function imagePreview(body, side) {
    const mime = imageContentType(body.contentType);
    if (!mime || !body.bodyBase64) return '';
    return `<div class="flex items-center justify-center" style="min-height:100%;padding:18px;background:${C.bg2}">
      <img src="data:${mime};base64,${body.bodyBase64}" alt="Captured ${side} image" style="display:block;max-width:100%;max-height:100%;object-fit:contain;border:1px solid ${C.line};background:${C.bg1};box-shadow:0 4px 16px rgba(0,0,0,.08)">
    </div>`;
  }

  function hexDump(body) {
    const bytes = bodyBytes(body);
    const src = bytes ? bytes.slice(0, 512) : Uint8Array.from(bodyText(body).slice(0, 512), (ch) => ch.charCodeAt(0));
    let out = '';
    for (let i = 0; i < src.length; i += 16) {
      const chunk = src.slice(i, i + 16);
      const hex = Array.from(chunk).map((byte) => byte.toString(16).padStart(2, '0')).join(' ');
      const ascii = Array.from(chunk).map((byte) => byte >= 32 && byte < 127 ? String.fromCharCode(byte) : '.').join('');
      out += `<div class="grid gap-3" style="grid-template-columns:80px 380px 1fr">
        <span style="color:${C.faint}">${i.toString(16).padStart(8, '0')}</span>
        <span style="color:${C.info}">${hex}</span>
        <span style="color:${C.dim}">${esc(ascii)}</span></div>`;
    }
    return `<div style="padding:12px 14px;font-family:'JetBrains Mono';font-size:11px;line-height:1.55">${out}</div>`;
  }

  // ─── request / response split panes ──────────────────────
  // Headers, Body and Raw all show both sides at once, each under a persistent
  // colour-coded header, so "which side am I looking at?" can never come up.
  // Clicking a side header focuses it and collapses the other to a rail.
  const SKIPPED_NOTE = 'Body skipped — it exceeded the capture size limit for this content type.';
  const sideAccent = (side) => (side === 'request' ? C.info : C.success);
  const sideLabel = (side) => (side === 'request' ? 'Request' : 'Response');

  // Per-side body metadata, preferring the detail DTO (authoritative for both
  // sides) and falling back to the row's response-only SSE flags.
  function sideBodyMeta(r, side) {
    const d = r.detail || {};
    if (d[side]) return d[side];
    if (side === 'response') return { bodyAvailable: !!r.bodyAvailable, bodySkipped: !!r.bodySkipped, bodySize: r.size };
    return null;
  }

  // Byte length of a loaded body without decoding it: base64 is 4 chars per
  // 3 bytes, minus the '=' padding.
  function bodySizeOf(b) {
    if (!b || !b.available) return null;
    const b64 = b.bodyBase64 || '';
    if (!b64) return b.text ? b.text.length : null;
    const pad = b64.endsWith('==') ? 2 : b64.endsWith('=') ? 1 : 0;
    return (b64.length / 4) * 3 - pad;
  }

  function sideSummary(r, side) {
    const b = r.bodies[side];
    const meta = sideBodyMeta(r, side);
    const loaded = bodySizeOf(b);
    const bytes = loaded != null ? loaded : (meta && meta.bodySize != null ? meta.bodySize : null);
    const head = side === 'request' ? r.method : (r.status != null ? String(r.status) : 'pending…');
    const ct = (b && b.contentType) || (side === 'response' ? r.mime : '');
    const mime = (ct || '').split(';')[0].trim();
    return [head, mime, bytes ? fmtBytes(bytes) : null].filter(Boolean).join(' · ');
  }

  function sideHeader(r, side) {
    const c = sideAccent(side);
    const focused = state.detailFocus === side;
    return `<button data-focus="${side}" title="${focused ? 'Show both sides' : `Focus the ${side}`}"
      style="display:flex;align-items:center;gap:7px;width:100%;text-align:left;flex-shrink:0;padding:5px 12px;
      background:${c}14;border:none;border-bottom:1px solid ${c}55;color:${c};cursor:pointer;
      font-family:Inter;font-size:10.5px;font-weight:600;letter-spacing:.6px;text-transform:uppercase">
      <span aria-hidden="true" style="font-size:12px;line-height:1">${side === 'request' ? '↑' : '↓'}</span>${sideLabel(side)}
      <span style="margin-left:auto;font-family:'JetBrains Mono';font-weight:500;text-transform:none;letter-spacing:0;color:${C.dim}">${esc(sideSummary(r, side))}</span>
      <span aria-hidden="true" style="color:${c};opacity:.65;font-size:11px">${focused ? '⤢' : '⤡'}</span></button>`;
  }

  function sideRail(side) {
    const c = sideAccent(side);
    return `<button data-focus="${side}" title="Show both sides" aria-label="Expand ${side}"
      style="display:flex;align-items:center;justify-content:center;height:100%;width:100%;padding:0;
      background:${c}14;border:none;border-left:1px solid ${c}55;border-right:1px solid ${c}55;cursor:pointer;color:${c};
      font-family:Inter;font-size:10.5px;font-weight:600;letter-spacing:.6px;text-transform:uppercase">
      <span style="writing-mode:vertical-rl;transform:rotate(180deg);white-space:nowrap">${side === 'request' ? '↑' : '↓'} ${sideLabel(side)}</span></button>`;
  }

  // Lays out the two sides according to state.detailFocus. `render(side)` returns
  // the scrollable content for one side.
  function splitPane(r, render) {
    const focus = state.detailFocus;
    const cols = focus === 'request' ? 'minmax(0,1fr) 30px'
      : focus === 'response' ? '30px minmax(0,1fr)'
      : 'minmax(0,1fr) minmax(0,1fr)';
    const col = (side) => {
      if (focus && focus !== side) return sideRail(side);
      return `<div class="flex flex-col min-h-0 min-w-0" style="${side === 'request' ? `border-right:1px solid ${C.line}` : ''}">
        ${sideHeader(r, side)}
        <div class="flex-1 min-h-0 overflow-auto hsl-scroll" style="background:${C.bg1}">${render(side)}</div></div>`;
    };
    return `<div class="grid h-full min-h-0" style="grid-template-columns:${cols}">${col('request')}${col('response')}</div>`;
  }

  function bodyEmptyNote(r, side) {
    if (side === 'request') return 'No request body — this request was sent without one.';
    if (r.stream) return 'Streaming response — the body is never captured, only frame metadata.';
    if (r.status == null) return 'Waiting for the response…';
    return 'No response body was captured.';
  }

  // Content-Encoding for one side, read from that side's own headers.
  function sideEncoding(r, side) {
    const d = r.detail || {};
    const headers = (d[side] && d[side].headers) || [];
    const hit = headers.find(([k]) => k.toLowerCase() === 'content-encoding');
    const enc = hit ? String(hit[1]).toLowerCase().trim() : '';
    return enc === 'identity' ? '' : enc;
  }

  // Bodies are stored exactly as they crossed the wire, so a compressed one is
  // compressed bytes. Say so rather than rendering mojibake — roughly half of
  // real request bodies are gzip'd.
  function encodingBanner(enc) {
    return `<div style="display:flex;align-items:center;gap:7px;padding:6px 14px;background:${C.warn}14;border-bottom:1px solid ${C.warn}40;color:${C.warn};font-family:Inter;font-size:11px">
      <span aria-hidden="true">⚠</span>Compressed with ${esc(enc)} — these are the raw wire bytes, not the decoded payload.</div>`;
  }

  function bodyContent(r, b, side) {
    const mode = state.bodyMode;
    if (mode === 'hex') return hexDump(b);
    const enc = sideEncoding(r, side);
    const text = bodyText(b);
    if (mode === 'pretty' && !enc) {
      if (imageContentType(b.contentType)) return imagePreview(b, side);
      if (mimeCategory(b.contentType) === 'json') {
        return `<pre style="margin:0;padding:12px 14px;font-family:'JetBrains Mono';font-size:11.5px;line-height:1.55;color:${C.ink};white-space:pre;overflow:auto">${jsonHighlight(text)}</pre>`;
      }
    }
    return (enc ? encodingBanner(enc) : '') +
      `<pre style="margin:0;padding:12px 14px;font-family:'JetBrains Mono';font-size:11.5px;line-height:1.55;color:${C.dim};white-space:pre-wrap;word-break:break-all;overflow:auto">${esc(text)}</pre>`;
  }

  // Resolves one side's body to markup, kicking the lazy fetch when the metadata
  // says there is something to fetch. Capture rows arrive with both sides already
  // loaded, so they short-circuit on the first branch.
  function bodySide(r, side) {
    const b = r.bodies[side];
    if (b && b.loaded) {
      if (b.error) return detailNote(b.error);
      if (b.skipped) return detailNote(SKIPPED_NOTE);
      if (!b.available) return detailNote(bodyEmptyNote(r, side));
      return bodyContent(r, b, side);
    }
    if (b && b.loading) return detailNote('Loading body…');

    const d = r.detail || {};
    if (side === 'response') {
      if (r.stream) return detailNote('Streaming response — the body is never captured, only frame metadata.');
      if (r.bodySkipped) return detailNote(SKIPPED_NOTE);
      if (r.status == null) return detailNote('Waiting for the response…');
      if (!r.bodyAvailable) return detailNote('No response body was captured.');
    } else {
      if (d.loading) return detailNote('Loading…');
      if (d.error) return detailNote(d.error);
      const meta = d.request;
      if (!meta) return detailNote('Request detail unavailable.');
      if (meta.bodySkipped) return detailNote(SKIPPED_NOTE);
      if (!meta.bodyAvailable) return detailNote(bodyEmptyNote(r, side));
    }
    ensureBody(r, side);
    return detailNote('Loading body…');
  }

  function bodyPane(r) {
    const mode = state.bodyMode;
    const tab = (id, label) => `<button data-bodymode="${id}" style="background:transparent;border:none;cursor:pointer;padding:8px 12px 7px;font-size:11.5px;font-family:Inter;font-weight:500;color:${mode === id ? C.ink : C.dim};border-bottom:2px solid ${mode === id ? C.mint : 'transparent'};margin-bottom:-1px">${label}</button>`;
    return `<div class="flex flex-col h-full min-h-0">
      <div class="flex" style="border-bottom:1px solid ${C.line};background:${C.bg1};padding-left:6px;flex-shrink:0">${tab('pretty', 'Pretty')}${tab('raw', 'Raw')}${tab('hex', 'Hex')}</div>
      <div class="flex-1 min-h-0">${splitPane(r, (side) => bodySide(r, side))}</div></div>`;
  }

  // A real, self-contained HTTP message per side — start line, that side's own
  // headers, blank line, that side's own body.
  function rawSide(r, side) {
    const d = r.detail || {};
    if (d.loading) return detailNote('Loading…');
    if (d.error) return detailNote(d.error);

    let start, headers;
    if (side === 'request') {
      const version = (d.request && d.request.httpVersion) || r.version || 'HTTP/1.1';
      start = `${r.method} ${r.path} ${version}`;
      headers = (d.request && d.request.headers) || [];
      // Go hoists Host out of the header map onto Request.Host, so it never
      // reaches us as a captured header. Put it back, or the message we print
      // isn't a valid HTTP/1.1 request.
      if (r.host && !headers.some(([k]) => k.toLowerCase() === 'host')) {
        headers = [['Host', r.host]].concat(headers);
      }
    } else {
      if (r.status == null) return detailNote('Waiting for the response…');
      const version = (d.response && d.response.httpVersion) || r.version || 'HTTP/1.1';
      start = `${version} ${r.status}${r.statusText ? ' ' + r.statusText : ''}`;
      headers = (d.response && d.response.headers) || [];
    }

    const b = r.bodies[side];
    let bodyText_ = '';
    if (b && b.loaded && b.available) bodyText_ = bodyText(b);
    else if (b && b.loading) bodyText_ = '⋯ loading body';
    else {
      const meta = sideBodyMeta(r, side);
      if (meta && meta.bodySkipped) bodyText_ = '⋯ body skipped (over the capture size limit)';
      else if (meta && meta.bodyAvailable) { ensureBody(r, side); bodyText_ = '⋯ loading body'; }
    }

    const head = `${start}\n${headers.map(([k, v]) => `${k}: ${v}`).join('\n')}`;
    return `<pre style="margin:0;padding:12px 14px;font-family:'JetBrains Mono';font-size:11.5px;line-height:1.55;color:${C.dim};white-space:pre-wrap;word-break:break-all">` +
      `<span style="color:${C.ink}">${esc(head)}</span>\n\n${esc(bodyText_)}</pre>`;
  }

  // Real per-phase timing from the backend (TimingDto, milliseconds). Present
  // only for decrypted HTTPS traffic; otherwise we say so.
  function timingBar(r) {
    const t = r.detail && r.detail.timing;
    if (!t) return detailNote('Per-phase timing is available only for decrypted HTTPS traffic.');
    const segs = [
      ['dns', t.dnsMs || 0, C.pink], ['connect', t.connectMs || 0, C.info],
      ['tls', t.tlsMs || 0, C.mint], ['wait', t.ttfbMs || 0, C.dim], ['download', t.downloadMs || 0, C.ink],
    ];
    const total = segs.reduce((s, x) => s + x[1], 0) || 1;
    return `<div style="padding:10px 14px;border-bottom:1px solid ${C.lineSoft}">
      <div class="flex" style="height:8px;border-radius:2px;overflow:hidden;background:${C.bg2}">
        ${segs.map(([l, ms, c]) => ms > 0 ? `<div title="${l}: ${ms}ms" style="flex:${ms / total};background:${c};opacity:.85"></div>` : '').join('')}</div>
      <div class="flex flex-wrap gap-[14px]" style="margin-top:8px;font-family:'JetBrains Mono';font-size:10.5px;color:${C.dim}">
        ${segs.map(([l, ms, c]) => ms > 0 ? `<span class="inline-flex items-center gap-1"><span style="width:6px;height:6px;background:${c};border-radius:1px"></span>${l} <span style="color:${C.ink}">${ms}ms</span></span>` : '').join('')}
        <span class="inline-flex items-center gap-1" style="margin-left:auto">total <span style="color:${C.ink}">${fmtMs(t.totalMs || total)}</span></span></div></div>`;
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
    if (state.detailHeight) {
      wrap.style.flex = `0 0 ${state.detailHeight}px`;
      $('#list-region').style.flex = '1 1 auto';
    } else {
      wrap.style.flex = '0.45 1 0%';
      $('#list-region').style.flex = '0.55 1 0%';
    }

    ensureDetail(r);
    const d = r.detail || {};
    const reqH = (d.request && d.request.headers) || [];
    const resH = (d.response && d.response.headers) || [];
    const headerCount = d.loading ? null : reqH.length + resH.length;
    const proto = (d.response && d.response.httpVersion) || (d.request && d.request.httpVersion) || r.version || '—';

    const tab = (id, label, count) => `<button data-tab="${id}" style="background:transparent;border:none;cursor:pointer;padding:8px 12px 7px;font-size:11.5px;font-family:Inter;font-weight:500;color:${state.detailTab === id ? C.ink : C.dim};border-bottom:2px solid ${state.detailTab === id ? C.mint : 'transparent'};margin-bottom:-1px" class="inline-flex items-center gap-[6px]">${label}${count != null ? `<span style="color:${C.faint};font-family:'JetBrains Mono';font-size:10px">${count}</span>` : ''}</button>`;

    let body = '';
    const t = state.detailTab;
    if (t === 'overview') {
      body = `<div style="padding:12px 16px">
        <div class="grid gap-6" style="grid-template-columns:1fr 1fr">
          <div><div style="font-size:10.5px;color:${C.faint};text-transform:uppercase;letter-spacing:.7px;margin-bottom:4px">General</div>
            ${kv('URL', `${r.scheme}://${r.host}${r.path}`)}${kv('Method', r.method)}${kv('Status', r.status != null ? `${r.status}${r.statusText ? ' ' + r.statusText : ''}` : 'pending…', r.status != null ? statusColor(r.status) : C.faint)}${kv('Protocol', proto)}</div>
          <div><div style="font-size:10.5px;color:${C.faint};text-transform:uppercase;letter-spacing:.7px;margin-bottom:4px">Transfer</div>
            ${kv('Content-Type', r.mime || '—')}${kv('Size', fmtBytes(r.size))}${kv('Duration', fmtMs(r.ms))}${kv('TLS', r.tls ? (r.decrypted ? 'decrypted · HSL root CA' : 'passthrough · not decrypted') : 'cleartext', r.tls ? (r.decrypted ? C.mint : C.warn) : C.faint)}</div>
        </div>
        <div style="margin-top:18px"><div style="font-size:10.5px;color:${C.faint};text-transform:uppercase;letter-spacing:.7px;margin-bottom:6px">Timing</div>${timingBar(r)}</div></div>`;
    } else if (t === 'headers') {
      if (d.loading) body = detailNote('Loading headers…');
      else if (d.error) body = detailNote(d.error);
      else body = splitPane(r, (side) => headersTable(side === 'request' ? reqH : resH));
    } else if (t === 'body') {
      body = bodyPane(r);
    } else if (t === 'timing') {
      body = timingBar(r);
    } else if (t === 'raw') {
      body = splitPane(r, (side) => rawSide(r, side));
    }
    // The split tabs scroll inside each column; the others scroll as one block.
    const split = t === 'headers' || t === 'body' || t === 'raw';

    wrap.innerHTML = `
      <div data-detail-resizer class="detail-resizer" title="Drag to resize · double-click to reset" aria-label="Resize request detail panel"></div>
      <div style="padding:10px 14px;border-bottom:1px solid ${C.line};background:${C.bg1};display:flex;align-items:center;gap:10px;flex-shrink:0">
        ${methodTag(r.method)}${statusPill(r.status)}
        <div class="flex-1 overflow-hidden">
          <div class="truncate" style="font-family:'JetBrains Mono';font-size:12px;color:${C.ink}"><span style="color:${C.faint}">${r.scheme}://</span><span style="color:${C.dim}">${r.host}</span>${esc(r.path)}</div>
          <div style="font-size:11px;color:${C.faint};margin-top:2px;font-family:'JetBrains Mono'">#${String(r.id).padStart(3, '0')} · ${r.mime || '—'} · ${fmtBytes(r.size)} · ${fmtMs(r.ms)} · ${r.tls ? (r.decrypted ? 'TLS · decrypted' : 'TLS · passthrough') : 'cleartext'}</div>
        </div>
        <button data-action="close-detail" style="background:transparent;border:none;color:${C.faint};cursor:pointer;font-size:18px;padding:4px 6px">×</button>
      </div>
      <div class="flex" style="border-bottom:1px solid ${C.line};background:${C.bg1};padding-left:6px;flex-shrink:0">
        ${tab('overview', 'Overview')}${tab('headers', 'Headers', headerCount)}${tab('body', 'Body')}${tab('timing', 'Timing')}${tab('raw', 'Raw')}</div>
      <div class="flex-1 min-h-0 ${split ? 'overflow-hidden' : 'overflow-auto hsl-scroll'}">${body}</div>`;
  }

  // ─── status bar ──────────────────────────────────────────
  function renderStatusBar() {
    const rows = state.rows;
    const totalBytes = rows.reduce((s, r) => s + (r.size || 0), 0);
    const timed = rows.filter((r) => r.ms != null);
    const avg = timed.length ? Math.round(timed.reduce((s, r) => s + r.ms, 0) / timed.length) : 0;
    const err = rows.filter((r) => r.status >= 400).length;
    $('#statusbar').innerHTML = `
      <span class="inline-flex items-center gap-[6px]" style="padding:0 10px 0 14px;color:${state.proxyRunning ? C.mint : C.danger}">proxy ${state.proxyRunning ? 'running' : 'stopped'}</span>
      <span class="inline-flex items-center gap-[6px]" style="padding:0 10px;color:${state.capturing ? C.mint : C.faint}"><span class="rec-dot ${state.capturing ? 'on' : ''}"></span>${state.capturing ? 'recording' : 'recording stopped'}</span>
      ${state.source.kind === 'capture' ? `<span style="padding:0 10px;color:${C.info}">read-only capture</span>` : ''}
      <span style="padding:0 10px;color:${C.dim}">${rows.length} req</span>
      <span style="padding:0 10px;color:${C.dim}">${err} errors</span>
      <span style="padding:0 10px;color:${C.dim}">avg ${avg}ms</span>
      <span style="padding:0 10px;color:${C.dim}">${fmtBytes(totalBytes)} total</span>
      <span class="flex-1"></span>
      <span class="inline-flex items-center gap-[6px]" style="padding:0 10px;color:${state.decryption ? C.mint : C.warn}">${lock(state.decryption, !state.decryption)} HTTPS ${state.decryption ? 'decrypted' : 'passthrough'}</span>
      <span style="padding:0 10px;color:${C.dim}">upstream ${state.upstream.on ? (state.upstream.ntlm ? 'NTLM' : 'direct') : 'off'}</span>
      <span style="padding:0 14px 0 10px;color:${C.dim}">access ${state.access.mode}</span>
      ${updateSegment()}
      ${versionSegment()}`;
  }

  // Update badge, shown only when the backend confirmed a newer GitHub release.
  // Links to the platform asset (or the release page) so one click starts the
  // download.
  function updateSegment() {
    if (!state.update.available || !state.update.url) return '';
    const label = state.update.latest ? `update → ${esc(state.update.latest)}` : 'update available';
    return `<a href="${esc(state.update.url)}" target="_blank" rel="noopener noreferrer"
      title="Download the latest release from GitHub"
      class="inline-flex items-center gap-[5px]"
      style="margin:0 4px;padding:2px 9px;border-radius:10px;background:${C.bg3};color:${C.mint};text-decoration:none;font-weight:500">↑ ${label}</a>`;
  }

  // Version indicator at the far right of the status bar. Links to the exact
  // commit on GitHub when the build injected a commit hash (release builds);
  // otherwise it's plain text (dev builds report "dev" / no commit_url).
  function versionSegment() {
    if (!state.build.version) return '';
    const label = esc(state.build.version);
    const url = state.build.commitUrl;
    const inner = url
      ? `<a href="${esc(url)}" target="_blank" rel="noopener noreferrer" title="View commit ${esc(state.build.commit || '')} on GitHub" style="color:${C.info};text-decoration:none">${label}</a>`
      : label;
    return `<span style="padding:0 14px 0 10px;border-left:1px solid var(--line);color:${C.dim}">${inner}</span>`;
  }

  // ─── toolbar state sync ──────────────────────────────────
  function renderToolbar() {
    const proxy = $('#btn-proxy');
    proxy.innerHTML = `<span style="width:7px;height:7px;border-radius:50%;background:${state.proxyRunning ? C.mint : C.danger}"></span>${state.proxyRunning ? 'Stop proxy' : 'Start proxy'}`;
    proxy.style.color = state.proxyRunning ? C.mint : C.danger;
    proxy.style.background = state.proxyRunning ? C.bg3 : 'transparent';

    const cap = $('#btn-capture');
    cap.innerHTML = `<span class="rec-dot ${state.capturing ? 'on' : ''}"></span>${state.capturing ? 'Stop recording' : 'Start recording'}`;
    cap.style.color = state.capturing ? C.mint : C.dim;
    cap.style.background = state.capturing ? C.bg3 : 'transparent';

    const clear = $('[data-action="clear"]');
    const archived = state.source.kind === 'capture';
    if (clear) {
      clear.disabled = archived;
      clear.title = archived ? 'Saved captures are read-only' : 'Clear the live session';
      clear.style.opacity = archived ? '.45' : '1';
      clear.style.cursor = archived ? 'not-allowed' : 'pointer';
    }

    const dec = $('#btn-decrypt');
    dec.innerHTML = `${lock(state.decryption)} HTTPS decryption · ${state.decryption ? 'On' : 'Off'}`;
    dec.style.color = state.decryption ? C.mint : C.dim;
    dec.style.background = state.decryption ? C.bg3 : 'transparent';

    const up = $('#btn-upstream');
    up.textContent = `Upstream · ${state.upstream.on ? (state.upstream.ntlm ? 'NTLM' : 'Direct') : 'Off'}`;
    up.style.color = state.upstream.on ? C.mint : C.dim;

    const acc = $('#btn-access');
    const modeLabel = { loopback: 'Loopback', lan: 'Private LAN', allowlist: 'Allowlist', open: 'Open' };
    acc.textContent = `Access · ${modeLabel[state.access.mode] || state.access.mode}`;
    renderProxySummary();
  }

  function renderProxySummary() {
    const summary = $('#proxy-summary');
    if (!summary) return;
    const color = state.proxyRunning ? C.mint : C.danger;
    summary.innerHTML = `<div class="flex items-center gap-[7px]" style="margin-bottom:8px">
      <span style="width:7px;height:7px;border-radius:4px;background:${color};box-shadow:0 0 0 3px ${color}22"></span>
      <span style="color:${C.dim};font-size:11.5px;font-weight:600">${state.proxyRunning ? 'Proxy listening' : 'Proxy stopped'}</span>
    </div><div style="font-family:'JetBrains Mono';font-size:12px;color:${state.proxyRunning ? C.ink : C.faint};word-break:break-all">${esc(state.proxyAddress || 'address unavailable')}</div>
    <div class="flex items-center gap-[7px]" style="margin-top:11px;padding-top:10px;border-top:1px solid ${C.lineSoft};font-size:11px;color:${C.dim}">
      <span aria-hidden="true" style="color:${C.faint};font-size:12px">▣</span><span>App memory</span>
      <span style="margin-left:auto;color:${C.ink};font-family:'JetBrains Mono';font-variant-numeric:tabular-nums">${state.runtime.memoryBytes == null ? '—' : fmtBytes(state.runtime.memoryBytes)}</span>
    </div>`;
  }

  // ─── modals ──────────────────────────────────────────────
  let modalKind = null;
  function openModal(html, width) {
    const root = $('#modal-root');
    root.innerHTML = `<div class="modal-backdrop" style="position:absolute;inset:0;background:rgba(43,41,38,.34);backdrop-filter:blur(3px);z-index:50;display:flex;align-items:center;justify-content:center">
      <div class="modal-card" style="width:${width || 620}px;max-width:92%;max-height:88%;display:flex;flex-direction:column;background:${C.bg1};border:1px solid ${C.line};border-radius:12px;box-shadow:0 24px 60px rgba(43,41,38,.22);overflow:hidden;color:${C.ink};font-family:Inter">${html}</div></div>`;
    root.style.pointerEvents = 'auto';
  }
  function closeModal() { modalKind = null; $('#modal-root').innerHTML = ''; $('#modal-root').style.pointerEvents = 'none'; }

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
      primary: `background:${C.mint};color:${C.onAccent};border:1px solid ${C.mint};font-weight:600`,
      danger: `background:transparent;color:${C.danger};border:1px solid ${C.danger}66`,
      ghost: `background:transparent;color:${C.dim};border:1px solid ${C.line}`,
      default: `background:${C.bg3};color:${C.ink};border:1px solid ${C.line}`,
    };
    return `<button data-action="${action}" style="${tones[tone] || tones.default};border-radius:4px;padding:6px 12px;font-size:12px;font-family:Inter;cursor:pointer;height:28px;display:inline-flex;align-items:center;gap:6px">${label}</button>`;
  }
  function toggle(on, action, sm, disabled) {
    const w = sm ? 28 : 34, h = sm ? 16 : 20, dot = sm ? 12 : 16;
    return `<button ${disabled ? '' : `data-action="${action}"`} style="width:${w}px;height:${h}px;border-radius:${h / 2}px;padding:2px;background:${on ? C.mint : C.bg3};border:1px solid ${on ? C.mint : C.line};cursor:${disabled ? 'not-allowed' : 'pointer'};display:flex;align-items:center;opacity:${disabled ? .5 : 1}">
      <span style="width:${dot}px;height:${dot}px;border-radius:${dot / 2}px;background:${on ? C.onAccent : C.dim};transform:translateX(${on ? w - dot - 6 : 0}px);transition:transform .15s"></span></button>`;
  }

  // cert wizard
  const cert = { step: 0 };
  function loadCertificateStatus(force) {
    const ca = state.certificate;
    if (ca.loading || (!force && ca.loaded)) return;
    ca.loading = true; ca.error = null;
    const fn = window.hslCertificateAction;
    if (typeof fn === 'function') setTimeout(() => fn('status'), 0);
    else { ca.loading = false; ca.error = 'Backend bridge unavailable.'; }
  }
  function certificateAction(action) {
    const ca = state.certificate;
    if (ca.busy) return;
    if (action === 'regenerate' && !window.confirm('Regenerate the root CA? Existing trust and previously issued site certificates will stop working.')) return;
    ca.busy = true; ca.action = action; ca.error = null;
    if (modalKind === 'cert') renderCert(); else renderSettings();
    const fn = window.hslCertificateAction;
    if (typeof fn === 'function') fn(action);
    else { ca.busy = false; ca.error = 'Backend bridge unavailable.'; if (modalKind === 'cert') renderCert(); else renderSettings(); }
  }
  function certCleanupSummary(r) {
    if (!r) return '';
    const parts = [];
    if (r.store_cleanup_supported) {
      const roots = r.root_certs_removed || 0, sites = r.domain_certs_removed || 0;
      parts.push(`Removed ${roots} root CA${roots === 1 ? '' : 's'} and ${sites} site certificate${sites === 1 ? '' : 's'} from the OS trust store.`);
    } else {
      parts.push('Automatic OS trust-store cleanup is unavailable on this platform — remove the root CA by hand if it was installed.');
    }
    if (r.domain_folder_removed) parts.push('Deleted the per-domain certificates folder.');
    const files = Array.isArray(r.removed_files) ? r.removed_files : [];
    if (files.length) parts.push(`Deleted ${files.length} CA file${files.length === 1 ? '' : 's'}.`);
    if (r.decryption_disabled) parts.push('Turned HTTPS decryption off and reset the in-memory certificate cache.');
    const warnings = Array.isArray(r.warnings) ? r.warnings : [];
    const warnHTML = warnings.length ? `<div style="color:${C.warn};margin-top:6px">${warnings.map(w => '⚠ ' + esc(w)).join('<br>')}</div>` : '';
    return `<div style="margin-top:10px;padding:10px;background:${C.mint}10;border:1px solid ${C.mint}40;border-radius:4px;color:${C.ink};font-size:11.5px;line-height:1.6">${parts.map(esc).join('<br>')}${warnHTML}</div>`;
  }
  function openCert() {
    modalKind = 'cert'; cert.step = state.certificate.status && state.certificate.status.available ? 2 : 0;
    loadCertificateStatus(false); renderCert();
  }
  function renderCert() {
    const stepper = ['Review', 'Generate', 'Trust'].map((s, i) => {
      const on = i === cert.step, done = i < cert.step;
      return `<div class="flex items-center gap-2 flex-1"><span style="width:20px;height:20px;border-radius:10px;display:inline-flex;align-items:center;justify-content:center;background:${done ? C.mint : on ? C.bg3 : C.bg2};border:1px solid ${done || on ? C.mint : C.line};color:${done ? C.onAccent : on ? C.mint : C.dim};font-size:11px;font-weight:600;font-family:'JetBrains Mono'">${done ? '✓' : i + 1}</span><span style="font-size:12px;color:${on || done ? C.ink : C.dim};font-weight:500">${s}</span></div>`;
    }).join(`<div style="height:1px;flex:.3;background:${C.line}"></div>`);

    let inner = '';
    if (cert.step === 0) {
      inner = `<div style="font-size:12.5px;line-height:1.65;color:${C.dim}">
        <p style="margin:0 0 10px">To decrypt HTTPS, HttpStackLens generates a self-signed root CA and signs a leaf certificate for every host you visit — acting as a man-in-the-middle on your own traffic, only on this machine.</p>
        <div style="background:${C.bg2};border:1px solid ${C.line};border-radius:4px;padding:10px 12px;font-family:'JetBrains Mono';font-size:11.5px;color:${C.ink};margin:12px 0"><div style="color:${C.faint};font-size:10.5px;margin-bottom:4px">will be created</div>CN=HttpStackLens Root CA · RSA 3072 · SHA-256 · valid 1825 days</div>
        <div style="background:${C.warn}12;border:1px solid ${C.warn}40;border-radius:4px;padding:10px 12px;color:${C.warn};font-size:11.5px;display:flex;gap:10px"><span style="font-size:14px;line-height:1">⚠</span><div style="color:${C.ink}">The root key can impersonate sites for this user. It stays in the configured local key file and must never be shared.</div></div>
        ${state.certificate.error ? `<div style="color:${C.danger};font-size:11.5px;margin-top:12px">${esc(state.certificate.error)}</div>` : ''}</div>`;
    } else if (cert.step === 1) {
      inner = `<div style="text-align:center;padding:24px 20px">
        <div class="spin" style="margin:0 auto 16px;width:40px;height:40px;border-radius:20px;border:2px solid ${C.bg2};border-top-color:${C.mint}"></div>
        <div style="font-size:12.5px;color:${C.ink};margin-bottom:4px">Generating the local CA…</div>
        <div style="font-size:11px;color:${C.dim}">This usually takes only a moment.</div></div>`;
    } else {
      const ca = state.certificate, s = ca.status || {};
      const trust = s.installed ? 'Trusted by the operating system.' : s.install_supported ? 'Not yet trusted by the operating system.' : 'Automatic installation is unavailable on this operating system.';
      inner = `<div>
        <div style="background:${C.mint}10;border:1px solid ${C.mint}40;border-radius:4px;padding:10px 12px;font-size:11.5px;margin-bottom:14px;display:flex;gap:10px"><span style="color:${C.mint}">✓</span><div style="color:${C.ink}">Local CA ready. <span style="color:${C.dim}">Fingerprint:</span> <span style="font-family:'JetBrains Mono';word-break:break-all">${esc(s.fingerprint_sha256 || '—')}</span></div></div>
        <div style="font-size:12.5px;color:${C.dim};margin-bottom:10px">${trust}</div>
        <div class="flex gap-2">${s.install_supported ? btn(s.installed ? 'Reinstall in OS trust store' : 'Install in OS trust store', 'cert-install', 'default') : ''}${btn('Export public certificate', 'cert-export', 'ghost')}</div>
        ${ca.error ? `<div style="color:${C.danger};font-size:11.5px;margin-top:12px">${esc(ca.error)}</div>` : ''}</div>`;
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
  function openSettings(tab) { modalKind = 'settings'; settingsTab = tab || 'body'; renderSettings(); }
  function renderSettings() {
    const tabs = [['cert', 'TLS / Certificate'], ['body', 'Body capture'], ['upstream', 'Upstream proxy'], ['access', 'Access control']];
    const nav = tabs.map(([id, l]) => `<button data-settab="${id}" style="display:block;width:100%;text-align:left;background:${settingsTab === id ? C.bg3 : 'transparent'};border:none;cursor:pointer;padding:7px 10px;border-radius:3px;margin-bottom:2px;color:${settingsTab === id ? C.ink : C.dim};font-size:12px;font-family:Inter;font-weight:500">${l}</button>`).join('');
    openModal(modalHeader('Settings') +
      `<div class="grid" style="grid-template-columns:180px 1fr;min-height:380px">
        <div style="border-right:1px solid ${C.line};padding:10px 8px;background:${C.bg2}">${nav}</div>
        <div style="padding:18px;overflow:auto" id="settings-body">${settingsBody()}</div></div>` +
      modalFooter(btn('Done', 'close-modal', 'primary')), 820);
  }
  function settingsBody() {
    if (settingsTab === 'cert') {
      const ca = state.certificate;
      if (!ca.loaded) {
        if (!ca.error) loadCertificateStatus(false);
        return `<div style="font-size:12px;color:${ca.error ? C.danger : C.dim};padding:20px 2px">${esc(ca.error || 'Loading certificate status…')}${ca.error ? `<div style="margin-top:10px">${btn('Retry', 'cert-refresh', 'ghost')}</div>` : ''}</div>`;
      }
      const s = ca.status || {}, available = !!s.available;
      const expires = s.not_after ? new Date(s.not_after).toLocaleDateString() : '—';
      const title = !available ? 'No usable local CA' : s.expired ? 'Root CA expired' : s.installed ? 'Root CA installed' : 'Root CA generated';
      const color = available && !s.expired ? (s.installed ? C.mint : C.warn) : C.danger;
      return `<div style="font-size:12px;color:${C.dim};line-height:1.7">
        <div style="padding:12px;background:${C.bg2};border:1px solid ${C.line};border-radius:4px;margin-bottom:12px">
          <div class="flex items-center gap-[10px]" style="margin-bottom:8px"><span style="color:${color};font-size:16px">●</span><div class="flex-1"><div style="font-size:12.5px;color:${C.ink};font-weight:600">${title}</div><div style="font-size:11px;color:${C.dim};font-family:'JetBrains Mono';word-break:break-all">${esc(s.ca_cert_subject || s.error || 'Generate a CA to inspect HTTPS traffic')} ${available ? `· ${esc(s.fingerprint_sha256 || '—')} · expires ${expires}` : ''}</div></div></div>
          <div class="flex gap-[6px] flex-wrap">${available ? btn('Export public certificate', 'cert-export') : btn('Generate root CA', 'cert-generate', 'primary')}${available && s.install_supported ? btn(s.installed ? 'Reinstall' : 'Install', 'cert-install') : ''}${available ? btn('Regenerate root', 'cert-regenerate', 'danger') : ''}${btn('Refresh', 'cert-refresh', 'ghost')}</div>
          ${ca.busy ? `<div style="color:${C.dim};margin-top:10px">Working…</div>` : ''}${ca.error ? `<div style="color:${C.danger};margin-top:10px">${esc(ca.error)}</div>` : ''}${certCleanupSummary(ca.cleanup)}</div>
        <p>Only the public certificate is exported. Tools with their own trust store (Firefox, Node.js, the JVM) may need it installed separately.</p>
        <div style="margin-top:14px;padding-top:12px;border-top:1px solid ${C.line}">
          <div style="font-size:12px;color:${C.ink};font-weight:600;margin-bottom:4px">Clean up polluted stores</div>
          <p style="margin:0 0 8px">Remove every HttpStackLens certificate from the OS trust store (matched by the CA signature so unrelated certificates are left alone), then delete the local CA files and the per-domain certificates folder.</p>
          ${btn('Clean up certificates', 'cert-cleanup', 'danger')}</div></div>`;
    }
    if (settingsTab === 'body') return bodyRulesPanel();
    if (settingsTab === 'upstream') return upstreamPanel();
    if (settingsTab === 'access') return accessPanel();
    return '';
  }

  function sizeLabel(b) { return b === 0 ? 'off' : b < 1048576 ? (b / 1024) + ' KB' : (b / 1048576) + ' MB'; }
  // Size options offered per rule (bytes). `null` is a sentinel whose meaning
  // (use-default vs. no-limit) depends on the caller, passed via nullLabel.
  const BODY_SIZE_OPTS = [null, 0, 65536, 262144, 524288, 1048576, 2097152, 8388608];
  function sizeSelect(dataAttr, current, nullLabel) {
    const opts = BODY_SIZE_OPTS
      .map((b) => `<option value="${b === null ? '' : b}" ${b === current ? 'selected' : ''}>${b === null ? nullLabel : sizeLabel(b)}</option>`).join('');
    return `<select ${dataAttr} style="background:${C.bg2};color:${C.ink};border:1px solid ${C.line};border-radius:3px;padding:4px 6px;font-family:'JetBrains Mono';font-size:11.5px">${opts}</select>`;
  }
  function bodyRulesPanel() {
    const bc = state.bodyCapture;
    // Never show the editor until the current settings loaded — otherwise a save
    // could overwrite real config with an empty form.
    if (!bc.loaded) {
      if (bc.error) {
        return `<div style="font-size:12px;color:${C.danger};margin-bottom:12px">${bc.error}</div>${btn('Retry', 'body-reload', 'ghost')}`;
      }
      // Deferred so the fetch never re-enters the render that kicked it off.
      if (!bc.loading && typeof window.hslLoadBodyCapture === 'function') { bc.loading = true; setTimeout(() => window.hslLoadBodyCapture(), 0); }
      return `<div style="font-size:12px;color:${C.dim};padding:20px 2px">Loading body capture settings…</div>`;
    }
    const head = `<div class="grid" style="grid-template-columns:1fr 150px 40px;padding:8px 12px;background:${C.bg2};border-bottom:1px solid ${C.line};font-size:10.5px;font-weight:600;letter-spacing:.6px;text-transform:uppercase;color:${C.faint};font-family:Inter"><span>MIME type / wildcard</span><span>Max per body</span><span></span></div>`;
    const rows = bc.mimeTypes.length
      ? bc.mimeTypes.map((r, i) => `<div class="grid items-center gap-2" style="grid-template-columns:1fr 150px 40px;padding:8px 12px;border-bottom:${i === bc.mimeTypes.length - 1 ? 'none' : `1px solid ${C.lineSoft}`}">
        <input data-bc-name="${i}" value="${(r.name || '').replace(/"/g, '&quot;')}" placeholder="application/json" style="background:${C.bg2};color:${C.ink};border:1px solid ${C.line};border-radius:3px;padding:5px 8px;font-family:'JetBrains Mono';font-size:11.5px;width:100%;outline:none">
        ${sizeSelect(`data-bc-max="${i}"`, typeof r.maxSizeBytes === 'number' ? r.maxSizeBytes : null, 'Use default')}
        <button data-action="body-remove-rule:${i}" title="Remove" style="background:transparent;border:1px solid ${C.line};color:${C.dim};border-radius:3px;cursor:pointer;height:26px;font-size:13px">×</button></div>`).join('')
      : `<div style="padding:14px 12px;font-size:11.5px;color:${C.faint}">No per-type rules — every body uses the default cap below.</div>`;
    const status = bc.error
      ? `<span style="color:${C.danger};font-size:11.5px">${bc.error}</span>`
      : bc.saving ? `<span style="color:${C.dim};font-size:11.5px">Saving…</span>`
      : bc.dirty ? `<span style="color:${C.warn};font-size:11.5px">Unsaved changes</span>`
      : bc.saved ? `<span style="color:${C.mint};font-size:11.5px">Saved ✓</span>` : '';
    return `<div style="font-size:12px;color:${C.dim};margin-bottom:12px;line-height:1.6">Choose how much of each response body is captured and kept in memory. Bodies larger than the cap are skipped (metadata only). Per-type rules override the default; a rule set to <b>off</b> skips that type entirely.</div>
      <div style="border:1px solid ${C.line};border-radius:4px;overflow:hidden;margin-bottom:12px">${head}${rows}</div>
      <div style="margin-bottom:14px">${btn('+ Add MIME rule', 'body-add-rule', 'ghost')}</div>
      ${field('Default cap', 'Applied to types without a rule', sizeSelect('data-bc-default', typeof bc.defaultMaxBytes === 'number' ? bc.defaultMaxBytes : null, 'No limit'))}
      <div class="flex items-center gap-3" style="margin-top:16px">${btn('Save changes', 'body-save', 'primary')}${status}</div>`;
  }

  function field(label, hint, control) {
    return `<div><div class="flex items-baseline gap-2" style="margin-bottom:5px"><span style="font-size:11.5px;color:${C.ink};font-family:Inter;font-weight:500">${label}</span>${hint ? `<span style="font-size:11px;color:${C.faint}">${hint}</span>` : ''}</div>${control}</div>`;
  }
  function input(val, ph, wide, disabled) {
    return `<input value="${val || ''}" placeholder="${ph || ''}" ${disabled ? 'disabled' : ''} style="background:${C.bg2};color:${C.ink};border:1px solid ${C.line};border-radius:3px;padding:6px 8px;font-family:'JetBrains Mono';font-size:11.5px;width:${wide ? '100%' : '220px'};outline:none">`;
  }
  function upstreamPanel() {
    const u = state.upstream;
    if (!u.loaded) {
      if (!u.error && !u.loading) {
        u.loading = true;
        if (typeof window.hslLoadUpstream === 'function') setTimeout(() => window.hslLoadUpstream(), 0);
        else { u.loading = false; u.error = 'Backend bridge unavailable.'; }
      }
      return `<div style="font-size:12px;color:${u.error ? C.danger : C.dim};padding:20px 2px">${esc(u.error || 'Loading upstream proxy settings…')}${u.error ? `<div style="margin-top:10px">${btn('Retry', 'upstream-reload', 'ghost')}</div>` : ''}</div>`;
    }
    const uri = String(u.host || '').replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
    const bypass = String((u.noProxy || []).join(', ')).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
    const status = u.error ? `<span style="color:${C.danger}">${esc(u.error)}</span>` : u.saving ? '<span>Applying…</span>' : u.dirty ? `<span style="color:${C.warn}">Not applied yet</span>` : u.saved ? `<span style="color:${C.mint}">Saved ✓</span>` : '';
    return `<div class="grid gap-[14px]">
    <div style="font-size:12px;color:${C.dim};line-height:1.6">Route outgoing traffic through a corporate proxy. HttpStackLens can handle NTLM / Negotiate auth on your behalf so apps that can't speak it still reach the outside world.</div>
      ${field('Upstream proxy', 'Include scheme and port, for example http://proxy:8080', `<div class="flex gap-[6px]"><input data-upstream-uri value="${uri}" placeholder="http://proxy.corp.local:8080" style="background:${C.bg2};color:${C.ink};border:1px solid ${C.line};border-radius:3px;padding:6px 8px;font-family:'JetBrains Mono';font-size:11.5px;width:100%;outline:none" ${u.on ? '' : 'disabled'}>${toggle(u.on, 'upstream-toggle')}</div>`)}
      ${field('Bypass hosts', 'Comma-separated hosts, IPs or suffixes', `<input data-upstream-no-proxy value="${bypass}" placeholder="localhost, .corp.local, 10.0.0.1" style="background:${C.bg2};color:${C.ink};border:1px solid ${C.line};border-radius:3px;padding:6px 8px;font-family:'JetBrains Mono';font-size:11.5px;width:100%;outline:none">`)}
      <div style="padding:14px;background:${C.bg2};border:1px solid ${C.line};border-radius:4px">
        <div class="flex items-center gap-[10px]" style="margin-bottom:10px">
            <span style="font-size:16px">⊞</span>
            <div class="flex-1">
                <div style="font-size:12.5px;color:${C.ink};font-weight:600">NTLM / Negotiate auth</div>
                <div style="font-size:11px;color:${C.dim};margin-top:2px">Windows only · use current Windows session to authenticate</div>
            </div>
            ${toggle(u.ntlm, 'ntlm-toggle', false, !u.on)}</div>
        </div>
      ${u.dirty ? `<div style="padding:9px 11px;background:${C.warn}12;border:1px solid ${C.warn}40;border-radius:4px;color:${C.ink};font-size:11.5px">These settings are a draft. Applying persists them to <code>config.yaml</code>; restart HttpStackLens to update the active proxy pipeline.</div>` : ''}
      <div class="flex items-center gap-2">${btn('Apply', 'upstream-apply', 'primary')}<span style="font-size:11.5px;margin-left:4px">${status}</span></div>
</div>`;
  }
  function accessPanel() {
    const a = state.access;
    if (!a.loaded) {
      if (!a.error && !a.loading) {
        a.loading = true;
        if (typeof window.hslLoadAccessControl === 'function') setTimeout(() => window.hslLoadAccessControl(), 0);
        else { a.loading = false; a.error = 'Backend bridge unavailable.'; }
      }
      return `<div style="font-size:12px;color:${a.error ? C.danger : C.dim};padding:20px 2px">${esc(a.error || 'Loading access control settings…')}${a.error ? `<div style="margin-top:10px">${btn('Retry', 'access-reload', 'ghost')}</div>` : ''}</div>`;
    }
    const radio = (mode, title, sub, danger) => `<button data-action="access-mode:${mode}" class="flex items-center gap-3 w-full text-left" style="padding:10px 12px;background:${a.mode === mode ? C.bg3 : C.bg2};border:1px solid ${a.mode === mode ? (danger ? C.danger : C.mint) : C.line};border-radius:4px;cursor:pointer;color:${C.ink};font-family:Inter">
      <span style="width:14px;height:14px;border-radius:7px;flex-shrink:0;border:1.5px solid ${a.mode === mode ? (danger ? C.danger : C.mint) : C.faint};display:inline-flex;align-items:center;justify-content:center">${a.mode === mode ? `<span style="width:6px;height:6px;border-radius:3px;background:${danger ? C.danger : C.mint}"></span>` : ''}</span>
      <div class="flex-1"><div style="font-size:12.5px;font-weight:500;color:${danger && a.mode === mode ? C.danger : C.ink}">${title}</div><div style="font-size:11px;color:${C.dim};margin-top:2px">${sub}</div></div></button>`;
    let networks = '';
    if (a.mode === 'allowlist') {
      const rows = a.networks.length
        ? a.networks.map((network, i) => `<div class="flex items-center gap-[8px]" style="padding:7px 10px;border-bottom:1px solid ${C.lineSoft};background:${C.bg2}"><input data-access-network="${i}" value="${String(network || '').replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;')}" placeholder="127.0.0.1 or 192.168.1.0/24" style="font-family:'JetBrains Mono';font-size:11.5px;color:${C.ink};background:${C.bg1};border:1px solid ${C.line};border-radius:3px;padding:5px 7px;flex:1;outline:none"><button data-action="access-remove-network:${i}" title="Remove network" style="background:transparent;border:1px solid ${C.line};color:${C.dim};border-radius:3px;cursor:pointer;height:27px;width:30px">×</button></div>`).join('')
        : `<div style="padding:10px;color:${C.warn};font-size:11.5px;background:${C.bg2}">Add at least one IP address or CIDR network.</div>`;
      networks = `<div><div style="font-size:11.5px;color:${C.dim};margin-bottom:6px;font-family:Inter">Allowed IP addresses / networks</div><div style="border:1px solid ${C.line};border-radius:4px;overflow:hidden">${rows}<div style="padding:8px;background:${C.bg1}">${btn('+ Add network', 'access-add-network', 'ghost')}</div></div><div style="font-size:11px;color:${C.faint};margin-top:6px">Localhost is always allowed over IPv4 and IPv6 to prevent locking you out of this UI.</div></div>`;
    }
    const status = a.error ? `<span style="color:${C.danger}">${esc(a.error)}</span>` : a.saving ? '<span>Applying…</span>' : a.dirty ? `<span style="color:${C.warn}">Not applied yet</span>` : a.saved ? `<span style="color:${C.mint}">Applied ✓</span>` : '';
    return `<div class="grid gap-[14px]">
      <div style="font-size:12px;color:${C.dim};line-height:1.6">Control which machines can connect to this proxy. By default only loopback (127.0.0.1) is accepted — safer when the machine is on an untrusted network.</div>
      <div class="grid gap-2">${radio('loopback', 'Loopback only', '127.0.0.1 and ::1 · recommended')}${radio('lan', 'Private LAN', 'RFC 1918 — 10/8 · 172.16/12 · 192.168/16')}${radio('allowlist', 'Explicit allowlist', 'Only the networks below')}${radio('open', 'Open — any source', 'Dangerous on untrusted networks', true)}</div>
      ${networks}
      ${a.dirty ? `<div style="padding:9px 11px;background:${C.warn}12;border:1px solid ${C.warn}40;border-radius:4px;color:${C.ink};font-size:11.5px">These changes are only a draft. They will not affect active connections until you click <b>Apply</b>.</div>` : ''}
      <div class="flex items-center gap-2">${btn('Apply', 'access-apply', 'primary')}${a.dirty ? btn('Revert', 'access-revert', 'ghost') : ''}<span style="font-size:11.5px;margin-left:4px">${status}</span></div>
      <div style="font-size:11px;color:${C.dim};font-family:'JetBrains Mono';padding:8px 10px;background:${C.bg2};border-radius:3px;border:1px solid ${C.line}">mode <span style="color:${C.ink}">${a.mode}</span> · applies to proxy and Web UI</div></div>`;
  }

  // ─── capture control ─────────────────────────────────────
  // Real requests arrive over SSE (pushed in by WASM). The toolbar just relays
  // pause / resume / clear to the backend through WASM; the authoritative
  // capturing flag comes back via the capture_state event → setCaptureState.
  function captureAction(action) {
    const fn = window.hslCapture;
    if (typeof fn === 'function') fn(action);
  }

  function proxyAction(action) {
    const fn = window.hslProxy;
    if (typeof fn === 'function') fn(action);
  }

  function decryptHttps(enabled) {
    const fn = window.hslDecryptHttps;
    if (typeof fn === 'function') fn(enabled);
  }

  function setAccessMode(accessMode) {
    const fn = window.hslSaveAccessControl;
    if (typeof fn === 'function') fn(JSON.stringify({
      mode: accessMode.mode,
      networks: accessMode.networks || [],
    }));
  }

  function saveUpstream() {
    const u = state.upstream;
    const fn = window.hslSaveUpstream;
    if (typeof fn === 'function') fn(JSON.stringify({
      output_proxy_uri: u.on ? u.host.trim() : '',
      no_proxy: (u.noProxy || []).map((v) => v.trim()).filter(Boolean),
      add_windows_authentication: !!u.ntlm,
    }));
    else { u.saving = false; u.error = 'Backend bridge unavailable.'; renderSettings(); }
  }

  // ─── body capture settings (B5.1) ────────────────────────
  // Serializes the editor state into the /api/settings/body-capture contract and
  // hands it to WASM. Validates locally first so obvious mistakes get a friendly
  // message instead of a round-trip 400.
  function saveBodyCapture() {
    const bc = state.bodyCapture;
    for (const r of bc.mimeTypes) {
      const name = (r.name || '').trim();
      if (!name) { bc.error = 'Every rule needs a MIME type (e.g. application/json).'; bc.saved = false; renderSettings(); return; }
      if (!name.includes('/')) { bc.error = `"${name}" is not a MIME type — use a "type/subtype" form or a wildcard like image/*.`; bc.saved = false; renderSettings(); return; }
    }
    const dto = {
      mime_types: bc.mimeTypes.map((r) => {
        const rule = { name: r.name.trim() };
        if (typeof r.maxSizeBytes === 'number') rule.max_size_bytes = r.maxSizeBytes;
        return rule;
      }),
    };
    if (typeof bc.defaultMaxBytes === 'number') dto.default_max_bytes = bc.defaultMaxBytes;
    bc.saving = true; bc.error = null; bc.saved = false;
    renderSettings();
    const fn = window.hslSaveBodyCapture;
    if (typeof fn === 'function') fn(JSON.stringify(dto));
    else { bc.saving = false; bc.error = 'Backend bridge unavailable.'; renderSettings(); }
  }

  // ─── event delegation ────────────────────────────────────
  function wire() {
    document.addEventListener('pointerdown', (e) => {
      const handle = e.target.closest('[data-detail-resizer]');
      if (!handle) return;
      e.preventDefault();
      const wrap = $('#detail-wrap');
      const parent = wrap.parentElement;
      const startY = e.clientY;
      const startHeight = wrap.getBoundingClientRect().height;
      const minHeight = 140;
      const maxHeight = Math.max(minHeight, parent.clientHeight - 150);
      handle.setPointerCapture(e.pointerId);
      document.body.style.cursor = 'row-resize';
      document.body.style.userSelect = 'none';

      const move = (event) => {
        state.detailHeight = Math.round(Math.min(maxHeight, Math.max(minHeight, startHeight - (event.clientY - startY))));
        wrap.style.flex = `0 0 ${state.detailHeight}px`;
        $('#list-region').style.flex = '1 1 auto';
      };
      const stop = () => {
        handle.removeEventListener('pointermove', move);
        handle.removeEventListener('pointerup', stop);
        handle.removeEventListener('pointercancel', stop);
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
        try { localStorage.setItem('hsl-detail-height', String(state.detailHeight)); } catch (err) {}
      };
      handle.addEventListener('pointermove', move);
      handle.addEventListener('pointerup', stop);
      handle.addEventListener('pointercancel', stop);
    });

    document.addEventListener('dblclick', (e) => {
      if (!e.target.closest('[data-detail-resizer]')) return;
      state.detailHeight = null;
      try { localStorage.removeItem('hsl-detail-height'); } catch (err) {}
      renderDetail();
    });

    document.addEventListener('click', (e) => {
      const row = e.target.closest('[data-row]');
      if (row) { const id = Number(row.dataset.row); state.selId = state.selId === id ? null : id; renderList(); renderDetail(); return; }

      const host = e.target.closest('[data-host]');
      if (host) { $('#filter').value = host.dataset.host; state.filter = host.dataset.host; renderList(); return; }

      const side = e.target.closest('[data-side]');
      if (side) { state.sidebar = side.dataset.side; renderList(); return; }

      const capture = e.target.closest('[data-open-capture]');
      if (capture) { openCapture(decodeURIComponent(capture.dataset.openCapture)); return; }

      const tab = e.target.closest('[data-tab]');
      if (tab) { state.detailTab = tab.dataset.tab; renderDetail(); return; }

      const bm = e.target.closest('[data-bodymode]');
      if (bm) { state.bodyMode = bm.dataset.bodymode; renderDetail(); return; }

      const focus = e.target.closest('[data-focus]');
      if (focus) {
        state.detailFocus = state.detailFocus === focus.dataset.focus ? null : focus.dataset.focus;
        renderDetail();
        return;
      }

      const settab = e.target.closest('[data-settab]');
      if (settab) { settingsTab = settab.dataset.settab; renderSettings(); return; }

      const acc = e.target.closest('[data-access]');
      if (acc) { state.access.mode = acc.dataset.access; renderSettings(); renderToolbar(); renderStatusBar(); return; }

      const act = e.target.closest('[data-action]');
      if (act) { handleAction(act.dataset.action); return; }

      if (e.target.closest('.modal-backdrop') && !e.target.closest('.modal-card')) closeModal();
    });

    // live edits inside the body-capture panel (B5.1). Selects and name inputs
    // mutate state without a re-render so focus/caret survive; add/remove/save
    // re-render explicitly. `input` covers typing, `change` covers select menus.
    const bcMarkDirty = () => { state.bodyCapture.dirty = true; state.bodyCapture.saved = false; state.bodyCapture.error = null; };
    const bcSize = (v) => (v === '' ? null : Number(v));
    document.addEventListener('change', (e) => {
      const max = e.target.closest('[data-bc-max]');
      if (max) { const r = state.bodyCapture.mimeTypes[Number(max.dataset.bcMax)]; if (r) { r.maxSizeBytes = bcSize(max.value); bcMarkDirty(); } return; }
      const def = e.target.closest('[data-bc-default]');
      if (def) { state.bodyCapture.defaultMaxBytes = bcSize(def.value); bcMarkDirty(); return; }
    });
    document.addEventListener('input', (e) => {
      const name = e.target.closest('[data-bc-name]');
      if (name) { const r = state.bodyCapture.mimeTypes[Number(name.dataset.bcName)]; if (r) { r.name = name.value; bcMarkDirty(); } }
      const network = e.target.closest('[data-access-network]');
      if (network) {
        state.access.networks[Number(network.dataset.accessNetwork)] = network.value;
        state.access.dirty = true; state.access.saved = false; state.access.error = null;
      }
      const upstreamUri = e.target.closest('[data-upstream-uri]');
      if (upstreamUri) { state.upstream.host = upstreamUri.value; state.upstream.dirty = true; state.upstream.saved = false; state.upstream.error = null; }
      const upstreamNoProxy = e.target.closest('[data-upstream-no-proxy]');
      if (upstreamNoProxy) { state.upstream.noProxy = upstreamNoProxy.value.split(','); state.upstream.dirty = true; state.upstream.saved = false; state.upstream.error = null; }
    });

    $('#filter').addEventListener('input', (e) => { state.filter = e.target.value; renderList(); });
    $('#filter-clear').addEventListener('click', () => { $('#filter').value = ''; state.filter = ''; renderList(); });
    $('#theme-select').addEventListener('change', (e) => applyTheme(e.target.value));

    $$('[data-density]').forEach((b) => b.addEventListener('click', () => {
      state.density = b.dataset.density;
      try { localStorage.setItem('hsl-density', state.density); } catch (e) {}
      renderDensityButtons();
      renderList(); renderDetail();
    }));

  }

  function handleAction(a) {
    if (a.startsWith('access-mode:')) {
      state.access.mode = a.slice('access-mode:'.length);
      state.access.dirty = true; state.access.saved = false; state.access.error = null;
      renderSettings();
      return;
    }
    if (a.startsWith('access-remove-network:')) {
      state.access.networks.splice(Number(a.slice('access-remove-network:'.length)), 1);
      state.access.dirty = true; state.access.saved = false; state.access.error = null;
      renderSettings();
      return;
    }
    if (a.startsWith('body-remove-rule:')) {
      const i = Number(a.split(':')[1]);
      state.bodyCapture.mimeTypes.splice(i, 1);
      state.bodyCapture.dirty = true; state.bodyCapture.saved = false; state.bodyCapture.error = null;
      renderSettings();
      return;
    }
    switch (a) {
      case 'body-add-rule':
        state.bodyCapture.mimeTypes.push({ name: '', maxSizeBytes: null });
        state.bodyCapture.dirty = true; state.bodyCapture.saved = false; state.bodyCapture.error = null;
        renderSettings();
        break;
      case 'body-save':
        saveBodyCapture();
        break;
      case 'body-reload':
        state.bodyCapture.error = null;
        renderSettings();
        break;
      case 'access-add-network':
        state.access.networks.push(''); state.access.dirty = true; state.access.saved = false; state.access.error = null; renderSettings();
        setTimeout(() => { const inputs = $$('[data-access-network]'); if (inputs.length) inputs[inputs.length - 1].focus(); }, 0);
        break;
      case 'access-apply': {
        if (!state.access.dirty || state.access.saving) break;
        const networks = state.access.networks.map((n) => n.trim());
        if (state.access.mode === 'allowlist' && (!networks.length || networks.some((n) => !n))) {
          state.access.error = 'Allowlist mode requires at least one non-empty IP address or CIDR network.'; renderSettings(); break;
        }
        const warning = state.access.mode === 'open'
          ? 'Apply OPEN access to both the proxy and Web UI? Any machine that can reach this computer may connect.'
          : state.access.mode === 'allowlist'
            ? 'Apply this allowlist to both the proxy and Web UI? Remote clients not included will disconnect. Localhost remains allowed over IPv4 and IPv6.'
            : state.access.mode === 'loopback'
              ? 'Apply loopback-only access to both the proxy and Web UI? Any current remote session will disconnect immediately.'
              : 'Apply private-LAN access to both the proxy and Web UI? Clients outside private networks will disconnect.';
        if (!window.confirm(warning)) break;
        state.access.networks = networks.filter(Boolean); state.access.saving = true; state.access.error = null; state.access.saved = false; renderSettings(); setAccessMode(state.access);
        break;
      }
      case 'access-revert':
        state.access.mode = state.access.appliedMode;
        state.access.networks = state.access.appliedNetworks.slice();
        state.access.dirty = false; state.access.saved = false; state.access.error = null; renderSettings();
        break;
      case 'access-reload': state.access.error = null; state.access.loaded = false; renderSettings(); break;
      case 'toggle-capture':
        captureAction(state.capturing ? 'stop' : 'start');
        break;
      case 'toggle-proxy':
        proxyAction(state.proxyRunning ? 'stop' : 'start');
        break;
      case 'clear':
        if (state.source.kind !== 'live') break;
        captureAction('clear');
        state.liveRows = []; state.rows = state.liveRows; state.selId = null; renderList(); renderDetail();
        break;
      case 'back-to-live': backToLive(); break;
      case 'refresh-captures': loadCaptureFiles(); break;
      case 'toggle-decrypt':
        if (state.decryption) decryptHttps(false);
        else openCert();
        break;
      case 'open-upstream': openSettings('upstream'); break;
      case 'open-access': openSettings('access'); break;
      case 'open-settings': openSettings('body'); break;
      case 'close-modal': closeModal(); break;
      case 'close-detail': state.selId = null; renderList(); renderDetail(); break;
      case 'cert-generate':
        cert.step = 1; renderCert(); certificateAction('generate');
        break;
      case 'cert-regenerate': certificateAction('regenerate'); break;
      case 'cert-cleanup':
        if (!window.confirm('Remove all HttpStackLens certificates from the OS trust store and delete the local CA files and per-domain certificates folder? HTTPS decryption will need a fresh CA afterwards.')) break;
        state.certificate.cleanup = null; certificateAction('cleanup');
        break;
      case 'cert-install': certificateAction('install'); break;
      case 'cert-refresh': loadCertificateStatus(true); renderSettings(); break;
      case 'cert-export': window.location.assign('/api/certificates/ca/export'); break;
      case 'cert-done': decryptHttps(true); closeModal(); break;
      case 'upstream-toggle': state.upstream.on = !state.upstream.on; state.upstream.dirty = true; state.upstream.saved = false; state.upstream.error = null; renderSettings(); break;
      case 'ntlm-toggle': if (state.upstream.on) { state.upstream.ntlm = !state.upstream.ntlm; state.upstream.dirty = true; state.upstream.saved = false; state.upstream.error = null; renderSettings(); } break;
      case 'upstream-apply':
        if (!state.upstream.dirty || state.upstream.saving) break;
        if (state.upstream.on && !state.upstream.host.trim()) { state.upstream.error = 'Enter an upstream proxy URL before enabling it.'; renderSettings(); break; }
        state.upstream.saving = true; state.upstream.error = null; state.upstream.saved = false; renderSettings(); saveUpstream();
        break;
      case 'upstream-reload': state.upstream.error = null; state.upstream.loaded = false; renderSettings(); break;
      default: break;
    }
  }

  // ─── theme ───────────────────────────────────────────────
  function setThemeVariables(colors) {
    const root = document.documentElement;
    root.style.colorScheme = themeName.endsWith('-dark') || themeName === 'dark' ? 'dark' : 'light';
    const variables = {
      '--desk': colors.desk, '--panel': colors.bg1, '--sidebar': colors.sidebar,
      '--bg0': colors.bg0, '--bg2': colors.bg2, '--bg3': colors.bg3,
      '--line': colors.line, '--lineSoft': colors.lineSoft,
      '--ink': colors.ink, '--dim': colors.dim, '--faint': colors.faint,
      '--mint': colors.mint, '--warn': colors.warn, '--danger': colors.danger,
      '--info': colors.info, '--pink': colors.pink, '--success': colors.success,
      '--pill-bg': colors.bg1, '--pill-border': colors.line,
      '--pill-shadow': themeName.endsWith('-dark') || themeName === 'dark' ? 'none' : '0 1px 2px rgba(0,0,0,.04)',
      '--scroll-thumb': colors.scroll, '--scroll-thumb-hover': colors.scrollHover,
      '--brand-shadow': `0 2px 8px ${colors.mint}55`,
    };
    Object.entries(variables).forEach(([name, value]) => root.style.setProperty(name, value));
  }

  function applyTheme(name) {
    if (!PALETTES[name]) return;
    themeName = name;
    C = PALETTES[name];
    METHOD_COLOR = methodColors(C);
    document.documentElement.dataset.theme = name;
    setThemeVariables(C);
    try { localStorage.setItem('hsl-theme', name); } catch (e) {}
    const select = $('#theme-select'); if (select) select.value = name;
    renderDensityButtons();
    renderToolbar(); renderList(); renderDetail(); renderStatusBar();
    if (modalKind === 'settings') renderSettings();
    else if (modalKind === 'cert') renderCert();
  }

  // ─── capture state (from SSE capture_state, via WASM) ────
  function setCaptureState(s) {
    if (!s) return;
    if (typeof s.recording === 'boolean') state.capturing = s.recording;
    else if (typeof s.capturing === 'boolean') state.capturing = s.capturing;
    if (s.proxy && typeof s.proxy.running === 'boolean') state.proxyRunning = s.proxy.running;
    if (s.proxy && typeof s.proxy.address === 'string') state.proxyAddress = s.proxy.address;
    // decrypt / upstream / access come from the backend (F3.2); the status bar
    // and toolbar reflect the real pipeline state rather than local toggles.
    if (s.decrypt && typeof s.decrypt.enabled === 'boolean') state.decryption = s.decrypt.enabled;
    if (s.upstream && !state.upstream.dirty) {
      if (typeof s.upstream.enabled === 'boolean') state.upstream.on = s.upstream.enabled;
      if (typeof s.upstream.ntlm === 'boolean') state.upstream.ntlm = s.upstream.ntlm;
    }
    if (s.access && s.access.mode && !state.access.dirty) state.access.mode = s.access.mode;
    renderToolbar();
    renderStatusBar();
  }

  // ─── body capture settings result (from /api/settings/body-capture) ────
  function setBodyCapture(s) {
    const bc = state.bodyCapture;
    bc.saving = false;
    bc.loading = false;
    if (!s) return;
    if (s.error) { bc.error = s.error; bc.saved = false; if (modalKind === 'settings' && settingsTab === 'body') renderSettings(); return; }
    // A load/save result replaces the editor state with the server's normalized view.
    if (s.loaded || 'mimeTypes' in s) {
      bc.loaded = true;
      bc.error = null;
      bc.defaultMaxBytes = (typeof s.defaultMaxBytes === 'number') ? s.defaultMaxBytes : null;
      bc.mimeTypes = Array.isArray(s.mimeTypes)
        ? s.mimeTypes.map((r) => ({ name: r.name || '', maxSizeBytes: (typeof r.maxSizeBytes === 'number') ? r.maxSizeBytes : null }))
        : [];
      bc.dirty = false;
    }
    bc.saved = !!s.saved;
    if (modalKind === 'settings' && settingsTab === 'body') renderSettings();
  }

  function setCertificate(s) {
    const ca = state.certificate;
    ca.loading = false; ca.busy = false; ca.action = '';
    if (!s) return;
    if (s.requestError) { ca.error = s.requestError; if (modalKind === 'cert') cert.step = 0; }
    else { ca.loaded = true; ca.status = s; ca.error = null; if (s.available && modalKind === 'cert') cert.step = 2; }
    if (modalKind === 'cert') renderCert();
    else if (modalKind === 'settings' && settingsTab === 'cert') renderSettings();
  }

  function setCertificateCleanup(s) {
    state.certificate.cleanup = s || null;
    if (modalKind === 'settings' && settingsTab === 'cert') renderSettings();
  }

  function setAccessControl(s) {
    const a = state.access;
    a.loading = false; a.saving = false;
    if (!s) return;
    if (s.error) a.error = s.error;
    else {
      a.loaded = true; a.error = null; a.mode = s.mode || 'loopback';
      a.networks = Array.isArray(s.networks) ? s.networks.slice() : [];
      a.appliedMode = a.mode; a.appliedNetworks = a.networks.slice();
      a.dirty = false; a.saved = !!s.saved;
    }
    if (modalKind === 'settings' && settingsTab === 'access') renderSettings();
  }

  function setUpstream(s) {
    const u = state.upstream;
    u.loading = false; u.saving = false;
    if (!s) return;
    if (s.error) u.error = s.error;
    else {
      u.loaded = true; u.error = null; u.host = s.outputProxyUri || '';
      u.on = !!u.host; u.noProxy = Array.isArray(s.noProxy) ? s.noProxy.slice() : [];
      u.ntlm = !!s.ntlm; u.dirty = false; u.saved = !!s.saved;
    }
    renderToolbar(); renderStatusBar();
    if (modalKind === 'settings' && settingsTab === 'upstream') renderSettings();
  }

  // ─── detail / body results (from /api/..., via WASM) ─────
  function setDetail(correlationId, detail) {
    const row = state.liveRows.find((x) => x.correlationId === correlationId);
    if (!row) return;
    row.detail = detail || { error: 'Detail not found.' };
    if (row.id === state.selId) renderDetail();
  }
  function setBody(correlationId, side, body) {
    const row = state.liveRows.find((x) => x.correlationId === correlationId);
    if (!row) return;
    row.bodies[side] = Object.assign({ loaded: true, loading: false }, body || {});
    if (row.id === state.selId && (state.detailTab === 'body' || state.detailTab === 'raw')) renderDetail();
  }

  // ─── boot ────────────────────────────────────────────────
  function boot() {
    document.documentElement.dataset.theme = themeName;
    setThemeVariables(C);
    $('#theme-select').value = themeName;
    wire();
    renderDensityButtons();
    renderToolbar();
    renderList();
    renderDetail();
    loadCaptureFiles();
    loadRuntimeStats();
    loadBuildInfo();
    loadUpdateCheck();
    window.setInterval(loadRuntimeStats, 5000);
  }

  // Contract with the WASM layer: WASM pushes data in through these; the row
  // templates and all rendering stay here in JS (easier to maintain).
  window.HttpStackLensMockup = {
    appendRow: appendExternalRow,       // request_occurred → new row
    updateRow: updateExternalRow,       // response_occurred → complete the row
    setDetail: setDetail,               // /api/requests/{id} result
    setBody: setBody,                   // /api/requests/{id}/body result
    setCaptureState: setCaptureState,   // capture_state event
    setBodyCapture: setBodyCapture,     // /api/settings/body-capture result
    setCertificate: setCertificate,     // B6 CA status/actions
    setCertificateCleanup: setCertificateCleanup, // certificate cleanup summary
    setAccessControl: setAccessControl, // B5.3 access settings
    setUpstream: setUpstream,           // B5.2 upstream settings
    clear: () => {
      state.liveRows = [];
      if (state.source.kind === 'live') { state.rows = state.liveRows; state.selId = null; renderList(); renderDetail(); }
      else renderCaptureSessions();
    },
    rowHTML: (r) => rowHTML(normalizeExternalRow(r)),
  };

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
  else boot();
})();
