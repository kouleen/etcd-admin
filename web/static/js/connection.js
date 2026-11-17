/* 连接设置：查看当前连接、测试目标、切换连接 */
(function () {
  'use strict';
  const { $, esc, get, post, toast, fmt } = window.App;

  async function refresh() {
    let d;
    try {
      d = await get('/api/connection/current');
    } catch (e) {
      $('#connDetail').innerHTML = `<p class="modal-tip">连接信息加载失败：${esc(e.message)}</p>`;
      return;
    }
    const info = d.info || {};
    $('#connDetail').innerHTML = `
      <div class="conn-metric"><label>连接状态</label>
        <span><i class="dot ${d.reachable ? 'on' : 'off'}" style="margin-right:6px"></i>${d.reachable ? '在线' : '不可达'}</span></div>
      <div class="conn-metric"><label>Endpoints</label><span>${(info.endpoints || []).map(esc).join(', ') || '-'}</span></div>
      <div class="conn-metric"><label>用户名</label><span>${esc(info.username) || '匿名'}</span></div>
      <div class="conn-metric"><label>服务端版本</label><span>${esc(d.version || '-')}</span></div>
      <div class="conn-metric"><label>Revision</label><span>${d.revision || '-'}</span></div>
      <div class="conn-metric"><label>DB 大小</label><span>${fmt.bytes(d.dbSize)}</span></div>
      <div class="conn-metric"><label>Leader ID</label><span>${d.leader || '-'}</span></div>`;

    // 仅在表单为空时回填，避免覆盖用户正在编辑的内容。
    if (!$('#cfgEndpoints').value) {
      $('#cfgEndpoints').value = (info.endpoints || []).join(', ');
      $('#cfgUsername').value = info.username || '';
    }
  }

  function body() {
    return {
      endpoints: $('#cfgEndpoints').value,
      username: $('#cfgUsername').value.trim(),
      password: $('#cfgPassword').value
    };
  }

  function bind() {
    $('#cfgTestBtn').addEventListener('click', async () => {
      const tip = $('#cfgResult');
      tip.textContent = '正在探测…';
      try {
        const r = await post('/api/connection/test', body());
        tip.innerHTML = `✓ 连接成功：etcd <strong>${esc(r.version)}</strong>，revision ${r.revision}，DB ${fmt.bytes(r.dbSize)}`;
        toast('目标 etcd 可连通', 'success');
      } catch (e) {
        tip.innerHTML = '✕ ' + esc(e.message);
        toast(e.message, 'error');
      }
    });

    $('#cfgSaveBtn').addEventListener('click', async () => {
      if (!$('#cfgEndpoints').value.trim()) { toast('Endpoints 不能为空', 'error'); return; }
      try {
        const info = await post('/api/connection/connect', body());
        toast('已切换到新连接：' + (info.endpoints || []).join(', '), 'success');
        $('#cfgPassword').value = '';
        window.App.updateConnPill();
        // 通知全部视图按新连接刷新。
        Object.values(window.App.pages).forEach((p) => p.refresh && p.refresh());
      } catch (e) { toast('连接失败：' + e.message, 'error'); }
    });
  }

  window.App.pages.connection = { refresh, bind };
})();
