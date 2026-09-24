/* ============================================================
   屏幕：对话工作区（主链路）
   变体：empty / live / streaming / approval / goal / error
   支持：右侧面板四形态、输入区五个弹层、流式剧本、审批决策
   ============================================================ */
(function () {
  'use strict';

  const U = window.U;
  const D = window.DATA;
  const esc = U.esc;
  const plate = U.plate;
  const S = {
    variant: 'live',
    panel: null,
    approval: 'pending',
    decided: false,
    goalState: 'active',
    todos: null,
    pop: null,
    stopped: false,
    running: false
  };

  const R = D.runScript;

  /* ---------- 会话头 ---------- */
  function headHTML() {
    const live = S.variant === 'streaming';
    return (
      '<div class="chat-head">' +
      '<div class="ch-id">' +
      '<div class="ch-name">' + esc(R.title) +
      '<span style="color:var(--ink-3)">' + ic('edit', 12) + '</span></div>' +
      '<div class="ch-sub">' +
      (live
        ? '<span class="row row-g2">' + U.led('accent is-live') + '执行中 · 第 3 轮</span>'
        : '<span class="row row-g2">' + U.led('ok') + '空闲</span>') +
      '<span>·</span><span class="t-mono">' + esc(R.model) + '</span>' +
      '</div></div>' +
      '<button class="workspace-chip is-on" data-act="ws-pick">' + ic('folder', 11) + esc(R.workspace) + ic('chev-down', 10) + '</button>' +
      '<span class="spacer"></span>' +
      '<div class="ch-tools">' +
      (live
        ? '<button class="icon-btn is-danger" data-act="stop" title="停止">' + ic('square', 13) + '</button>'
        : '') +
      '<button class="icon-btn" data-act="pop-context" title="上下文占用">' + ic('activity', 15) + '</button>' +
      '<button class="icon-btn" data-act="focus" title="焦点模式">' + ic('crosshair', 15) + '</button>' +
      '<button class="icon-btn" data-act="panel-changes" title="文件变更">' + ic('branch', 15) +
      '<i class="led is-accent" style="position:absolute;transform:translate(9px,-9px);box-shadow:none"></i></button>' +
      '<button class="icon-btn' + (S.panel === 'ws' ? ' is-on' : '') + '" data-act="panel-ws" title="工作区">' + ic('folder', 15) + '</button>' +
      '<button class="icon-btn' + (S.panel === 'tasks' ? ' is-on' : '') + '" data-act="panel-tasks" title="后台任务">' + ic('inbox', 15) + '</button>' +
      '<button class="icon-btn' + (S.panel === 'side' ? ' is-on' : '') + '" data-act="panel-side" title="辅助对话">' + ic('quote', 15) + '</button>' +
      '<span class="divider"></span>' +
      '<button class="icon-btn" data-act="more" title="更多">' + ic('more', 15) + '</button>' +
      '</div></div>'
    );
  }

  /* ---------- 空态（新任务欢迎） ---------- */
  function welcomeHTML() {
    const cards = [
      { icon: 'code', t: '重构一段代码', d: '把 internal/rag 的打分逻辑拆成可测纯函数' },
      { icon: 'search', t: '分析一批日志', d: '找出昨天 14:00 之后的 5xx 突增原因' },
      { icon: 'book', t: '读一份文档', d: '把这份 PDF 的要点整理进知识库' },
      { icon: 'terminal', t: '跑一个任务', d: '构建 NSIS 安装包并校验产物' }
    ];
    return (
      '<div class="empty" style="padding-top:9vh">' +
      '<div class="boot-mark" style="width:52px;height:52px">' + ic('bot', 26) + '</div>' +
      '<div class="t-title" style="margin-top:14px">下午好，今天做点什么？</div>' +
      '<div class="t-sub" style="max-width:420px">我是一个能动手的助手：读写工作区文件、执行白名单命令、检索本地知识库。需要授权时我会停下来问你。</div>' +
      '<div style="display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-top:22px;width:560px">' +
      cards.map((c) =>
        '<button class="preset" data-act="demo-prompt" style="flex-direction:row;align-items:center;gap:10px">' +
        '<span style="color:var(--accent)">' + ic(c.icon, 16) + '</span>' +
        '<span class="grow"><span class="p-name" style="font-size:12.5px">' + esc(c.t) + '</span>' +
        '<span class="p-url" style="display:block;margin-top:2px">' + esc(c.d) + '</span></span></button>'
      ).join('') +
      '</div></div>'
    );
  }

  /* ---------- thinking ---------- */
  function thinkHTML(open) {
    return (
      '<div class="think">' +
      '<span class="think-head" data-act="toggle-think">' + ic('brain', 13) +
      '<span>' + (open ? '思考中' : '已思考 3.2k 字') + '</span>' +
      '<span>' + ic(open ? 'chev-down' : 'chev-right', 11) + '</span></span>' +
      (open ? '<div class="think-body">' + esc(R.thinking) + '</div>' : '') +
      '</div>'
    );
  }

  /* ---------- 工具时间线 ---------- */
  function timelineHTML(cfg) {
    cfg = cfg || {};
    const upto = cfg.upto == null ? R.steps.length : cfg.upto;
    let out = '<div class="tl">';
    R.steps.forEach((st, i) => {
      if (i > upto) return;
      const running = cfg.runningIndex === i;
      out += U.tlNode(st, i, running ? 'run' : 'done', cfg.openAll || running || false);
    });
    out += '</div>';
    return out;
  }

  /* ---------- 审批卡 ---------- */
  function approvalHTML() {
    const a = D.approval;
    const decided = S.decided;
    return (
      '<div class="approve is-danger">' +
      '<div class="approve-head">' + ic('shield', 14) +
      '<span class="ap-title">需要确认：执行本机命令</span>' +
      U.tag('不可逆', 'bad') +
      '<span class="spacer"></span>' +
      '<span class="t-mono faint">已等待 ' + (decided ? '0s' : a.elapsed + 's') + '</span></div>' +
      '<div class="approve-body">' +
      '<div class="cmd">' + esc(a.cmd) + '</div>' +
      '<div class="row row-g3" style="font-size:12px;color:var(--ink-2)">' +
      '<span class="row row-g2">' + ic('folder', 12) + '<span class="t-mono">' + esc(a.cwd) + '</span></span>' +
      '</div>' +
      '<div class="row row-g2" style="font-size:12px;color:var(--ink-3)">' + ic('lock', 12) +
      '<span>' + esc(a.reason) + '</span></div>' +
      '</div>' +
      (decided
        ? '<div class="approve-foot" style="background:var(--ok-soft);border-top-color:transparent">' +
          '<span style="color:var(--ok)">' + ic('check', 14) + '</span>' +
          '<span style="font-size:12px;color:var(--ok)">已批准（仅这一次）——继续执行</span></div>'
        : '<div class="approve-foot">' +
          '<span class="ap-note">拒绝会以回执形式返回给模型，它会换一条路继续</span>' +
          '<button class="btn btn-outline btn-sm" data-act="deny">拒绝</button>' +
          '<button class="btn btn-outline btn-sm" data-act="allow-session">本会话允许</button>' +
          '<button class="btn btn-danger btn-sm" data-act="allow-once">仍要批准</button></div>') +
      '</div>'
    );
  }

  /* ---------- 变更聚合条 ---------- */
  function changesBarHTML(withDiff) {
    const add = R.changes.reduce((s, c) => s + c.add, 0);
    const del = R.changes.reduce((s, c) => s + c.del, 0);
    return (
      '<div class="card" style="overflow:hidden">' +
      '<div class="row row-g3" style="padding:8px 12px;cursor:pointer" data-act="toggle-diff">' +
      ic('branch', 14) +
      '<span style="font-size:12.5px">' + R.changes.length + ' 个文件已更改</span>' +
      '<span class="t-mono" style="font-size:11px"><span style="color:var(--ok)">+' + add + '</span> ' +
      '<span style="color:var(--bad)">−' + del + '</span></span>' +
      '<span class="spacer"></span>' +
      '<button class="btn btn-outline btn-sm" data-act="rollback">全部撤销</button>' +
      '<span class="faint">' + ic(withDiff ? 'chev-up' : 'chev-down', 12) + '</span>' +
      '</div>' +
      (withDiff
        ? '<div style="border-top:1px solid var(--line)">' +
          R.changes.map((c) =>
            '<div class="change-row">' +
            '<span class="ops">' + (c.state === 'new' ? '<span class="add">+</span>' : '<span style="color:var(--accent)">~</span>') + '</span>' +
            '<span class="grow"><span class="path">' + esc(c.path) + '</span></span>' +
            '<span class="t-mono" style="font-size:11px"><span style="color:var(--ok)">+' + c.add + '</span> ' +
            '<span style="color:var(--bad)">−' + c.del + '</span></span></div>'
          ).join('') +
          '<div style="padding:12px;background:var(--bg-sunken)">' +
          '<pre class="diff">' +
          '<span class="d-meta">@@ internal/rag/retriever.go @@</span>\n' +
          '<span class="d-del">-func (r *Retriever) scoreByFTS(doc Doc, q string) float64 {</span>\n' +
          '<span class="d-del">-    base := r.fts(rawQuery(q))</span>\n' +
          '<span class="d-del">-    return base * r.weights.FTS</span>\n' +
          '<span class="d-del">-}</span>\n' +
          '<span class="d-add">+func (r *Retriever) scoreByFTS(doc Doc, q string) float64 {</span>\n' +
          '<span class="d-add">+    return ScoreFTS(r.fts, doc, q, r.weights)</span>\n' +
          '<span class="d-add">+}</span>' +
          '</pre></div></div>'
        : '') +
      '</div>'
    );
  }

  /* ---------- 一轮对话 ---------- */
  function turnHTML(cfg) {
    cfg = cfg || {};
    const phase = cfg.phase || 'done';
    const body = [];
    if (phase === 'streaming') body.push(U.tlNode(R.steps[2], 2, 'run', false));
    else if (phase !== 'early') body.push(timelineHTML({ upto: cfg.upto == null ? 3 : cfg.upto, runningIndex: cfg.runningIndex }));

    if (phase !== 'done') {
      body.push('<div class="prose">' + esc(R.finalText).replace(/\n\n/g, '</p><p>').replace(/\n/g, '<br>') + '<span class="caret"></span></div>');
    }

    return (
      '<div class="turn">' +
      '<div class="msg-u-wrap"><div class="msg-u">' + esc(R.userMsg) + '</div></div>' +
      '<div class="msg-a">' + U.avatar() +
      '<div class="msg-a-body">' +
      '<div class="msg-a-name">WorkBaby <span class="model">' + esc(R.model) + '</span>' +
      (phase === 'streaming' ? '<span class="t-mono faint" style="font-weight:400">· 第 3 轮</span>' : '') +
      '</div>' +
      thinkHTML(false) +
      body.join('') +
      (phase === 'done'
        ? '<div class="prose"><p>' + R.finalText.split('\n\n').map(esc).join('</p><p>') + '</p></div>' +
          changesBarHTML(cfg.diffOpen) +
          '<div class="row mt2"><span class="grow">' + U.usageBadge(R.usage, false) + '</span>' +
          '<div class="msg-acts" style="opacity:1">' +
          '<span class="act" data-act="copy">' + ic('copy', 12) + '复制</span>' +
          '<span class="act" data-act="regen">' + ic('refresh', 12) + '重新生成</span>' +
          '<span class="act" data-act="fork">' + ic('branch', 12) + '分叉</span>' +
          '<span class="sep"></span>' +
          '<span class="act" data-act="del">' + ic('trash', 12) + '删除</span>' +
          '</div></div>'
        : phase === 'streaming'
          ? '<div class="row mt1"><span class="grow">' + U.usageBadge({ in: '11.8k', out: '2.1k', cache: 91, sec: '14s' }, true) + '</span></div>'
          : '') +
      '</div></div></div>'
    );
  }

  /* ---------- 目标卡 / 计划 / 待办 ---------- */
  function goalCardHTML() {
    const g = D.goal;
    const todos = S.todos || g.todos;
    const done = todos.filter((t) => t.state === 'done').length;
    return (
      '<div class="goal-card">' +
      '<div class="gc-head">' + ic('target', 14) +
      '<span style="font-size:12.5px;font-weight:600">会话目标</span>' +
      U.tag(S.goalState === 'active' ? '推进中' : '已暂停', S.goalState === 'active' ? 'accent' : 'warn') +
      '<span class="spacer"></span>' +
      '<button class="icon-btn" data-act="goal-toggle" title="暂停 / 继续" style="width:22px;height:22px">' +
      ic(S.goalState === 'active' ? 'square' : 'play', 11) + '</button>' +
      '<button class="icon-btn" data-act="goal-close" title="清除目标" style="width:22px;height:22px">' + ic('x', 11) + '</button>' +
      '</div>' +
      '<div class="gc-body">' +
      '<div style="font-size:12.5px;line-height:1.6">' + esc(g.text) + '</div>' +
      '<div class="gc-rounds">' + ic('refresh', 11) + '<span>第 ' + g.round + ' / ' + g.maxRound + ' 轮</span>' +
      '<span class="spacer"></span><span>' + done + '/' + todos.length + ' 待办</span></div>' +
      '<div class="mt2">' + U.progress(Math.round((done / todos.length) * 100)) + '</div>' +
      '<div class="mt3">' + plate('本轮下一步') +
      '<div style="font-size:12px;color:var(--ink-2);margin-top:4px">' + esc(g.next) + '</div></div>' +
      '</div></div>'
    );
  }

  function todoPanelHTML() {
    const todos = S.todos || D.goal.todos;
    return (
      '<div class="card card-pad">' + U.plate('待办 · ' + todos.length) +
      '<div class="todo-list mt2">' +
      todos.map((t, i) =>
        '<div class="todo is-' + t.state + '" data-todo="' + i + '"><span class="dot"></span>' +
        '<span class="grow">' + esc(t.text) + '</span>' +
        (t.state === 'doing' ? U.tag('进行中', 'accent') : '') + '</div>'
      ).join('') +
      '</div></div>'
    );
  }

  function planPillHTML() {
    const todos = S.todos || D.goal.todos;
    const done = todos.filter((t) => t.state === 'done').length;
    return (
      '<button class="plan-pill" data-act="plan-open">' + ic('list', 13) +
      '<span>计划 ' + done + '/' + todos.length + '</span>' +
      U.progress(Math.round((done / todos.length) * 100)) +
      '<span>' + ic('chev-up', 11) + '</span></button>'
    );
  }

  /* ---------- 停止原因横幅 ---------- */
  function stopBannerHTML() {
    return (
      '<div class="alert is-warn">' + ic('square', 14) +
      '<div class="grow"><div class="a-t">运行在 12 秒后被停止</div>' +
      '<div class="mt1">已完成 2 个工具调用，检查点已写入第 2 轮——可以从中断处继续。</div></div>' +
      '<button class="btn btn-solid btn-sm">继续</button>' +
      '<button class="btn btn-ghost btn-sm">' + ic('x', 13) + '</button></div>'
    );
  }

  /* ---------- 输入区 ---------- */
  function composerHTML() {
    const live = S.variant === 'streaming';
    const pop = S.pop;
    let popHTML = '';
    if (pop === 'slash') popHTML = slashPopHTML();
    else if (pop === 'mention') popHTML = mentionPopHTML();
    else if (pop === 'model') popHTML = modelPopHTML();
    else if (pop === 'params') popHTML = paramsPopHTML();
    else if (pop === 'context') popHTML = contextPopHTML();

    return (
      '<div class="composer-wrap">' +
      (S.variant === 'goal' ? planPillHTML() : '') +
      (S.variant === 'streaming'
        ? '<div class="composer queue-dock" style="max-width:var(--w-read)">' +
          '<div class="qd-head">' + ic('layers', 12) + '<span>等待发送 · 2 条</span>' +
          '<span class="spacer"></span>' +
          '<button class="btn btn-outline btn-sm">' + ic('send', 12) + '立即发送</button>' +
          '<button class="icon-btn" style="width:24px;height:24px">' + ic('x', 12) + '</button>' +
          '</div>' +
          '<div class="qd-item">顺便把 chunker 的用例也补上</div>' +
          '<div class="qd-item">测试名统一用 TestScore_ 前缀</div>' +
          '</div>'
        : '') +
      '<div class="composer">' +
      (popHTML ? '<div class="pop composer-pop">' + popHTML + '</div>' : '') +
      '<textarea rows="1" placeholder="' + (live ? '输入指令，回车插入到当前运行（steer）' : '描述任务，或输入 / 调用命令、@ 引用技能与文件') + '"></textarea>' +
      '<div class="attach-row">' +
      '<span class="attach">' + ic('file-text', 12) + 'retriever.go<span class="x">' + ic('x', 11) + '</span></span>' +
      '<span class="attach">' + ic('image', 12) + '慢查询截图.png<span class="x">' + ic('x', 11) + '</span></span>' +
      '</div>' +
      '<div class="composer-bar">' +
      '<button class="cbar-chip" data-act="pop-model">' + ic('sparkle', 13) +
      '<span class="mono">' + esc(R.model) + '</span>' + ic('chev-down', 10) + '</button>' +
      '<button class="cbar-chip is-warn" data-act="pop-perm">' + ic('shield', 13) +
      '<span>标准</span>' + ic('chev-down', 10) + '</button>' +
      '<button class="cbar-chip" data-act="pop-params">' + ic('cpu', 13) +
      '<span class="mono">T 0.7 · 思考中</span></button>' +
      '<button class="cbar-chip" data-act="pop-slash">' + ic('slash', 13) + '<span>/ 命令</span></button>' +
      '<button class="cbar-chip" data-act="pop-mention">' + ic('at', 13) + '<span>@ 引用</span></button>' +
      '<button class="cbar-chip" data-act="pop-attach">' + ic('paperclip', 13) + '</button>' +
      '<span class="spacer"></span>' +
      '<button class="cbar-chip" data-act="pop-context" title="上下文占用">' +
      '<span class="ring" style="--p:34"></span><span class="mono">34%</span></button>' +
      '<button class="cbar-chip" data-act="pop-mode" title="权限档位">' + ic('crosshair', 13) + '</button>' +
      '<span style="width:6px"></span>' +
      (live
        ? '<button class="cbar-chip is-accent" data-act="steer">' + ic('zap', 14) + '<span>插入</span></button>' +
          '<button class="send-btn is-stop" data-act="stop" title="停止">' + ic('square', 15) + '</button>'
        : '<button class="send-btn" data-act="send" title="发送">' + ic('send', 15) + '</button>') +
      '</div></div></div>'
    );
  }

  /* ---------- 输入区弹层 ---------- */
  function slashPopHTML() {
    return (
      '<div class="pop-search">' + ic('search', 14) +
      '<input class="input" style="border:0;background:none;box-shadow:none;padding:0" placeholder="搜索命令" value="/" readonly>' +
      '<span class="kbd">Esc</span></div>' +
      '<div class="pop-list">' +
      D.slashes.map((s, i) =>
        '<div class="pop-row' + (i === 0 ? ' is-hi' : '') + '">' +
        '<span class="pr-glyph">' + ic('slash', 13) + '</span>' +
        '<span class="pr-t">' + esc(s.id) + '</span>' +
        (s.args ? '<span class="pr-t faint">' + esc(s.args) + '</span>' : '') +
        '<span class="pr-d grow truncate">' + esc(s.desc) + '</span>' +
        (s.src ? '<span class="pr-src">' + U.tag(s.src, 'solid') + '</span>' : '') +
        '</div>'
      ).join('') +
      '</div>'
    );
  }

  function mentionPopHTML() {
    const M = D.mentions;
    const groups = [
      { k: '技能', icon: 'zap', items: M.skill },
      { k: '知识库', icon: 'book', items: M.kdoc },
      { k: '文件', icon: 'file-text', items: M.file },
      { k: '文件夹', icon: 'folder', items: M.folder }
    ];
    return (
      '<div class="pop-search">' + ic('at', 14) +
      '<input class="input" style="border:0;background:none;box-shadow:none;padding:0" placeholder="搜索技能 / 知识 / 文件" value="@re" readonly>' +
      '</div><div class="pop-list">' +
      groups.map((g) =>
        '<div class="menu-label">' + g.k + '</div>' +
        g.items.map((it) =>
          '<div class="pop-row"><span class="pr-glyph">' + ic(g.icon, 13) + '</span>' +
          '<span class="pr-t">' + esc(it.name) + '</span>' +
          '<span class="pr-d truncate">' + esc(it.desc) + '</span></div>'
        ).join('')
      ).join('') +
      '</div>'
    );
  }

  function modelPopHTML() {
    return (
      '<div class="pop-search">' + ic('search', 14) +
      '<input class="input" style="border:0;background:none;box-shadow:none;padding:0" placeholder="搜索模型" readonly></div>' +
      '<div class="pop-list">' +
      '<div class="menu-label">最近使用</div>' +
      '<div class="pop-row is-hi"><span class="pr-glyph">' + ic('sparkle', 13) + '</span>' +
      '<span class="pr-t">deepseek-chat</span><span class="pr-d">DeepSeek 主力</span>' +
      '<span class="pr-src">' + U.led('ok') + '</span></div>' +
      '<div class="pop-row"><span class="pr-glyph">' + ic('brain', 13) + '</span>' +
      '<span class="pr-t">deepseek-reasoner</span><span class="pr-d">DeepSeek 推理</span>' +
      '<span class="pr-src">' + U.led('ok') + '</span></div>' +
      '<div class="menu-label">全部</div>' +
      '<div class="pop-row"><span class="pr-glyph">' + ic('sparkle', 13) + '</span>' +
      '<span class="pr-t">gpt-4o</span><span class="pr-d">OpenAI 备用</span>' +
      '<span class="pr-src">' + U.tag('熔断', 'bad') + '</span></div>' +
      '<div class="pop-row"><span class="pr-glyph">' + ic('cpu', 13) + '</span>' +
      '<span class="pr-t">qwen2.5:14b</span><span class="pr-d">本地 Ollama</span>' +
      '<span class="pr-src">' + U.tag('已停用', 'solid') + '</span></div>' +
      '</div>'
    );
  }

  function paramsPopHTML() {
    return (
      '<div style="padding:12px;min-width:320px">' +
      '<div class="row"><span class="grow" style="font-size:12.5px">temperature</span>' +
      '<span class="t-mono" style="color:var(--accent)">0.70</span></div>' +
      '<input type="range" min="0" max="2" step="0.05" value="0.7" style="width:100%;margin:6px 0 4px;accent-color:var(--accent)">' +
      '<div class="row between" style="font-size:11px;color:var(--ink-3)"><span>0</span><span>2.0</span></div>' +
      '<div class="hr"></div>' +
      '<div class="row"><span class="grow" style="font-size:12.5px">思考强度</span></div>' +
      '<div class="row row-g2 mt2">' + U.chip('关', false) + U.chip('低', false) + U.chip('中', true) + U.chip('高', false) + '</div>' +
      '<div class="hr"></div>' +
      '<div class="kv" style="font-size:12px">' +
      '<dt>生效值来源</dt><dd>会话覆盖</dd>' +
      '<dt>上下文窗口</dt><dd>128k</dd>' +
      '<dt>压缩比</dt><dd>0.55</dd>' +
      '<dt>单轮预算</dt><dd>60k tokens</dd>' +
      '</div>' +
      '<div class="mt3"><button class="btn btn-outline btn-sm">恢复跟随全局</button></div>' +
      '</div>'
    );
  }

  function contextPopHTML() {
    const segs = [
      { name: 'system 段', p: 8, c: 'var(--accent)' },
      { name: '记忆与技能', p: 5, c: 'var(--ok)' },
      { name: '历史消息', p: 14, c: 'var(--ink-3)' },
      { name: '工具结果', p: 7, c: 'var(--warn)' }
    ];
    return (
      '<div style="padding:14px;min-width:340px">' +
      '<div class="row row-g3"><span class="t-num" style="font-size:22px">34%</span>' +
      '<div class="grow"><div style="font-size:12px">已用 43.5k / 128k</div>' +
      '<div class="faint" style="font-size:11px">实测口径 · 上一轮返回</div></div>' +
      '<span class="ring" style="--p:34;width:26px;height:26px"></span></div>' +
      '<div class="mt3" style="display:flex;flex-direction:column;gap:6px">' +
      segs.map((s) =>
        '<div class="dist-row"><span class="dl" style="width:76px">' + s.name + '</span>' +
        '<span class="bar"><i style="width:' + s.p * 4 + '%;background:' + s.c + '"></i></span>' +
        '<span class="dn">' + s.p + '%</span></div>'
      ).join('') +
      '</div>' +
      '<div class="mt3 row row-g2"><button class="btn btn-outline btn-sm">' + ic('activity', 13) + '明细</button>' +
      '<button class="btn btn-solid btn-sm">压缩历史</button></div>' +
      '</div>'
    );
  }

  /* ---------- 右侧面板 ---------- */
  function panelHTML() {
    if (!S.panel) return '';
    const wide = S.panel === 'side' ? ' is-wide' : '';
    let title = '';
    let body = '';
    if (S.panel === 'ws') {
      title = '工作区';
      body = wsPanelBody();
    } else if (S.panel === 'changes') {
      title = '变更与产物';
      body = changesPanelBody();
    } else if (S.panel === 'tasks') {
      title = '后台任务';
      body = tasksPanelBody();
    } else {
      title = '辅助对话';
      body = sidePanelBody();
    }
    return (
      '<aside class="panel' + wide + '">' +
      '<div class="panel-head">' + U.plate(title) + '<span class="spacer"></span>' +
      U.iconBtn('x', '收起面板', '') + '</div>' +
      '<div class="panel-body">' + body + '</div></aside>'
    );
  }

  function wsPanelBody() {
    const tree = [
      { d: 0, n: 'internal', dir: true, open: true },
      { d: 1, n: 'rag', dir: true, open: true },
      { d: 2, n: 'retriever.go', dir: false, sz: '5.2 KB', on: true },
      { d: 2, n: 'scorer.go', dir: false, sz: '1.4 KB', isNew: true },
      { d: 2, n: 'scorer_test.go', dir: false, sz: '2.8 KB', isNew: true },
      { d: 2, n: 'chunker.go', dir: false, sz: '3.1 KB' },
      { d: 1, n: 'core', dir: true, open: false },
      { d: 1, n: 'tool', dir: true, open: false },
      { d: 0, n: 'doc', dir: true, open: false },
      { d: 0, n: 'go.mod', dir: false, sz: '412 B' }
    ];
    return (
      '<div class="panel-seg"><button class="is-on">目录树</button><button>产物</button></div>' +
      '<div class="row row-g2" style="padding:10px 12px 4px">' +
      '<div class="search grow">' + ic('search', 13) + '<input class="input" placeholder="筛选文件"></div>' +
      U.iconBtn('refresh', '刷新') + '</div>' +
      '<div class="tree">' +
      tree.map((t) =>
        '<div class="tree-row' + (t.dir ? ' is-dir' : '') + (t.on ? ' is-on' : '') + ' tree-indent" style="padding-left:' + (6 + t.d * 16) + 'px">' +
        ic(t.dir ? (t.open ? 'folder-open' : 'folder') : 'file-text', 13) +
        '<span class="truncate">' + esc(t.n) + '</span>' +
        (t.isNew ? U.tag('新', 'ok') : '') +
        (t.sz ? '<span class="sz">' + t.sz + '</span>' : '') +
        '</div>'
      ).join('') +
      '</div>' +
      '<div style="padding:0 12px 12px">' + U.plate('选中预览') +
      '<pre class="code mt2" style="max-height:180px;overflow:auto">' +
      '<span class="ln">64</span> func ScoreFTS(fts FTSFn, doc Doc, q string, w Weights) float64 {\n' +
      '<span class="ln">65</span>     base := fts(rawQuery(q))\n' +
      '<span class="ln">66</span>     if base == 0 { return 0 }\n' +
      '<span class="ln">67</span>     return base * w.FTS\n' +
      '<span class="ln">68</span> }' +
      '</pre></div>'
    );
  }

  function changesPanelBody() {
    return (
      '<div class="panel-seg"><button class="is-on">变更 3</button><button>工件 1</button></div>' +
      '<div style="padding:10px 12px">' + U.plate('本轮 · 21:04') + '</div>' +
      R.changes.map((c) =>
        '<div class="change-row">' +
        '<span style="color:' + (c.state === 'new' ? 'var(--ok)' : 'var(--accent)') + '">' +
        ic(c.state === 'new' ? 'plus' : 'edit', 13) + '</span>' +
        '<span class="grow"><span class="path">' + esc(c.path) + '</span>' +
        '<div class="t-mono" style="font-size:10.5px;color:var(--ink-3);margin-top:3px">' +
        '+ ' + c.add + ' 行 · − ' + c.del + ' 行</div></span>' +
        '<button class="btn btn-outline btn-sm" data-act="rollback">回滚</button>' +
        '</div>'
      ).join('') +
      '<div style="padding:12px">' + U.plate('diff · scorer.go') +
      '<pre class="diff mt2 card" style="padding:10px;overflow:auto">' +
      '<span class="d-meta">@@ -0,0 +1,6 @@</span>\n' +
      '<span class="d-add">+// ScoreFTS 纯函数：权重显式入参，便于单测</span>\n' +
      '<span class="d-add">+func ScoreFTS(fts FTSFn, doc Doc, q string, w Weights) float64 {</span>\n' +
      '<span class="d-add">+    base := fts(rawQuery(q))</span>\n' +
      '<span class="d-add">+    return base * w.FTS</span>\n' +
      '<span class="d-add">+}</span>' +
      '</pre></div>'
    );
  }

  function tasksPanelBody() {
    const tasks = [
      { name: '重跑全量回归测试', state: 'running', t: '2 分 11 秒', agent: 'log-hunter' },
      { name: '生成 doc/09 的接口差异表', state: 'pending', t: '排队中', agent: 'doc-writer' },
      { name: '扫描依赖里的过期包', state: 'done', t: '1 分 02 秒', agent: '—' },
      { name: '导出本周 token 报表', state: 'failed', t: '失败：网络超时', agent: '—' }
    ];
    const kind = { running: 'accent', pending: 'solid', done: 'ok', failed: 'bad' };
    return (
      '<div style="padding:12px">' +
      '<button class="btn btn-primary btn-sm" style="width:100%">' + ic('plus', 13) + '提交后台任务</button></div>' +
      tasks.map((t) =>
        '<div style="padding:10px 12px;border-bottom:1px solid var(--line)">' +
        '<div class="row row-g2"><span class="grow" style="font-size:12.5px">' + esc(t.name) + '</span>' +
        U.tag(t.state === 'running' ? '运行中' : t.state === 'pending' ? '排队' : t.state === 'done' ? '完成' : '失败', kind[t.state]) +
        '</div>' +
        '<div class="row row-g3 mt1" style="font-size:11px;color:var(--ink-3);font-family:var(--font-mono)">' +
        '<span>' + esc(t.t) + '</span><span>agent: ' + esc(t.agent) + '</span>' +
        '<span class="spacer"></span>' +
        (t.state === 'running' ? '<span style="color:var(--bad);cursor:pointer">取消</span>' : '') +
        '<span style="cursor:pointer">查看</span></div></div>'
      ).join('')
    );
  }

  function sidePanelBody() {
    return (
      '<div style="padding:12px">' +
      '<div class="card card-pad is-accent" style="border-color:var(--accent-line);background:var(--accent-soft)">' +
      '<div class="row row-g2" style="font-size:12.5px;font-weight:600">' + ic('quote', 13) + '从主会话带上下文提问</div>' +
      '<div class="faint mt1" style="font-size:11.5px">辅助对话不会写入主会话历史，适合临时追问与试算。</div></div>' +
      '<div class="mt3">' + U.plate('辅助对话 · 1') +
      '<div style="display:flex;flex-direction:column;gap:8px;margin-top:8px">' +
      '<div class="pm-u" style="align-self:flex-end;background:var(--user-bubble);color:var(--ink);border:1px solid var(--line-2);padding:6px 10px;border-radius:5px;max-width:90%;font-size:12px">用 scorer.go 的写法，chunker 那边怎么拆更合适？</div>' +
      '<div class="card" style="padding:9px 11px;font-size:12px;line-height:1.7;color:var(--ink-2);max-width:90%">chunker 的耦合点在 Split 里读了 config。建议同样把窗口参数提出来，拆成 <code class="t-mono">Chunk(text string, opts ChunkOpts) []Chunk</code>，先补用例再改调用方。</div>' +
      '</div></div>' +
      '<div class="mt3"><button class="btn btn-outline btn-sm" style="width:100%">' + ic('search', 13) + '进入审查流（对未暂存改动）</button></div>' +
      '</div>'
    );
  }

  /* ---------- 装配 ---------- */
  function msgsHTML() {
    if (S.variant === 'empty') return welcomeHTML();
    let out = '';
    if (S.variant === 'goal') out += todoPanelHTML() + '<div style="height:4px"></div>';
    if (S.variant === 'error') out += '<div style="max-width:var(--w-read);width:100%">' + stopBannerHTML() + '</div>';
    out += turnHTML({
      phase: S.variant === 'streaming' ? 'streaming' : 'done',
      diffOpen: S.variant === 'live' || S.variant === 'panel-changes',
      runningIndex: S.variant === 'streaming' ? 3 : -1
    });
    if (S.variant === 'approval') {
      out +=
        '<div class="msg-a">' + U.avatar() +
        '<div class="msg-a-body"><div class="msg-a-name">WorkBaby <span class="model">' + esc(R.model) + '</span></div>' +
        approvalHTML() + '</div></div>';
    }
    return out;
  }

  function render() {
    return (
      '<div class="chat">' +
      headHTML() +
      '<div class="msgs"><div class="msgs-inner" id="wb-msgs">' + msgsHTML() + '</div></div>' +
      composerHTML() +
      (S.variant === 'goal' ? goalCardHTML() : '') +
      '</div>' + panelHTML()
    );
  }

  /* ---------- 交互 ---------- */
  function mount(root, ctx) {
    const rerender = () => ctx.rerender();

    root.addEventListener('click', (e) => {
      const el = (sel) => e.target.closest(sel);
      let t;
      /* 面板开关 */
      if ((t = el('[data-act="panel-ws"]'))) return togglePanel('ws', rerender);
      if ((t = el('[data-act="panel-changes"]'))) return togglePanel('changes', rerender);
      if ((t = el('[data-act="panel-tasks"]'))) return togglePanel('tasks', rerender);
      if ((t = el('[data-act="panel-side"]'))) return togglePanel('side', rerender);
      /* 输入区弹层 */
      if ((t = el('[data-act="pop-slash"]'))) return setPop('slash', rerender);
      if ((t = el('[data-act="pop-mention"]'))) return setPop('mention', rerender);
      if ((t = el('[data-act="pop-model"]'))) return setPop('model', rerender);
      if ((t = el('[data-act="pop-params"]'))) return setPop('params', rerender);
      if ((t = el('[data-act="pop-context"]'))) return setPop('context', rerender);
      if ((t = el('[data-act="pop-attach"]'))) return ctx.toast('已选择：retriever.go、慢查询截图.png');
      if ((t = el('[data-act="pop-perm"]'))) return permMenu(t, rerender, ctx);
      if ((t = el('[data-act="pop-mode"]'))) return ctx.toast('焦点模式：仅保留消息与输入框');
      /* 审批 */
      if ((t = el('[data-act="allow-once"]'))) return decide('once', rerender, ctx);
      if ((t = el('[data-act="allow-session"]'))) return decide('session', rerender, ctx);
      if ((t = el('[data-act="deny"]'))) return decide('deny', rerender, ctx);
      /* 目标 / 待办 */
      if ((t = el('[data-act="goal-toggle"]'))) {
        S.goalState = S.goalState === 'active' ? 'paused' : 'active';
        return rerender();
      }
      if ((t = el('[data-act="goal-close"]'))) {
        S.variant = 'live';
        return rerender();
      }
      if ((t = el('[data-todo]'))) {
        const i = +t.getAttribute('data-todo');
        const todos = (S.todos || D.goal.todos).map((x) => ({ ...x }));
        todos[i].state = todos[i].state === 'done' ? 'todo' : 'done';
        S.todos = todos;
        return rerender();
      }
      /* 变更 */
      if ((t = el('[data-act="toggle-diff"], [data-act="rollback"]'))) {
        if (t.getAttribute('data-act') === 'rollback') return ctx.toast('已回滚 3 个文件（快照恢复）', 'warn');
        S.diffOpen = !S.diffOpen;
        return rerender();
      }
      /* 时间线展开 */
      if ((t = el('[data-tl]'))) {
        const node = t;
        node.classList.toggle('is-open');
        const body = node.querySelector('.tl-body');
        if (body) body.hidden = !body.hidden;
        return;
      }
      if ((t = el('[data-act="toggle-think"]'))) {
        ctx.toast('思维链：3.2k 字（点击可展开原文）');
        return;
      }
      /* 消息操作 */
      if ((t = el('[data-act="copy"]'))) return ctx.toast('已复制到剪贴板');
      if ((t = el('[data-act="regen"]'))) return ctx.toast('重新生成这一轮…');
      if ((t = el('[data-act="fork"]'))) return ctx.toast('已从该消息分叉出新会话');
      if ((t = el('[data-act="del"]'))) return ctx.toast('已删除该条消息', 'warn');
      /* 发送 / 停止 / steer */
      if ((t = el('[data-act="send"]'))) return startRun(root, ctx);
      if ((t = el('[data-act="steer"]'))) return ctx.toast('已插入当前运行（下个工具调用后生效）');
      if ((t = el('[data-act="stop"]'))) {
        S.variant = 'error';
        return rerender();
      }
      if ((t = el('[data-act="demo-prompt"]'))) return startRun(root, ctx);
      if ((t = el('[data-act="ws-pick"]'))) return ctx.toast('工作区选择器（D:\\GoFiles\\WorkBaby 已绑定）');
      if ((t = el('[data-act="more"]'))) return moreMenu(t, ctx);
      if ((t = el('[data-act="lang"]'))) return ctx.toast('已切换为 English');
      if ((t = el('[data-act="settings"]'))) return ctx.go('set.models');
      if ((t = el('[data-sess]'))) return ctx.toast('切换到会话：' + t.querySelector('.si-name').textContent.trim());
      /* 点击空白关闭弹层 */
      if (S.pop && !e.target.closest('.composer')) {
        S.pop = null;
        return rerender();
      }
    });

    /* 输入框：Enter 发送 */
    const ta = root.querySelector('.composer textarea');
    if (ta) {
      ta.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault();
          startRun(root, ctx);
        }
      });
    }

    /* 新消息 / 审批到来时滚到底部（真实产品同样需要） */
    const msgs = root.querySelector('#wb-msgs');
    if (msgs) msgs.scrollTop = msgs.scrollHeight;
  }

  function togglePanel(kind, rerender) {
    S.panel = S.panel === kind ? null : kind;
    rerender();
  }
  function setPop(kind, rerender) {
    S.pop = S.pop === kind ? null : kind;
    rerender();
  }

  function permMenu(anchor, rerender, ctx) {
    const rect = anchor.getBoundingClientRect();
    const win = anchor.closest('.win').getBoundingClientRect();
    const pop = document.createElement('div');
    pop.className = 'pop';
    pop.style.left = rect.left - win.left + 'px';
    pop.style.bottom = win.bottom - rect.top + 8 + 'px';
    pop.innerHTML =
      '<div class="menu-label">权限档位</div>' +
      U.menuItem('标准模式', 'shield', '', '写操作先问') +
      U.menuItem('自动编辑', 'edit', '', '文件免问') +
      U.menuItem('完全放行', 'zap', 'is-danger', '工作区全放行');
    anchor.closest('.win').appendChild(pop);
    const off = (ev) => {
      if (!pop.contains(ev.target) && ev.target !== anchor) {
        pop.remove();
        document.removeEventListener('click', off);
      }
    };
    setTimeout(() => document.addEventListener('click', off), 0);
    pop.addEventListener('click', (ev) => {
      const item = ev.target.closest('.menu-item');
      if (item) {
        ctx.toast('权限档位已切换：' + item.textContent.trim());
        pop.remove();
        rerender();
      }
    });
  }

  function moreMenu(anchor, ctx) {
    const rect = anchor.getBoundingClientRect();
    const win = anchor.closest('.win').getBoundingClientRect();
    const pop = document.createElement('div');
    pop.className = 'pop';
    pop.style.right = win.right - rect.right + 'px';
    pop.style.top = rect.bottom - win.top + 6 + 'px';
    pop.innerHTML =
      U.menuItem('压缩历史', 'activity', '', '保留结论') +
      U.menuItem('导出会话 Markdown', 'download') +
      U.menuItem('转为后台任务', 'inbox') +
      '<div class="menu-sep"></div>' +
      U.menuItem('清空消息', 'x', 'is-danger') +
      U.menuItem('删除会话', 'trash', 'is-danger');
    anchor.closest('.win').appendChild(pop);
    const off = (ev) => {
      if (!pop.contains(ev.target) && ev.target !== anchor) {
        pop.remove();
        document.removeEventListener('click', off);
      }
    };
    setTimeout(() => document.addEventListener('click', off), 0);
    pop.addEventListener('click', (ev) => {
      if (ev.target.closest('.menu-item')) {
        ctx.toast('已执行：' + ev.target.closest('.menu-item').textContent.trim());
        pop.remove();
      }
    });
  }

  /* ---------- 流式剧本 ---------- */
  let timer = null;
  function startRun(root, ctx) {
    if (S.running) return;
    S.pop = null;
    S.running = true;
    S.variant = 'live';
    S.decided = false;
    ctx.rerender();

    window.setTimeout(() => {
      S.variant = 'streaming';
      ctx.rerender();
      const msgs = root.querySelector('#wb-msgs');
      if (msgs) msgs.scrollTop = msgs.scrollHeight;
      step(root, ctx, 0);
    }, 340);
  }

  function stopTimer() {
    if (timer) {
      window.clearTimeout(timer);
      timer = null;
    }
  }

  function step(root, ctx, i) {
    stopTimer();
    if (i === 0) {
      timer = window.setTimeout(() => step(root, ctx, 1), 700);
      return;
    }
    if (i === 1) {
      /* 审批插入 */
      S.variant = 'approval';
      S.decided = false;
      ctx.rerender();
      const msgs = root.querySelector('#wb-msgs');
      if (msgs) msgs.scrollTop = msgs.scrollHeight;
      return;
    }
  }

  function decide(kind, rerender, ctx) {
    S.decided = true;
    ctx.toast(kind === 'deny' ? '已拒绝：模型收到回执，将换一条路继续' : '已批准' + (kind === 'session' ? '（本会话免问）' : '（仅这一次）'), kind === 'deny' ? 'warn' : 'ok');
    rerender();
    const root = document.getElementById('wb-screen');
    window.setTimeout(() => {
      S.variant = 'live';
      rerender();
      ctx.toast('本轮完成：3 个文件已更改 · go test 通过', 'ok');
    }, 1200);
  }

  /* ---------- 屏幕注册 ---------- */
  const VARIANTS = {
    empty: ['空态 · 新任务', '草稿态不落库：点一下就出空会话的情况已消失，示例卡可直接起一个任务'],
    live: ['会话进行中', '一轮完整 run：思维链折叠、工具时间线、变更聚合、用量徽标'],
    streaming: ['流式执行中', '工具正在跑 + 正文边出边渲染；可插话（steer）、可排队、可停止'],
    approval: ['审批暂停', '危险命令停下来问：批准 / 本会话允许 / 拒绝，拒绝是回执不是错误'],
    goal: ['目标模式', '目标卡 + 待办推进 + 计划胶囊；未达成会自动续跑'],
    error: ['中断与恢复', '停止原因横幅：「继续」从检查点续跑，不丢已完成的工具调用'],
    'panel-ws': ['面板 · 工作区', '右侧 336px：目录树 / 产物 / 选中预览，右键可加入对话'],
    'panel-changes': ['面板 · 变更', '本轮文件变更与 diff，逐条或整体回滚'],
    'panel-tasks': ['面板 · 后台任务', '提交即返回，后台跑完出结果；状态实时更新'],
    'panel-side': ['面板 · 辅助对话', '不污染主历史的临时追问；也可进入审查流'],
    palette: ['命令面板', 'Ctrl+K 全局命令：操作 / 权限 / 工具 / 会话 / 导航五类'],
    slash: ['斜杠命令', '/ 唤起命令面板，带来源标记（内置 / 用户 / 工作区）'],
    mention: ['@ 引用', '@ 唤起技能、知识、文件、文件夹四组引用'],
    model: ['模型选择', '搜索 + 最近使用 + 按供应商分组 + 熔断状态'],
    params: ['采样参数', 'temperature / 思考强度 / 预算，显示生效值来源'],
    context: ['上下文占用', '分段占用明细；接近 70% 提示压缩历史']
  };

  function makeScreen(key) {
    return {
      id: 'chat.' + key,
      title: VARIANTS[key] ? VARIANTS[key][0] : '对话工作区',
      desc: VARIANTS[key] ? VARIANTS[key][1] : '',
      shell: 'app',
      render: function () {
        return render();
      },
      mount: function (root, ctx) {
        mount(root, ctx);
      }
    };
  }

  /* 变体 → 初始状态 */
  function applyVariant(key) {
    stopTimer();
    S.running = false;
    S.pop = null;
    S.decided = false;
    S.todos = null;
    S.goalState = 'active';
    S.diffOpen = false;
    if (key.indexOf('panel-') === 0) {
      S.variant = 'live';
      S.panel = key.slice(6);
    } else if (['empty', 'live', 'streaming', 'approval', 'goal', 'error'].indexOf(key) >= 0) {
      S.variant = key;
      S.panel = null;
      if (key === 'live') S.diffOpen = true;
    } else {
      S.variant = 'live';
      S.panel = null;
      S.pop = key;
      if (key === 'palette') S.pop = null;
    }
  }

  window.WB_CHAT = { makeScreen, applyVariant };
})();
