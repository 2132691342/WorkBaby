/* ============================================================
   原型主控：屏幕注册 / 导航 / 窗口装配 / 主题 / 快捷键 / Toast
   ============================================================ */
(function () {
  'use strict';

  const U = window.U;
  const CHAT = window.WB_CHAT;
  const SET = window.WB_SETTINGS;
  const APPS = window.WB_APPS;

  /* ---------- 屏幕注册 ---------- */
  const SCREENS = {};
  function reg(s) { SCREENS[s.id] = s; }

  /* 主链路 */
  const chatKeys = ['empty', 'live', 'streaming', 'approval', 'goal', 'error'];
  chatKeys.forEach((k) => {
    const s = CHAT.makeScreen(k);
    s.navGroup = '对话工作区';
    s.navName = s.title;
    reg(s);
  });
  /* 输入区与浮层 */
  ['slash', 'mention', 'model', 'params', 'context'].forEach((k) => {
    const s = CHAT.makeScreen(k);
    s.navGroup = '输入区';
    s.navName = s.title;
    reg(s);
  });
  const pal = APPS.palette();
  pal.navGroup = '输入区';
  pal.navName = pal.title;
  reg(pal);
  /* 右侧面板 */
  ['panel-ws', 'panel-changes', 'panel-tasks', 'panel-side'].forEach((k) => {
    const s = CHAT.makeScreen(k);
    s.navGroup = '右侧面板';
    s.navName = s.title;
    reg(s);
  });
  /* 设置中心 */
  SET.NAV.forEach((g) => {
    g.items.forEach((it) => {
      const s = SET.screen(it.id);
      s.navGroup = '设置 · ' + g.group;
      s.navName = it.name;
      reg(s);
    });
  });
  /* 工作台（看板式首页） */
  const boardScr = APPS.workspace();
  reg(boardScr);

  /* 独立窗口与系统 */
  [
    ['桌宠窗口', 'pet.desktop', APPS.pet()],
    ['托盘与系统', 'tray', APPS.tray()],
    ['启动引导', 'boot.start', APPS.boot('start')],
    ['启动失败', 'boot.error', APPS.boot('error')],
    ['设计系统', 'kit', APPS.kit()]
  ].forEach((row) => {
    const s = row[2];
    s.navGroup = '独立窗口与系统';
    s.navName = row[0];
    reg(s);
  });
  /* 首屏：界面地图（全部界面的入口） */
  reg({
    id: 'map',
    title: '界面地图',
    desc: '全部界面的入口：点任意卡片进入，界面内可直接操作',
    shell: 'bare',
    map: true,
    navGroup: '地图',
    navName: '界面地图',
    render: renderMap
  });

  /* ---------- 状态 ---------- */
  let current = 'chat.live';
  let theme = 'light';
  /* 支持 ?theme=dark 对照 */
  let toasts = [];

  /* ---------- 上下文 ---------- */
  const ctx = {
    go: function (id) { go(id); },
    rerender: function () { rerenderScreen(); },
    toast: function (text, kind) { pushToast(text, kind); },
    toggleTheme: function () { setTheme(theme === 'dark' ? 'light' : 'dark'); },
    setTheme: function (t) { setTheme(t); render(); mountScreen(); },
    state: function () { return { current, theme }; }
  };

  /* ---------- 主题 ---------- */
  function setTheme(t) {
    theme = t;
    document.documentElement.setAttribute('data-theme', t);
    const btn = document.querySelector('[data-act="theme"]');
    if (btn) btn.innerHTML = ic(t === 'dark' ? 'sun' : 'moon', 14);
    const kitBtn = document.querySelector('[data-act="kit-theme"]');
    if (kitBtn) kitBtn.innerHTML = ic(t === 'dark' ? 'sun' : 'moon', 14) + '切换主题';
  }

  /* ---------- Toast ---------- */
  function pushToast(text, kind) {
    toasts.push({ text, kind, id: Math.random() });
    renderToasts();
    const id = toasts[toasts.length - 1].id;
    window.setTimeout(() => {
      toasts = toasts.filter((t) => t.id !== id);
      renderToasts();
    }, 2600);
  }
  function renderToasts() {
    const host = document.querySelector('.win .toasts');
    if (!host) return;
    host.innerHTML = toasts.map((t) =>
      '<div class="toast">' +
      '<span style="color:var(--' + (t.kind === 'bad' ? 'bad' : t.kind === 'warn' ? 'warn' : 'ok') + ')">' +
      ic(t.kind === 'bad' || t.kind === 'warn' ? 'warn' : 'check', 14) + '</span>' +
      '<span class="grow">' + U.esc(t.text) + '</span></div>'
    ).join('');
  }

  /* ---------- 路由 ---------- */
  function mountScreen() {
    const sc = SCREENS[current];
    const host = document.getElementById('wb-screen');
    if (host && sc.mount) sc.mount(host, ctx);
  }
  function go(id) {
    if (!SCREENS[id]) id = 'chat.live';
    if (id.indexOf('chat.') === 0 && CHAT) CHAT.applyVariant(id.slice(5));
    current = id;
    location.hash = id;
    render();
    mountScreen();
  }

  /* ---------- 渲染：导航 ---------- */
  function renderNav() {
    let html = '';
    let lastGroup = null;
    Object.keys(SCREENS).forEach((id) => {
      const s = SCREENS[id];
      if (s.navGroup !== lastGroup) {
        lastGroup = s.navGroup;
        html += '<div class="protox-group">' + U.esc(lastGroup) + '</div>';
      }
      html +=
        '<button class="protox-item' + (id === current ? ' is-on' : '') + '" data-nav="' + id + '">' +
        ic(s.icon || groupIcon(s.navGroup), 13) +
        '<span class="grow truncate">' + U.esc(s.navName || s.title) + '</span>' +
        '</button>';
    });
    return html;
  }
  function groupIcon(g) {
    if (!g) return 'chat';
    if (g.indexOf('设置') === 0) {
      return ({ '设置 · 模型': 'sparkle', '设置 · 能力': 'wrench', '设置 · 数据': 'memory', '设置 · 系统': 'settings' }[g] || 'settings');
    }
    return ({ '对话工作区': 'chat', '输入区': 'slash', '右侧面板': 'layers', '独立窗口与系统': 'server' }[g] || 'chat');
  }

  /* ---------- 渲染：界面地图（首屏；一屏看懂有什么、点哪去哪） ---------- */
  function renderMap() {
    const ids = Object.keys(SCREENS).filter((id) => id !== 'map');
    let html = '';
    let lastGroup = null;
    ids.forEach((id, i) => {
      const s = SCREENS[id];
      if (s.navGroup !== lastGroup) {
        if (lastGroup) html += '</div>';
        lastGroup = s.navGroup;
        html += '<div class="map-group"><header><b>' + U.esc(lastGroup) + '</b><span>' +
          ids.filter((x) => SCREENS[x].navGroup === lastGroup).length + ' 屏</span></header><div class="map-grid">';
      }
      html +=
        '<button class="map-card" data-nav="' + id + '">' +
        '<div class="mc-top"><span class="mc-glyph">' + ic(groupIcon(s.navGroup), 15) + '</span>' +
        '<span class="mc-name">' + U.esc(s.navName || s.title) + '</span>' +
        '<span class="mc-idx">' + (i + 1) + '</span></div>' +
        '<div class="mc-desc">' + U.esc(s.desc || '') + '</div></button>';
    });
    if (lastGroup) html += '</div>';
    return (
      '<div class="map"><div class="map-inner">' +
      '<div class="map-hero"><div class="logo">' + ic('bot', 26) + '</div>' +
      '<div><b>WorkBaby 界面原型</b>' +
      '<p>能干活的本机 AI 助手 · Windows 桌面 · 共 ' + ids.length + ' 屏可交互界面</p></div>' +
      '<span class="spacer"></span><span class="tag is-accent">点卡片进入</span></div>' +
      '<div class="map-note">怎么看：点任意卡片进入该界面 → 界面里可以直接操作' +
      '（发消息、点审批、切设置项、开右侧面板）；顶部导览条随时可返回本页或翻上一屏 / 下一屏。' +
      '</div>' + html + '</div></div>'
    );
  }

  /* ---------- 渲染：舞台（导览条 + 全屏应用） ---------- */
  function render() {
    const sc = SCREENS[current];
    const app = document.getElementById('wb-app');
    app.innerHTML =
      '<div class="stage">' + renderTourbar(sc) +
      '<div class="win">' + renderWindow(sc) + '<div class="toasts"></div></div></div>';
    setTheme(theme);
    renderToasts();
  }

  function renderTourbar(sc) {
    const ids = Object.keys(SCREENS).filter((id) => id !== 'map');
    const i = ids.indexOf(current);
    const prev = sc.map ? ids[ids.length - 1] : (ids[i - 1] || ids[ids.length - 1]);
    const next = sc.map ? ids[0] : (ids[i + 1] || ids[0]);
    return (
      '<div class="tourbar">' +
      '<button class="tb-home" data-nav="map">' + ic('dashboard', 14) + '界面地图</button>' +
      '<button class="tb-btn" data-nav="' + prev + '" title="上一屏">' + ic('arrow-left', 15) + '</button>' +
      '<button class="tb-btn" data-nav="' + next + '" title="下一屏">' + ic('arrow-right', 15) + '</button>' +
      '<span class="tb-now">' + U.esc(sc.title) + '</span>' +
      '<span class="tb-idx">' + (sc.map ? '总览' : (i + 1) + ' / ' + ids.length) + '</span>' +
      '<span class="tb-desc">' + U.esc(sc.desc || '') + '</span>' +
      '<span class="spacer"></span>' +
      '<button class="tb-btn" data-act="theme" title="切换深浅色">' + ic(theme === 'dark' ? 'sun' : 'moon', 15) + '</button>' +
      '</div>'
    );
  }

  function renderWindow(sc) {
    if (sc.map) return sc.render();
    const bare = sc.shell === 'bare';
    const ws = sc.id.indexOf('chat') === 0 ? window.DATA.runScript.workspace : '';
    if (bare) {
      return '<div class="win-body"><main class="main" id="wb-screen" style="flex-direction:column">' +
        sc.render() + '</main></div>';
    }
    return U.titlebar(sc.title, ws) +
      '<div class="win-body">' +
      U.sideBar(sc.id.indexOf('chat') === 0 ? 's1' : '', sc.nav === 'settings' ? 'settings' : '') +
      '<main class="main" id="wb-screen">' + sc.render() + '</main></div>';
  }

  /* ---------- 屏幕内重渲染（交互驱动） ---------- */
  function rerenderScreen() {
    const sc = SCREENS[current];
    const host = document.getElementById('wb-screen');
    if (!host) return;
    host.innerHTML = sc.render();
    if (sc.mount) sc.mount(host, ctx);
    renderToasts();
  }

  /* 应用全屏铺满视口，无需缩放适配 */

  /* ---------- 全局事件 ---------- */
  document.addEventListener('click', (e) => {
    const t = (sel) => e.target.closest(sel);
    let el;
    if ((el = t('[data-nav]'))) return go(el.getAttribute('data-nav'));
    if ((el = t('[data-act="theme"]'))) return ctx.toggleTheme();
  });

  document.addEventListener('keydown', (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      go('palette');
    }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'n') {
      e.preventDefault();
      go('chat.empty');
    }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'b') {
      e.preventDefault();
      pushToast('导航栏已收起 / 展开（原型演示）');
    }
  });

  /* ---------- 引导 ---------- */
  let booted = false;
  function boot() {
    if (booted) return;
    booted = true;
    const start = (location.hash || '').replace('#', '');
    current = SCREENS[start] ? start : 'map';
    /* URL 参数便于对照：?theme=dark */
    try {
      if (new URLSearchParams(location.search).get('theme') === 'dark') theme = 'dark';
    } catch (e) { /* 老浏览器忽略 */ }
    if (current.indexOf('chat.') === 0 && CHAT) CHAT.applyVariant(current.slice(5));
    render();
    mountScreen();
  }

  window.addEventListener('hashchange', () => {
    const id = location.hash.replace('#', '');
    if (id && SCREENS[id] && id !== current) go(id);
  });

  window.WB_GO = go;

  document.addEventListener('DOMContentLoaded', boot);
  if (document.readyState !== 'loading') boot();
})();
