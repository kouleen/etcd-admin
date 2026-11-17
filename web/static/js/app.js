/* 主入口：主题、导航切换、连接状态轮询、移动端抽屉、各页面装配 */
(function () {
  'use strict';
  const { $, $$, get, post, toast, confirmDialog, Theme } = window.App;

  const TITLES = {
    dashboard: '仪表盘',
    kv: '键值浏览',
    watch: '实时监听',
    lease: '租约管理',
    auth: '用户与角色',
    metrics: '指标监控',
    connection: '连接设置'
  };

  let currentView = 'dashboard';

  function switchView(name) {
    if (!window.App.pages[name]) return;
    if (name === currentView) return;

    document.dispatchEvent(new CustomEvent('view:leave', { detail: currentView }));
    currentView = name;

    $$('.nav-item').forEach((b) => b.classList.toggle('active', b.dataset.view === name));
    $$('.view').forEach((v) => {
      const active = v.id === 'view-' + name;
      v.classList.toggle('active', active);
      v.hidden = !active;
    });
    $('#pageTitle').textContent = TITLES[name];
    closeDrawer();

    document.dispatchEvent(new CustomEvent('view:enter', { detail: name }));
    safeRefresh(name);
  }

  async function safeRefresh(name) {
    try {
      await window.App.pages[name].refresh();
    } catch (e) {
      toast('页面加载失败：' + e.message, 'error');
    }
  }

  /* ---------------- 连接状态指示 ---------------- */
  async function updateConnPill() {
    const pill = $('#connPill');
    const text = $('#connText');
    try {
      const d = await get('/api/connection/current');
      const ep = (d.info && d.info.endpoints && d.info.endpoints[0]) || '未知端点';
      if (d.reachable) {
        pill.className = 'conn-pill online';
        text.textContent = '已连接 · ' + ep;
      } else {
        pill.className = 'conn-pill offline';
        text.textContent = '重连中 · ' + ep;
      }
    } catch (e) {
      pill.className = 'conn-pill offline';
      text.textContent = '连接不可用';
    }
  }
  window.App.updateConnPill = updateConnPill;

  /* ---------------- 移动端抽屉 ---------------- */
  function openDrawer() {
    $('.sidebar').classList.add('open');
    const mask = $('#drawerMask');
    mask.hidden = false;
    mask.style.pointerEvents = 'auto';
  }
  function closeDrawer() {
    $('.sidebar').classList.remove('open');
    const mask = $('#drawerMask');
    mask.hidden = true;
    mask.style.pointerEvents = 'none';
  }

  /* ---------------- 断开连接 ---------------- */
  async function doDisconnect() {
    const ok = await confirmDialog(
      '断开当前 etcd 连接？',
      '断开后所有操作将不可用，需要重新在连接入口页建立连接。',
      '断开连接'
    );
    if (!ok) return;
    try {
      await post('/api/connection/disconnect', {});
      toast('已断开 etcd 连接', 'success');
      // 给后端 Router 足够时间感知 Manager.Disconnect 状态
      setTimeout(() => location.replace('/'), 400);
    } catch (e) {
      toast('断开失败：' + e.message, 'error');
    }
  }

  /* ---------------- 启动 ---------------- */
  document.addEventListener('DOMContentLoaded', async () => {
    Theme.init();

    // 核心导航绑定优先注册 —— 即使某个页面模块的 bind() 抛错也不影响基础导航
    $$('.nav-item').forEach((btn) => btn.addEventListener('click', () => switchView(btn.dataset.view)));
    $('#refreshBtn').addEventListener('click', () => safeRefresh(currentView));
    $('#menuBtn').addEventListener('click', openDrawer);
    $('#drawerMask').addEventListener('click', closeDrawer);
    $('#disconnectBtn').addEventListener('click', doDisconnect);

    // 启动前守卫：若后端当前未持有连接，直接把浏览器送到入口页
    try {
      const statusResp = await fetch('/api/status', { cache: 'no-store' });
      if (statusResp.ok) {
        const body = await statusResp.json();
        if (body && body.data && body.data.connected === false) {
          location.replace('/entry');
          return;
        }
      }
    } catch (e) { /* 网络异常继续启动 */ }

    // 装配各页面事件 —— 每个模块独立 try/catch，一个挂不影响其他
    Object.values(window.App.pages).forEach((p) => {
      try { if (p.bind) p.bind(); } catch (e) { console.warn('[page bind failed]', e); }
    });

    // 首屏数据
    safeRefresh('dashboard');
    updateConnPill();
    setInterval(updateConnPill, 10000);
  });
})();
