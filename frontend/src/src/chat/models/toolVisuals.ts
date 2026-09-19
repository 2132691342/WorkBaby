/**
 * 工具视觉映射：工具 → 图标的唯一实现（时间线、计划胶囊、审批卡共用）。
 *
 * <p>展示优先用后端下发的 `activity`（工具自述的动词，如「正在编辑 app.ts」），
 * 名称映射只在历史数据（无 activity）与需要图标的场景兜底。
 * 新增工具时多数情况无需改这里——后端 ActivityDesc / Category 会先一步给出人话。
 */
import type { Component } from 'vue'
import {
  Clock,
  Type,
  Braces,
  Table,
  Database,
  BarChart3,
  Hash,
  Dices,
  Globe,
  Code2,
  Image as ImageIcon,
  LayoutTemplate,
  Cat,
  ScanText,
  FileText,
  FolderOpen,
  Calculator,
  Regex,
  PackageCheck,
  Sparkles,
  Brain,
  GitBranch,
  BookOpen,
  Bot
} from '@/components/common/icons'

/** 工具名 → 分类图标；未识别回落 Sparkles（保持与旧时间线一致的表现）。 */
export function toolIcon(name: string): Component {
  const n = (name ?? '').toLowerCase()
  if (n === 'delegate_task' || n.startsWith('delegate')) return Bot
  if (n.includes('time') || n.startsWith('date_')) return Clock
  if (n.startsWith('text_')) return Type
  if (n.startsWith('json_')) return Braces
  if (n.startsWith('csv_')) return Table
  if (n.startsWith('data_')) return Database
  if (n.startsWith('chart_')) return BarChart3
  if (n.startsWith('hash_') || n.startsWith('base64_') || n.startsWith('url_')) return Hash
  if (n.startsWith('random_')) return Dices
  if (n.startsWith('http_') || n.startsWith('ip_')) return Globe
  if (n.startsWith('code_')) return Code2
  if (n.startsWith('image_') || n.startsWith('video_') || n.startsWith('audio_') || n.startsWith('model3d') || n.startsWith('vfx')) return ImageIcon
  if (n === 'gen_ui') return LayoutTemplate
  if (n.startsWith('pet_')) return Cat
  if (n.startsWith('ocr_')) return ScanText
  if (n.startsWith('pdf_') || n.startsWith('word_') || n.startsWith('excel_')) return FileText
  if (n.startsWith('file_') || n.startsWith('folder_') || n.startsWith('archive_')) return FolderOpen
  if (n.startsWith('math_')) return Calculator
  if (n.startsWith('regex_')) return Regex
  if (n.startsWith('present_')) return PackageCheck
  if (n === 'memory_write' || n.startsWith('memory_')) return Brain
  if (n === 'run_workflow' || n.startsWith('workflow_')) return GitBranch
  if (n.startsWith('knowledge_')) return BookOpen
  return Sparkles
}

/** 工具行的展示文案：优先工具自述的动作，缺失时退回工具名。 */
export function toolLabel(name: string, activity?: string): string {
  return activity && activity.trim() !== '' ? activity : name
}
