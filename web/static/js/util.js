/* 公共工具：DOM、请求、Toast、Modal、主题、格式化、高亮、Diff、SVG 图表 */
(function () {
  'use strict';

  const $ = (sel, root) => (root || document).querySelector(sel);
  const $$ = (sel, root) => Array.from((root || document).querySelectorAll(sel));

  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => (
    { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]
  ));

  /* ---------- 统一 API 调用 ---------- */
  async function api(method, url, body) {
    const opt = { method, headers: {} };
    if (body !== undefined) {
      opt.headers['Content-Type'] = 'application/json';
      opt.body = JSON.stringify(body);
    }
    let resp;
    try {
      resp = await fetch(url, opt);
    } catch (e) {
      throw new Error('网络请求失败：' + e.message);
    }
    let data;
    try {
      data = await resp.json();
    } catch (e) {
      throw new Error('服务端返回了非 JSON 响应（HTTP ' + resp.status + '）');
    }
    if (!resp.ok || (data.code !== undefined && data.code !== 0)) {
      throw new Error(data.message || ('HTTP ' + resp.status));
    }
    return data.data;
  }
  const get = (url) => api('GET', url);
  const post = (url, body) => api('POST', url, body || {});
  const put = (url, body) => api('PUT', url, body || {});
  const del = (url, body) => api('DELETE', url, body);

  /* ---------- Toast ---------- */
  function toast(msg, type) {
    const root = $('#toastRoot');
    const el = document.createElement('div');
    el.className = 'toast ' + (type || 'info');
    const ico = { success: '✓', error: '✕', info: 'ℹ' }[type || 'info'] || 'ℹ';
    el.innerHTML = `<span class="t-ico">${ico}</span><span>${esc(msg)}</span>`;
    root.appendChild(el);
    setTimeout(() => {
      el.classList.add('out');
      setTimeout(() => el.remove(), 320);
    }, 3600);
  }

  /* ---------- Modal ---------- */
  function openModal(html, wide) {
    const root = $('#modalRoot');
    const mask = document.createElement('div');
    mask.className = 'modal-mask';
    mask.innerHTML = `<div class="modal${wide ? ' wide' : ''}">${html}</div>`;
    root.appendChild(mask);
    const close = () => mask.remove();
    mask.addEventListener('click', (e) => {
      if (e.target === mask) close();
    });
    mask.querySelectorAll('[data-close]').forEach((b) => b.addEventListener('click', close));
    return { mask, close, el: mask.querySelector('.modal') };
  }

  function confirmDialog(title, desc, okText) {
    return new Promise((resolve) => {
      const m = openModal(`
        <h3>${esc(title)}</h3>
        <p class="modal-tip">${esc(desc)}</p>
        <div class="modal-actions">
          <button class="btn ghost" data-close>取消</button>
          <button class="btn danger" id="cfmOk">${esc(okText || '确认')}</button>
        </div>`);
      m.mask.querySelector('#cfmOk').addEventListener('click', () => { m.close(); resolve(true); });
      m.mask.addEventListener('remove', () => resolve(false));
    });
  }

  /* ---------- 主题 ---------- */
  const Theme = {
    key: 'etcd-console-theme',
    init() {
      const saved = localStorage.getItem(this.key) || 'dark';
      this.apply(saved);
      const btn = $('#themeBtn');
      if (!btn) return;
      btn.addEventListener('click', () => {
        const next = document.body.dataset.theme === 'dark' ? 'light' : 'dark';
        this.apply(next);
        localStorage.setItem(this.key, next);
      });
    },
    apply(theme) {
      document.body.dataset.theme = theme;
      const btn = $('#themeBtn');
      if (!btn) return;
      btn.textContent = theme === 'dark' ? '🌙' : '☀️';
      btn.title = theme === 'dark' ? '切换到浅色主题' : '切换到深色主题';
    }
  };

  /* ---------- 格式化 ---------- */
  const fmt = {
    bytes(n) {
      if (n === undefined || n === null) return '-';
      if (n < 1024) return n + ' B';
      const units = ['KB', 'MB', 'GB', 'TB'];
      let v = n / 1024, i = 0;
      while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
      return v.toFixed(2) + ' ' + units[i];
    },
    time(ts) {
      if (!ts) return '-';
      return new Date(ts).toLocaleString('zh-CN', { hour12: false });
    },
    duration(sec) {
      if (sec <= 0) return '已过期';
      const d = Math.floor(sec / 86400);
      const h = Math.floor((sec % 86400) / 3600);
      const m = Math.floor((sec % 3600) / 60);
      const s = sec % 60;
      if (d > 0) return `${d}天 ${h}时`;
      if (h > 0) return `${h}时 ${m}分`;
      if (m > 0) return `${m}分 ${s}秒`;
      return `${s}秒`;
    },
    truncate(s, n) {
      s = String(s ?? '');
      return s.length > n ? s.slice(0, n) + '…' : s;
    }
  };

  /* ---------- 本地存储 ---------- */
  const store = {
    get(key, fallback) {
      try { const v = localStorage.getItem(key); return v === null ? fallback : JSON.parse(v); }
      catch (e) { return fallback; }
    },
    set(key, val) { localStorage.setItem(key, JSON.stringify(val)); }
  };

  function download(filename, content, mime) {
    const blob = new Blob([content], { type: mime || 'application/json' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = filename;
    a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 2000);
  }

  /* ---------- JSON 美化与高亮 ---------- */
  function prettyJSON(raw) {
    try { return JSON.stringify(JSON.parse(raw), null, 2); }
    catch (e) { return raw; }
  }
  function highlightJSON(raw) {
    const json = prettyJSON(raw);
    if (!json) return '';
    return esc(json)
      .replace(/("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(\.\d+)?([eE][+-]?\d+)?)/g,
        (m) => {
          let cls = 'tok-num';
          if (/^"/.test(m)) cls = /:$/.test(m) ? 'tok-key' : 'tok-str';
          else if (/true|false/.test(m)) cls = 'tok-bool';
          else if (/null/.test(m)) cls = 'tok-null';
          return `<span class="${cls}">${m}</span>`;
        });
  }

  /* ---------- 行级 Diff（LCS） ---------- */
  function lineDiff(oldText, newText) {
    const a = String(oldText ?? '').split('\n');
    const b = String(newText ?? '').split('\n');
    const n = a.length, m = b.length;
    const dp = Array.from({ length: n + 1 }, () => new Int32Array(m + 1));
    for (let i = n - 1; i >= 0; i--) {
      for (let j = m - 1; j >= 0; j--) {
        dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
      }
    }
    const out = [];
    let i = 0, j = 0;
    while (i < n && j < m) {
      if (a[i] === b[j]) { out.push({ type: 'eq', text: a[i] }); i++; j++; }
      else if (dp[i + 1][j] >= dp[i][j + 1]) { out.push({ type: 'del', text: a[i] }); i++; }
      else { out.push({ type: 'add', text: b[j] }); j++; }
    }
    while (i < n) out.push({ type: 'del', text: a[i++] });
    while (j < m) out.push({ type: 'add', text: b[j++] });
    return out;
  }

  /* ---------- 纯 SVG 折线/面积图 ---------- */
  let gradSeq = 0;
  function sparkline(container, points, opts) {
    opts = opts || {};
    if (!points || points.length === 0) {
      container.innerHTML = '<div class="chart-empty">暂无采样数据，等待后端采样…</div>';
      return;
    }
    const w = container.clientWidth || 400;
    const h = container.clientHeight || 180;
    const pad = { l: 48, r: 14, t: 14, b: 26 };
    const iw = w - pad.l - pad.r;
    const ih = h - pad.t - pad.b;
    const vals = points.map((p) => p.value);
    let min = Math.min(...vals), max = Math.max(...vals);
    if (min === max) { min -= 1; max += 1; }
    const gid = 'g' + (++gradSeq);
    const x = (idx) => pad.l + (points.length === 1 ? iw / 2 : (idx / (points.length - 1)) * iw);
    const y = (v) => pad.t + ih - ((v - min) / (max - min)) * ih;

    const linePts = points.map((p, idx) => `${x(idx)},${y(p.value)}`);
    const areaPts = `${pad.l},${pad.t + ih} ${linePts.join(' ')} ${x(points.length - 1)},${pad.t + ih}`;
    const gridY = [0, 0.25, 0.5, 0.75, 1].map((r) => {
      const gy = pad.t + ih * r;
      const gv = max - (max - min) * r;
      return `<line class="spark-grid" x1="${pad.l}" y1="${gy}" x2="${w - pad.r}" y2="${gy}"/>
        <text class="spark-label" x="${pad.l - 6}" y="${gy + 3}" text-anchor="end">${opts.format ? opts.format(gv) : Math.round(gv)}</text>`;
    }).join('');
    const dots = points.map((p, idx) =>
      `<circle class="spark-dot" cx="${x(idx)}" cy="${y(p.value)}" r="2.6"><title>${esc(p.label || '')}: ${opts.format ? opts.format(p.value) : p.value}</title></circle>`
    ).join('');
    const xLabels = [0, Math.floor((points.length - 1) / 2), points.length - 1]
      .filter((v, i, arr) => arr.indexOf(v) === i)
      .map((idx) => `<text class="spark-label" x="${x(idx)}" y="${h - 8}" text-anchor="middle">${esc(points[idx].label || '')}</text>`)
      .join('');

    container.innerHTML = `
      <svg viewBox="0 0 ${w} ${h}" preserveAspectRatio="none">
        <defs>
          <linearGradient id="${gid}" x1="0" y1="0" x2="1" y2="0">
            <stop offset="0%" stop-color="#38bdf8"/><stop offset="100%" stop-color="#6366f1"/>
          </linearGradient>
          <linearGradient id="${gid}f" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stop-color="#38bdf8" stop-opacity=".45"/><stop offset="100%" stop-color="#38bdf8" stop-opacity="0"/>
          </linearGradient>
        </defs>
        ${gridY}
        <polygon class="spark-area" points="${areaPts}" fill="url(#${gid}f)"/>
        <polyline class="spark-line" points="${linePts.join(' ')}" stroke="url(#${gid})"/>
        ${dots}${xLabels}
      </svg>`;
  }

  window.App = {
    pages: {},
    $, $$, esc, api, get, post, put, del,
    toast, openModal, confirmDialog,
    Theme, fmt, store, download,
    prettyJSON, highlightJSON, lineDiff, sparkline
  };
})();
