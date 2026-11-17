/* 租约管理：列表卡片、本地 TTL 倒计时、续期、吊销、创建 */
(function () {
  'use strict';
  const { $, esc, get, post, del, toast, confirmDialog, fmt } = window.App;

  let leases = [];
  let tickTimer = null;
  let refreshTimer = null;
  let lastFetch = 0;

  const REFRESH_INTERVAL_MS = 10000; // 每 10s 从服务端校准 TTL / 租约增删

  async function refresh() {
    try {
      leases = (await get('/api/leases')).items || [];
      lastFetch = Date.now();
      render();
    } catch (e) {
      $('#leaseGrid').innerHTML = `<div class="lease-card"><p class="modal-tip">加载失败：${esc(e.message)}</p></div>`;
    }
  }

  function startAutoRefresh() {
    stopAutoRefresh();
    refresh();
    refreshTimer = setInterval(refresh, REFRESH_INTERVAL_MS);
  }
  function stopAutoRefresh() {
    if (refreshTimer) { clearInterval(refreshTimer); refreshTimer = null; }
  }

  function render() {
    const grid = $('#leaseGrid');
    if (!leases.length) {
      grid.innerHTML = '<div class="lease-card"><p class="modal-tip">当前没有租约</p></div>';
      return;
    }
    grid.innerHTML = leases.map((l) => {
      const pct = l.grantedTtl > 0 ? Math.max(4, Math.min(100, (l.ttl / l.grantedTtl) * 100)) : 0;
      return `
      <div class="lease-card" data-id="${l.id}">
        <div class="lc-id mono">Lease #${l.id}</div>
        <div class="lc-ttl"><span class="ttl-text">${fmt.duration(l.ttl)}</span></div>
        <small style="color:var(--text-3)">剩余 <span class="ttl-sec">${l.ttl}</span> 秒 / 授予 ${l.grantedTtl} 秒</small>
        <div class="lc-bar"><i style="width:${pct}%"></i></div>
        <div class="lc-keys">绑定键 ${l.keys.length} 个：
          ${l.keys.length ? l.keys.slice(0, 3).map((k) => `<div class="mono">• ${esc(fmt.truncate(k, 48))}</div>`).join('') + (l.keys.length > 3 ? `<div>…另 ${l.keys.length - 3} 个</div>` : '') : '无'}
        </div>
        <div class="lc-actions">
          <button class="btn primary sm" data-act="keep" data-id="${l.id}">续期一次</button>
          <button class="btn danger sm" data-act="revoke" data-id="${l.id}">吊销</button>
        </div>
      </div>`;
    }).join('');

    grid.querySelectorAll('button[data-act]').forEach((btn) => {
      // 租约 ID 为 int64 字符串，禁止 Number 转换，否则会丢失精度。
      btn.addEventListener('click', () => act(btn.dataset.act, btn.dataset.id));
    });
  }

  // 本地每秒递减显示，避免高频打后端；下次刷新时以服务端 TTL 校准。
  function startTicker() {
    if (tickTimer) return;
    tickTimer = setInterval(() => {
      if (!leases.length) return;
      leases.forEach((l) => { if (l.ttl > 0) l.ttl -= 1; });
      $('#leaseGrid').querySelectorAll('.lease-card').forEach((card) => {
        const l = leases.find((x) => String(x.id) === card.dataset.id);
        if (!l) return;
        const sec = card.querySelector('.ttl-sec');
        const txt = card.querySelector('.ttl-text');
        const bar = card.querySelector('.lc-bar i');
        if (sec) sec.textContent = Math.max(0, l.ttl);
        if (txt) txt.textContent = fmt.duration(Math.max(0, l.ttl));
        if (bar && l.grantedTtl > 0) bar.style.width = Math.max(4, Math.min(100, (l.ttl / l.grantedTtl) * 100)) + '%';
      });
    }, 1000);
  }

  async function act(action, id) {
    if (action === 'revoke') {
      const ok = await confirmDialog('确认吊销该租约？', '吊销后绑定该租约的所有键会被 etcd 立即删除。', '吊销租约');
      if (!ok) return;
      try {
        await del('/api/leases/' + id);
        toast('租约已吊销', 'success');
      } catch (e) { toast(e.message, 'error'); return; }
    } else {
      try {
        await post('/api/leases/' + id + '/keepalive', {});
        toast('续期成功', 'success');
      } catch (e) { toast(e.message, 'error'); return; }
    }
    refresh();
  }

  function bind() {
    $('#leaseGrantBtn').addEventListener('click', async () => {
      const ttl = Number($('#leaseTTL').value);
      if (!ttl || ttl <= 0) { toast('请输入大于 0 的 TTL（秒）', 'error'); return; }
      try {
        const res = await post('/api/leases', { ttl });
        toast(`租约 #${res.id} 创建成功`, 'success');
        $('#leaseTTL').value = '';
        refresh();
      } catch (e) { toast(e.message, 'error'); }
    });
    $('#leaseRefreshBtn').addEventListener('click', refresh);
    startTicker();

    // 进入租约页启动定时刷新，离开停止，避免后台空转打后端
    document.addEventListener('view:enter', (e) => {
      if (e.detail === 'lease') startAutoRefresh();
    });
    document.addEventListener('view:leave', (e) => {
      if (e.detail === 'lease') stopAutoRefresh();
    });
  }

  window.App.pages.lease = { refresh, bind };
})();
