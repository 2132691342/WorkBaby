/* ============================================================
   屏幕：设置中心（19 个 section）
   信息架构改进：左导航一级化（全部子项直接列出，去掉「组按钮 + 顶部 chip」两跳）
   ============================================================ */
(function () {
  'use strict';

  const U = window.U;
  const D = window.DATA;

  /* ---------- 导航定义 ---------- */
  const NAV = [
    { group: '模型', items: [
      { id: 'models', name: '模型服务', icon: 'sparkle' },
      { id: 'search', name: '网络搜索', icon: 'search' }
    ] },
    { group: '能力', items: [
      { id: 'tools', name: '工具', icon: 'wrench', badge: '23' },
      { id: 'skills', name: '技能', icon: 'zap', badge: '5' },
      { id: 'mcp', name: 'MCP 服务', icon: 'server', badge: '3' },
      { id: 'subagents', name: '子智能体', icon: 'bot' },
      { id: 'commands', name: '自定义命令', icon: 'slash' },
      { id: 'hooks', name: '用户钩子', icon: 'webhook' }
    ] },
    { group: '数据', items: [
      { id: 'memory', name: '记忆中心', icon: 'memory' },
      { id: 'kdocs', name: '知识库', icon: 'book' },
      { id: 'wiki', name: '仓库导读', icon: 'wiki' },
      { id: 'files', name: '文件', icon: 'file' },
      { id: 'dashboard', name: '仪表盘', icon: 'chart' },
      { id: 'runs', name: '运行历史', icon: 'play' }
    ] },
    { group: '系统', items: [
      { id: 'appearance', name: '外观', icon: 'sun' },
      { id: 'advanced', name: '安全与高级', icon: 'shield' },
      { id: 'pet', name: '桌宠', icon: 'paw' },
      { id: 'docs', name: '文档', icon: 'scroll' },
      { id: 'about', name: '关于', icon: 'info' }
    ] }
  ];

  const META = {
    models: ['模型服务', '三种协议（OpenAI 兼容 / Anthropic / Ollama）+ 全局默认采样参数；熔断状态直接可见'],
    search: ['网络搜索', '免 Key 的 DuckDuckGo，可整体开关'],
    tools: ['工具', '按能力域分组；风险等级用颜色标注，启停即时生效'],
    skills: ['技能', 'SKILL.md 解析出的可执行能力；内置只读、导入可编辑'],
    mcp: ['MCP 服务', '文件为源（mcp.json）+ DB 同步；env 掩码显示'],
    subagents: ['子智能体', '人设 / 工具策略 / 预算 / 模型四重隔离的委派对象'],
    commands: ['自定义命令', '/ 唤起；可来自设置页、文件或工作区'],
    hooks: ['用户钩子', '外部脚本裁决工具调用：事件 + 匹配器 + 决策'],
    memory: ['记忆中心', '单一 MEMORY.md + FTS5 派生索引；四个分节直接编辑'],
    kdocs: ['知识库', '文件 / URL / 文本三类来源；索引状态与片段数可见'],
    wiki: ['仓库导读', '确定性扫描生成，不依赖 LLM；按语言与入口文件建立全局认知'],
    files: ['文件', '粘贴与上传的附件统一管理；受管文件走本地服务预览'],
    dashboard: ['仪表盘', 'Token 三线、缓存命中率、执行审计；数字全部来自本地表'],
    runs: ['运行历史', '每次 run 的模型、轮次、耗时构成与事件回放'],
    appearance: ['外观', '主题、背景、代码字体与界面缩放'],
    advanced: ['安全与高级', 'exec 白名单、免审授权、托盘行为与记忆开关'],
    pet: ['桌宠', '形象资产、行为配置与预览；独立窗口跟随 Agent 状态'],
    docs: ['文档', '内置设计文档，与仓库 doc/ 同步'],
    about: ['关于', '版本、数据目录、内置运行时与计数']
  };

  /* ---------- 通用片段 ---------- */
  function hero(icon, title, sub, acts) {
    return (
      '<div class="set-hero"><div class="hero-glyph">' + ic(icon, 19) + '</div>' +
      '<div class="hero-txt"><div class="t-title">' + U.esc(title) + '</div>' +
      '<div class="t-sub mt1">' + sub + '</div></div>' +
      '<div class="hero-act">' + (acts || '') + '</div></div>'
    );
  }
  function card(title, body, foot, headActs) {
    return (
      '<div class="set-card">' +
      (title ? '<header>' + U.plate(title) + '<span class="spacer"></span>' + (headActs || '') + '</header>' : '') +
      '<div class="body">' + body + '</div>' +
      (foot ? '<footer>' + foot + '</footer>' : '') +
      '</div>'
    );
  }
  function sw(on, label, hint) {
    return (
      '<div class="row row-g3" style="padding:2px 0">' +
      '<div class="grow">' + (label ? '<div style="font-size:13px">' + U.esc(label) + '</div>' : '') +
      (hint ? '<div class="faint" style="font-size:11.5px">' + hint + '</div>' : '') + '</div>' +
      U.switchEl(on, 'js-sw') + '</div>'
    );
  }
  function capTag(v) {
    if (v === true) return U.tag('支持', 'ok');
    if (v === false) return U.tag('不支持', 'solid');
    return U.tag('自动', 'solid');
  }

  /* ============================================================
     模型
     ============================================================ */
  function tabModels() {
    const params = [
      ['默认 temperature', '0.7', '未在会话与会话级覆盖时生效'],
      ['默认思考强度', '中', 'off / low / medium / high'],
      ['压缩比', '0.55', '上下文达到窗口的该比例时自动折叠'],
      ['单次输入上限（字符）', '120000', '超过即截断并给出告警'],
      ['单 run token 预算', '60000', '超出后以 token_budget 停止，可续跑']
    ];
    const stateTag = (s) => s === 'ok' ? U.tag('正常', 'ok') : s === 'open' ? U.tag('熔断', 'bad') : U.tag('停用', 'solid');
    return (
      hero('sparkle', '模型服务', '只支持三种协议：OpenAI 兼容 / Anthropic / Ollama；model.json 为配置源，密钥 AES-256-GCM 加密',
        U.btn('新增模型', 'primary', 'plus')) +
      card('全局默认参数',
        '<div class="form-grid">' +
        params.map((p) =>
          '<div class="field">' + (p[2] ? '' : '') + '<label>' + U.esc(p[0]) + '</label>' +
          '<input class="input mono" value="' + U.esc(p[1]) + '">' +
          '<div class="hint">' + U.esc(p[2]) + '</div></div>'
        ).join('') +
        '</div>',
        '<span class="grow"></span>' + U.btn('保存', 'primary')) +
      card('已配置 ' + D.providers.length,
        '<table class="tbl"><thead><tr>' +
        '<th>名称</th><th>模型</th><th class="num">上下文</th><th>工具调用</th><th>视觉</th><th>推理</th><th>状态</th><th></th>' +
        '</tr></thead><tbody>' +
        D.providers.map((p) =>
          '<tr><td><div class="row row-g2">' + U.led(p.state === 'ok' ? 'ok' : p.state === 'open' ? 'bad' : '') +
          '<b style="font-weight:600">' + U.esc(p.name) + '</b>' + U.tag(p.kind, 'solid') + '</div></td>' +
          '<td class="t-mono" style="font-size:12px">' + U.esc(p.model) + '</td>' +
          '<td class="num">' + U.esc(p.ctx) + '</td>' +
          '<td>' + capTag(p.tools) + '</td><td>' + capTag(p.vision) + '</td><td>' + capTag(p.reason) + '</td>' +
          '<td>' + stateTag(p.state) + '</td>' +
          '<td><div class="row-act">' +
          U.iconBtn('play', '连通性测试') + U.iconBtn('edit', '编辑') + U.iconBtn('trash', '删除', 'is-danger') +
          '</div></td></tr>'
        ).join('') +
        '</tbody></table>',
        '<span class="faint" style="font-size:11.5px">点击行内状态徽标可重置熔断并重建连接</span>' +
        '<span class="grow"></span>' + U.btn('从 model.json 重载', 'outline', 'refresh'),
        U.tag('密钥已加密', 'ok')) +
      card('支持的协议',
        '<div class="row row-g3">' +
        [['openai', 'OpenAI 兼容', 'DeepSeek / 通义 / Kimi / OpenAI 等绝大多数厂商'],
         ['anthropic', 'Anthropic', 'Claude 系列'],
         ['ollama', 'Ollama', '本地模型（127.0.0.1:11434）']].map((k) =>
          '<div style="flex:1;padding:12px 14px;border:1px solid var(--line-2);border-radius:12px">' +
          '<div class="row row-g2"><b style="font-size:13px">' + k[1] + '</b>' + U.tag(k[0], 'solid') + '</div>' +
          '<div class="t-sub mt2" style="font-size:11.5px">' + k[2] + '</div></div>'
        ).join('') + '</div>' +
        '<div class="t-sub mt3" style="font-size:11.5px">新增模型时选协议 + 填 Base URL 与模型名即可，不再按厂商维护预设。</div>')
    );
  }

  function tabSearch() {
    return (
      hero('search', '网络搜索', 'DuckDuckGo HTML 解析，无需 API Key；结果进入上下文前会去除广告节点') +
      card('搜索开关', sw(true, '启用网络搜索', '关闭后 web_search 与 webfetch 工具将从工具列表移除'),
        '<span class="grow"></span>' + U.btn('保存', 'primary'))
    );
  }

  /* ============================================================
     能力
     ============================================================ */
  function tabTools() {
    const total = D.toolGroups.reduce((s, g) => s + g.tools.length, 0);
    const on = D.toolGroups.reduce((s, g) => s + g.tools.filter((t) => t.on).length, 0);
    const riskTag = (r) => ({
      ok: U.tag('只读', 'solid'), warn: U.tag('写本地', 'warn'),
      bad: U.tag('执行', 'bad'), info: U.tag('网络', 'accent')
    }[r] || '');
    return (
      hero('wrench', '工具', '共 ' + total + ' 个 · 启用 ' + on + ' 个。关闭后模型看不到该工具，调用会被拒绝',
        '<div class="search" style="width:240px">' + ic('search', 14) + '<input class="input" placeholder="搜索工具"></div>') +
      D.toolGroups.map((g) =>
        card(g.name,
          g.tools.map((t) =>
            '<div class="tool-row" style="padding-left:0;padding-right:0">' +
            '<span class="tl-glyph">' + ic('wrench', 14) + '</span>' +
            '<div class="grow"><div class="row row-g2"><b style="font-weight:600;font-size:12.5px">' + U.esc(t.name) + '</b>' +
            riskTag(t.risk) + '</div>' +
            '<div class="faint" style="font-size:11.5px;margin-top:2px">' + U.esc(t.desc) + '</div></div>' +
            U.switchEl(t.on, 'js-sw') + '</div>'
          ).join(''),
          null, U.count(on + ' / ' + total)
        )
      ).join('')
    );
  }

  function tabSkills() {
    return (
      hero('zap', '技能', 'SKILL.md 声明能力边界与允许工具；脚本在沙箱内执行并按需审批',
        '<div class="row row-g2">' + U.btn('导入 zip', 'outline', 'upload') + U.btn('新建技能', 'primary', 'plus') + '</div>') +
      card('技能 ' + D.skills.length,
        '<table class="tbl"><thead><tr><th>名称</th><th>描述</th><th class="num">脚本</th><th>来源</th><th>启用</th><th></th></tr></thead><tbody>' +
        D.skills.map((s) =>
          '<tr><td><div class="row row-g2">' + U.led(s.on ? 'ok' : '') + '<b style="font-weight:600">' + U.esc(s.name) + '</b></div></td>' +
          '<td class="dim">' + U.esc(s.desc) + '</td>' +
          '<td class="num">' + s.scripts + '</td>' +
          '<td>' + (s.builtin ? U.tag('内置', 'solid') : U.tag('导入', 'accent')) + '</td>' +
          '<td>' + U.switchEl(s.on, 'js-sw') + '</td>' +
          '<td><div class="row-act">' + U.iconBtn('eye', '查看') + U.iconBtn('edit', '编辑') +
          U.iconBtn('trash', '删除', 'is-danger') + '</div></td></tr>'
        ).join('') +
        '</tbody></table>',
        '<span class="faint" style="font-size:11.5px">内置技能只读；同名技能工作区版本优先于用户级</span>')
    );
  }

  function tabMcp() {
    return (
      hero('server', 'MCP 服务', 'mcp.json 是配置真相源；env 值以掩码形式展示，明文仅本地加密存储',
        '<div class="row row-g2">' + U.btn('打开配置目录', 'outline', 'folder-open') +
        U.btn('重载', 'outline', 'refresh') + U.btn('新增服务', 'primary', 'plus') + '</div>') +
      card('服务 ' + D.mcpServers.length + ' · 活跃 2',
        '<table class="tbl"><thead><tr><th>名称</th><th>传输</th><th>命令 / 地址</th><th class="num">工具</th><th>启用</th><th></th></tr></thead><tbody>' +
        D.mcpServers.map((m) =>
          '<tr><td><div class="row row-g2">' + U.led(m.on ? 'ok' : '') + '<b style="font-weight:600">' + U.esc(m.name) + '</b></div></td>' +
          '<td>' + U.tag(m.transport, 'solid') + '</td>' +
          '<td class="t-mono" style="font-size:11.5px;color:var(--ink-2)">' + U.esc(m.cmd) + '</td>' +
          '<td class="num">' + m.tools + '</td>' +
          '<td>' + U.switchEl(m.on, 'js-sw') + '</td>' +
          '<td><div class="row-act">' + U.iconBtn('refresh', '重连') + U.iconBtn('trash', '删除', 'is-danger') + '</div></td></tr>'
        ).join('') + '</tbody></table>') +
      card('mcp.json 原文', '<textarea class="textarea mono" rows="9">' + U.esc(D.mcpRaw) + '</textarea>',
        '<span class="faint" style="font-size:11.5px">保存即写入并热重载</span><span class="grow"></span>' + U.btn('保存', 'primary'))
    );
  }

  function tabSubagents() {
    return (
      hero('bot', '子智能体', '委派执行时四重隔离：上下文 / 预算 / 工具 / 正文都不回到主会话',
        U.btn('新建子智能体', 'primary', 'plus')) +
      card('已定义 ' + D.agents.length,
        '<table class="tbl"><thead><tr><th>名称</th><th>描述</th><th>工具策略</th><th>模型</th><th class="num">轮次上限</th><th>启用</th><th></th></tr></thead><tbody>' +
        D.agents.map((a) =>
          '<tr><td><b style="font-weight:600">' + U.esc(a.name) + '</b></td>' +
          '<td class="dim">' + U.esc(a.desc) + '</td>' +
          '<td class="t-mono" style="font-size:11px">' + U.esc(a.tools) + '</td>' +
          '<td class="t-mono" style="font-size:11.5px">' + U.esc(a.model) + '</td>' +
          '<td class="num">' + a.turns + '</td>' +
          '<td>' + U.switchEl(a.on, 'js-sw') + '</td>' +
          '<td><div class="row-act">' + U.iconBtn('edit', '编辑') + U.iconBtn('trash', '删除', 'is-danger') + '</div></td></tr>'
        ).join('') + '</tbody></table>')
    );
  }

  function tabCommands() {
    return (
      hero('slash', '自定义命令', '三个来源合并：设置页记录 / 用户 commands 目录 / 工作区 .workbaby/commands',
        U.btn('新建命令', 'primary', 'plus')) +
      card('命令 ' + D.commands.length,
        '<table class="tbl"><thead><tr><th>命令</th><th>描述</th><th>Prompt 模板</th><th></th></tr></thead><tbody>' +
        D.commands.map((c) =>
          '<tr><td class="t-mono" style="color:var(--accent)">/' + U.esc(c.name) + '</td>' +
          '<td class="dim">' + U.esc(c.desc) + '</td>' +
          '<td class="t-mono truncate" style="font-size:11px;max-width:320px;color:var(--ink-3)">' + U.esc(c.prompt) + '</td>' +
          '<td><div class="row-act">' + U.iconBtn('edit', '编辑') + U.iconBtn('trash', '删除', 'is-danger') + '</div></td></tr>'
        ).join('') + '</tbody></table>',
        '<span class="faint" style="font-size:11.5px">同名时优先级：工作区文件 &gt; 用户级文件 &gt; 设置页记录</span>')
    );
  }

  function tabHooks() {
    const evTag = (e) => ({
      PreToolUse: U.tag(e, 'bad'), PostToolUse: U.tag(e, 'accent'),
      PermissionRequest: U.tag(e, 'warn'), UserPromptSubmit: U.tag(e, 'warn'),
      Stop: U.tag(e, 'warn')
    }[e] || U.tag(e, 'solid'));
    return (
      hero('webhook', '用户钩子', '外部脚本按事件裁决工具调用：返回 allow / deny / ask 与注入上下文',
        U.btn('新建钩子', 'primary', 'plus')) +
      card('钩子 ' + D.hooks.length,
        '<table class="tbl"><thead><tr><th>名称</th><th>事件</th><th>匹配器</th><th>命令</th><th>启用</th><th></th></tr></thead><tbody>' +
        D.hooks.map((h) =>
          '<tr><td><b style="font-weight:600">' + U.esc(h.name) + '</b></td>' +
          '<td>' + evTag(h.event) + '</td>' +
          '<td class="t-mono" style="font-size:11px">' + U.esc(h.matcher) + '</td>' +
          '<td class="t-mono truncate" style="font-size:11px;max-width:300px;color:var(--ink-2)">' + U.esc(h.cmd) + '</td>' +
          '<td>' + U.switchEl(h.on, 'js-sw') + '</td>' +
          '<td><div class="row-act">' + U.iconBtn('play', '试跑') + U.iconBtn('edit', '编辑') +
          U.iconBtn('trash', '删除', 'is-danger') + '</div></td></tr>'
        ).join('') + '</tbody></table>',
        '<span class="faint" style="font-size:11.5px">「试跑」用样例载荷执行脚本，返回决策、理由与耗时</span>')
    );
  }

  /* ============================================================
     数据
     ============================================================ */
  function tabMemory() {
    const M = D.memory;
    const total = M.sections.reduce((s, x) => s + x.items.length, 0);
    const colors = ['var(--accent)', 'var(--ok)', 'var(--ink-3)', 'var(--warn)'];
    return (
      hero('memory', '记忆中心', '单一 MEMORY.md：人可直接编辑，FTS5 派生索引丢失可重建',
        U.btn('整篇编辑', 'outline', 'edit')) +
      card('概览',
        '<div class="row row-g3"><div class="grow">' +
        '<div class="row row-g2"><span class="t-num" style="font-size:22px">' + total + '</span>' +
        U.tag('条', 'solid') + '<span class="faint" style="font-size:12px">跨 4 个分节</span></div>' +
        '<div class="t-mono mt2" style="font-size:11.5px;color:var(--ink-3)">' + U.esc(M.path) + '</div>' +
        '</div>' + U.btn('打开目录', 'outline', 'folder-open') + '</div>') +
      card('快速添加',
        '<div class="row row-g2">' +
        '<select class="select" style="width:150px">' + M.sections.map((s) => '<option>' + U.esc(s.name) + '</option>').join('') + '</select>' +
        '<input class="input grow" placeholder="写下一条稳定事实（Enter 提交）">' +
        U.btn('添加', 'primary', 'plus') + '</div>') +
      card('分节分布',
        '<div class="dist">' +
        M.sections.map((s, i) =>
          '<div class="dist-row"><span class="dl">' + U.esc(s.name) + '</span>' +
          '<span class="bar"><i style="width:' + Math.round((s.items.length / total) * 100) + '%;background:' + colors[i] + '"></i></span>' +
          '<span class="dn">' + s.items.length + '</span></div>'
        ).join('') + '</div>') +
      card('条目',
        M.sections.map((s, i) =>
          '<div style="margin-bottom:14px">' + U.plate(s.name) +
          '<div style="margin-top:6px">' +
          s.items.map((it) =>
            '<div class="row row-g3" style="padding:7px 0;border-bottom:1px solid var(--line)">' +
            '<span style="width:3px;height:14px;border-radius:1px;background:' + colors[i] + '"></span>' +
            '<span class="grow" style="font-size:12.5px">' + U.esc(it) + '</span>' +
            U.iconBtn('trash', '删除', 'is-danger') + '</div>'
          ).join('') + '</div></div>'
        ).join(''),
        '<div class="search grow">' + ic('search', 14) + '<input class="input" placeholder="搜索记忆（FTS5 trigram）"></div>' +
        U.btn('检索', 'solid'))
    );
  }

  function tabKdocs() {
    const kTag = (k) => ({
      file: U.tag('文件', 'solid'), url: U.tag('URL', 'accent'), text: U.tag('文本', 'solid')
    }[k] || '');
    const typeTag = (t) => ({
      pdf: U.tag('pdf', 'bad'), docx: U.tag('docx', 'accent'), xlsx: U.tag('xlsx', 'warn'),
      md: U.tag('md', 'solid'), html: U.tag('html', 'solid')
    }[t] || U.tag(t, 'solid'));
    const stTag = (s) => ({
      indexed: '<span class="row row-g2">' + U.led('ok') + U.tag('已索引', 'ok') + '</span>',
      indexing: '<span class="row row-g2">' + U.led('warn is-live') + U.tag('索引中', 'warn') + '</span>',
      failed: '<span class="row row-g2">' + U.led('bad') + U.tag('失败', 'bad') + '</span>',
      pending: '<span class="row row-g2">' + U.led('') + U.tag('待索引', 'solid') + '</span>'
    }[s]);
    return (
      hero('book', '知识库', D.kdocs.length + ' 篇文档 · ' + (D.kdocGroups.length - 1) + ' 个分组；导入后可重建派生索引',
        '<div class="row row-g2">' + U.btn('导入文件', 'outline', 'upload') + U.btn('新建文档', 'primary', 'plus') + '</div>') +
      card('文档',
        '<div class="row row-g2" style="margin-bottom:12px">' +
        '<div class="search grow">' + ic('search', 14) + '<input class="input" placeholder="全文检索（标题 + 正文）"></div>' +
        '<select class="select" style="width:130px">' + D.kdocGroups.map((g) => '<option>' + U.esc(g) + '</option>').join('') + '</select>' +
        U.btn('检索', 'solid') + '</div>' +
        '<table class="tbl"><thead><tr><th>标题</th><th>来源</th><th>格式</th><th class="num">大小</th><th class="num">片段</th><th>分组</th><th>索引</th><th>更新</th><th></th></tr></thead><tbody>' +
        D.kdocs.map((k) =>
          '<tr><td><b style="font-weight:600">' + U.esc(k.name) + '</b></td>' +
          '<td>' + kTag(k.src) + '</td><td>' + typeTag(k.type) + '</td>' +
          '<td class="num">' + U.esc(k.size) + '</td>' +
          '<td class="num">' + k.chunks + '</td>' +
          '<td class="dim">' + U.esc(k.group) + '</td>' +
          '<td>' + stTag(k.state) + '</td>' +
          '<td class="dim" style="font-size:11.5px">' + U.esc(k.time) + '</td>' +
          '<td><div class="row-act">' + U.iconBtn('eye', '预览') + U.iconBtn('refresh', '重建索引') +
          U.iconBtn('trash', '删除', 'is-danger') + '</div></td></tr>'
        ).join('') + '</tbody></table>',
        '<span class="faint" style="font-size:11.5px">检索按片段返回并标注来源文档与相似度</span>')
    );
  }

  function tabWiki() {
    const tree = [
      { d: 0, n: 'internal', dir: true, open: true },
      { d: 1, n: 'core', dir: true, open: true },
      { d: 2, n: 'runner.go', sz: '812', on: true },
      { d: 2, n: 'compressor.go', sz: '436' },
      { d: 1, n: 'rag', dir: true, open: true },
      { d: 2, n: 'retriever.go', sz: '186' },
      { d: 2, n: 'scorer.go', sz: '48' },
      { d: 0, n: 'doc', dir: true, open: false },
      { d: 0, n: 'main.go', sz: '64' }
    ];
    return (
      hero('wiki', '仓库导读', '确定性扫描（跳过依赖与构建目录），不调用模型；语言统计与入口文件一屏建立全局认知',
        U.btn('重新扫描', 'outline', 'refresh')) +
      card('画像',
        '<div class="row row-g4 wrap">' +
        '<div><div class="t-plate">仓库</div><div class="t-mono mt1">WorkBaby</div></div>' +
        '<div><div class="t-plate">文件 / 行数</div><div class="t-num mt1" style="font-size:17px">312 · 48.6k</div></div>' +
        '<div class="grow"><div class="t-plate">语言构成</div><div class="row row-g2 wrap mt1">' +
        [['Go', '58%'], ['Vue', '21%'], ['TS', '14%'], ['CSS', '5%'], ['PowerShell', '2%']].map((l) =>
          '<span class="mini-chip">' + l[0] + ' · ' + l[1] + '</span>'
        ).join('') + '</div></div></div>' +
        '<div class="hr"></div>' +
        U.plate('入口文件') +
        '<div class="row row-g2 wrap mt2">' +
        ['main.go', 'app.go', 'internal/server/router.go', 'internal/agent/loop.go', 'frontend/src/main.ts'].map((f) =>
          '<button class="chip">' + ic('file-text', 12) + f + '</button>'
        ).join('') + '</div>') +
      '<div class="wiki-split">' +
      '<div class="wiki-tree">' +
      tree.map((t) =>
        '<div class="tree-row' + (t.dir ? ' is-dir' : '') + (t.on ? ' is-on' : '') + '" style="padding-left:' + (6 + t.d * 14) + 'px">' +
        ic(t.dir ? (t.open ? 'folder-open' : 'folder') : 'file-text', 13) +
        '<span class="truncate">' + U.esc(t.n) + '</span>' +
        (t.sz ? '<span class="sz">' + t.sz + ' 行</span>' : '') + '</div>'
      ).join('') + '</div>' +
      '<div class="wiki-page">' +
      '<div class="row row-g2" style="padding:10px 14px;border-bottom:1px solid var(--line)">' +
      ic('file-text', 14) + '<b style="font-size:12.5px">runner.go</b>' +
      U.tag('812 行', 'solid') + '<span class="spacer"></span>' +
      U.btn('在对话中引用', 'outline', 'at', 'sm') + '</div>' +
      '<pre class="code" style="border:0;border-radius:0;max-height:340px;overflow:auto;background:transparent">' +
      '<span class="ln"> 1</span> package core\n' +
      '<span class="ln"> 2</span> \n' +
      '<span class="ln"> 3</span> // Runner 承载一次 run 的 ReAct 循环：\n' +
      '<span class="ln"> 4</span> // 模型调用与工具执行交替推进，护栏链在两侧生效。\n' +
      '<span class="ln"> 5</span> type Runner struct {\n' +
      '<span class="ln"> 6</span>     llm      llm.Client\n' +
      '<span class="ln"> 7</span>     registry *tool.Registry\n' +
      '<span class="ln"> 8</span>     sink     Sink\n' +
      '<span class="ln"> 9</span>     hooks    []Hook\n' +
      '<span class="ln">10</span>     maxTurn  int\n' +
      '<span class="ln">11</span> }\n' +
      '<span class="ln">12</span> \n' +
      '<span class="ln">13</span> func (r *Runner) Run(ctx context.Context, msgs []*llm.Message) (*Outcome, error) {</span>' +
      '<span class="ln">\n…（截断：单页上限 512KB，此处为预览）</span>' +
      '</pre></div></div>'
    );
  }

  function tabFiles() {
    const thumb = (t) => ({ image: 'image', doc: 'file-text', code: 'code', sheet: 'chart', archive: 'archive' }[t] || 'file');
    return (
      hero('file', '文件', D.files.length + ' 个 · 共 2.7 MB。粘贴的图片与上传的本地文件统一登记，预览走本地受管服务',
        U.btn('上传文件', 'primary', 'upload')) +
      card(null,
        '<div class="row row-g3" style="margin-bottom:14px">' +
        '<div class="search grow">' + ic('search', 14) + '<input class="input" placeholder="搜索文件名"></div>' +
        '<span class="faint" style="font-size:11.5px">6 个 · 总大小 2.7 MB</span></div>' +
        '<div style="display:grid;grid-template-columns:repeat(4,1fr);gap:12px">' +
        D.files.map((f) =>
          '<div class="fcard"><div class="fc-thumb">' + ic(thumb(f.type), 22) + '</div>' +
          '<div class="fc-meta"><div class="fc-name truncate">' + U.esc(f.name) + '</div>' +
          '<div class="faint" style="font-size:11px;margin-top:2px">' + U.esc(f.size) + ' · ' + U.esc(f.time) + '</div></div></div>'
        ).join('') + '</div>')
    );
  }

  /* --- 图表辅助 --- */
  function linePath(vals, w, h, max) {
    return vals.map((v, i) =>
      (i ? 'L' : 'M') + (i * (w / (vals.length - 1))).toFixed(1) + ' ' + (h - (v / max) * h).toFixed(1)
    ).join(' ');
  }

  function tabDashboard() {
    const T = D.tokenTrend;
    const W = 680, H = 150, MAX = 600;
    const xs = T.map((d) => d.label);
    const inVals = T.map((d) => d.in), outVals = T.map((d) => d.out), cacheVals = T.map((d) => d.cache);
    const cacheRate = 74;
    return (
      hero('chart', '仪表盘', '所有数字来自本机 token_usages 与会话表，不上报任何云端',
        '<select class="select" style="width:120px"><option>最近 7 天</option><option>今日</option><option>本月</option></select>' +
        U.btn('刷新', 'outline', 'refresh')) +
      '<div class="tiles">' +
      [['今日消息', D.stats.msg, 'chart'], ['今日会话', D.stats.sess, 'chat'],
       ['累计 Token', D.stats.token, 'activity'], ['可用工具', D.stats.tools, 'wrench']].map((s) =>
        '<div class="tile"><div class="tv">' + s[1] + '</div>' +
        '<div class="tl">' + ic(s[2], 12) + '<span class="t-plate" style="letter-spacing:.06em">' + s[0] + '</span></div></div>'
      ).join('') + '</div>' +
      card('Token 三线',
        '<div class="row row-g4" style="margin-bottom:10px">' +
        '<span class="t-num" style="font-size:22px">4.2M</span>' +
        '<div class="legend"><span class="lg"><i class="sw in"></i>输入</span>' +
        '<span class="lg"><i class="sw out"></i>输出</span>' +
        '<span class="lg"><i class="sw cache"></i>缓存命中</span></div></div>' +
        '<svg class="chart" viewBox="0 0 ' + W + ' ' + H + '" preserveAspectRatio="none" style="height:180px">' +
        [0, 1, 2, 3].map((i) =>
          '<line class="grid-line" x1="0" y1="' + (i * H / 3) + '" x2="' + W + '" y2="' + (i * H / 3) + '"></line>'
        ).join('') +
        '<path class="area" d="' + linePath(inVals, W, H, MAX) + ' L' + W + ' ' + H + ' L0 ' + H + ' Z"></path>' +
        '<path class="ln-in" d="' + linePath(inVals, W, H, MAX) + '"></path>' +
        '<path class="ln-out" d="' + linePath(outVals, W, H, MAX) + '"></path>' +
        '<path class="ln-cache" d="' + linePath(cacheVals, W, H, MAX) + '"></path>' +
        '</svg>' +
        '<div class="row" style="justify-content:space-between;margin-top:4px">' +
        xs.map((x) => '<span class="t-mono faint" style="font-size:10px">' + x + '</span>').join('') + '</div>',
        null, U.tag('今日', 'accent')) +
      '<div style="display:grid;grid-template-columns:repeat(3,1fr);gap:16px">' +
      card('缓存命中率',
        '<div class="row row-g4"><div class="donut" style="--p:' + cacheRate + '"><span class="dv">' + cacheRate + '%</span></div>' +
        '<div class="grow" style="font-size:12px;color:var(--ink-2)">' +
        '<div class="row between"><span>命中</span><span class="t-mono">3.1M</span></div>' +
        '<div class="row between mt1"><span>未命中</span><span class="t-mono">1.1M</span></div>' +
        '<div class="row between mt1"><span>估算省下</span><span class="t-mono" style="color:var(--ok)">$18.40</span></div>' +
        '</div></div>') +
      card('任务成功率',
        '<div class="t-num" style="font-size:26px">94%</div>' +
        '<div class="faint mt1" style="font-size:11.5px">近 50 次 run · 失败 3 次（工具超时 2 / 网络 1）</div>' +
        '<div class="mt3">' + U.progress(94, true) + '</div>') +
      card('最近活动',
        [['消息完成 · 重构 RAG 检索打分', '21:04'], ['工具执行 · go test ./internal/rag/...', '21:03'],
         ['后台任务 · 扫描依赖过期包', '20:51'], ['记忆写入 · 用户偏好', '18:22']].map((a) =>
          '<div class="row row-g2" style="padding:5px 0;font-size:12px">' + U.led('accent') +
          '<span class="grow truncate">' + U.esc(a[0]) + '</span>' +
          '<span class="t-mono faint" style="font-size:10.5px">' + a[1] + '</span></div>'
        ).join('')) +
      '</div>'
    );
  }

  function tabRuns() {
    const st = (s) => ({
      running: '<span class="row row-g2">' + U.led('warn is-live') + U.tag('运行中', 'warn') + '</span>',
      done: '<span class="row row-g2">' + U.led('ok') + U.tag('完成', 'ok') + '</span>',
      error: '<span class="row row-g2">' + U.led('bad') + U.tag('失败', 'bad') + '</span>'
    }[s]);
    return (
      hero('play', '运行历史', '每次 run 的耗时构成、轮次与事件流；回放用于排查「第 3 轮为什么停」',
        '<select class="select" style="width:120px"><option>全部状态</option><option>失败</option></select>') +
      card(null,
        '<table class="tbl"><thead><tr><th>开始</th><th>模型</th><th>状态</th><th>触发</th><th class="num">轮次</th>' +
        '<th class="num">Tokens</th><th>耗时构成</th><th></th></tr></thead><tbody>' +
        D.runs.map((r) =>
          '<tr><td class="t-mono" style="font-size:11.5px">' + U.esc(r.time) + '</td>' +
          '<td class="t-mono" style="font-size:11.5px">' + U.esc(r.model) + '</td>' +
          '<td>' + st(r.state) + '</td>' +
          '<td class="dim" style="font-size:11.5px">' + U.esc(r.reason) + '</td>' +
          '<td class="num">' + r.turns + '</td>' +
          '<td class="num" style="font-size:11.5px">↑' + U.esc(r.in) + ' ↓' + U.esc(r.out) + '</td>' +
          '<td><div class="run-cell"><div class="rc-bar">' +
          '<i class="llm" style="width:' + r.llm + '%"></i><i class="tool" style="width:' + r.tool + '%"></i></div>' +
          '<span class="t-mono" style="font-size:10px;color:var(--ink-3)">等模型 ' + r.llm + '% · 跑工具 ' + r.tool + '%</span>' +
          '</div></td>' +
          '<td><div class="row-act">' + U.btn('回放', 'outline', 'play', 'sm') + '</div></td></tr>'
        ).join('') + '</tbody></table>')
    );
  }

  /* ============================================================
     系统
     ============================================================ */
  function tabAppearance() {
    const cur = document.documentElement.getAttribute('data-theme') || 'light';
    const themes = [
      { id: 'dark', name: '紫夜工作台', bg: '#0b0a0e', card: '#14121a', acc: '#7c5cf8', fg: '#ece9f3', sub: '#8b85a3' },
      { id: 'light', name: '晴空工作台', bg: '#f2f6fc', card: '#ffffff', acc: '#2f80ed', fg: '#0f1e33', sub: '#6b7d95' }
    ];
    return (
      hero('sun', '外观', '两套主题共用一组语义令牌；组件不写死色值，暗色不漏白') +
      card('主题',
        '<div class="row row-g3">' +
        themes.map((t) => {
          const on = cur === t.id;
          return '<button class="theme-card' + (on ? ' is-on' : '') + '" data-act="theme-card" ' +
            'data-theme-id="' + t.id + '" style="background:' + t.bg + ';border-color:' + (on ? t.acc : 'transparent') + '">' +
            '<span class="tc-thumb" style="background:' + t.card + ';border-color:' + t.acc + '59">' +
            '<span style="background:' + t.acc + '"></span></span>' +
            '<span class="tc-txt"><b style="color:' + t.fg + '">' + t.name + '</b>' +
            '<i style="color:' + t.sub + '">' + (on ? '当前使用' : '点击切换') + '</i></span>' +
            (on ? '<span class="tc-check" style="color:' + t.acc + '">' + ic('check', 15) + '</span>' : '') +
            '</button>';
        }).join('') + '</div>') +
      card('背景（可选）',
        '<div class="row row-g4">' +
        '<div style="width:200px;height:112px;border-radius:6px;border:1px dashed var(--line-2);display:grid;place-items:center;color:var(--ink-3)">' +
        ic('image', 20) + '</div>' +
        '<div class="grow"><div style="font-size:12.5px">未设置自定义背景</div>' +
        '<div class="faint mt1" style="font-size:11.5px">设置后自动提取主色并应用到全局强调色；可随时移除恢复默认</div>' +
        '<div class="row row-g2 mt3">' + U.btn('上传背景', 'outline', 'upload') + U.btn('移除', 'ghost') + '</div></div></div>') +
      card('排版',
        '<div class="form-grid">' +
        U.field('界面语言', '<select class="select"><option>简体中文</option><option>English</option></select>') +
        U.field('代码字体', '<select class="select"><option>Cascadia Code（默认）</option><option>JetBrains Mono</option><option>Consolas</option></select>') +
        '<div class="span2">' + U.field('界面缩放',
          '<input type="range" min="85" max="125" value="100" style="width:100%;accent-color:var(--accent)">' +
          '<div class="row between faint" style="font-size:11px"><span>85%</span><span>100%</span><span>125%</span></div>',
          '仅影响界面密度，不改变代码字号') + '</div>' +
        '</div>',
        '<span class="faint" style="font-size:11.5px">正文 13px / 元信息 11px / 读数 28px，共 6 级刻度</span>' +
        '<span class="grow"></span>' + U.btn('保存', 'primary'))
    );
  }

  function tabAdvanced() {
    const list = ['go', 'git', 'pwsh', 'node', 'npm', 'python', 'uv', 'wails'];
    return (
      hero('shield', '安全与高级', '命令白名单与免审授权是本地安全的最后一道闸；两者都可随时撤销') +
      card('系统行为',
        sw(true, '关闭窗口时最小化到托盘', '托盘菜单可彻底退出；右上角关闭 = 隐藏') +
        '<div class="hr"></div>' +
        sw(true, '启用长期记忆', '关闭后 memory 工具从工具列表移除，已有记忆保留') +
        '<div class="hr"></div>' +
        sw(false, '开屏自动检查更新', '仅访问 GitHub Release 元信息')) +
      card('exec 命令白名单',
        '<div class="chip-list">' +
        list.map((c) =>
          '<span class="mini-chip">' + U.led('ok') + c + '<span class="x">' + ic('x', 11) + '</span></span>'
        ).join('') +
        '</div>' +
        '<div class="row row-g2 mt4"><input class="input mono grow" placeholder="输入可执行文件名（如 cargo）">' +
        U.btn('添加', 'primary', 'plus') + '</div>',
        '<span class="faint" style="font-size:11.5px">未在白名单内的命令即使模型请求也会被拒绝；危险模式（如 rm -rf）额外拦截</span>' +
        '<span class="grow"></span>' + U.tag('已修改', 'warn') + U.btn('保存', 'primary')) +
      card('免审授权（本会话允许的持久化）',
        [['go test ./internal/...', '今天 20:41'], ['git status', '昨天 22:03']].map((a) =>
          '<div class="row row-g3" style="padding:7px 0;border-bottom:1px solid var(--line)">' +
          '<span class="t-mono grow" style="font-size:12px">' + U.esc(a[0]) + '</span>' +
          '<span class="faint" style="font-size:11.5px">' + a[1] + '</span>' +
          U.btn('撤销', 'danger-ghost', null, 'sm') + '</div>'
        ).join(''))
    );
  }

  function tabPet() {
    return (
      hero('paw', '桌宠', '独立透明窗口：跟随 Agent 状态切换动作；点击穿透按命中区域动态设置',
        U.tag('3 个形象', 'accent')) +
      '<div style="display:grid;grid-template-columns:280px 1fr 260px;gap:16px">' +
      card('行为配置',
        sw(true, '启用桌宠') +
        '<div class="hr"></div>' +
        U.field('当前形象', '<select class="select"><option>气泡小工（内置）</option><option>纸箱猫</option><option>导线小人</option></select>') +
        '<div class="mt3">' + U.field('缩放', '<input type="range" min="50" max="300" value="100" style="width:100%;accent-color:var(--accent)">') + '</div>' +
        '<div class="mt3">' + sw(true, '说话气泡') + '</div>' +
        '<div class="mt2">' + sw(true, '点击穿透') + '</div>' +
        '<div class="mt4 row row-g2">' + U.btn('召唤', 'primary', 'play') + U.btn('收起', 'outline', 'minus') + '</div>') +
      card('预览',
        '<div class="sprite-stage">' +
        '<div style="display:flex;flex-direction:column;align-items:center;gap:10px">' +
        '<div class="pet-bubble">这次的目标已达成 🎉</div>' +
        '<div style="width:96px;height:96px;border-radius:50%;background:radial-gradient(circle at 35% 30%, var(--accent-hi), var(--accent));display:grid;place-items:center;color:var(--accent-ink);box-shadow:0 12px 24px -10px rgba(0,0,0,.5)">' +
        ic('bot', 44) + '</div>' +
        '<div class="pet-chip">' + U.led('ok') + '发呆中</div></div></div>' +
        '<div class="row between mt3">' + U.plate('动作预览') +
        '<div class="mood-grid">' + ['😺', '🤔', '😴', '🎉', '💻'].map((m, i) =>
          '<button class="mood' + (i === 0 ? ' is-on' : '') + '">' + m + '</button>'
        ).join('') + '</div></div>') +
      card('形象资产',
        '<button class="btn btn-outline btn-sm" style="width:100%">' + ic('upload', 13) + '上传形象（≤5MB）</button>' +
        '<div class="mt3">' +
        [['气泡小工', '内置 · svg'], ['纸箱猫', 'PNG · 128KB'], ['导线小人', 'GIF · 512KB']].map((s) =>
          '<div class="row row-g2" style="padding:6px 0;border-bottom:1px solid var(--line)">' +
          '<span class="grow" style="font-size:12px">' + s[0] + '</span>' +
          '<span class="faint" style="font-size:10.5px">' + s[1] + '</span>' +
          U.iconBtn('trash', '删除', 'is-danger') + '</div>'
        ).join('') + '</div>') +
      '</div>'
    );
  }

  function tabDocs() {
    const list = ['00 项目概览', '01 总体架构', '02 数据模型', '04 Agent 内核', '05 工具系统',
      '06 技能与 MCP', '07 聊天与会话', '16 API 契约总表', '17 内置运行时'];
    return (
      hero('scroll', '文档', '与仓库 doc/ 同步的内置文档；改动契约时先改文档再改代码') +
      '<div style="display:grid;grid-template-columns:230px 1fr;gap:16px">' +
      card(null,
        list.map((d, i) =>
          '<button class="menu-item' + (i === 3 ? ' is-on' : '') + '">' + ic('file-text', 13) +
          '<span class="grow">' + U.esc(d) + '</span></button>'
        ).join('')) +
      card(null,
        '<div class="t-title">04 · Agent 内核</div>' +
        '<div class="prose mt4" style="color:var(--ink-2);font-size:12.5px">' +
        '<p>一次会话 = 若干 run；一次 run = 一个 ReAct 循环。</p>' +
        '<p><b style="color:var(--ink)">循环骨架</b>：装配上下文 → 调模型 → 若有工具调用则执行 → 结果回填 → 再调模型，直到模型给出终态或触发护栏。</p>' +
        '<p><b style="color:var(--ink)">五件护栏</b>：最大轮次、token 预算、工具失败上限、注入检测、审批门。任一护栏触发都以显式 stop_reason 结束，可续跑。</p>' +
        '<p><b style="color:var(--ink)">检查点</b>：每轮结束写入检查点；中断或崩溃后从最近检查点恢复，不重复已完成的工具调用。</p>' +
        '</div>')
      + '</div>'
    );
  }

  function tabAbout() {
    return (
      hero('info', '关于', 'WorkBaby ' + D.version + ' · 本地单用户',
        '<div class="row row-g2">' + U.btn('查看更新', 'outline', 'refresh') + U.btn('导出诊断包', 'primary', 'download') + '</div>') +
      '<div class="tiles">' +
      [['版本', D.version, 'info'], ['工具', '23 / 26', 'wrench'], ['技能', '5', 'zap'], ['MCP', '2 活跃', 'server']].map((s) =>
        '<div class="tile"><div class="tv" style="font-size:20px">' + s[1] + '</div>' +
        '<div class="tl">' + ic(s[2], 12) + '<span class="t-plate">' + s[0] + '</span></div></div>'
      ).join('') + '</div>' +
      card('数据目录',
        '<div class="row row-g2"><span class="t-mono grow" style="font-size:12px;color:var(--ink-2)">' + U.esc(D.homeDir) + '</span>' +
        U.btn('打开', 'outline', 'folder-open') + U.btn('复制路径', 'ghost', 'copy') + '</div>' +
        '<div class="hr"></div>' +
        '<div class="kv" style="font-size:12px">' +
        '<dt>数据库</dt><dd>workbaby.db · 18.4 MB · WAL</dd>' +
        '<dt>记忆</dt><dd>memory/MEMORY.md</dd>' +
        '<dt>模型配置</dt><dd>model.json（含加密密钥）</dd>' +
        '<dt>日志</dt><dd>logs/workbaby.log · 轮转 10MB × 5</dd>' +
        '</div>') +
      card('内置运行时',
        '<div class="kv" style="font-size:12px">' +
        '<dt>Go</dt><dd>1.24.6（编译期嵌入）</dd>' +
        '<dt>Node</dt><dd>20.19.0 · 就绪</dd>' +
        '<dt>Python</dt><dd>3.12.7 · 就绪</dd>' +
        '<dt>SQLite</dt><dd>3.45 · pure-Go · FTS5 trigram</dd>' +
        '</div>',
        '<span class="row row-g2" style="font-size:11.5px;color:var(--ink-3)">' + U.led('ok') + '运行时全部就绪，命令找不到的情况不会再出现</span>')
    );
  }

  /* ---- 看板（对齐参考稿）：大标题 + 大数字 + 迷你图 + 淡蓝装饰 ---- */
  function kpiCard(k) {
    const max = Math.max.apply(null, k.bars);
    return (
      '<div class="kpi">' +
      '<div class="kpi-top"><span class="kpi-ic">' + ic(k.icon, 16) + '</span>' +
      '<span class="kpi-label">' + U.esc(k.label) + '</span>' +
      '<span class="kpi-delta">' + U.esc(k.delta) + '</span></div>' +
      '<div class="kpi-value">' + U.esc(k.value) + '<span class="kpi-unit">' + U.esc(k.unit) + '</span></div>' +
      '<div class="kpi-foot">' + U.plate(k.hint) +
      '<div class="mini-bars">' +
      k.bars.map((v) =>
        '<i class="' + (v === max ? 'is-peak' : '') + '" style="height:' + Math.round((v / max) * 100) + '%"></i>'
      ).join('') + '</div></div></div>'
    );
  }

  function tabBoard() {
    const T = D.tokenTrend;
    const W = 680, H = 150, MAX = 600;
    const inVals = T.map((d) => d.in);
    const outVals = T.map((d) => d.out);
    const cacheVals = T.map((d) => d.cache);
    const kpis = [
      { icon: 'chat', label: '今日消息', value: D.stats.msg, unit: '条', delta: '+12%', hint: '近 7 天', bars: [42, 58, 51, 74, 66, 92, 80] },
      { icon: 'inbox', label: '活跃会话', value: D.stats.sess, unit: '个', delta: '+4', hint: '近 7 天', bars: [30, 44, 38, 52, 61, 55, 68] },
      { icon: 'activity', label: '累计 Token', value: D.stats.token, unit: '', delta: '缓存 74%', hint: '近 7 天', bars: [50, 62, 58, 80, 72, 96, 86] },
      { icon: 'wrench', label: '可用工具', value: D.stats.tools, unit: '个', delta: '全部就绪', hint: '共 26 个', bars: [60, 61, 62, 62, 64, 65, 66] }
    ];
    return (
      '<div class="board-hero">' +
      '<div class="bh-deco"></div><div class="bh-deco-2"></div>' +
      '<div class="bh-txt">' +
      '<span class="bh-tag">' + ic('sparkle', 11) + '本周概览</span>' +
      '<h1>工作台数据</h1>' +
      '<p>所有数字来自本机 token_usages 与会话表，不上报任何云端</p>' +
      '</div>' +
      '<div class="bh-actions">' +
      U.chip('今日', false) + U.chip('本周', true) + U.chip('本月', false) +
      U.btn('刷新', 'primary', 'refresh') +
      '</div></div>' +

      '<div class="kpi-grid">' + kpis.map(kpiCard).join('') + '</div>' +

      '<div class="board-card" style="margin-bottom:16px">' +
      '<div class="bc-head"><span class="kpi-ic">' + ic('chart', 16) + '</span>' +
      '<span class="bc-t">Token 消耗</span>' +
      '<span class="spacer"></span>' +
      '<div class="legend"><span class="lg"><i class="sw in"></i>输入</span>' +
      '<span class="lg"><i class="sw out"></i>输出</span>' +
      '<span class="lg"><i class="sw cache"></i>缓存命中</span></div>' +
      U.btn('导出', 'outline', 'download', 'sm') + '</div>' +
      '<div class="row row-g3" style="align-items:baseline;margin-bottom:8px">' +
      '<span class="t-num" style="font-size:28px;font-weight:700;letter-spacing:-.02em">4.2M</span>' +
      '<span class="t-sub">总消耗</span></div>' +
      '<svg class="chart" viewBox="0 0 ' + W + ' ' + H + '" preserveAspectRatio="none" style="height:190px">' +
      [0, 1, 2, 3].map((i) =>
        '<line class="grid-line" x1="0" y1="' + (i * H / 3) + '" x2="' + W + '" y2="' + (i * H / 3) + '"></line>'
      ).join('') +
      '<path class="area" d="' + linePath(inVals, W, H, MAX) + ' L' + W + ' ' + H + ' L0 ' + H + ' Z"></path>' +
      '<path class="ln-in" d="' + linePath(inVals, W, H, MAX) + '"></path>' +
      '<path class="ln-out" d="' + linePath(outVals, W, H, MAX) + '"></path>' +
      '<path class="ln-cache" d="' + linePath(cacheVals, W, H, MAX) + '"></path>' +
      '</svg>' +
      '<div class="row" style="justify-content:space-between;margin-top:6px">' +
      T.map((d) => '<span class="t-mono faint" style="font-size:10px">' + d.label + '</span>').join('') +
      '</div></div>' +

      '<div class="board-row" style="grid-template-columns:repeat(3,1fr)">' +
      '<div class="board-card"><div class="bc-head"><span class="kpi-ic">' + ic('target', 16) + '</span>' +
      '<span class="bc-t">缓存命中率</span></div>' +
      '<div class="row row-g4"><div class="donut" style="--p:74"><span class="dv">74%</span></div>' +
      '<div class="grow" style="font-size:12px;color:var(--ink-2)">' +
      '<div class="row between"><span>命中</span><span class="t-mono">3.1M</span></div>' +
      '<div class="row between mt1"><span>未命中</span><span class="t-mono">1.1M</span></div>' +
      '<div class="row between mt1"><span>估算省下</span><span class="t-mono" style="color:var(--ok)">$18.40</span></div>' +
      '</div></div></div>' +
      '<div class="board-card"><div class="bc-head"><span class="kpi-ic">' + ic('check', 16) + '</span>' +
      '<span class="bc-t">任务成功率</span></div>' +
      '<div class="kpi-value" style="font-size:34px">94%</div>' +
      '<div class="t-sub mt2">近 50 次 run · 失败 3 次</div>' +
      '<div class="mt3">' + U.progress(94, true) + '</div>' +
      '<div class="row row-g3 mt3" style="font-size:11.5px;color:var(--ink-3)">' +
      '<span>工具超时 2</span><span>网络 1</span></div></div>' +
      '<div class="board-card"><div class="bc-head"><span class="kpi-ic">' + ic('activity', 16) + '</span>' +
      '<span class="bc-t">最近活动</span></div>' +
      [['消息完成 · 重构 RAG 检索打分', '21:04'], ['工具执行 · go test', '21:03'],
       ['后台任务 · 扫描依赖', '20:51'], ['记忆写入 · 用户偏好', '18:22']].map((a) =>
        '<div class="row row-g2" style="padding:6px 0;font-size:12px">' + U.led('accent') +
        '<span class="grow truncate">' + U.esc(a[0]) + '</span>' +
        '<span class="t-mono faint" style="font-size:10.5px">' + a[1] + '</span></div>'
      ).join('') + '</div>' +
      '</div>'
    );
  }

  const TABS = {
    models: tabModels, search: tabSearch, tools: tabTools, skills: tabSkills, mcp: tabMcp,
    subagents: tabSubagents, commands: tabCommands, hooks: tabHooks, memory: tabMemory,
    kdocs: tabKdocs, wiki: tabWiki, files: tabFiles, dashboard: tabBoard, runs: tabRuns,
    appearance: tabAppearance, advanced: tabAdvanced, pet: tabPet, docs: tabDocs, about: tabAbout
  };

  /* ---------- 装配 ---------- */
  function render(tabID) {
    const meta = META[tabID] || ['设置', ''];
    return (
      '<div class="set-layout">' +
      '<aside class="set-nav">' +
      '<button class="set-back" data-act="back-chat">' + ic('arrow-left', 13) + '<span>返回工作区</span></button>' +
      '<div class="set-scroll">' +
      NAV.map((g) =>
        '<div class="set-group t-plate">' + g.group + '</div>' +
        g.items.map((it) =>
          '<button class="set-item' + (it.id === tabID ? ' is-on' : '') + '" data-set-tab="' + it.id + '">' +
          ic(it.icon, 14) + '<span class="grow">' + it.name + '</span>' +
          (it.badge ? U.count(it.badge) : '') + '</button>'
        ).join('')
      ).join('') +
      '</div></aside>' +
      '<div class="set-body"><div class="set-page" id="wb-set-page">' +
      (TABS[tabID] || tabAbout)() +
      '</div></div></div>'
    );
  }

  function mount(root, ctx) {
    root.addEventListener('click', (e) => {
      const t = e.target.closest('[data-set-tab]');
      if (t) return ctx.go('set.' + t.getAttribute('data-set-tab'));
      if (e.target.closest('[data-act="back-chat"]')) return ctx.go('chat.live');
      if (e.target.closest('.js-sw')) {
        e.target.closest('.js-sw').classList.toggle('is-on');
        return ctx.toast('开关已切换（原型演示）');
      }
      if (e.target.closest('[data-act="use-preset"]')) return ctx.toast('已用预设预填表单：' + e.target.closest('.preset').textContent.trim());
      if (e.target.closest('[data-act="theme-card"]')) {
        const card = e.target.closest('[data-theme-id]');
        const id = card ? card.getAttribute('data-theme-id') : null;
        return id && ctx.setTheme ? ctx.setTheme(id) : ctx.toggleTheme();
      }
      if (e.target.closest('.tbl tbody tr')) return ctx.toast('打开详情（原型演示）');
    });
  }

  function screen(tabID) {
    const meta = META[tabID] || ['设置', ''];
    return {
      id: 'set.' + tabID,
      title: meta[0],
      desc: meta[1],
      shell: 'app',
      nav: 'settings',
      render: function () { return render(tabID); },
      mount: mount
    };
  }

  window.WB_SETTINGS = { screen: screen, NAV: NAV, META: META };
})();
