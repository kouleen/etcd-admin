/* 指标监控：5 秒轮询 overview，纯 SVG 绘制四张时序图 */
(function () {
  'use strict';
  const { $, esc, get, fmt, sparkline } = window.App;

  let timer = null;
  let lastData = null;

  function toPoints(samples, field) {
    return (samples || []).map((s) => ({
      value: s[field],
      label: new Date(s.ts).toLocaleTimeString('zh-CN', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })
    }));
  }

  function renderCards(d) {
    const latest = d.hasLatest ? d.latest : null;
    $('#metricCards').innerHTML = `
      <div class="stat-card metric-card">
        <div class="mc-ico">🔑</div>
        <div><div class="mc-val">${d.keys}</div><div class="mc-label">键总数</div></div>
      </div>
      <div class="stat-card metric-card">
        <div class="mc-ico">⏱</div>
        <div><div class="mc-val">${d.leases}</div><div class="mc-label">租约数量</div></div>
      </div>
      <div class="stat-card metric-card">
        <div class="mc-ico">🖧</div>
        <div><div class="mc-val">${d.members}</div><div class="mc-label">集群成员</div></div>
      </div>
      <div class="stat-card metric-card">
        <div class="mc-ico">${d.alarms ? '⚠️' : '✅'}</div>
        <div><div class="mc-val">${d.alarms}</div><div class="mc-label">活动告警</div></div>
      </div>
      <div class="stat-card metric-card">
        <div class="mc-ico">💾</div>
        <div><div class="mc-val">${latest ? fmt.bytes(latest.dbSize) : '-'}</div><div class="mc-label">DB 大小</div></div>
      </div>
      <div class="stat-card metric-card">
        <div class="mc-ico">⎘</div>
        <div><div class="mc-val">${latest ? latest.revision : '-'}</div><div class="mc-label">当前 Revision</div></div>
      </div>`;

    $('#dbSizeNow').textContent = latest ? fmt.bytes(latest.dbSize) : '';
    $('#revisionNow').textContent = latest ? 'rev ' + latest.revision : '';
    $('#keysNow').textContent = latest ? latest.keys + ' keys' : '';
    $('#leasesNow').textContent = latest ? latest.leases + ' leases' : '';
  }

  function renderCharts(d) {
    const samples = d.samples || [];
    sparkline($('#chartDbSize'), toPoints(samples, 'dbSize'), { format: fmt.bytes });
    sparkline($('#chartRevision'), toPoints(samples, 'revision'), { format: (v) => String(Math.round(v)) });
    sparkline($('#chartKeys'), toPoints(samples, 'keys'), { format: (v) => String(Math.round(v)) });
    sparkline($('#chartLeases'), toPoints(samples, 'leases'), { format: (v) => String(Math.round(v)) });
  }

  async function refresh() {
    try {
      lastData = await get('/api/metrics/overview');
      renderCards(lastData);
      renderCharts(lastData);
    } catch (e) {
      $('#metricCards').innerHTML = `<div class="stat-card"><p class="modal-tip">指标加载失败：${esc(e.message)}</p></div>`;
    }
  }

  function startPolling() {
    stopPolling();
    refresh();               // 进入指标页立即拉一次，避免用户等 5s
    timer = setInterval(refresh, 5000);
  }
  function stopPolling() {
    if (timer) { clearInterval(timer); timer = null; }
  }

  function bind() {
    document.addEventListener('view:enter', (e) => {
      // refresh 由导航统一切换时触发，这里只负责开启轮询。
      if (e.detail === 'metrics') startPolling();
    });
    document.addEventListener('view:leave', (e) => {
      if (e.detail === 'metrics') stopPolling();
    });
    window.addEventListener('resize', () => {
      if (lastData) renderCharts(lastData);
    });
  }

  window.App.pages.metrics = { refresh, bind };
})();
