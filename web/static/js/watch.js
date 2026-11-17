/* 实时监听：EventSource(SSE) 订阅 etcd watch 事件 */
(function () {
  'use strict';
  const { $, esc, toast, fmt } = window.App;

  let es = null;
  let eventCount = 0;

  function setStatus(on, text) {
    const dot = $('#watchDot');
    dot.className = 'dot ' + (on ? 'on' : 'off');
    $('#watchStatusText').textContent = text;
    $('#watchStartBtn').disabled = on;
    $('#watchStopBtn').disabled = !on;
    $('#watchNavDot').hidden = !on;
  }

  function appendEvent(ev) {
    const box = $('#eventStream');
    const empty = box.querySelector('.event-empty');
    if (empty) empty.remove();

    const isPut = ev.type === 'PUT';
    const el = document.createElement('div');
    el.className = 'event-item';
    el.innerHTML = `
      <div class="ev-head">
        <span class="badge ${isPut ? 'put' : 'delete'}">${esc(ev.type)}</span>
        <span class="ev-key mono">${esc(ev.key)}</span>
        <span class="ev-time">${fmt.time(ev.timestamp)}</span>
      </div>
      ${isPut && ev.value ? `<div class="ev-val">${esc(ev.value)}</div>` : ''}
      <div class="ev-meta">modRevision ${ev.modRevision} · version ${ev.version}${ev.lease && ev.lease !== '0' ? ' · lease ' + ev.lease : ''}</div>`;
    box.appendChild(el);

    if ($('#autoScroll').checked) box.scrollTop = box.scrollHeight;
    $('#watchEventCount').textContent = `事件数 ${++eventCount}`;
  }

  function start() {
    const prefix = $('#watchPrefix').value.trim();
    if (es) stop();
    eventCount = 0;
    $('#watchEventCount').textContent = '事件数 0';
    setStatus(true, '连接中…');

    es = new EventSource('/api/watch?prefix=' + encodeURIComponent(prefix));
    es.onmessage = (e) => {
      let ev;
      try { ev = JSON.parse(e.data); } catch (err) { return; }
      if (ev.type === 'CONNECTED') {
        setStatus(true, `监听中：${ev.prefix || '(全量)'}`);
        toast('Watch 连接已建立', 'success');
        return;
      }
      if (ev.type === 'ERROR') {
        toast('Watch 错误：' + (ev.message || ''), 'error');
        stop();
        return;
      }
      appendEvent(ev);
    };
    es.onerror = () => {
      // EventSource 会自动重连；若服务端已关闭则标记离线。
      if (es && es.readyState === EventSource.CLOSED) {
        setStatus(false, '连接已断开');
      } else {
        setStatus(true, '网络抖动，自动重连中…');
      }
    };
  }

  function stop() {
    if (es) {
      es.close();
      es = null;
    }
    setStatus(false, '已停止');
  }

  function refresh() {
    // 进入页面时恢复上一次填写的前缀
    const saved = window.App.store.get('etcd-watch-prefix', '');
    if (saved && !$('#watchPrefix').value) $('#watchPrefix').value = saved;
  }

  function bind() {
    $('#watchStartBtn').addEventListener('click', () => {
      window.App.store.set('etcd-watch-prefix', $('#watchPrefix').value.trim());
      start();
    });
    $('#watchStopBtn').addEventListener('click', stop);
    $('#watchClearBtn').addEventListener('click', () => {
      $('#eventStream').innerHTML = '<div class="event-empty">暂无事件，等待键变更…</div>';
      eventCount = 0;
      $('#watchEventCount').textContent = '事件数 0';
    });
    window.addEventListener('beforeunload', stop);
    // 离开本视图后自动停止，避免后台空占 watcher。
    document.addEventListener('view:leave', (e) => {
      if (e.detail === 'watch' && es) stop();
    });
  }

  window.App.pages.watch = { refresh, bind };
})();
