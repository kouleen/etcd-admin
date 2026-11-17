/* 仪表盘：端点状态卡片、集群成员、告警、压缩与碎片整理 */
(function () {
  'use strict';
  const { $, esc, get, post, toast, openModal, confirmDialog, fmt } = window.App;

  let leaderID = 0;

  async function refresh() {
    let data;
    try {
      data = await get('/api/cluster/status');
    } catch (e) {
      $('#endpointGrid').innerHTML = `<div class="stat-card"><p class="modal-tip">加载失败：${esc(e.message)}</p></div>`;
      $('#memberBody').innerHTML = `<tr><td colspan="5" class="modal-tip">集群暂不可达</td></tr>`;
      return;
    }
    leaderID = (data.endpoints[0] && data.endpoints[0].leader) || 0;
    renderEndpoints(data.endpoints || []);
    renderMembers(data.members || []);
  }

  function renderEndpoints(list) {
    $('#endpointGrid').innerHTML = list.map((ep) => `
      <div class="stat-card">
        <div class="sc-head">
          <i class="dot ${ep.healthy ? 'on' : 'off'}"></i>
          <strong class="mono">${esc(ep.endpoint)}</strong>
        </div>
        <div class="sc-body">
          <div class="metric"><label>版本</label><span>${esc(ep.version || '-')}</span></div>
          <div class="metric"><label>健康</label><span>${ep.healthy ? '正常' : '异常'}</span></div>
          <div class="metric"><label>DB 大小</label><span>${fmt.bytes(ep.dbSize)}</span></div>
          <div class="metric"><label>Revision</label><span>${ep.revision || '-'}</span></div>
          <div class="metric"><label>Leader ID</label><span class="mono">${ep.leader || '-'}</span></div>
          <div class="metric"><label>Learner</label><span>${ep.isLearner ? '是' : '否'}</span></div>
        </div>
        ${ep.error ? `<p class="modal-tip" style="margin-top:10px">${esc(ep.error)}</p>` : ''}
      </div>`).join('');
  }

  function renderMembers(members) {
    if (!members.length) {
      $('#memberBody').innerHTML = `<tr class="placeholder-row"><td colspan="5">暂无成员数据</td></tr>`;
      return;
    }
    $('#memberBody').innerHTML = members.map((m) => {
      const isLeader = m.id === leaderID;
      return `
      <tr>
        <td data-label="名称"><strong>${esc(m.name || '(未命名)')}</strong>
          ${isLeader ? '<span class="badge info" style="margin-left:6px">Leader</span>' : ''}
          ${m.isLearner ? '<span class="badge lease" style="margin-left:6px">Learner</span>' : ''}
        </td>
        <td data-label="成员 ID" class="mono">${m.id}</td>
        <td data-label="Client URL" class="mono">${(m.clientUrls || []).map(esc).join('<br>') || '-'}</td>
        <td data-label="Peer URL" class="mono">${(m.peerUrls || []).map(esc).join('<br>') || '-'}</td>
        <td data-label="角色">${isLeader ? '主节点' : 'Follower'}</td>
      </tr>`;
    }).join('');
  }

  async function showAlarms() {
    let items = [];
    try {
      items = (await get('/api/cluster/alarms')).items || [];
    } catch (e) { toast('告警查询失败：' + e.message, 'error'); return; }

    const m = openModal(`
      <h3>集群告警</h3>
      <div id="alarmList"></div>
      <div class="modal-actions">
        <button class="btn warn" id="disarmBtn" ${items.length ? '' : 'disabled'}>解除全部告警</button>
        <button class="btn ghost" data-close>关闭</button>
      </div>`);
    const box = m.mask.querySelector('#alarmList');
    box.innerHTML = items.length
      ? items.map((a) => `<div class="perm-line"><span class="badge delete">${esc(a.alarm)}</span> 成员 ID：<span class="mono">${a.memberID}</span></div>`).join('')
      : '<p class="modal-tip">当前没有任何告警，集群状态良好。</p>';
    m.mask.querySelector('#disarmBtn').addEventListener('click', async () => {
      try {
        await post('/api/cluster/alarms/disarm', {});
        toast('告警已解除', 'success');
        m.close();
      } catch (e) { toast(e.message, 'error'); }
    });
  }

  function bind() {
    $('#alarmBtn').addEventListener('click', showAlarms);

    $('#compactBtn').addEventListener('click', async () => {
      const rev = Number($('#compactRevision').value);
      if (!rev || rev <= 0) { toast('请输入有效的 revision', 'error'); return; }
      const ok = await confirmDialog('确认压缩历史版本？',
        `Compact 到 revision ${rev} 后，该版本之前的历史将被物理清除且不可恢复（历史版本对比功能会丢失旧数据）。`, '执行压缩');
      if (!ok) return;
      try {
        await post('/api/maintenance/compact', { revision: rev });
        toast(`已压缩至 revision ${rev}`, 'success');
        $('#compactRevision').value = '';
      } catch (e) { toast(e.message, 'error'); }
    });

    $('#defragBtn').addEventListener('click', async () => {
      const ok = await confirmDialog('确认执行碎片整理？',
        'Defragment 会阻塞节点请求，生产环境建议在低峰期对每个节点依次执行。', '执行整理');
      if (!ok) return;
      try {
        const res = await post('/api/maintenance/defrag', {});
        const failed = (res.results || []).filter((r) => r.error);
        failed.length ? toast(`部分节点整理失败：${failed.map((f) => f.endpoint).join(', ')}`, 'error')
          : toast('碎片整理完成', 'success');
        refresh();
      } catch (e) { toast(e.message, 'error'); }
    });
  }

  window.App.pages.dashboard = { refresh, bind };
})();
