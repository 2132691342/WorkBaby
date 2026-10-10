// 开发态假数据：只在 vite dev 下装载，用来在浏览器里看真实界面。
// 字段名严格照抄 types/api.ts，不允许为了好看而另起一套命名。
import type {
  BootstrapVO,
  KnowledgeDocVO,
  MessageVO,
  ProviderVO,
  RuntimeInfo,
  SessionVO,
  SkillVO,
  StatsRESP,
  ToolVO,
} from '../types/api'

const DAY = 86_400_000
const now = Date.now()

export const boot: BootstrapVO = {
  version: '0.6.0',
  contract_version: 1,
  default_provider_id: 'PROVIDER_01',
  default_model: 'claude-sonnet-4-5',
  workspace: 'C:\\Users\\demo\\工作',
  permission: 'ask',
  python_ready: true,
  settings: { theme: 'light', autostart: 'false' },
}

export const sessions: SessionVO[] = [
  ['整理季度销售数据', 'claude-sonnet-4-5', 26, 184_320, 0],
  ['帮我写这周的周报', 'gpt-4o', 14, 62_180, 1],
  ['把这份合同翻译成英文', 'deepseek-chat', 8, 21_400, 1],
  ['看看这个报错是什么原因', 'gpt-4o', 22, 143_900, 4],
  ['给团队做一份培训材料', 'claude-sonnet-4-5', 41, 302_650, 6],
  ['整理客户反馈', 'qwen-max', 12, 38_200, 9],
].map(([title, model, count, tokens, daysAgo], i) => ({
  id: `SESSION_${String(i + 1).padStart(4, '0')}`,
  title: title as string,
  workspace: boot.workspace,
  provider_id: boot.default_provider_id,
  model: model as string,
  permission: 'ask',
  leaf_entry_id: `ENTRY_${i}`,
  message_count: count as number,
  total_tokens: tokens as number,
  created_at: now - (daysAgo as number) * DAY,
  updated_at: now - (daysAgo as number) * DAY + 3_600_000,
}))

const md = (id: string, role: 'user' | 'assistant' | 'tool', content: string, ts: number): MessageVO => ({
  id,
  role,
  type: role === 'user' ? 'text' : role === 'assistant' ? 'text' : 'tool_result',
  content,
  created_at: ts,
  latency_ms: role === 'assistant' ? 4_200 : undefined,
  usage: role === 'assistant'
    ? { input: 1_820, output: 640, cached: 1_240, total: 2_460, context: 9_300, latency_ms: 4_200 }
    : undefined,
})

export const messages: Record<string, MessageVO[]> = {
  SESSION_0001: [
    md('M1', 'user', '这是本季度的销售明细，帮我整理成一份给老板看的总结。', now - 1_800_000),
    {
      ...md('M2', 'assistant', '', now - 1_700_000),
      thinking:
        '用户要一份给老板看的总结。附件是 sales-q3.csv，我先读原始数据，把销售额按区域聚合，算环比，再挑两个异常点提醒。',
      tool_calls: [
        {
          id: 'TC_1',
          name: 'read_file',
          args: { path: 'C:\\Users\\demo\\工作\\sales-q3.csv', limit: 50 },
        },
      ],
    },
    {
      ...md('M3', 'tool', '区域,月份,销售额(万)\n华东,7,188\n华东,8,201\n华东,9,223\n华南,7,102\n华南,8,115\n华南,9,121\n华北,7,119\n华北,8,108\n华北,9,107', now - 1_690_000),
      tool_call_id: 'TC_1',
      tool_name: 'read_file',
    },
    md(
      'M4',
      'assistant',
      '## 季度结论\n\n整体销售额 **¥1,284 万**，环比增长 **18.4%**，其中华东区贡献最大。\n\n| 区域 | 销售额 | 同比 |\n| --- | --- | --- |\n| 华东 | ¥612 万 | +24% |\n| 华南 | ¥338 万 | +12% |\n| 华北 | ¥334 万 | +9% |\n\n### 需要留意\n\n- 华北区连续两个季度增速低于目标，建议复盘渠道结构\n- 9 月中旬有一笔 ¥47 万退货，尚未确认原因\n\n要我把这份总结直接写成一个 Word 文档吗？',
      now - 1_600_000,
    ),
  ],
}

export const providers: ProviderVO[] = [
  {
    id: 'PROVIDER_01',
    name: 'Anthropic',
    api: 'anthropic',
    base_url: 'https://api.anthropic.com',
    has_key: true,
    models: ['claude-sonnet-4-5', 'claude-haiku-4-5'],
    is_default: true,
    enabled: true,
    created_at: now - 30 * DAY,
  },
  {
    id: 'PROVIDER_02',
    name: 'OpenAI',
    api: 'openai',
    base_url: 'https://api.openai.com/v1',
    has_key: true,
    models: ['gpt-4o', 'gpt-4o-mini'],
    is_default: false,
    enabled: true,
    created_at: now - 20 * DAY,
  },
  {
    id: 'PROVIDER_03',
    name: '本地 Ollama',
    api: 'ollama',
    base_url: 'http://127.0.0.1:11434',
    has_key: false,
    models: ['qwen3:8b'],
    is_default: false,
    enabled: true,
    created_at: now - 5 * DAY,
  },
]

export const skills: SkillVO[] = [
  {
    id: 'office-docs@builtin',
    name: 'office-docs',
    description: '生成和修改 Word / Excel / PPT 文件',
    location: 'assets/skills/office-docs',
    source: 'builtin',
    enabled: true,
  },
  {
    id: 'create-skill@builtin',
    name: 'create-skill',
    description: '帮你把一套固定流程写成技能',
    location: 'assets/skills/create-skill',
    source: 'builtin',
    enabled: true,
  },
  {
    id: 'weekly-report@global',
    name: 'weekly-report',
    description: '按我那套格式写周报',
    location: 'skills/weekly-report',
    source: 'global',
    enabled: true,
  },
  {
    id: 'meeting-notes@global',
    name: 'meeting-notes',
    description: '把会议记录整理成待办清单',
    location: 'skills/meeting-notes',
    source: 'global',
    enabled: false,
  },
]

export const tools: ToolVO[] = [
  ['read', '读文件', 'file', '读取工作目录里的文件内容，写文件前必须先读过它', 'low', false, 'sequential'],
  ['write', '写文件', 'file', '新建或整体覆盖一个文件', 'high', true, 'sequential'],
  ['edit', '改文件', 'file', '按上下文片段替换文件内容', 'high', true, 'sequential'],
  ['ls', '看目录', 'file', '列出目录里的文件与文件夹', 'low', false, 'parallel'],
  ['find', '找文件', 'file', '按文件名模式找文件', 'low', false, 'parallel'],
  ['grep', '搜内容', 'file', '在文件内容里搜关键词', 'low', false, 'parallel'],
  ['powershell', '跑命令', 'shell', '执行 PowerShell 命令', 'high', true, 'sequential'],
  ['python', '跑脚本', 'code', '用内置 Python 执行脚本', 'high', true, 'sequential'],
  ['web_search', '搜网页', 'web', '在网上搜资料', 'low', false, 'parallel'],
  ['web_fetch', '开网页', 'web', '打开一个网页并读取正文', 'low', false, 'parallel'],
  ['knowledge_search', '查知识库', 'data', '在放进来的私有资料里检索', 'low', false, 'parallel'],
].map(([name, label, category, description, risk, approval, mode]) => ({
  name: name as string,
  label: label as string,
  category: category as string,
  description: description as string,
  risk: risk as string,
  approval: approval as boolean,
  mode: mode as string,
  params: ['path', 'pattern'],
  enabled: true,
  builtin: true,
}))

export const docs: KnowledgeDocVO[] = [
  {
    id: 'DOC_0001',
    path: '产品手册-v3.md',
    title: '产品手册-v3.md',
    ext: 'md',
    size: 48_210,
    chunks: 26,
    status: 'indexed',
    created_at: now - 3 * DAY,
  },
  {
    id: 'DOC_0002',
    path: '2026年度销售政策.pdf',
    title: '2026年度销售政策.pdf',
    ext: 'pdf',
    size: 1_240_000,
    chunks: 48,
    status: 'indexed',
    created_at: now - 2 * DAY,
  },
  {
    id: 'DOC_0003',
    path: '客户名单.xlsx',
    title: '客户名单.xlsx',
    ext: 'xlsx',
    size: 88_400,
    chunks: 12,
    status: 'failed',
    error: '文件被占用，请先在 Excel 里关掉',
    created_at: now - 1 * DAY,
  },
]

/** 造一段有起伏的用量，空态与满态都能看到。 */
export function stats(days: number): StatsRESP {
  const daily = Array.from({ length: days }, (_, i) => {
    const d = new Date(now - (days - 1 - i) * DAY)
    const date = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
    const wave = Math.sin(i / 2.2) * 0.5 + 0.5
    const input = Math.round((900 + wave * 5_600 + (i % 3) * 320) * (i % 7 === 5 ? 0.25 : 1))
    const output = Math.round((280 + wave * 2_100) * (i % 7 === 5 ? 0.25 : 1))
    // 缓存命中随轮次走高：同一段前缀反复出现时，命中才是常态
    const cached = Math.round(input * (0.45 + wave * 0.4))
    return { date, input, output, cached, total: input + output }
  })
  const sum = daily.reduce(
    (a, d) => ({
      input: a.input + d.input,
      output: a.output + d.output,
      cached: a.cached + d.cached,
      total: a.total + d.total,
    }),
    { input: 0, output: 0, cached: 0, total: 0 },
  )
  return {
    days,
    totals: {
      ...sum,
      calls: Math.round(sum.total / 2_400),
      sessions: 12,
      latency_ms: sum.total > 0 ? 312_000 : 0,
      avg_latency_ms: sum.total > 0 ? 3_900 : 0,
      cache_hit_rate: sum.input > 0 ? sum.cached / sum.input : 0,
      avg_context: sum.input > 0 ? Math.round(sum.input / Math.max(1, Math.round(sum.total / 2_400))) : 0,
      peak_context: 186_400,
    },
    daily,
    models: [
      { model: 'claude-sonnet-4-5', total: 142_800, calls: 48, input: 121_000, output: 21_800, cached: 96_500 },
      { model: 'gpt-4o', total: 86_400, calls: 31, input: 74_600, output: 11_800, cached: 52_300 },
      { model: 'deepseek-chat', total: 41_200, calls: 19, input: 35_900, output: 5_300, cached: 19_400 },
      { model: 'qwen-max', total: 18_600, calls: 12, input: 15_200, output: 3_400, cached: 6_100 },
    ],
    sessions: [
      { session_id: 'SESSION_0005', title: '给团队做一份培训材料', total: 302_650 },
      { session_id: 'SESSION_0001', title: '整理季度销售数据', total: 184_320 },
      { session_id: 'SESSION_0004', title: '看看这个报错是什么原因', total: 143_900 },
      { session_id: 'SESSION_0002', title: '帮我写这周的周报', total: 62_180 },
    ],
  }
}

export const emptyStats: StatsRESP = {
  days: 14,
  totals: {
    input: 0, output: 0, cached: 0, total: 0, calls: 0, sessions: 0,
    latency_ms: 0, avg_latency_ms: 0, cache_hit_rate: 0, avg_context: 0, peak_context: 0,
  },
  daily: [],
  models: [],
  sessions: [],
}

/** 运行时状态：内置 Python 就绪 + PowerShell 回退系统版本，两种状态都能看到 */
export const runtimeInfo: RuntimeInfo = {
  python_exe: 'C:\\Users\\demo\\AppData\\Roaming\\WorkBaby\\runtime\\python\\python.exe',
  python_source: 'bundled',
  python_version: '3.13.14',
  python_error: '',
  powershell_exe: 'C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe',
  powershell_source: 'system',
  powershell_version: '',
  powershell_error: '内置 PowerShell 解压失败：磁盘空间不足',
}

/** 帮助文档夹具：目录与正文对齐 assets/docs 的形态，预览里能完整走「帮助」页 */
export const helpDocs = [
  { name: 'getting-started', title: '快速上手' },
  { name: 'models', title: '配置模型服务' },
  { name: 'tools-and-permissions', title: '工具与安全' },
  { name: 'knowledge-and-skills', title: '知识库与技能' },
  { name: 'faq', title: '常见问题' },
]

export const helpDocContent: Record<string, string> = {
  'getting-started': `# 快速上手

## 第一次启动

1. 双击 \`WorkBaby.exe\`，窗口会自己出现
2. 点左侧导航栏的 **设置** 图标 → **模型服务**，选一个你常用的服务商模板
3. 去服务商官网申请一个 API Key，粘贴进去，点 **拉取模型列表**，勾一个模型，**保存**，再点 **测试**
4. 点导航栏的 **对话** 图标回到聊天界面，直接说人话就行

## 日常使用

| 你想要 | 就这么说 |
|---|---|
| 读文件 | 「帮我看看桌面上那个报告文件夹里有什么」 |
| 处理表格 | 「把这个 Excel 按月份汇总，给我个总数」 |
| 写东西 | 「帮我写一封请假邮件，客气一点」 |

- **发送**：回车；换行：Shift + 回车
- 助手干活的每一步都会展开成卡片，点开能看细节`,
  models: `# 配置模型服务

WorkBaby 自己不生产智能，它把你说的话转给一家「模型服务」去回答。

## 怎么配

1. **设置 → 模型服务 → 新增服务商**
2. 选一个模板，接口地址会自动填好
3. 把 API Key 粘贴进去
4. 点 **拉取模型列表**，从结果里勾一个模型
5. **保存**，然后点 **测试** —— 显示「连接正常」就配好了`,
  'tools-and-permissions': `# 工具与安全

## 三级执行方式

**设置 → 行为 → 执行方式**，选一档：

| 档位 | 助手的行为 |
|---|---|
| 每次问我（默认） | 改文件、跑命令之前都弹一张卡问你 |
| 自动改文件 | 改文件不再打断你 |
| 全自动 | 什么都不问，直接干活 |`,
  'knowledge-and-skills': `# 知识库与技能

## 知识库

设置 → 知识库 → **添加文档**，支持 md / txt / pdf / docx / xlsx 等格式。

## 技能

技能 = 给助手看的「操作秘籍」。对话里打 \`/\` 就能召唤。`,
  faq: `# 常见问题

**问：发送后一直转圈，没有回答？**
先看输入框上方有没有审批卡片在等你点「放行」。

**问：它会偷偷改我的文件吗？**
默认不会。每次改文件、跑命令前都要你点「放行」。`,
}
