/* ============================================================
   UI 片段工厂 —— 组件语法在这一层实现，屏幕只做拼装
   ============================================================ */
(function () {
  'use strict';

  const esc = (s) =>
    String(s == null ? '' : s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');

  /* ---- 基础片段 ---- */
  function tag(text, kind, icon) {
    return (
      '<span class="tag' + (kind ? ' is-' + kind : '') + '">' +
      (icon ? ic(icon, 11) : '') + esc(text) + '</span>'
    );
  }
  function led(kind) {
    return '<i class="led' + (kind ? ' is-' + kind : '') + '"></i>';
  }
  function kbd(text) {
    return '<span class="kbd">' + esc(text) + '</span>';
  }
  function count(n, kind) {
    return '<span class="count' + (kind ? ' is-' + kind : '') + '">' + esc(n) + '</span>';
  }
  function chip(label, on, icon) {
    return (
      '<button class="chip' + (on ? ' is-on' : '') + '">' +
      (icon ? ic(icon, 12) : '') + esc(label) + '</button>'
    );
  }
  function btn(label, kind, icon, size) {
    return (
      '<button class="btn' + (kind ? ' btn-' + kind : '') + (size ? ' btn-' + size : '') + '">' +
      (icon ? ic(icon, 14) : '') + (label ? esc(label) : '') + '</button>'
    );
  }
  function iconBtn(icon, title, cls) {
    return '<button class="icon-btn ' + (cls || '') + '" title="' + esc(title || '') + '">' + ic(icon, 15) + '</button>';
  }
  function switchEl(on, cls) {
    return '<span class="switch' + (on ? ' is-on' : '') + ' ' + (cls || '') + '"></span>';
  }
  /* 列表/表格里的启用态：文字标签 + 状态点（图形开关在密集行里看不清） */
  function enTag(on) {
    return '<button class="en-tag' + (on ? ' is-on' : '') + ' js-sw">' + (on ? '启用' : '停用') + '</button>';
  }
  function progress(p, ok) {
    return '<div class="progress' + (ok ? ' is-ok' : '') + '"><i style="width:' + p + '%"></i></div>';
  }
  function plate(text) {
    return '<div class="t-plate">' + esc(text) + '</div>';
  }
  function empty(glyph, title, sub, action) {
    return (
      '<div class="empty"><div class="glyph">' + ic(glyph, 20) + '</div>' +
      '<div class="em-t">' + esc(title) + '</div>' +
      (sub ? '<div class="em-s">' + esc(sub) + '</div>' : '') +
      (action || '') + '</div>'
    );
  }
  function alert(kind, icon, title, body, acts) {
    return (
      '<div class="alert' + (kind ? ' is-' + kind : '') + '">' + ic(icon || 'info', 14) +
      '<div class="grow"><div class="a-t">' + esc(title) + '</div>' +
      (body ? '<div class="mt1">' + body + '</div>' : '') + '</div>' +
      (acts || '') + '</div>'
    );
  }
  function field(label, control, hint) {
    return (
      '<div class="field">' + (label ? '<label>' + esc(label) + '</label>' : '') +
      control + (hint ? '<div class="hint">' + hint + '</div>' : '') + '</div>'
    );
  }
  function menuItem(label, icon, cls, sub) {
    return (
      '<button class="menu-item ' + (cls || '') + '">' + (icon ? ic(icon, 14) : '') +
      '<span class="grow">' + esc(label) + '</span>' +
      (sub ? '<span class="sub">' + esc(sub) + '</span>' : '') + '</button>'
    );
  }

  /* ---- 聊天专用片段 ---- */
  function avatar(kind, icon, size) {
    const s = size || 15;
    return '<div class="avatar' + (kind ? ' is-' + kind : '') + '">' + ic(icon || 'sparkle', s) + '</div>';
  }

  function usageBadge(u, live) {
    return (
      '<div class="row row-g3" style="font-family:var(--font-mono);font-size:11px;color:var(--ink-3)">' +
      (live ? '<span class="row row-g2"><i class="led is-accent is-live"></i>输出中</span>' : '') +
      '<span title="输入">↑' + esc(u.in) + '</span>' +
      '<span title="输出">↓' + esc(u.out) + '</span>' +
      '<span title="缓存命中">缓存 ' + esc(u.cache) + '%</span>' +
      '<span title="耗时">' + esc(u.sec) + '</span>' +
      '</div>'
    );
  }

  /* 工具时间线节点 */
  function tlNode(step, i, state, open) {
    const st = state || 'done';
    return (
      '<div class="tl-node is-' + st + (open ? ' is-open' : '') + '" data-tl="' + i + '">' +
      '<div class="tl-head">' +
      '<span class="tl-caret">' + ic('chev-right', 11) + '</span>' +
      '<span class="tl-name">' + esc(step.name) + '</span>' +
      '<span class="tl-args">' + esc(step.args) + '</span>' +
      '<span class="tl-time">' + esc(step.note || '') + '</span>' +
      '</div>' +
      '<div class="tl-body"' + (open ? '' : ' hidden') + '>' +
      '<pre class="code" style="max-height:150px;overflow:auto">' + esc(step.result) + '</pre>' +
      '</div></div>'
    );
  }

  /* ---- 应用外壳 ---- */
  function titlebar(title, ws) {
    return (
      '<div class="titlebar">' +
      '<div class="tb-mark">' + ic('bot', 12) + '</div>' +
      '<span class="tb-name truncate">' + esc(title) + '</span>' +
      (ws ? '<span class="tb-chip">' + ic('folder', 10) + esc(ws) + '</span>' : '') +
      '<span class="tb-sp"></span>' +
      '<button class="icon-btn" data-act="theme" title="切换主题">' + ic('moon', 14) + '</button>' +
      '<div class="winctl">' +
      '<span title="最小化"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"><path d="M5 12h14"/></svg></span>' +
      '<span title="最大化"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4"><rect x="4" y="5" width="16" height="14" rx="1"/></svg></span>' +
      '<span class="close" title="关闭"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg></span>' +
      '</div></div>'
    );
  }

  function sessionList(activeID) {
    const D = window.DATA;
    let out = '';
    let lastGroup = null;
    const ordered = D.sessions.slice();
    ordered.forEach((s) => {
      if (s.group !== lastGroup) {
        lastGroup = s.group;
        out += '<div class="sess-group">' + esc(s.group) +
          '<span class="n">' + ordered.filter((x) => x.group === s.group).length + '</span></div>';
      }
      const on = s.id === activeID;
      out +=
        '<div class="sess-item' + (on ? ' is-on' : '') + '" data-sess="' + s.id + '">' +
        '<div class="si-name">' +
        (s.pinned ? '<span style="color:var(--accent)">' + ic('pin', 11) + '</span>' : '') +
        '<span class="truncate">' + esc(s.name) + '</span>' +
        '</div>' +
        '<div class="si-meta"><span>' + esc(s.time) + '</span>' +
        (s.branch ? '<span class="row row-g2" style="gap:3px">' + ic('branch', 10) + s.branch + '</span>' : '') +
        '</div>' +
        '<div class="si-act">' +
        '<button class="icon-btn" title="置顶">' + ic('pin', 12) + '</button>' +
        '<button class="icon-btn" title="归档">' + ic('archive', 12) + '</button>' +
        '<button class="icon-btn" title="删除">' + ic('trash', 12) + '</button>' +
        '</div></div>';
    });
    return out;
  }

  function sideBar(activeID, activeNav) {
    return (
      '<aside class="side">' +
      '<div class="side-brand">' +
      '<div class="logo">' + ic('bot', 17) + '</div>' +
      '<div class="grow"><b>WorkBaby</b><small>本地 · 单用户</small></div>' +
      iconBtn('arrow-left', '收起导航') +
      '</div>' +
      '<div class="side-actions">' +
      '<button class="side-btn is-primary" data-act="new-session">' + ic('plus', 15) +
      '<span class="grow" style="text-align:left">新建任务</span>' + kbd('Ctrl N') + '</button>' +
      '<button class="side-btn" data-act="palette">' + ic('search', 15) +
      '<span class="grow" style="text-align:left">搜索</span>' + kbd('Ctrl K') + '</button>' +
      '</div>' +
      '<div class="sess-head">' + plate('任务') +
      '<span class="spacer"></span>' +
      iconBtn('layers', '分组 / 平铺') +
      '</div>' +
      '<div class="sess-scroll">' + sessionList(activeID) + '</div>' +
      '<div class="side-foot">' +
      '<button class="foot-btn" data-act="lang">' + ic('globe2', 14) + '<span>中文</span></button>' +
      '<span class="spacer"></span>' +
      '<button class="foot-btn' + (activeNav === 'settings' ? ' is-on' : '') + '" data-act="settings">' +
      ic('settings', 14) + '<span>设置</span></button>' +
      '</div></aside>'
    );
  }

  function toasts(list) {
    return (
      '<div class="toasts">' +
      list.map((t) =>
        '<div class="toast">' +
        '<span style="color:var(--' + (t.kind === 'bad' ? 'bad' : t.kind === 'warn' ? 'warn' : 'ok') + ')">' +
        ic(t.kind === 'bad' ? 'warn' : t.kind === 'warn' ? 'warn' : 'check', 14) + '</span>' +
        '<span class="grow">' + esc(t.text) + '</span></div>'
      ).join('') +
      '</div>'
    );
  }

  window.U = {
    esc, tag, led, kbd, count, chip, btn, iconBtn, switchEl, enTag, progress, plate,
    empty, alert, field, menuItem, avatar, usageBadge, tlNode,
    titlebar, sideBar, toasts
  };
})();
