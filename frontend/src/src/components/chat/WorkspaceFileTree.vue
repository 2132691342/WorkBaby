<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { FileText, FolderOpen, Image, RefreshCw } from '@/components/common/icons'
import { apiGet, apiPost } from '@/api/client'
import { t } from '@/i18n'
import { useDialog } from '@/composables/useDialog'
import { useToast } from '@/composables/useToast'
import type { WorkspaceEntry, WorkspaceListRESP } from '@/types/api'

/**
 * 工作区真实文件树（懒加载）。
 *
 * <p>一层一层按需拉取（GET /chat/workspace/:id/ls），点开文件夹才请求下一层，大目录不爆；
 * 顶部筛选框在已加载范围内匹配；右键条目唤起操作菜单（添加到聊天 / 复制路径 / 重命名 /
 * 创建副本 / 删除），「添加到聊天」插入<b>绝对路径</b>（相对路径解析依赖工作区语义，易歧义）。
 */
const props = defineProps<{ session_id: string | null }>()
const emit = defineEmits<{ attach: [path: string] }>()

const dialog = useDialog()
const toast = useToast()

/** 目录路径（不带尾斜杠）→ 已加载的子项。 */
const cache = ref(new Map<string, WorkspaceEntry[]>())
const expanded = ref(new Set<string>())
const loadingDirs = ref(new Set<string>())
const truncated = ref(new Set<string>())
const filter = ref('')
/** 工作区根绝对路径（后端 ls 随响应返回）。 */
const root = ref('')

const rootLoading = computed(() => loadingDirs.value.has(''))
const rootEntries = computed(() => cache.value.get('') ?? [])

/** 相对路径 → 工作区内绝对路径（Windows 应用统一反斜杠；无根信息时原样返回）。 */
function absPath(rel: string): string {
  if (!root.value) return rel
  return root.value.replace(/[\\/]+$/, '') + '\\' + rel.replace(/\//g, '\\')
}

async function loadChildren(dir: string, force = false): Promise<void> {
  if (!props.session_id) return
  if (!force && cache.value.has(dir)) return
  loadingDirs.value.add(dir)
  try {
    const r = await apiGet<WorkspaceListRESP>(
      `/api/v1/chat/workspace/${props.session_id}/ls?path=${encodeURIComponent(dir)}`
    )
    cache.value.set(dir, r.items ?? [])
    if (r.root) root.value = r.root
    if (r.truncated) truncated.value.add(dir)
    else truncated.value.delete(dir)
  } catch {
    // 目录不可读（权限 / 已删除）：按空目录处理，不阻断整棵树
    cache.value.set(dir, [])
  } finally {
    loadingDirs.value.delete(dir)
  }
}

/** 某条目的父目录（相对路径，根为 ''）。 */
function parentDir(rel: string): string {
  const clean = rel.replace(/\/$/, '')
  const parts = clean.split('/')
  parts.pop()
  return parts.join('/')
}

/** 操作后局部刷新父目录；若是目录被删，顺带收起。 */
async function refreshAfter(rel: string, wasDir = false): Promise<void> {
  if (wasDir) expanded.value.delete(rel)
  await loadChildren(parentDir(rel), true)
}

async function toggle(e: WorkspaceEntry): Promise<void> {
  if (!e.is_dir) return
  const dir = e.path.replace(/\/$/, '')
  if (expanded.value.has(e.path)) {
    expanded.value.delete(e.path)
    return
  }
  expanded.value.add(e.path)
  await loadChildren(dir)
}

function refresh(): void {
  cache.value.clear()
  truncated.value.clear()
  void loadChildren('')
}

function fmtSize(size: number): string {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

// ===== 右键上下文菜单 =====
const ctx = ref<{ x: number; y: number; e: WorkspaceEntry } | null>(null)

function openCtxMenu(e: MouseEvent, entry: WorkspaceEntry): void {
  ctx.value = { x: e.clientX, y: e.clientY, e: entry }
}
function closeCtxMenu(): void {
  ctx.value = null
}
/** 菜单显示位置防出屏（菜单约 180×220）。 */
const ctxStyle = computed(() => {
  if (!ctx.value) return {}
  const x = Math.min(ctx.value.x, window.innerWidth - 196)
  const y = Math.min(ctx.value.y, window.innerHeight - 240)
  return { left: `${x}px`, top: `${y}px` }
})

function ctxAddToChat(): void {
  if (!ctx.value) return
  emit('attach', absPath(ctx.value.e.path))
  closeCtxMenu()
}

async function ctxCopyPath(): Promise<void> {
  if (!ctx.value) return
  const p = absPath(ctx.value.e.path)
  closeCtxMenu()
  try {
    await navigator.clipboard.writeText(p)
    toast.success(t('chat.file.pathCopied'), p)
  } catch {
    toast.error(t('chat.file.opFailed'), p)
  }
}

async function ctxRename(): Promise<void> {
  if (!ctx.value) return
  const entry = ctx.value.e
  closeCtxMenu()
  const name = await dialog.prompt({
    title: t('chat.file.renameTitle'),
    content: t('chat.file.renamePrompt'),
    defaultValue: entry.name
  })
  if (!name || !name.trim() || name.trim() === entry.name) return
  try {
    await apiPost(`/api/v1/chat/workspace/${props.session_id}/rename`, {
      path: entry.path,
      new_name: name.trim()
    })
    await refreshAfter(entry.path, entry.is_dir)
    toast.success(t('chat.file.renamed'))
  } catch (err) {
    toast.error(t('chat.file.opFailed'), err instanceof Error ? err.message : String(err))
  }
}

async function ctxDuplicate(): Promise<void> {
  if (!ctx.value) return
  const entry = ctx.value.e
  closeCtxMenu()
  try {
    const r = await apiPost<{ path: string }>(`/api/v1/chat/workspace/${props.session_id}/copy`, {
      path: entry.path
    })
    await refreshAfter(r.path)
    toast.success(t('chat.file.dupDone', r.path.split('/').pop() ?? r.path))
  } catch (err) {
    toast.error(t('chat.file.opFailed'), err instanceof Error ? err.message : String(err))
  }
}

async function ctxDelete(): Promise<void> {
  if (!ctx.value) return
  const entry = ctx.value.e
  closeCtxMenu()
  const ok = await dialog.confirm({
    title: t('chat.file.deleteTitle'),
    content: t('chat.file.deleteConfirm', entry.name),
    danger: true
  })
  if (!ok) return
  try {
    await apiPost(`/api/v1/chat/workspace/${props.session_id}/remove`, { path: entry.path })
    await refreshAfter(entry.path, entry.is_dir)
    toast.success(t('chat.file.deleted'))
  } catch (err) {
    toast.error(t('chat.file.opFailed'), err instanceof Error ? err.message : String(err))
  }
}

function onGlobalClick(): void {
  closeCtxMenu()
}
function onGlobalKey(e: KeyboardEvent): void {
  if (e.key === 'Escape') closeCtxMenu()
}
onMounted(() => {
  window.addEventListener('click', onGlobalClick)
  window.addEventListener('keydown', onGlobalKey)
})
onBeforeUnmount(() => {
  window.removeEventListener('click', onGlobalClick)
  window.removeEventListener('keydown', onGlobalKey)
})

interface Row {
  e: WorkspaceEntry
  depth: number
}

/** 展开的可见行（扁平化渲染，避免递归组件的 props 透传）。 */
const rows = computed<Row[]>(() => {
  const out: Row[] = []
  const walk = (dir: string, depth: number): void => {
    for (const e of cache.value.get(dir) ?? []) {
      out.push({ e, depth })
      if (e.is_dir && expanded.value.has(e.path)) walk(e.path.replace(/\/$/, ''), depth + 1)
    }
  }
  walk('', 1)
  return out
})

/** 筛选态：在已加载范围内扁平匹配（不触网，所见即所得）。 */
const matches = computed<Row[]>(() => {
  const q = filter.value.trim().toLowerCase()
  if (!q) return []
  const out: Row[] = []
  for (const [dir, kids] of cache.value) {
    const depth = dir ? dir.split('/').length + 1 : 1
    for (const e of kids) {
      if (e.name.toLowerCase().includes(q)) out.push({ e, depth })
    }
  }
  return out.slice(0, 200)
})

watch(
  () => props.session_id,
  (id) => {
    cache.value.clear()
    expanded.value.clear()
    truncated.value.clear()
    filter.value = ''
    if (id) void loadChildren('')
  }
)

onMounted(() => {
  if (props.session_id) void loadChildren('')
})

defineExpose({ refresh })
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <!-- 工具行：筛选 + 刷新 -->
    <div class="flex shrink-0 items-center gap-1.5 px-2 py-1.5">
      <input
        v-model="filter"
        class="input input-xs"
        :placeholder="t('chat.filterFiles')"
      />
      <button
        type="button"
        class="btn-icon shrink-0"
        :title="t('chat.refresh')"
        @click="refresh"
      >
        <RefreshCw class="h-3.5 w-3.5" />
      </button>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto px-1 pb-2">
      <!-- 筛选态：扁平列出已加载范围内的命中 -->
      <template v-if="filter.trim()">
        <p v-if="matches.length === 0" class="px-2 py-3 text-[11px] text-wb-muted">
          {{ t('chat.filterNoMatch') }}
        </p>
        <ul v-else class="space-y-0.5">
          <li
            v-for="m in matches"
            :key="m.e.path"
            class="flex cursor-default items-center gap-2 rounded-lg px-2 py-1.5 text-xs text-wb-ink transition-colors hover:bg-wb-primary/5"
            :style="{ paddingLeft: `${m.depth * 10 + 8}px` }"
            @contextmenu.prevent="openCtxMenu($event, m.e)"
          >
            <component :is="m.e.kind === 'image' ? Image : FileText" class="h-3.5 w-3.5 shrink-0 text-wb-muted" />
            <span class="min-w-0 flex-1 truncate" :title="m.e.path">{{ m.e.name }}</span>
          </li>
        </ul>
      </template>

      <!-- 树形态 -->
      <template v-else>
        <div v-if="rootLoading" class="space-y-2 px-2 py-3">
          <div v-for="i in 4" :key="i" class="h-3.5 animate-pulse rounded bg-wb-surface-hover" />
        </div>
        <el-empty
          v-else-if="rootEntries.length === 0"
          :description="t('chat.dirEmpty')"
          :image-size="48"
        />
        <ul v-else class="space-y-0.5">
          <li
            v-for="r in rows"
            :key="r.e.path"
            class="group flex cursor-default items-center gap-1.5 rounded-lg py-1.5 pr-2 text-xs transition-colors hover:bg-wb-primary/5"
            :class="r.e.is_dir ? 'font-medium text-wb-ink' : 'text-wb-ink'"
            :style="{ paddingLeft: `${r.depth * 12 + 6}px` }"
            :title="t('chat.ctxMenuHint')"
            @click="toggle(r.e)"
            @contextmenu.prevent="openCtxMenu($event, r.e)"
          >
            <FolderOpen
              class="h-3.5 w-3.5 shrink-0"
              :class="expanded.has(r.e.path) ? 'text-wb-primary' : 'text-wb-muted'"
            />
            <span class="min-w-0 flex-1 truncate">{{ r.e.name }}</span>
            <span v-if="!r.e.is_dir" class="shrink-0 text-[10px] text-wb-muted">{{ fmtSize(r.e.size) }}</span>
          </li>
        </ul>
      </template>
    </div>

    <p
      v-if="[...truncated].length > 0 && !filter.trim()"
      class="shrink-0 border-t border-wb-border px-3 py-1.5 text-[10px] text-wb-warning"
    >
      {{ t('chat.dirTruncated') }}
    </p>

    <!-- 右键操作菜单（点击任意处 / Esc 关闭） -->
    <Teleport to="body">
      <div
        v-if="ctx"
        class="wb-ctx-menu"
        :style="ctxStyle"
        @contextmenu.prevent
        @click.stop
      >
        <button type="button" class="wb-ctx-item" @click="ctxAddToChat">
          {{ t('chat.addToChat') }}
        </button>
        <button type="button" class="wb-ctx-item" @click="ctxCopyPath">
          {{ t('chat.file.copyPath') }}
        </button>
        <div class="wb-ctx-sep" />
        <button type="button" class="wb-ctx-item" @click="ctxRename">
          {{ t('chat.file.rename') }}
        </button>
        <button
          v-if="!ctx.e.is_dir"
          type="button"
          class="wb-ctx-item"
          @click="ctxDuplicate"
        >
          {{ t('chat.file.duplicate') }}
        </button>
        <button type="button" class="wb-ctx-item wb-ctx-item--danger" @click="ctxDelete">
          {{ t('chat.file.delete') }}
        </button>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
/* 右键菜单：固定定位浮层，轻投影 + 细边框（Teleport 到 body，不走 scoped 命中，用全局类） */
.wb-ctx-menu {
  position: fixed;
  z-index: 3000;
  min-width: 176px;
  padding: 4px;
  border: 1px solid var(--wb-border);
  border-radius: 10px;
  background: var(--wb-surface);
  box-shadow: 0 8px 28px rgb(0 0 0 / 14%);
}
</style>

<style>
/* Teleport 到 body 后 scoped 样式不生效，菜单子项样式放全局（wb-ctx- 前缀防冲突） */
.wb-ctx-item {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--wb-ink);
  font-size: 12.5px;
  text-align: left;
  cursor: pointer;
}
.wb-ctx-item:hover {
  background: var(--wb-surface-hover);
}
.wb-ctx-item--danger {
  color: var(--wb-danger);
}
.wb-ctx-item--danger:hover {
  background: color-mix(in srgb, var(--wb-danger) 10%, transparent);
}
.wb-ctx-sep {
  height: 1px;
  margin: 4px 6px;
  background: var(--wb-border);
}
</style>
