<script setup lang="ts">
// 会话列表：对话页的窄栏与会话页的主栏共用。
// 数据来自磁盘上的会话文件，终端里聊过的这里也看得到。
// 按时间分组（今天 / 7 天 / 更早）：找会话几乎总是「刚才那个」，分组一次命中。
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import AppIcon from '@/components/common/AppIcon.vue'
import { useAppStore } from '@/stores/app'
import { useChatStore } from '@/stores/chat'
import { useToasts } from '@/composables/useToasts'
import { api } from '@/api/http'
import type { SessionRow } from '@/types/api'

const props = withDefaults(
  defineProps<{
    active?: string
    compact?: boolean
    workspaceOnly?: boolean
    /** 对话页窄栏才显示「新对话」：会话页是账本视图，开新对话要去对话页 */
    showNew?: boolean
  }>(),
  {
    active: '',
    compact: false,
    workspaceOnly: false,
    showNew: false,
  },
)

const emit = defineEmits<{ open: [path: string]; changed: [path?: string]; newChat: [] }>()

const app = useAppStore()
const chat = useChatStore()
const toasts = useToasts()
const { t } = useI18n()

// 会话属于某个引擎（由文件所在目录决定），列表跟着当前引擎走。

const filter = ref('')
const rows = ref<SessionRow[]>([])
const loading = ref(false)
// 服务端全文搜索：正文在会话文件里，得让后端扫；标题/模型/目录过滤留在客户端即时做。
// 两个字起步才打后端：单字命中太多，扫全部文件得不偿失。
const serverQuery = ref('')
let searchTimer = 0

/** 引擎范围：列表默认只显示当前引擎的会话（两个引擎的会话不能互换接着聊）。 */
const scope = computed(() => (props.workspaceOnly ? app.workspace : ''))

const filtered = computed(() => {
  // 正文搜索走后端：返回的行已按「标题或正文」命中，客户端再过一遍会把正文命中的挤掉。
  if (serverQuery.value) return rows.value
  const query = filter.value.trim().toLowerCase()
  if (!query) return rows.value
  return rows.value.filter((row) =>
    [row.session_title, row.session_model, row.session_cwd].some((field) => String(field ?? '').toLowerCase().includes(query)),
  )
})

watch(filter, (value) => {
  window.clearTimeout(searchTimer)
  const query = value.trim()
  if (query.length < 2) {
    const had = serverQuery.value !== ''
    serverQuery.value = ''
    if (had) void load()
    return
  }
  searchTimer = window.setTimeout(() => {
    serverQuery.value = query
    void load()
  }, 400)
})

interface Group {
  key: string
  label: string
  rows: SessionRow[]
}

/** 按更新时间分三组。列表本身已按时间倒序，分组只做切分不再排序。 */
const groups = computed<Group[]>(() => {
  const dayStart = new Date()
  dayStart.setHours(0, 0, 0, 0)
  const todayStart = dayStart.getTime()
  const weekStart = todayStart - 6 * 24 * 60 * 60_000
  const today: SessionRow[] = []
  const week: SessionRow[] = []
  const older: SessionRow[] = []
  for (const row of filtered.value) {
    const ts = row.updated_at
    if (!Number.isNaN(ts) && ts >= todayStart) today.push(row)
    else if (!Number.isNaN(ts) && ts >= weekStart) week.push(row)
    else older.push(row)
  }
  const out: Group[] = []
  if (today.length) out.push({ key: 'today', label: t('sessions.groupToday'), rows: today })
  if (week.length) out.push({ key: 'week', label: t('sessions.groupWeek'), rows: week })
  if (older.length) out.push({ key: 'older', label: t('sessions.groupEarlier'), rows: older })
  return out
})

async function load() {
  loading.value = true
  try {
    const data = await api.sessions(scope.value || undefined, serverQuery.value || undefined)
    rows.value = data.sessions ?? []
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

async function remove(row: SessionRow) {
  try {
    await ElMessageBox.confirm(t('sessions.deleteConfirm', { title: row.session_title }), t('sessions.deleteTitle'), {
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    await api.deleteSession(row.session_path)
    toasts.push('ok', t('sessions.deleted'))
    await load()
    // 带上被删的路径：对话页据此判断「删的是不是正看着的这个」。
    emit('changed', row.session_path)
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

async function rename(row: SessionRow) {
  try {
    const result = await ElMessageBox.prompt(t('sessions.renamePlaceholder'), t('sessions.renameTitle'), {
      inputValue: row.session_title,
      confirmButtonText: t('common.save'),
      cancelButtonText: t('common.cancel'),
    })
    await api.renameSession(row.session_path, result.value)
    toasts.push('ok', t('common.saved'))
    await load()
    emit('changed', row.session_path)
  } catch {
    /* 取消或失败：取消不该弹错 */
  }
}

function relative(ms: number): string {
  if (!ms) return ''
  const time = ms
  if (Number.isNaN(time)) return ''
  const diff = Date.now() - time
  const minute = 60_000
  if (diff < minute) return t('history.justNow')
  if (diff < 60 * minute) return t('history.minutesAgo', { n: Math.floor(diff / minute) })
  if (diff < 24 * 60 * minute) return t('history.hoursAgo', { n: Math.floor(diff / (60 * minute)) })
  return t('history.daysAgo', { n: Math.floor(diff / (24 * 60 * minute)) })
}

watch(
  () => app.workspace,
  () => {
    rows.value = []
    void load()
  },
)

onMounted(load)
onUnmounted(() => window.clearTimeout(searchTimer))
defineExpose({ load })
</script>

<template>
  <div class="session-list" :class="{ compact }">
    <!-- 新对话在这里而不是输入框底栏：它是会话级的动作，属于会话列表 -->
    <div v-if="props.showNew" class="list-new">
      <button
        class="wb-btn primary sm new-btn"
        :class="{ 'is-loading': chat.switching === 'new' }"
        :disabled="chat.switching === 'new'"
        @click="emit('newChat')"
      >
        <AppIcon name="plus" :size="14" />
        {{ t('chat.newChat') }}
      </button>
    </div>

    <div class="list-head">
      <input v-model="filter" class="wb-input sm" :placeholder="t('sessions.filterPlaceholder')" />
      <button class="wb-btn icon ghost" :title="t('common.refresh')" :class="{ 'is-loading': loading }" @click="load">
        <AppIcon name="refresh" :size="14" />
      </button>
    </div>

    <div class="list-body">
      <div v-if="!filtered.length && !loading" class="list-empty">
        <p class="empty-title">{{ t('sessions.emptyTitle') }}</p>
        <p class="empty-hint">{{ t('sessions.emptyHint') }}</p>
      </div>

      <template v-for="group in groups" :key="group.key">
        <div class="group-head">{{ group.label }}</div>
        <div
          v-for="row in group.rows"
          :key="row.session_path"
          class="session-row wb-row-click"
          :class="{ 'is-active': row.session_path === active }"
          @click="emit('open', row.session_path)"
        >
          <div class="row-top">
            <span class="row-title">{{ row.session_title }}</span>
            <span class="row-time">{{ relative(row.updated_at) }}</span>
          </div>
          <div class="row-meta">
            <span v-if="row.session_model" class="row-model">{{ row.session_model }}</span>
            <span class="row-dot">·</span>
            <span>{{ row.message_count }} {{ t('sessions.messages') }}</span>
            <span v-if="row.cost_usd" class="row-dot">·</span>
            <span v-if="row.cost_usd">${{ row.cost_usd.toFixed(3) }}</span>
          </div>
          <div v-if="!compact" class="row-path">{{ row.session_cwd }}</div>
          <div class="row-actions">
            <!-- 窄栏用图标：文字按钮会把一行的宽度吃光；会话页主栏仍用文字 -->
            <button class="wb-btn text sm" :title="t('sessions.rename')" @click.stop="rename(row)">
              <AppIcon v-if="compact" name="edit" :size="13" />
              <template v-else>{{ t('sessions.rename') }}</template>
            </button>
            <button class="wb-btn text sm danger" :title="t('common.delete')" @click.stop="remove(row)">
              <AppIcon v-if="compact" name="trash" :size="13" />
              <template v-else>{{ t('common.delete') }}</template>
            </button>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.session-list {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
}

.list-head {
  display: flex;
  gap: var(--wb-sp-1);
  padding: var(--wb-sp-2);
  flex: none;
}

.list-head .wb-input {
  height: var(--wb-ctl-sm);
  font-size: var(--wb-fs-xs);
}

/* 新对话独占一行：它是这一栏最主要的动作，值得一个整行的主色按钮 */
.list-new {
  padding: var(--wb-sp-2) var(--wb-sp-2) 0;
  flex: none;
}

.new-btn {
  width: 100%;
  justify-content: center;
}

/* 分组标题：一行小字，只做扫视的锚点，不抢行的视觉重量 */
.group-head {
  padding: var(--wb-sp-3) var(--wb-sp-3) var(--wb-sp-1);
  font-size: var(--wb-fs-3xs);
  font-weight: 700;
  letter-spacing: var(--wb-ls-plate);
  text-transform: uppercase;
  color: var(--wb-faint);
  user-select: none;
}

.list-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 var(--wb-sp-2) var(--wb-sp-3);
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.session-row {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  cursor: pointer;
  position: relative;
}

.row-top {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-2);
}

.row-title {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-sm);
  font-weight: 550;
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-time {
  flex: none;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.row-meta {
  display: flex;
  align-items: center;
  gap: 4px;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
  overflow: hidden;
  white-space: nowrap;
}

.row-model {
  max-width: 150px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.row-dot {
  color: var(--wb-faint);
}

.row-path {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-actions {
  display: none;
  gap: var(--wb-sp-1);
}

.session-row:hover .row-actions {
  display: flex;
}

/* 窄栏只藏路径：改名与删除收成两个小图标，悬停才出现——
   管理动作在对话页同样要够得着，藏掉它们等于只能去会话页删会话。 */
.compact .row-path {
  display: none !important;
}

.list-empty {
  padding: var(--wb-sp-6) var(--wb-sp-3);
  text-align: center;
}

.empty-title {
  margin: 0 0 4px;
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
}

.empty-hint {
  margin: 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
  line-height: 1.6;
}
</style>
