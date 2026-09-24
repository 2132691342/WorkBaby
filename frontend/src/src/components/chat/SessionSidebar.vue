<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Archive, Check, Delete, GitBranch, Layers, Pin, Search, X } from '@/components/common/icons'
import EmptyState from '@/components/common/EmptyState.vue'
import type { Session } from '@/types/api'
import { t } from '@/i18n'
import { useDialog } from '@/composables/useDialog'
import { useChatStore } from '@/stores/chat'
import { formatRelativeTime } from '@/utils/time'

/**
 * 任务列表（左栏主体）：搜索 + 分组/平铺 + 多选批量删除 + 置顶/归档。
 *
 * 树形血缘：parent_id 相同的会话挂在同一个父节点下；根会话按时段分组。
 * 归档视图：默认隐藏 archived 会话，切到归档视图只看它们。
 */
const props = defineProps<{
  sessions: Session[]
  currentID: string | null
  loading: boolean
  creating?: boolean
  /** 平铺模式：不按时段分组，单列直排。 */
  flat?: boolean
}>()

const emit = defineEmits<{
  select: [id: string]
  create: []
  remove: [id: string]
  'delete-batch': [ids: string[]]
  'toggle-flat': []
  pin: [id: string, pinned: boolean]
  archive: [id: string, archived: boolean]
}>()

const dialog = useDialog()
const chat = useChatStore()
const multiSelect = ref(false)
const selectedIds = ref<Set<string>>(new Set())
const search = ref('')
/** 归档视图：true = 只看已归档会话。 */
const archivedView = ref(false)

/** 按视图过滤：普通视图隐藏 archived；归档视图只看 archived。 */
const visibleSessions = computed<Session[]>(() =>
  props.sessions.filter((s) => (archivedView.value ? s.status === 'archived' : s.status !== 'archived'))
)

// 后端内容搜索：输入去抖后跨会话检索（标题 + 正文），
// 命中的会话合并进列表——本地标题过滤抓不到「内容里提过」的会话。
let searchTimer: number | undefined
watch(search, (q) => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => void chat.searchSessions(q), 250)
})

/** 后端内容搜索命中且尚未在加载列表里的会话（搜索态下并入展示）。 */
const remoteHits = computed<Session[]>(() => {
  if (!search.value.trim()) return []
  const known = new Set(props.sessions.map((s) => s.id))
  return chat.searchResults.filter((s) => !known.has(s.id))
})

function toggleSelect(id: string): void {
  const next = new Set(selectedIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedIds.value = next
}

function enterMultiSelect(): void {
  multiSelect.value = true
  selectedIds.value = new Set()
}

function exitMultiSelect(): void {
  multiSelect.value = false
  selectedIds.value = new Set()
}

async function confirmDeleteBatch(): Promise<void> {
  if (selectedIds.value.size === 0) return
  const ok = await dialog.confirm({
    title: t('chat.deleteBatchTitle'),
    content: t('chat.deleteBatchConfirm', selectedIds.value.size),
    danger: true
  })
  if (!ok) return
  emit('delete-batch', Array.from(selectedIds.value))
  exitMultiSelect()
}

/** 单个渲染节点（根或分支），扁平化以便分组遍历。 */
interface SessionNode {
  session: Session
  depth: number
  isRoot: boolean
}

interface Group {
  label: string
  count: number
  items: SessionNode[]
}

const searchLower = computed(() => search.value.trim().toLowerCase())

/**
 * 构建树状节点序列：
 *   - 根会话按时段分组（今天 / 昨天 / 本周 / 更早）；
 *   - 每个根会话下挂其直接子分支（按 last_message_at 倒序）；
 *   - 多层分叉暂不展开（分叉可递归，但平铺更易读）。
 */
function buildTree(): { roots: Session[]; children: Map<string, Session[]> } {
  const roots: Session[] = []
  const children = new Map<string, Session[]>()
  for (const s of visibleSessions.value) {
    if (!s.parent_id) {
      roots.push(s)
    } else {
      const arr = children.get(s.parent_id) ?? []
      arr.push(s)
      children.set(s.parent_id, arr)
    }
  }
  // 后端内容搜索命中、未在加载列表里的会话并入根列表
  roots.push(...remoteHits.value)
  // 置顶优先（后端同序），其余按时段倒序
  roots.sort((a, b) => Number(b.pinned ?? false) - Number(a.pinned ?? false) || (b.last_message_at ?? 0) - (a.last_message_at ?? 0))
  // 子分支按时段倒序
  for (const arr of children.values()) {
    arr.sort((a, b) => (b.last_message_at ?? 0) - (a.last_message_at ?? 0))
  }
  return { roots, children }
}

const tree = computed(() => buildTree())

/** 根会话的分支数。 */
function branchCount(id: string): number {
  return (tree.value.children.get(id) ?? []).length
}

/** 单个节点是否命中搜索（标题或子分支标题任一命中即视为命中）。 */
function nodeMatches(node: SessionNode): boolean {
  if (!searchLower.value) return true
  if ((node.session.name ?? '').toLowerCase().includes(searchLower.value)) return true
  // 子分支标题命中也要把根带出来
  const kids = tree.value.children.get(node.session.id) ?? []
  return kids.some((c) => (c.name ?? '').toLowerCase().includes(searchLower.value))
}

const grouped = computed<Group[]>(() => {
  const { roots, children } = tree.value
  const filteredRoots = searchLower.value
    ? roots.filter((r) => nodeMatches({ session: r, depth: 0, isRoot: true }))
    : roots
  if (filteredRoots.length === 0) return []

  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const yesterday = today - 86400000
  const weekAgo = today - 7 * 86400000

  const todayList: SessionNode[] = []
  const yesterdayList: SessionNode[] = []
  const weekList: SessionNode[] = []
  const olderList: SessionNode[] = []
  const bucket = (ts: number): SessionNode[] => {
    if (ts >= today) return todayList
    if (ts >= yesterday) return yesterdayList
    if (ts >= weekAgo) return weekList
    return olderList
  }

  for (const root of filteredRoots) {
    const rootNode: SessionNode = { session: root, depth: 0, isRoot: true }
    bucket(sessionTime(root)).push(rootNode)
    const kids = children.get(root.id) ?? []
    for (const child of kids) {
      bucket(sessionTime(child)).push({
        session: child,
        depth: 1,
        isRoot: false
      })
    }
  }

  // 平铺模式：单组无标题，按更新时间倒序直排
  if (props.flat) {
    const flat = [...todayList, ...yesterdayList, ...weekList, ...olderList].sort(
      (a, b) => (b.session.last_message_at ?? 0) - (a.session.last_message_at ?? 0)
    )
    return flat.length ? [{ label: '', count: flat.length, items: flat }] : []
  }

  const groups: Group[] = []
  const push = (label: string, list: SessionNode[]) => {
    if (list.length) groups.push({ label: t(label), count: list.length, items: list })
  }
  push('chat.today', todayList)
  push('chat.yesterday', yesterdayList)
  push('chat.week', weekList)
  push('chat.earlier', olderList)
  return groups
})

/** 会话时间统一走 utils/time（含「昨天」分支，与全站口径一致）。 */
const fmtTime = formatRelativeTime

/**
 * 会话「最近活跃时间」：优先 last_message_at，尚未发过消息（后端给 0）时回落到 created_at。
 * 直接用 last_message_at 会让空会话显示成 1970-01-01，并被错误归入「较早」分组。
 */
function sessionTime(s: Session): number {
  return s.last_message_at || s.created_at
}

// 命中搜索时自动清空多选（避免搜索态的 checkbox 残留）
watch(searchLower, () => {
  if (searchLower.value) exitMultiSelect()
})
</script>

<template>
  <aside class="flex h-full w-full flex-col">
    <!-- 列表头：任务铭牌 + 归档视图 / 分组平铺 / 批量管理 -->
    <div class="sess-head">
      <div class="t-plate">{{ t('chat.tasks') }}</div>
      <button
        type="button"
        class="icon-btn"
        :class="{ 'is-on': archivedView }"
        :title="archivedView ? t('chat.backToActive') : t('chat.archivedView')"
        @click="archivedView = !archivedView"
      >
        <Archive class="ic-sm" />
      </button>
      <button
        type="button"
        class="icon-btn"
        :class="{ 'is-on': flat }"
        :title="flat ? t('chat.viewGrouped') : t('chat.viewFlat')"
        @click="emit('toggle-flat')"
      >
        <Layers class="ic-sm" />
      </button>
      <button
        v-if="!multiSelect && sessions.length > 1"
        type="button"
        class="icon-btn"
        :title="t('chat.enterMultiSelect')"
        @click="enterMultiSelect"
      >
        <Delete class="ic-sm" />
      </button>
    </div>

    <div class="flex-none space-y-2 px-3 pb-2">
      <!-- 多选模式：取消 + 批量删除 -->
      <div v-if="multiSelect" class="flex items-center gap-1.5">
        <button type="button" class="btn btn-sm flex-1" @click="exitMultiSelect">
          <X class="ic-xs" />
          {{ t('chat.cancelMultiSelect') }}
        </button>
        <button
          type="button"
          class="btn btn-danger btn-sm flex-1"
          :disabled="selectedIds.size === 0"
          @click="confirmDeleteBatch"
        >
          <Delete class="ic-xs" />
          {{ t('chat.deleteBatch', selectedIds.size) }}
        </button>
      </div>

      <div class="search">
        <Search class="ic ic-sm" />
        <input
          v-model="search"
          type="text"
          class="input"
          :placeholder="t('chat.searchSession')"
          autocomplete="off"
        />
      </div>
    </div>

    <div class="sess-scroll">
      <div v-if="loading" class="px-3 py-3 fs12 muted">{{ t('ui.status.loading') }}</div>
      <!-- 空态复用 EmptyState：窄栏里不画插画（120/64px 插画会盖住文案），只留结论 + 下一步 -->
      <EmptyState
        v-else-if="sessions.length === 0"
        size="sm"
        :illustration="false"
        :title="t('chat.noSessionsTitle')"
        :subtitle="t('chat.noSessionsHint')"
      />
      <EmptyState
        v-else-if="grouped.length === 0"
        size="sm"
        :illustration="false"
        :title="t('chat.noMatch')"
        :subtitle="t('chat.noMatchHint')"
      />
      <template v-else>
        <div v-for="g in grouped" :key="g.label || 'flat'" class="mb-1">
          <div v-if="g.label" class="sess-group">
            {{ g.label }}
            <span class="n">{{ g.count }}</span>
          </div>
          <ul class="space-y-0.5">
            <li
              v-for="node in g.items"
              :key="node.session.id"
              class="sess-item"
              :class="[
                node.session.id === currentID && !multiSelect ? 'is-on' : '',
                node.isRoot ? '' : 'tree-indent'
              ]"
              :style="node.isRoot ? undefined : { paddingLeft: '26px' }"
              @click="multiSelect ? toggleSelect(node.session.id) : emit('select', node.session.id)"
            >
              <div class="si-name">
                <button
                  v-if="multiSelect"
                  type="button"
                  class="icon-btn h-4 w-4 p-0"
                  :class="selectedIds.has(node.session.id) ? 'is-on' : ''"
                  @click.stop="toggleSelect(node.session.id)"
                >
                  <Check v-if="selectedIds.has(node.session.id)" class="ic-xs" />
                  <span v-else class="block h-2.5 w-2.5 rounded-full border border-wb-line-2" />
                </button>
                <span
                  v-if="node.session.pinned && !multiSelect"
                  class="text-wb-primary"
                  :title="t('chat.pinned')"
                >
                  <Pin class="ic-xs" />
                </span>
                <span v-else-if="!node.isRoot" class="shrink-0 text-wb-muted" :title="t('chat.branch')">
                  <GitBranch class="ic-xs" />
                </span>
                <span class="min-w-0 flex-1 truncate">{{ node.session.name || t('chat.unnamed') }}</span>
              </div>
              <div class="si-meta">
                <span>{{ fmtTime(sessionTime(node.session)) }}</span>
                <span
                  v-if="node.isRoot && branchCount(node.session.id) > 0"
                  class="flex items-center gap-0.5"
                  :title="t('chat.branchCount', branchCount(node.session.id))"
                >
                  <GitBranch style="width: 10px; height: 10px" />
                  {{ branchCount(node.session.id) }}
                </span>
              </div>
              <!-- 悬浮操作：置顶 / 归档 / 删除 -->
              <div v-if="!multiSelect" class="si-act">
                <button
                  type="button"
                  class="icon-btn"
                  :title="node.session.pinned ? t('chat.unpin') : t('chat.pinned')"
                  @click.stop="emit('pin', node.session.id, !node.session.pinned)"
                >
                  <Pin class="ic-xs" />
                </button>
                <button
                  type="button"
                  class="icon-btn"
                  :title="archivedView ? t('chat.unarchive') : t('chat.archive')"
                  @click.stop="emit('archive', node.session.id, !archivedView)"
                >
                  <Archive class="ic-xs" />
                </button>
                <button
                  type="button"
                  class="icon-btn is-danger"
                  :title="t('chat.deleteSession')"
                  @click.stop="emit('remove', node.session.id)"
                >
                  <Delete class="ic-xs" />
                </button>
              </div>
            </li>
          </ul>
        </div>
      </template>
    </div>
  </aside>
</template>
