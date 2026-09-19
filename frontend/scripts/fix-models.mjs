// 修复 wailsjs/go/models.ts：把带 ? 的 camelCase 属性声明改为 snake_case，
// 与后端 json tag / 构造函数赋值保持一致。
import { readFileSync, writeFileSync } from 'node:fs'

const p = 'src/wailsjs/go/models.ts'
let src = readFileSync(p, 'utf8')

// camelCase → snake_case（属性声明 key?）
const MAP = {
  positionX: 'position_x',
  positionY: 'position_y',
  bubbleEnabled: 'bubble_enabled',
  bubbleDurationMs: 'bubble_duration_ms',
  frameCount: 'frame_count',
  thinkingEffort: 'thinking_effort',
  allowedTools: 'allowed_tools',
  sourceRef: 'source_ref',
  lastRunAt: 'last_run_at',
  nextRunAt: 'next_run_at',
  finishedAt: 'finished_at',
  errorMsg: 'error_msg',
  apiKey: 'api_key',
  parentID: 'parent_id',
  workspaceID: 'workspace_id',
  capabilitiesJson: 'capabilities_json',
  pricingJson: 'pricing_json',
  folderID: 'folder_id',
}

for (const k of Object.keys(MAP)) {
  const v = MAP[k]
  // 1) 属性声明：`k?: type` 或 `k: type`
  src = src.replace(new RegExp(`\\b${k}\\?:`, 'g'), `${v}?:`)
  src = src.replace(new RegExp(`\\b${k}:`, 'g'), `${v}:`)
  // 2) source 索引
  src = src.replace(new RegExp(`source\\["${k}"\\]`, 'g'), `source["${v}"]`)
}

writeFileSync(p, src, 'utf8')
console.log('models.ts fixed')
