/* 键值浏览：键树、分页表格、搜索、编辑器、历史版本 Diff、书签、导入导出、批量删除 */
(function () {
  'use strict';
  const { $, esc, get, post, put, toast, openModal, confirmDialog,
    fmt, store, download, prettyJSON, highlightJSON, lineDiff } = window.App;

  const BM_KEY = 'etcd-bookmarks';
  const state = {
    mode: 'prefix',         // prefix | search
    prefix: '',
    items: [],
    nextKey: '',
    more: false,
    total: 0,
    selected: new Set(),
    treeRoot: null,
    searchKeyword: '',
    searchInValue: false
  };

  /* ---------------- 初始化与刷新 ---------------- */
  async function refresh() {
    await Promise.all([loadTree(), loadList('', false)]);
  }

  async function loadTree() {
    try {
      const data = await get('/api/kv/tree?prefix=');
      state.treeRoot = data.root;
      renderTree(data.root, data);
    } catch (e) {
      $('#treeBox').innerHTML = `<div class="tree-empty">加载失败：${esc(e.message)}</div>`;
    }
  }

  function renderTree(root, meta) {
    const box = $('#treeBox');
    if (!root || !root.children || !root.children.length) {
      box.innerHTML = '<div class="tree-empty">暂无键</div>';
      return;
    }
    const node = (n, depth) => {
      const hasChildren = n.dir && n.children && n.children.length;
      const twisty = hasChildren ? '<span class="twisty">▼</span>' : '<span class="twisty"></span>';
      const ico = hasChildren ? '📁' : '🔑';
      const sub = n.path === 'kitex' || depth < 1 ? '' : ' collapsed';
      return `
        <div class="tree-node${sub}" data-path="${esc(n.path)}">
          <div class="tree-row${n.path === state.prefix ? ' active' : ''}" role="button" tabindex="0" data-path="${esc(n.path)}" data-dir="${hasChildren ? 1 : 0}" data-key="${esc(n.key || '')}">
            ${twisty}<span class="t-ico">${ico}</span><span class="t-name">${esc(n.name)}</span>
          </div>
          ${hasChildren ? `<div class="tree-children">${n.children.map((c) => node(c, depth + 1)).join('')}</div>` : ''}
        </div>`;
    };
    box.innerHTML = root.children.map((c) => node(c, 0)).join('') +
      (meta && meta.truncated ? '<div class="tree-empty">键数量超过扫描上限，树可能不完整</div>' : '');

    box.querySelectorAll('.tree-row').forEach((row) => {
      const onActivate = (e) => {
        e.stopPropagation();
        const nodeEl = row.closest('.tree-node');
        if (row.dataset.dir === '1') nodeEl.classList.toggle('collapsed');
        const path = row.dataset.path;
        box.querySelectorAll('.tree-row.active').forEach((r) => r.classList.remove('active'));
        row.classList.add('active');
        // 目录按前缀浏览；叶子直接展示该键。
        loadList(row.dataset.dir === '1' ? path + '/' : path, false);
      };
      row.addEventListener('click', onActivate);
      row.addEventListener('keydown', (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onActivate(e); } });
    });
  }

  /* ---------------- 键表 ---------------- */
  async function loadList(prefix, append) {
    state.mode = 'prefix';
    state.prefix = prefix;
    try {
      const q = new URLSearchParams({ prefix, limit: '100' });
      if (append && state.nextKey) q.set('start', state.nextKey);
      const data = await get('/api/kv?' + q.toString());
      state.items = append ? state.items.concat(data.items) : data.items;
      state.nextKey = data.nextKey || '';
      state.more = !!data.more;
      state.total = data.count;
      if (!append) state.selected.clear();
      $('#kvTableTitle').textContent = prefix ? `前缀：${prefix}` : '全部键';
      $('#kvTableHint').textContent = `共 ${data.count} 个键${data.more ? '，已加载 ' + state.items.length : ''}`;
      renderTable();
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function runSearch() {
    const keyword = $('#kvSearch').value.trim();
    if (!keyword) { toast('请输入搜索关键词', 'info'); return; }
    state.mode = 'search';
    state.searchKeyword = keyword;
    state.searchInValue = $('#searchInValue').checked;
    try {
      const q = new URLSearchParams({ keyword, inValue: state.searchInValue ? '1' : '0' });
      const data = await get('/api/kv/search?' + q.toString());
      state.items = data.items;
      state.more = false;
      state.selected.clear();
      $('#kvTableTitle').textContent = `搜索：${keyword}${state.searchInValue ? '（含值）' : ''}`;
      $('#kvTableHint').textContent = `扫描 ${data.scanned} 个键，命中 ${data.matched} 个`;
      renderTable();
    } catch (e) { toast(e.message, 'error'); }
  }

  // 租约 ID 为最长 19 位的长整数，表格列宽有限：保留头部 4 位与尾部 4 位（同批租约靠尾部区分），
  // 完整 ID 仍保留在 badge 的 title 与详情弹窗中
  function shortLease(id) {
    const s = String(id);
    return s.length > 10 ? `${s.slice(0, 4)}…${s.slice(-4)}` : s;
  }

  function renderTable() {
    const body = $('#kvBody');
    $('#kvFoot').hidden = false;
    $('#kvCount').textContent = state.mode === 'search'
      ? `命中 ${state.items.length} 个键`
      : `已显示 ${state.items.length} / ${state.total} 个键`;
    $('#kvLoadMoreBtn').hidden = !(state.mode === 'prefix' && state.more);
    updateBatchBtn();

    if (!state.items.length) {
      body.innerHTML = '<tr class="placeholder-row"><td colspan="7">没有匹配的键</td></tr>';
      return;
    }
    const bm = store.get(BM_KEY, []);
    body.innerHTML = state.items.map((it) => {
      const checked = state.selected.has(it.key) ? 'checked' : '';
      const marked = bm.includes(it.key);
      return `
      <tr>
        <td data-label="选择" class="col-check"><input type="checkbox" class="row-check" data-key="${esc(it.key)}" ${checked}/></td>
        <td data-label="Key"><span class="kv-key mono${marked ? ' is-bookmarked' : ''}" role="button" tabindex="0" data-key="${esc(it.key)}" title="${marked ? '书签键 · ' : ''}点击查看详情与历史版本">${esc(it.key)}</span></td>
        <td data-label="Value"><span class="kv-val mono" title="${esc(it.value)}">${esc(fmt.truncate(it.value, 80)) || '<em style="color:var(--text-3)">空</em>'}</span></td>
        <td data-label="版本" class="mono">${it.version}</td>
        <td data-label="修改版本" class="mono">${it.modRevision}</td>
        <td data-label="租约">${it.lease && it.lease !== '0' ? `<span class="badge lease" title="租约 ${esc(it.lease)}">lease ${esc(shortLease(it.lease))}</span>` : '<span class="badge on">永久键</span>'}</td>
        <td data-label="操作">
          <button class="link-btn" data-act="bm" data-key="${esc(it.key)}">${marked ? '取消书签' : '书签'}</button>
        </td>
      </tr>`;
    }).join('');

    body.querySelectorAll('.row-check').forEach((cb) => {
      cb.addEventListener('change', () => {
        cb.checked ? state.selected.add(cb.dataset.key) : state.selected.delete(cb.dataset.key);
        updateBatchBtn();
      });
    });
    // 点击 Key 打开详情弹窗（内含美化、复制、历史版本、编辑入口）
    body.querySelectorAll('.kv-key[data-key]').forEach((el) => {
      el.addEventListener('click', () => openDetail(el.dataset.key));
      el.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openDetail(el.dataset.key); }
      });
    });
    body.querySelectorAll('button[data-act]').forEach((btn) => {
      btn.addEventListener('click', () => handleRowAction(btn.dataset.act, btn.dataset.key));
    });
  }

  function updateBatchBtn() {
    const btn = $('#kvBatchDelBtn');
    btn.disabled = state.selected.size === 0;
    btn.textContent = state.selected.size ? `批量删除 (${state.selected.size})` : '批量删除';
    $('#kvCheckAll').checked = state.items.length > 0 && state.items.every((i) => state.selected.has(i.key));
  }

  function handleRowAction(act, key) {
    // 行内仅保留书签切换；查看/历史经点击 Key 进入详情弹窗，编辑在详情弹窗与顶部「新建键」中
    if (act === 'bm') toggleBookmark(key);
  }

  /* ---------------- 详情 ---------------- */
  async function openDetail(key) {
    let it;
    try {
      it = await get('/api/kv/detail?key=' + encodeURIComponent(key));
    } catch (e) { toast(e.message, 'error'); return; }

    const m = openModal(`
      <h3 class="mono">${esc(it.key)}</h3>
      <div style="display:flex;gap:8px;flex-wrap:wrap;margin-bottom:14px">
        <span class="badge info">version ${it.version}</span>
        <span class="badge info">modRev ${it.modRevision}</span>
        <span class="badge info">createRev ${it.createRevision}</span>
        ${it.lease && it.lease !== '0' ? `<span class="badge lease" title="租约 ${esc(it.lease)}">lease ${esc(fmt.truncate(it.lease, 19))}</span>` : '<span class="badge on">永久键</span>'}
      </div>
      <div class="editor-tools">
        <button class="btn ghost sm" id="dFmt">美化 JSON</button>
        <button class="btn ghost sm" id="dCopy">复制值</button>
        <button class="btn ghost sm" id="dHistory">历史版本</button>
      </div>
      <pre class="code-view" id="dCode"></pre>
      <div class="modal-actions">
        <button class="btn primary" id="dEdit">编辑</button>
        <button class="btn ghost" data-close>关闭</button>
      </div>`);
    const codeEl = m.mask.querySelector('#dCode');
    let raw = it.value;
    const renderCode = (text) => { codeEl.innerHTML = text ? highlightJSON(text) : '<span style="color:var(--text-3)">（空值）</span>'; };
    renderCode(raw);
    m.mask.querySelector('#dFmt').addEventListener('click', () => { raw = prettyJSON(raw); renderCode(raw); });
    m.mask.querySelector('#dCopy').addEventListener('click', () => {
      navigator.clipboard.writeText(raw).then(() => toast('已复制到剪贴板', 'success'), () => toast('复制失败', 'error'));
    });
    m.mask.querySelector('#dHistory').addEventListener('click', () => { m.close(); openHistory(key); });
    m.mask.querySelector('#dEdit').addEventListener('click', () => { m.close(); openEditor(key); });
  }

  /* ---------------- 编辑器 ---------------- */
  async function openEditor(existingKey) {
    let data = { key: existingKey || '', value: '', lease: 0 };
    if (existingKey) {
      try { data = await get('/api/kv/detail?key=' + encodeURIComponent(existingKey)); }
      catch (e) { toast(e.message, 'error'); return; }
    }
    const m = openModal(`
      <h3>${existingKey ? '编辑键' : '新建键'}</h3>
      <div class="field"><label>Key</label>
        <input id="eKey" class="mono" value="${esc(data.key)}" ${existingKey ? 'readonly' : ''} placeholder="例如 config/app/name"/>
      </div>
      <div class="field"><label>Value</label>
        <div class="editor-tools"><button class="btn ghost sm" id="eFmt" type="button">美化 JSON</button></div>
        <textarea id="eValue" rows="10" spellcheck="false" style="font-family:'SF Mono',Menlo,monospace">${esc(data.value)}</textarea>
      </div>
      <div class="field"><label>租约 ID（可选，填 0 表示永久键）</label>
        <input id="eLease" type="text" inputmode="numeric" pattern="[0-9]*" value="${data.lease || 0}" placeholder="0"/>
      </div>
      <div class="modal-actions">
        <button class="btn ghost" data-close>取消</button>
        <button class="btn primary" id="eSave">保存</button>
      </div>`);
    m.mask.querySelector('#eFmt').addEventListener('click', () => {
      const ta = m.mask.querySelector('#eValue');
      try { ta.value = JSON.stringify(JSON.parse(ta.value), null, 2); }
      catch (e) { toast('当前内容不是合法 JSON', 'error'); }
    });
    m.mask.querySelector('#eSave').addEventListener('click', async () => {
      const key = m.mask.querySelector('#eKey').value.trim();
      if (!key) { toast('Key 不能为空', 'error'); return; }
      const body = {
        key,
        value: m.mask.querySelector('#eValue').value,
        // 租约 ID 可能超过 JS 安全整数范围，按字符串原样回传。
        leaseId: m.mask.querySelector('#eLease').value.trim() || '0'
      };
      try {
        await put('/api/kv', body);
        toast('保存成功', 'success');
        m.close();
        await Promise.all([loadTree(), reloadCurrent()]);
      } catch (e) { toast(e.message, 'error'); }
    });
  }

  /* ---------------- 历史版本 Diff ---------------- */
  async function openHistory(key) {
    let entries = [];
    try {
      entries = (await get('/api/kv/history?key=' + encodeURIComponent(key) + '&limit=30')).entries || [];
    } catch (e) { toast(e.message, 'error'); return; }

    const m = openModal(`
      <h3 class="mono">历史版本：${esc(key)}</h3>
      <div class="diff-box">
        <div class="diff-rev-list" id="revList"></div>
        <div class="diff-content" id="diffContent"></div>
      </div>
      <div class="modal-actions"><button class="btn ghost" data-close>关闭</button></div>`, true);

    const listEl = m.mask.querySelector('#revList');
    const contentEl = m.mask.querySelector('#diffContent');
    const current = entries[0];
    listEl.innerHTML = entries.map((en, idx) => `
      <div class="diff-rev${idx === 0 ? ' active' : ''}" data-idx="${idx}">
        rev ${en.modRevision} <small>version ${en.version} · ${idx === 0 ? '当前版本' : '历史版本'}</small>
      </div>`).join('');

    const showDiff = (idx) => {
      listEl.querySelectorAll('.diff-rev').forEach((r) => r.classList.toggle('active', Number(r.dataset.idx) === idx));
      if (idx === 0) {
        contentEl.innerHTML = `<p class="modal-tip" style="margin-bottom:8px">当前版本值：</p><pre class="code-view">${highlightJSON(current.value)}</pre>`;
        return;
      }
      const target = entries[idx];
      const lines = lineDiff(target.value, current.value);
      contentEl.innerHTML = `
        <p class="modal-tip" style="margin-bottom:8px">rev ${target.modRevision} → 当前 rev ${current.modRevision} 的差异（红删 / 绿增）：</p>
        ${lines.map((l) => `<div class="diff-line ${l.type}">${l.type === 'del' ? '- ' : l.type === 'add' ? '+ ' : '  '}${esc(l.text) || ' '}</div>`).join('')}`;
    };
    listEl.querySelectorAll('.diff-rev').forEach((r) => r.addEventListener('click', () => showDiff(Number(r.dataset.idx))));
    showDiff(0);
  }

  /* ---------------- 删除（仅保留底部批量删除，单键删除请勾选后执行） ---------------- */
  async function batchDelete() {
    const keys = Array.from(state.selected);
    if (!keys.length) return;
    const ok = await confirmDialog(`确认批量删除 ${keys.length} 个键？`, '该操作通过 etcd 事务原子提交，不可撤销。', '全部删除');
    if (!ok) return;
    try {
      const res = await post('/api/kv/batch-delete', { keys });
      toast(`事务提交成功，删除 ${res.deleted} 个键`, 'success');
      state.selected.clear();
      await Promise.all([reloadCurrent(), loadTree()]);
    } catch (e) { toast(e.message, 'error'); }
  }

  function reloadCurrent() {
    return state.mode === 'search' ? runSearch() : loadList(state.prefix, false);
  }

  /* ---------------- 书签 ---------------- */
  function toggleBookmark(key) {
    const bm = store.get(BM_KEY, []);
    const idx = bm.indexOf(key);
    if (idx >= 0) { bm.splice(idx, 1); toast('已移除书签', 'info'); }
    else { bm.push(key); toast('已加入书签', 'success'); }
    store.set(BM_KEY, bm);
    renderBookmarks();
    renderTable();
  }

  function renderBookmarks() {
    const bm = store.get(BM_KEY, []);
    $('#bookmarkCount').textContent = bm.length ? `${bm.length} 个` : '';
    const box = $('#bookmarkList');
    if (!bm.length) { box.innerHTML = '<div class="bookmark-empty">还没有书签</div>'; return; }
    box.innerHTML = bm.map((k) => `
      <div class="bookmark-item">
        <span class="bm-name mono" role="button" tabindex="0" data-key="${esc(k)}" title="${esc(k)}">🔖 ${esc(k)}</span>
        <button class="bm-del" data-key="${esc(k)}">✕</button>
      </div>`).join('');
    box.querySelectorAll('.bm-name').forEach((el) => el.addEventListener('click', () => openDetail(el.dataset.key)));
    box.querySelectorAll('.bm-del').forEach((el) => el.addEventListener('click', () => toggleBookmark(el.dataset.key)));
  }

  /* ---------------- 导入 / 导出 ---------------- */
  function doExport() {
    // 利用 Content-Disposition 附件头直接触发下载，不会离开页面。
    window.location.href = '/api/kv/export?prefix=' + encodeURIComponent(state.prefix);
  }

  function doImport(file) {
    const reader = new FileReader();
    reader.onload = () => {
      let items;
      try {
        const parsed = JSON.parse(reader.result);
        if (!Array.isArray(parsed)) throw new Error('文件根节点必须是数组');
        items = parsed.filter((x) => x && x.key);
      } catch (e) { toast('文件解析失败：' + e.message, 'error'); return; }
      if (!items.length) { toast('文件中没有可导入的键', 'error'); return; }

      const m = openModal(`
        <h3>导入确认</h3>
        <p class="modal-tip">共解析出 <strong style="color:var(--blue-1)">${items.length}</strong> 个键，请选择导入模式：</p>
        <p class="modal-tip" style="margin-top:8px"><strong>覆盖模式</strong>：无条件写入，同名键的值会被替换；<br><strong>事务模式</strong>：仅当键不存在时写入，已存在的键自动跳过。</p>
        <div class="modal-actions">
          <button class="btn ghost" data-close>取消</button>
          <button class="btn ghost" id="impSkip">事务模式（不覆盖）</button>
          <button class="btn primary" id="impOver">覆盖模式</button>
        </div>`);
      const submit = async (overwrite) => {
        m.close();
        try {
          const res = await post('/api/kv/import', { overwrite, items });
          toast(`导入完成：成功 ${res.success}，跳过 ${res.skipped}，失败 ${res.failed.length}`, res.failed.length ? 'error' : 'success');
          if (res.failed.length) openModal(`<h3>导入失败明细</h3>${res.failed.map((f) => `<div class="perm-line mono">${esc(f.key)}：${esc(f.error)}</div>`).join('')}<div class="modal-actions"><button class="btn ghost" data-close>关闭</button></div>`);
          await Promise.all([loadTree(), reloadCurrent()]);
        } catch (e) { toast(e.message, 'error'); }
      };
      m.mask.querySelector('#impOver').addEventListener('click', () => submit(true));
      m.mask.querySelector('#impSkip').addEventListener('click', () => submit(false));
    };
    reader.readAsText(file);
  }

  /* ---------------- 事件绑定 ---------------- */
  function bind() {
    $('#kvSearchBtn').addEventListener('click', runSearch);
    $('#kvSearch').addEventListener('keydown', (e) => { if (e.key === 'Enter') runSearch(); });
    $('#kvCreateBtn').addEventListener('click', () => openEditor(''));
    $('#treeReloadBtn').addEventListener('click', loadTree);
    $('#kvLoadMoreBtn').addEventListener('click', () => loadList(state.prefix, true));
    $('#kvBatchDelBtn').addEventListener('click', batchDelete);
    $('#kvExportBtn').addEventListener('click', doExport);
    $('#kvImportBtn').addEventListener('click', () => $('#importFile').click());
    $('#importFile').addEventListener('change', (e) => {
      if (e.target.files[0]) doImport(e.target.files[0]);
      e.target.value = '';
    });
    $('#kvCheckAll').addEventListener('change', (e) => {
      state.items.forEach((i) => e.target.checked ? state.selected.add(i.key) : state.selected.delete(i.key));
      renderTable();
    });
    renderBookmarks();
  }

  window.App.pages.kv = { refresh, bind };
})();
