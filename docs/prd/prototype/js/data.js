/* ============================================================
   演示数据 —— 全部为静态示例，仅服务原型浏览
   ============================================================ */
(function () {
  'use strict';

  const DATA = {
    version: 'v0.9.4',
    homeDir: 'C:\\Users\\LIKX\\AppData\\Roaming\\WorkBaby',

    /* ---- 会话列表 ---- */
    sessions: [
      { id: 's1', name: '重构 RAG 检索打分', group: '置顶', pinned: true, time: '2 分钟前', model: 'deepseek-chat', ws: true },
      { id: 's2', name: '分析上周的慢查询日志', group: '今天', time: '10:24', model: 'deepseek-chat' },
      { id: 's3', name: '给 core 包补一组护栏测试', group: '今天', time: '09:41', model: 'deepseek-reasoner' },
      { id: 's4', name: '整理会议纪要（docx → 知识库）', group: '昨天', time: '昨天', model: 'deepseek-chat' },
      { id: 's5', name: 'MCP 的文件服务调通', group: '昨天', time: '昨天', model: 'gpt-4o' },
      { id: 's6', name: '调研 eino 的图编排', group: '一周内', time: '周三', model: 'deepseek-reasoner' },
      { id: 's7', name: '打包脚本排查（NSIS 报错）', group: '一周内', time: '周二', model: 'deepseek-chat', branch: 2 },
      { id: 's8', name: 'Python 环境清理', group: '更早', time: '8月12日', model: 'qwen2.5:14b' }
    ],

    /* ---- 当前会话：一条完整的 run 脚本 ---- */
    runScript: {
      title: '重构 RAG 检索打分',
      workspace: 'D:\\GoFiles\\WorkBaby',
      permission: 'default',
      model: 'deepseek-chat',
      userMsg: '帮我把 internal/rag 的检索打分逻辑重构成可测试的：把打分函数从 retriever.go 里拆成纯函数，再补一组 table-driven 测试。',
      thinking: '用户要的是可测试性重构，不是改行为。先确认 retriever.go 里打分相关的耦合点：它读取了 index 的字段和配置权重，直接搬会带依赖。方案是把权重作为参数注入，打分本体变成纯函数。测试要覆盖：空查询、全命中、无命中、权重边界。',
      steps: [
        {
          name: 'read_file',
          args: 'internal/rag/retriever.go',
          result: '1 │ package rag\n2 │ \n3 │ // Retriever 组合 FTS 与向量两路召回\n… 186 行',
          note: '读取 186 行'
        },
        {
          name: 'grep',
          args: 'scoreByFTS|scoreByVector|weights',
          result: 'retriever.go:64  func (r *Retriever) scoreByFTS(doc Doc, q string) float64\nretriever.go:88  func (r *Retriever) scoreByVector(doc Doc, vec []float32) float64\nretriever.go:31  weights Weights',
          note: '3 处命中'
        },
        {
          name: 'edit_file',
          args: 'internal/rag/scorer.go（新建）',
          result: '+ 48 行：ScoreFTS / ScoreVector / Blend（纯函数，权重显式入参）',
          note: '+48 −0'
        },
        {
          name: 'exec',
          args: 'go test ./internal/rag/...',
          result: 'ok  WorkBaby/internal/rag  0.42s\n?   WorkBaby/internal/rag/chunker  [no test files]',
          note: '0.42s'
        }
      ],
      changes: [
        { path: 'internal/rag/scorer.go', add: 48, del: 0, state: 'new' },
        { path: 'internal/rag/retriever.go', add: 6, del: 41, state: 'mod' },
        { path: 'internal/rag/scorer_test.go', add: 96, del: 0, state: 'new' }
      ],
      finalText: '拆完了，行为不变、覆盖到位：\n\n- 新增 `scorer.go`：两个纯函数 + 一个融合函数，权重从参数进，不再读 Retriever 字段；\n- `retriever.go` 里原方法退化为一行委托，删掉 41 行耦合代码；\n- 新增 `scorer_test.go`：7 个 table-driven 用例，含空查询与权重边界。\n\n`go test ./internal/rag/...` 已通过（0.42s）。要我顺手把 `chunker` 也补上吗？',
      usage: { in: '12.4k', out: '3.2k', cache: 88, sec: '21s' }
    },

    /* ---- 审批示例（C04） ---- */
    approval: {
      tool: 'exec',
      risk: 'danger',
      cmd: 'git push --force-with-lease origin main',
      cwd: 'D:\\GoFiles\\WorkBaby',
      reason: '命令命中受控清单（git push）：需要你确认后再执行。',
      elapsed: 12
    },

    /* ---- 目标模式（C05） ---- */
    goal: {
      text: '把 internal/rag 的重构完整落地：拆分 → 测试 → 文档同步，全部通过验收。',
      round: 3,
      maxRound: 8,
      state: 'active',
      next: '补齐 chunker 测试并同步 doc/09-知识库与RAG.md',
      todos: [
        { text: '拆分打分函数为纯函数', state: 'done' },
        { text: '补充 table-driven 测试', state: 'done' },
        { text: 'chunker 测试补齐', state: 'doing' },
        { text: '同步 doc/09 接口说明', state: 'todo' },
        { text: '全量 go test 复跑', state: 'todo' }
      ]
    },

    /* ---- 侧栏搜索命中的跨会话结果 ---- */
    searchHits: [
      { id: 's6', name: '调研 eino 的图编排', hit: '…打分与召回顺序在 graph 里是显式节点…' },
      { id: 's3', name: '给 core 包补一组护栏测试', hit: '…rag 的 scorer 需要显式权重入参才好测…' }
    ],

    /* ---- Provider ---- */
    providers: [
      { id: 'p1', name: 'DeepSeek 主力', kind: 'openai', model: 'deepseek-chat', ctx: '128k', tier: 'primary', tools: true, vision: false, reason: true, state: 'ok', key: 'sk-…f3a2' },
      { id: 'p2', name: 'DeepSeek 推理', kind: 'openai', model: 'deepseek-reasoner', ctx: '128k', tier: 'primary', tools: true, vision: false, reason: true, state: 'ok', key: 'sk-…f3a2' },
      { id: 'p3', name: 'OpenAI 备用', kind: 'openai', model: 'gpt-4o', ctx: '128k', tier: 'backup', tools: true, vision: true, reason: false, state: 'open', key: 'sk-…9c11' },
      { id: 'p4', name: '本地 Ollama', kind: 'ollama', model: 'qwen2.5:14b', ctx: '32k', tier: 'backup', tools: true, vision: false, reason: false, state: 'off', key: '—' }
    ],


    /* ---- 工具（按 5 组） ---- */
    toolGroups: [
      {
        name: '文件与目录', tools: [
          { name: 'file_read', desc: '读取工作区内文件内容（支持行区间）', risk: 'ok', on: true },
          { name: 'file_write', desc: '写入或新建工作区文件', risk: 'warn', on: true },
          { name: 'file_edit', desc: '对文件做精确字符串替换编辑', risk: 'warn', on: true },
          { name: 'file_list', desc: '列出目录内容（含大小与时间）', risk: 'ok', on: true },
          { name: 'grep', desc: '按正则检索文件内容', risk: 'ok', on: true },
          { name: 'find', desc: '按通配符查找文件路径', risk: 'ok', on: true }
        ]
      },
      {
        name: '命令与网络', tools: [
          { name: 'exec', desc: '执行白名单内的本机命令', risk: 'bad', on: true },
          { name: 'web_search', desc: 'DuckDuckGo 搜索（免 Key）', risk: 'ok', on: true },
          { name: 'webfetch', desc: '抓取网页正文并转 Markdown', risk: 'ok', on: true },
          { name: 'http', desc: '通用 HTTP 请求（仅 GET/POST）', risk: 'info', on: true }
        ]
      },
      {
        name: '文档与知识', tools: [
          { name: 'doc_extract', desc: '抽取 pdf / docx / xlsx / html 正文', risk: 'ok', on: true },
          { name: 'kb_search', desc: '检索本地知识库（FTS5 trigram）', risk: 'ok', on: true },
          { name: 'wiki_page', desc: '读取仓库导读页面', risk: 'ok', on: false }
        ]
      },
      {
        name: '记忆·计划·委派', tools: [
          { name: 'memory_write', desc: '写入长期记忆（指定分节）', risk: 'warn', on: true },
          { name: 'todo_write', desc: '维护当前任务待办', risk: 'ok', on: true },
          { name: 'request_input', desc: '向用户补问一个关键问题', risk: 'ok', on: true },
          { name: 'delegate_task', desc: '委派隔离子智能体执行子任务', risk: 'warn', on: true }
        ]
      },
      {
        name: '技能与 MCP', tools: [
          { name: 'skill_run', desc: '执行技能内的脚本', risk: 'warn', on: true },
          { name: 'mcp_filesystem', desc: 'MCP：文件系统服务（filesystem）', risk: 'warn', on: true },
          { name: 'mcp_fetch', desc: 'MCP：网页抓取服务（fetch）', risk: 'info', on: false }
        ]
      }
    ],

    /* ---- 技能 ---- */
    skills: [
      { name: 'commit-craft', desc: '按仓库习惯生成规范提交信息并分块提交', scripts: 1, builtin: false, on: true },
      { name: 'sql-explain', desc: '解析 EXPLAIN 输出，指出缺索引与回表', scripts: 2, builtin: false, on: true },
      { name: 'doc-sync', desc: '代码改动后同步 doc/ 下的契约表', scripts: 1, builtin: true, on: false },
      { name: 'ppt-outline', desc: '从 Markdown 生成逐页讲稿大纲', scripts: 1, builtin: true, on: true },
      { name: 'log-digest', desc: '压缩日志文件，抽取异常序列', scripts: 1, builtin: false, on: true }
    ],

    /* ---- MCP ---- */
    mcpRaw: '{\n  "mcpServers": {\n    "filesystem": {\n      "command": "npx",\n      "args": ["-y", "@modelcontextprotocol/server-filesystem", "D:\\\\GoFiles"],\n      "enabled": true\n    },\n    "fetch": {\n      "command": "npx",\n      "args": ["-y", "@modelcontextprotocol/server-fetch"],\n      "enabled": false\n    }\n  }\n}',
    mcpServers: [
      { name: 'filesystem', transport: 'stdio', cmd: 'npx @modelcontextprotocol/server-filesystem', on: true, tools: 5 },
      { name: 'fetch', transport: 'stdio', cmd: 'npx @modelcontextprotocol/server-fetch', on: false, tools: 1 },
      { name: 'sqlite', transport: 'stdio', cmd: 'uvx mcp-server-sqlite --db workbaby.db', on: true, tools: 3 }
    ],

    /* ---- 子智能体 ---- */
    agents: [
      { name: 'code-reviewer', desc: '只读审查：找缺陷与风险，不改代码', tools: 'file_read, grep, file_list', model: 'deepseek-reasoner', turns: 12, on: true },
      { name: 'doc-writer', desc: '按 doc/ 规范补写文档，允许写 markdown', tools: 'file_read, file_write, grep', model: '—', turns: 8, on: true },
      { name: 'log-hunter', desc: '大日志文件检索与异常归纳', tools: 'file_read, grep, exec', model: '—', turns: 16, on: false }
    ],

    /* ---- 自定义命令 ---- */
    commands: [
      { name: 'review', desc: '对当前工作区未提交改动做一轮审查', prompt: '阅读 git diff 的输出，按「正确性 / 边界 / 可测性」三段给出审查意见…' },
      { name: 'reindex', desc: '重建知识库派生索引', prompt: '调用 kb 相关工具，对分组 {group} 全量重建索引，报告耗时与失败项…' },
      { name: 'ship', desc: '构建并产出 NSIS 安装包', prompt: '执行 wails build -nsis，校验产物哈希与体积，输出变更点…' }
    ],

    /* ---- 用户钩子 ---- */
    hooks: [
      { name: '禁止 force push', event: 'PreToolUse', matcher: 'exec', cmd: 'pwsh -File .workbaby/hooks/no-force-push.ps1', on: true },
      { name: '自动格式化', event: 'PostToolUse', matcher: 'file_write,file_edit', cmd: 'pwsh -File .workbaby/hooks/fmt.ps1', on: true },
      { name: '完成时通知', event: 'Stop', matcher: '—', cmd: 'pwsh -File .workbaby/hooks/notify.ps1', on: false }
    ],

    /* ---- 知识库 ---- */
    kdocs: [
      { id: 'k1', name: 'Go 工程规范速查', src: 'file', type: 'md', size: '18 KB', chunks: 42, group: '工程规范', state: 'indexed', time: '2 天前' },
      { id: 'k2', name: 'WorkBaby 架构评审记录', src: 'text', type: 'md', size: '6 KB', chunks: 14, group: '项目', state: 'indexed', time: '5 天前' },
      { id: 'k3', name: 'SQLite FTS5 中文分词调研', src: 'url', type: 'html', size: '31 KB', chunks: 58, group: '调研', state: 'indexing', time: '1 小时前' },
      { id: 'k4', name: '慢查询优化手册.pdf', src: 'file', type: 'pdf', size: '2.4 MB', chunks: 186, group: '数据库', state: 'indexed', time: '昨天' },
      { id: 'k5', name: '内网部署清单.xlsx', src: 'file', type: 'xlsx', size: '88 KB', chunks: 24, group: '运维', state: 'failed', time: '3 天前' }
    ],
    kdocGroups: ['全部', '工程规范', '项目', '调研', '数据库', '运维'],

    /* ---- 记忆（单一 MEMORY.md，四个分节） ---- */
    memory: {
      path: 'C:\\Users\\LIKX\\AppData\\Roaming\\WorkBaby\\memory\\MEMORY.md',
      sections: [
        { name: '用户偏好', color: 'var(--accent)', items: ['回复用中文，代码注释保持英文', '提交信息用 conventional commits，正文写清场景', '不要替我做 git push，我自己确认'] },
        { name: '稳定事实', color: 'var(--ok)', items: ['主力模型 DeepSeek，备用 OpenAI', '工作区固定在 D:\\GoFiles', '常用终端是 PowerShell 7'] },
        { name: '流程约定', color: 'var(--ink-3)', items: ['改后端先跑 scripts/check-boundaries.ps1', '新增依赖必须说明理由'] },
        { name: '项目约定', color: 'var(--warn)', items: ['JSON 契约字段一律 snake_case', 'internal/pkg 是叶子包，不许反向依赖'] }
      ]
    },

    /* ---- 运行历史 ---- */
    runs: [
      { time: '10:42:11', model: 'deepseek-chat', state: 'running', reason: '用户消息', turns: 3, in: '12.4k', out: '3.2k', llm: 62, tool: 38 },
      { time: '10:24:03', model: 'deepseek-chat', state: 'done', reason: '用户消息', turns: 9, in: '48.1k', out: '9.7k', llm: 55, tool: 45 },
      { time: '09:58:47', model: 'deepseek-reasoner', state: 'done', reason: '目标续跑', turns: 6, in: '31.5k', out: '12.9k', llm: 78, tool: 22 },
      { time: '09:41:20', model: 'deepseek-reasoner', state: 'error', reason: '用户消息', turns: 2, in: '8.2k', out: '1.1k', llm: 91, tool: 9 },
      { time: '昨天 21:07', model: 'gpt-4o', state: 'done', reason: '后台任务', turns: 5, in: '22.8k', out: '4.4k', llm: 60, tool: 40 }
    ],

    /* ---- 仪表盘 ---- */
    stats: { msg: '1,284', sess: '86', token: '4.2M', tools: '23' },
    tokenTrend: [
      { label: '09-15', in: 210, out: 88, cache: 120 },
      { label: '09-16', in: 340, out: 121, cache: 210 },
      { label: '09-17', in: 280, out: 96, cache: 190 },
      { label: '09-18', in: 465, out: 158, cache: 300 },
      { label: '09-19', in: 388, out: 130, cache: 255 },
      { label: '09-20', in: 520, out: 186, cache: 402 },
      { label: '09-21', in: 296, out: 104, cache: 233 }
    ],

    /* ---- 文件 ---- */
    files: [
      { name: '架构草图.png', size: '482 KB', type: 'image', time: '2 小时前' },
      { name: '评审记录.docx', size: '36 KB', type: 'doc', time: '昨天' },
      { name: '慢查询样本.sql', size: '12 KB', type: 'code', time: '昨天' },
      { name: '接口契约.xlsx', size: '88 KB', type: 'sheet', time: '3 天前' },
      { name: '部署清单.md', size: '4 KB', type: 'doc', time: '5 天前' },
      { name: '切图-首页.zip', size: '2.1 MB', type: 'archive', time: '上周' }
    ],

    /* ---- 命令面板条目 ---- */
    palette: {
      action: [
        { name: '新建任务', hint: 'Ctrl+N' },
        { name: '收起 / 展开导航', hint: 'Ctrl+B' },
        { name: '焦点模式', hint: 'Ctrl+Shift+F' },
        { name: '压缩当前会话历史', hint: '' },
        { name: '切换主题', hint: '' }
      ],
      mode: [
        { name: '标准模式', hint: '每次写操作都问我', on: false },
        { name: '自动编辑', hint: '写文件不打扰', on: false },
        { name: '完全放行', hint: '工作区内全部放行', on: false }
      ],
      session: [],
      nav: [
        { name: '模型服务', hint: '设置 · 模型' },
        { name: '工具', hint: '设置 · 能力' },
        { name: '技能', hint: '设置 · 能力' },
        { name: 'MCP 服务', hint: '设置 · 能力' },
        { name: '子智能体', hint: '设置 · 能力' },
        { name: '记忆中心', hint: '设置 · 数据' },
        { name: '知识库', hint: '设置 · 数据' },
        { name: '仪表盘', hint: '设置 · 数据' },
        { name: '运行历史', hint: '设置 · 数据' },
        { name: '外观', hint: '设置 · 系统' },
        { name: '桌宠', hint: '设置 · 系统' }
      ]
    },

    /* ---- 斜杠命令 ---- */
    slashes: [
      { id: '/compact', args: '', desc: '压缩当前会话的历史上下文' },
      { id: '/clear', args: '', desc: '清空当前会话消息' },
      { id: '/goal', args: '<目标描述>', desc: '设置会话目标，逐轮推进直到达成' },
      { id: '/plan', args: '<任务>', desc: '只出计划不执行，等我确认' },
      { id: '/review', args: '[路径]', desc: '对未提交改动做一轮审查', src: '用户' },
      { id: '/reindex', args: '<group>', desc: '重建知识库派生索引', src: '用户' },
      { id: '/task', args: '<任务描述>', desc: '提交到后台任务队列异步执行' },
      { id: '/run', args: '<技能名>', desc: '直接执行某个技能脚本', src: '工作区' }
    ],

    /* ---- @ 提及 ---- */
    mentions: {
      skill: [{ name: 'sql-explain', desc: '解析 EXPLAIN 输出' }, { name: 'log-digest', desc: '压缩日志抽取异常' }],
      kdoc: [{ name: 'Go 工程规范速查', desc: '42 个片段' }, { name: '慢查询优化手册.pdf', desc: '186 个片段' }],
      file: [{ name: 'internal/rag/retriever.go', desc: '186 行' }, { name: 'doc/09-知识库与RAG.md', desc: '212 行' }],
      folder: [{ name: 'internal/rag', desc: '7 个文件' }]
    }
  };

  window.DATA = DATA;
})();
