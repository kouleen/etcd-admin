/* 用户与角色：认证开关、用户 CRUD/改密/授撤角色、角色 CRUD/权限授撤 */
(function () {
  'use strict';
  const { $, esc, get, post, del, toast, openModal, confirmDialog } = window.App;

  let authEnabled = false;

  async function refresh() {
    await loadStatus();
    await Promise.all([loadUsers(), loadRoles()]);
  }

  async function loadStatus() {
    try {
      const st = await get('/api/auth/status');
      authEnabled = st.enabled;
      const badge = $('#authStatusBadge');
      badge.textContent = st.enabled ? '认证已开启' : '认证未开启';
      badge.className = 'badge ' + (st.enabled ? 'on' : 'off');
      $('#authEnableBtn').hidden = st.enabled;
      $('#authDisableBtn').hidden = !st.enabled;
      $('#authTip').innerHTML = st.enabled
        ? `authRevision：${st.authRevision}。当前连接若未使用账号密码，部分管理操作可能被拒绝。`
        : '集群未开启认证，所有客户端均可匿名读写。开启认证前必须先创建 <strong>root</strong> 用户并授予 <strong>root</strong> 角色，否则 etcd 会拒绝开启。';
    } catch (e) {
      $('#authStatusBadge').textContent = '状态未知';
      $('#authTip').textContent = '认证状态查询失败：' + e.message;
    }
  }

  /* ---------------- 用户 ---------------- */
  async function loadUsers() {
    try {
      const rows = (await get('/api/auth/users')).items || [];
      $('#userBody').innerHTML = rows.length ? rows.map((u) => `
        <tr>
          <td data-label="用户名" class="mono"><strong>${esc(u.name)}</strong></td>
          <td data-label="角色">${(u.roles || []).map((r) => `<span class="badge info">${esc(r)}</span>`).join(' ') || '<span style="color:var(--text-3)">无</span>'}</td>
          <td data-label="操作">
            <button class="link-btn" data-act="pwd" data-name="${esc(u.name)}">改密</button>
            <button class="link-btn" data-act="grant" data-name="${esc(u.name)}">授角色</button>
            <button class="link-btn danger" data-act="revoke" data-name="${esc(u.name)}">撤角色</button>
            <button class="link-btn danger" data-act="del" data-name="${esc(u.name)}">删除</button>
          </td>
        </tr>`).join('')
        : '<tr class="placeholder-row"><td colspan="3">暂无用户（认证关闭时 etcd 不持久化用户列表）</td></tr>';
      $('#userBody').querySelectorAll('button[data-act]').forEach((b) =>
        b.addEventListener('click', () => userAction(b.dataset.act, b.dataset.name)));
    } catch (e) {
      $('#userBody').innerHTML = `<tr class="placeholder-row"><td colspan="3">加载失败：${esc(e.message)}</td></tr>`;
    }
  }

  function userAction(act, name) {
    if (act === 'del') return deleteUser(name);
    if (act === 'pwd') return promptModal(`修改密码：${name}`, [
      { id: 'password', label: '新密码', type: 'password' }
    ], async (v) => {
      await post('/api/auth/users/password', { name, password: v.password });
      toast('密码已更新', 'success');
    });
    if (act === 'grant') return promptModal(`给 ${name} 授予角色`, [
      { id: 'role', label: '角色名', placeholder: '例如 root、admin' }
    ], async (v) => {
      await post('/api/auth/users/roles', { user: name, role: v.role.trim() });
      toast('角色已授予', 'success');
    });
    if (act === 'revoke') return promptModal(`撤销 ${name} 的角色`, [
      { id: 'role', label: '要撤销的角色名' }
    ], async (v) => {
      await del('/api/auth/users/roles?user=' + encodeURIComponent(name) + '&role=' + encodeURIComponent(v.role.trim()));
      toast('角色已撤销', 'success');
    });
  }

  async function deleteUser(name) {
    const ok = await confirmDialog(`确认删除用户 ${name}？`, '该用户将立即无法再通过认证。', '删除用户');
    if (!ok) return;
    try {
      await del('/api/auth/users?name=' + encodeURIComponent(name));
      toast('用户已删除', 'success');
      loadUsers();
    } catch (e) { toast(e.message, 'error'); }
  }

  /* ---------------- 角色 ---------------- */
  async function loadRoles() {
    try {
      const rows = (await get('/api/auth/roles')).items || [];
      $('#roleBody').innerHTML = rows.length ? rows.map((r) => `
        <tr>
          <td data-label="角色名" class="mono"><strong>${esc(r.name)}</strong></td>
          <td data-label="权限">
            ${(r.permissions || []).map((p) => `
              <div class="perm-line">
                <span class="badge ${p.perm === 'READ' ? 'info' : p.perm === 'WRITE' ? 'lease' : 'put'}">${esc(p.perm)}</span>
                <span class="mono">${esc(p.key)}${p.prefix ? '*' : ''}</span>
                <button class="link-btn danger" data-act="revperm" data-role="${esc(r.name)}" data-key="${esc(p.key)}" data-range="${esc(p.rangeEnd)}">撤销</button>
              </div>`).join('') || '<span style="color:var(--text-3)">无权限</span>'}
          </td>
          <td data-label="操作">
            <button class="link-btn" data-act="grantperm" data-name="${esc(r.name)}">授权限</button>
            <button class="link-btn danger" data-act="del" data-name="${esc(r.name)}">删除角色</button>
          </td>
        </tr>`).join('')
        : '<tr class="placeholder-row"><td colspan="3">暂无角色</td></tr>';
      $('#roleBody').querySelectorAll('button[data-act]').forEach((b) =>
        b.addEventListener('click', () => roleAction(b.dataset.act, b)));
    } catch (e) {
      $('#roleBody').innerHTML = `<tr class="placeholder-row"><td colspan="3">加载失败：${esc(e.message)}</td></tr>`;
    }
  }

  function roleAction(act, btn) {
    const name = btn.dataset.name;
    if (act === 'del') return deleteRole(name);
    if (act === 'revperm') return revokePerm(btn);
    if (act === 'grantperm') return grantPermModal(name);
  }

  function grantPermModal(role) {
    const m = openModal(`
      <h3>给角色 ${esc(role)} 授予权限</h3>
      <div class="field"><label>Key 或前缀</label><input id="pKey" class="mono" placeholder="例如 kitex/ 或 config/app.json"/></div>
      <div class="field"><label>权限类型</label>
        <select id="pPerm">
          <option value="read">READ（只读）</option>
          <option value="write">WRITE（只写）</option>
          <option value="readwrite">READWRITE（读写）</option>
        </select>
      </div>
      <label class="switch" style="margin-bottom:16px"><input type="checkbox" id="pPrefix" checked/><span class="slider"></span><em>按前缀匹配（key 下所有键）</em></label>
      <div class="modal-actions">
        <button class="btn ghost" data-close>取消</button>
        <button class="btn primary" id="pOk">授予</button>
      </div>`);
    m.mask.querySelector('#pOk').addEventListener('click', async () => {
      const key = m.mask.querySelector('#pKey').value.trim();
      if (!key) { toast('请输入 Key', 'error'); return; }
      try {
        await post('/api/auth/roles/permissions', {
          role, key,
          prefix: m.mask.querySelector('#pPrefix').checked,
          perm: m.mask.querySelector('#pPerm').value
        });
        toast('权限已授予', 'success');
        m.close();
        loadRoles();
      } catch (e) { toast(e.message, 'error'); }
    });
  }

  async function revokePerm(btn) {
    const ok = await confirmDialog('确认撤销该权限？', `${btn.dataset.role} → ${btn.dataset.key}`, '撤销');
    if (!ok) return;
    try {
      const q = new URLSearchParams({ role: btn.dataset.role, key: btn.dataset.key, rangeEnd: btn.dataset.range || '' });
      await del('/api/auth/roles/permissions?' + q.toString());
      toast('权限已撤销', 'success');
      loadRoles();
    } catch (e) { toast(e.message, 'error'); }
  }

  async function deleteRole(name) {
    const ok = await confirmDialog(`确认删除角色 ${name}？`, '拥有该角色的用户会同时失去对应权限。', '删除角色');
    if (!ok) return;
    try {
      await del('/api/auth/roles?name=' + encodeURIComponent(name));
      toast('角色已删除', 'success');
      loadRoles();
    } catch (e) { toast(e.message, 'error'); }
  }

  /* ---------------- 通用输入弹窗 ---------------- */
  function promptModal(title, fields, onOk) {
    const m = openModal(`
      <h3>${esc(title)}</h3>
      ${fields.map((f) => `
        <div class="field"><label>${esc(f.label)}</label>
          <input id="${f.id}" type="${f.type || 'text'}" placeholder="${esc(f.placeholder || '')}" autocomplete="off"/>
        </div>`).join('')}
      <div class="modal-actions">
        <button class="btn ghost" data-close>取消</button>
        <button class="btn primary" id="pmOk">确定</button>
      </div>`);
    m.mask.querySelector('#pmOk').addEventListener('click', async () => {
      const values = {};
      for (const f of fields) {
        values[f.id] = m.mask.querySelector('#' + f.id).value;
        if (!values[f.id]) { toast(f.label + ' 不能为空', 'error'); return; }
      }
      try {
        await onOk(values);
        m.close();
        refresh();
      } catch (e) { toast(e.message, 'error'); }
    });
  }

  function bind() {
    $('#userCreateBtn').addEventListener('click', () => promptModal('新建用户', [
      { id: 'name', label: '用户名' },
      { id: 'password', label: '密码', type: 'password' }
    ], async (v) => {
      await post('/api/auth/users', { name: v.name.trim(), password: v.password });
      toast('用户已创建', 'success');
    }));

    $('#roleCreateBtn').addEventListener('click', () => promptModal('新建角色', [
      { id: 'name', label: '角色名' }
    ], async (v) => {
      await post('/api/auth/roles', { name: v.name.trim() });
      toast('角色已创建', 'success');
    }));

    $('#authEnableBtn').addEventListener('click', async () => {
      const ok = await confirmDialog('确认开启 etcd 认证？',
        '开启后所有请求都必须携带有效账号密码。请确认已创建 root 用户与 root 角色，否则可能锁死集群访问。', '开启认证');
      if (!ok) return;
      try {
        await post('/api/auth/enable', {});
        toast('认证已开启', 'success');
        refresh();
      } catch (e) { toast(e.message, 'error'); }
    });

    $('#authDisableBtn').addEventListener('click', async () => {
      const ok = await confirmDialog('确认关闭 etcd 认证？', '关闭后集群将接受匿名访问，请谨慎操作。', '关闭认证');
      if (!ok) return;
      try {
        await post('/api/auth/disable', {});
        toast('认证已关闭', 'success');
        refresh();
      } catch (e) { toast(e.message, 'error'); }
    });
  }

  window.App.pages.auth = { refresh, bind };
})();
