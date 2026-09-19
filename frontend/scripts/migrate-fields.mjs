// 一次性迁移脚本：把前端源码中后端数据对象的 camelCase 字段引用批量替换为 snake_case。
// 只替换「.fieldName」属性访问 和 「fieldName:」对象字面量 key，避免误伤本地变量。
// 用法：node scripts/migrate-fields.mjs
import { readFileSync, writeFileSync, readdirSync, statSync } from 'node:fs'
import { join, extname } from 'node:path'

// camelCase → snake_case 映射表（后端 domain json tag）
const MAP = {
  // 通用
  userID: 'user_id',
  sessionID: 'session_id',
  providerID: 'provider_id',
  workspaceID: 'workspace_id',
  messageCount: 'message_count',
  lastMessageAt: 'last_message_at',
  metadataJSON: 'metadata_json',
  createdAt: 'created_at',
  updatedAt: 'updated_at',
  // Message
  runID: 'run_id',
  toolCallID: 'tool_call_id',
  toolCallsJson: 'tool_calls_json',
  stopReason: 'stop_reason',
  inputTokens: 'input_tokens',
  outputTokens: 'output_tokens',
  totalTokens: 'total_tokens',
  latencyMs: 'latency_ms',
  // AiProvider
  apiKeyMasked: 'api_key_masked',
  baseUrl: 'base_url',
  contextWindow: 'context_window',
  compressRatio: 'compress_ratio',
  thinkingEffort: 'thinking_effort',
  capabilitiesJson: 'capabilities_json',
  pricingJson: 'pricing_json',
  apiKey: 'api_key',
  // Circuit
  consecutiveFailures: 'consecutive_failures',
  retryInMs: 'retry_in_ms',
  // Skill
  whenToUse: 'when_to_use',
  allowedTools: 'allowed_tools',
  sourceKind: 'source_kind',
  sourceRef: 'source_ref',
  // Mcp
  toolCount: 'tool_count',
  baseURL: 'base_url',
  // Workflow
  definitionYaml: 'graph',
  workflowID: 'workflow_id',
  errorMsg: 'error_msg',
  startedAt: 'started_at',
  finishedAt: 'finished_at',
  inputJson: 'inputs',
  outputJson: 'outputs',
  stateJson: 'state_json',
  // Memory
  lastUsedAt: 'last_used_at',
  successCount: 'success_count',
  failureCount: 'failure_count',
  // Media
  paramsJson: 'params_json',
  isDefault: 'is_default',
  presetID: 'preset_id',
  previewUrl: 'preview_url',
  downloadUrl: 'download_url',
  mimeType: 'mime_type',
  fileSize: 'file_size',
  // Folder / File
  parentID: 'parent_id',
  childCount: 'child_count',
  originalName: 'original_name',
  fileType: 'file_type',
  folderID: 'folder_id',
  // Knowledge
  sizeBytes: 'size_bytes',
  chunkCount: 'chunk_count',
  groupName: 'source_type',
  // Cron
  lastRunAt: 'last_run_at',
  lastStatus: 'last_status',
  nextRunAt: 'next_run_at',
  actionArgs: 'action_args',
  // Pet
  spriteID: 'sprite_id',
  positionX: 'position_x',
  positionY: 'position_y',
  bubbleEnabled: 'bubble_enabled',
  bubbleDurationMs: 'bubble_duration_ms',
  filePath: 'file_path',
  frameCount: 'frame_count',
  isBuiltin: 'is_builtin',
  // Channel
  channelType: 'channel_type',
  configJson: 'config_json',
  webhookUrl: 'webhook_url',
  lastError: 'last_error',
  lastActiveAt: 'last_active_at',
  channelID: 'channel_id',
  messageType: 'message_type',
  userExternalID: 'user_external_id',
  contentSummary: 'content_summary',
  errorMessage: 'error_message',
  emailTo: 'email_to',
  emailDisplayName: 'email_display_name',
  // Token
  accessToken: 'access_token',
  tokenType: 'token_type',
  expiresIn: 'expires_in',
  // Dashboard
  memoryEpisodes: 'memory_episodes',
  memoryFacts: 'memory_facts',
  memoryProcedures: 'memory_procedures',
  mediaArtifacts: 'media_artifacts',
  cronJobs: 'cron_jobs',
  aiToolsTotal: 'ai_tools_total',
  todaySessions: 'today_sessions',
  todayMessages: 'today_messages',
  todayTokens: 'today_tokens',
  recentMessages: 'recent_messages',
  recentMedia: 'recent_media',
  recentExecutions: 'recent_executions',
  // Runtime / Health
  uptimeMs: 'uptime_ms',
  dbEnabled: 'db_enabled',
  osName: 'os_name',
  osArch: 'os_arch',
  goVersion: 'go_version',
  userHome: 'user_home',
  numCPU: 'num_cpu',
  goroutines: 'goroutines',
  heapAllocMB: 'heap_alloc_mb',
  heapSysMB: 'heap_sys_mb',
  bundledDir: 'bundled_dir',
  archiveFound: 'archive_found',
  // Schema / Meta
  schemaVersion: 'schema_version',
  nodeID: 'node_id',
  // GenUI
  genUi: 'gen_ui',
}

const keys = Object.keys(MAP).sort((a, b) => b.length - a.length) // 长 key 优先

// 只替换文件内「.field」/「field:」两种形态；跳过注释与字符串内（保守：整体替换，字段名罕见出现在字符串）
function replaceContent(src) {
  let out = src
  for (const k of keys) {
    const v = MAP[k]
    // 0) Wails 生成 models.ts 的 source["camelCase"] 索引
    out = out.replace(new RegExp(`source\\["${k}"\\]`, 'g'), `source["${v}"]`)
    // 1) 属性访问 .field / ?.field
    out = out.replace(new RegExp(`\\.${k}\\b`, 'g'), `.${v}`)
    out = out.replace(new RegExp(`\\?\\.${k}\\b`, 'g'), `?.${v}`)
    // 2) 对象字面量 key：field:
    out = out.replace(new RegExp(`\\b${k}:`, 'g'), `${v}:`)
  }
  return out
}

function walk(dir) {
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules' || name === 'dist' || name === '.git') continue
    const p = join(dir, name)
    if (statSync(p).isDirectory()) {
      walk(p)
    } else if (extname(p) === '.vue' || extname(p) === '.ts') {
      const before = readFileSync(p, 'utf8')
      const after = replaceContent(before)
      if (after !== before) {
        writeFileSync(p, after, 'utf8')
        console.log('migrated:', p)
      }
    }
  }
}

walk(join(import.meta.dirname, '../src'))
console.log('done')
