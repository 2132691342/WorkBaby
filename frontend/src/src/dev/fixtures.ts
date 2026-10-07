// 开发态假数据：只在 vite dev 下装载，用来在浏览器里看真实界面。
// 字段名严格照抄 types/api.ts，不允许为了好看而另起一套命名。
import type {
  BootstrapVO,
  KnowledgeDocVO,
  MessageVO,
  ProviderVO,
  SessionVO,
  SkillVO,
  StatsRESP,
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
