// 右侧工作台状态：多标签常驻面板，每个标签是一个文件的「差异 / 文件」检查。
// 模块级单例（与旧 useFilePeek 同一理由）：聊天里任何入口（工具卡 / 轮尾变更）
// 都开同一份面板；重复点同一文件只激活已有标签，不堆重复标签。
import { computed, ref } from 'vue'

/** 一个工作台标签：path 是去重键，mode 记住用户在看差异还是内容。 */
export interface WorkTab {
  id: string
  path: string
  title: string
  mode: 'diff' | 'file'
}

const WIDTH_KEY = 'wb.workpanel.width'
const MIN_WIDTH = 300
const MAX_WIDTH = 720

const tabs = ref<WorkTab[]>([])
const activeId = ref('')
const width = ref(clampWidth(Number(localStorage.getItem(WIDTH_KEY)) || 420))

let seq = 0

function clampWidth(px: number): number {
  if (!Number.isFinite(px)) return 420
  return Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, Math.round(px)))
}

function basename(path: string): string {
  const i = Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'))
  return i >= 0 ? path.slice(i + 1) : path
}

/** 打开（或激活）一个文件标签。聊天入口统一走这里。 */
export function openWorkFile(path: string, title = ''): void {
  if (!path) return
  const existing = tabs.value.find((t) => t.path === path)
  if (existing) {
    activeId.value = existing.id
    return
  }
  const id = `wt-${++seq}`
  tabs.value = [...tabs.value, { id, path, title: title || basename(path), mode: 'diff' }]
  activeId.value = id
}

/** 关一个标签：关掉的是激活标签时，把激活位让给右邻（没有则左邻）。 */
export function closeWorkTab(id: string): void {
  const index = tabs.value.findIndex((t) => t.id === id)
  if (index < 0) return
  const next = tabs.value.filter((t) => t.id !== id)
  tabs.value = next
  if (activeId.value === id) {
    const fallback = next[Math.min(index, next.length - 1)]
    activeId.value = fallback ? fallback.id : ''
  }
}

/** 切换某标签的差异 / 文件视图。 */
export function setWorkMode(id: string, mode: 'diff' | 'file'): void {
  tabs.value = tabs.value.map((t) => (t.id === id ? { ...t, mode } : t))
}

/** 拖宽：夹在上下限内并记住，下次打开还是这个宽度。 */
export function setWorkWidth(px: number): void {
  width.value = clampWidth(px)
  localStorage.setItem(WIDTH_KEY, String(width.value))
}

/** 工作台是否展开：有标签才占地方，全关了就收起。 */
const open = computed(() => tabs.value.length > 0)
const active = computed(() => tabs.value.find((t) => t.id === activeId.value) ?? null)

/** 工作台状态与动作。 */
export function useWorkPanel() {
  return {
    tabs,
    activeId,
    active,
    open,
    width,
    openFile: openWorkFile,
    closeTab: closeWorkTab,
    setMode: setWorkMode,
    setWidth: setWorkWidth,
  }
}