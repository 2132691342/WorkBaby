<script setup lang="ts">
// 会话列表：对话页的窄栏与会话页的主栏共用。
// 数据来自磁盘上的会话文件，终端里聊过的这里也看得到。
// 按时间分组（今天 / 7 天 / 更早）：找会话几乎总是「刚才那个」，分组一次命中。
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { confirmDanger } from '@/utils/confirm'
import { ElMessageBox } from 'element-plus'
import AppIcon from '@/components/common/AppIcon.vue'
import SessionCardRow from '@/components/sessions/SessionCardRow.vue'
import { useAppStore } from '@/stores/app'
import { useChatStore } from '@/stores/chat'
import { useRefreshStore } from '@/stores/refresh'
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
const refresh = useRefreshStore()
const toasts = useToasts()
const { t } = useI18n()

// 会话属于某个引擎（由文件所在目录决定），列表跟着当前引擎走。

const filter = ref('')
const rows = ref<SessionRow[]>([])
const loading = ref(false)
// 首屏取数是否失败：失败时不能显示「还没有会话」——那是一句假断言。
const loadFailed = ref(false)
// 正在删的那一条：删除要转圈，别让用户重复点。
const removing = ref('')
// 正在改名的那一条：落盘那一下要转圈，行内按钮同时锁住防连点。
const renaming = ref('')
// 服务端全文搜索：正文在会话文件里，得让后端扫；标题/模型/目录过滤留在客户端即时做。
// 两个字起步才打后端：单字命中太多，扫全部文件得不偿失。
const serverQuery = ref('')
let searchTimer = 0

/** 列表默认只显示当前工作区的会话；开了「全部目录」才看别的。 */
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
// 客户端分页：一次全渲染上千行会拖慢滚动与挂载。先出前两页，
// 底部一个「显示更多」逐页放开——搜索命中走后端，分页只是渲染闸门。
const PAGE_SIZE = 60
const shownCount = ref(PAGE_SIZE * 2)

watch([filtered, serverQuery, scope], () => {
  shownCount.value = PAGE_SIZE * 2
})

const limited = computed(() => filtered.value.slice(0, shownCount.value))
const hiddenCount = computed(() => Math.max(0, filtered.value.length - limited.value.length))

function showMore(): void {
  shownCount.value += PAGE_SIZE * 2
}

const groups = computed<Group[]>(() => {
  const dayStart = new Date()
  dayStart.setHours(0, 0, 0, 0)
  const todayStart = dayStart.getTime()
  const weekStart = todayStart - 6 * 24 * 60 * 60_000
  const today: SessionRow[] = []
  const week: SessionRow[] = []
  const older: SessionRow[] = []
  for (const row of limited.value) {
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

// 连点搜索/切范围会并发发起请求：旧响应晚到会把新结果覆盖回旧的。
// 用序号判新旧，旧响应直接丢掉（与 @ 菜单同一套守卫）。
let loadSeq = 0

async function load() {
  const mine = ++loadSeq
  loading.value = true
  try {
    const data = await api.sessions(scope.value || undefined, serverQuery.value || undefined)
    if (mine !== loadSeq) return
    rows.value = data.sessions ?? []
    loadFailed.value = false
  } catch (err) {
    if (mine !== loadSeq) return
    loadFailed.value = true
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    if (mine === loadSeq) loading.value = false
  }
}

async function remove(row: SessionRow) {
  // 确认框里复述会话标题：列表里一行行长得像，不带标题的「确定删除？」等于让人盲点。
  const ok = await confirmDanger(
    t('sessions.deleteConfirm', { title: row.session_title }),
    t('sessions.deleteTitle'),
    t('common.delete'),
    t('common.cancel'),
  )
  if (!ok) return
  removing.value = row.session_path
  try {
    await api.deleteSession(row.session_path)
    toasts.push('ok', t('sessions.deleted'))
    await load()
    // 带上被删的路径：对话页据此判断「删的是不是正看着的这个」。
    emit('changed', row.session_path)
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    removing.value = ''
  }
}

async function rename(row: SessionRow) {
  let value = ''
  try {
    const result = await ElMessageBox.prompt(t('sessions.renamePlaceholder'), t('sessions.renameTitle'), {
      inputValue: row.session_title,
      confirmButtonText: t('common.save'),
      cancelButtonText: t('common.cancel'),
      inputPattern: /\S+/,
      inputErrorMessage: t('sessions.renamePlaceholder'),
    })
    value = result.value
  } catch {
    return // 取消：不该弹错
  }
  // 改名失败必须出声：catch 到外面会把真实的失败也当成「用户取消」吞掉，
  // 表现是「点了改名，列表还是旧名字」。
  renaming.value = row.session_path
  try {
    await api.renameSession(row.session_path, value)
    toasts.push('ok', t('common.saved'))
    await load()
    emit('changed', row.session_path)
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    renaming.value = ''
  }
}

// 相对时间精确到分 / 时 / 天 / 月，超过一个月才落具体日期——「3 个月前」
// 更快回答「多久以前聊的」，要定位那天时悬停里有完整时间。
function relative(ms: number): string {
  if (!ms || Number.isNaN(ms)) return ''
  const diff = Date.now() - ms
  const minute = 60_000
  if (diff < minute) return t('history.justNow')
  if (diff < 60 * minute) return t('history.minutesAgo', { n: Math.floor(diff / minute) })
  if (diff < 24 * 60 * minute) return t('history.hoursAgo', { n: Math.floor(diff / (60 * minute)) })
  const day = 24 * 60 * minute
  if (diff < 30 * day) return t('history.daysAgo', { n: Math.floor(diff / day) })
  if (diff < 365 * day) return t('history.monthsAgo', { n: Math.floor(diff / (30 * day)) })
  return new Date(ms).toLocaleDateString()
}

/** 悬停上的完整时刻：精确定位某一天要靠它。 */
function stamp(ms: number): string {
  if (!ms || Number.isNaN(ms)) return ''
  const date = new Date(ms)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

// 把标题按过滤词切成命中 / 未命中片段：高亮只在标题里做——
// 标题是用户认会话的唯一线索，全文命中但标题不含词时高亮会落空。
function highlight(title: string): { text: string; hit: boolean }[] {
  const query = serverQuery.value.trim()
  if (!query) return [{ text: title, hit: false }]
  const lower = title.toLowerCase()
  const needle = query.toLowerCase()
  const out: { text: string; hit: boolean }[] = []
  let from = 0
  for (;;) {
    const at = lower.indexOf(needle, from)
    if (at < 0) break
    if (at > from) out.push({ text: title.slice(from, at), hit: false })
    out.push({ text: title.slice(at, at + needle.length), hit: true })
    from = at + needle.length
  }
  if (from < title.length) out.push({ text: title.slice(from), hit: false })
  return out.length ? out : [{ text: title, hit: false }]
}

watch(
  () => app.workspace,
  () => {
    rows.value = []
    void load()
  },
)

// 一轮聊完 / 改名 / 删除会 bump 这个信号：窄栏的标题与消息数要跟着更新，
// 不能只靠 mount 时那一次加载——否则发完话侧栏还停在「（空会话）· 0 条」。
watch(
  () => refresh.sessions,
  () => {
    void load()
  },
)

onMounted(load)
// 当前会话要能看见：切到一条排得很靠后的会话时，侧栏高亮停在屏外等于没高亮。
// 只在本组件的滚动容器里滚，不惊动页面其它部分。
const listBody = ref<HTMLElement | null>(null)
watch(
  () => props.active,
  async (active) => {
    if (!active) return
    await nextTick()
    listBody.value?.querySelector('.session-row.is-active')?.scrollIntoView({ block: 'nearest' })
  },
  { immediate: true },
)
onUnmounted(() => window.clearTimeout(searchTimer))
defineExpose({ load })
</script>

<template>
  <div class="session-list" :class="{ compact }">
    <!-- 新对话在这里而不是输入框底栏：它是会话级的动作，属于会话列表 -->
    <div v-if="props.showNew" class="list-new">
      <button type="button"
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
      <button class="wb-btn icon ghost" type="button" :title="t('common.refresh')" :aria-label="t('common.refresh')" :class="{ 'is-loading': loading }" @click="load">
        <AppIcon name="refresh" :size="14" />
      </button>
    </div>

    <div ref="listBody" class="list-body">
      <!-- 取数失败：说清「没取到」并给重试，不说「还没有会话」 -->
      <div v-if="loadFailed && !rows.length && !loading" class="list-empty">
        <AppIcon name="alert" :size="28" class="empty-ic" />
        <p class="empty-title">{{ t('common.loadFailed') }}</p>
        <p class="empty-hint">{{ t('sessions.loadFailedHint') }}</p>
        <button class="wb-btn sm primary list-empty-act" type="button" @click="load">{{ t('common.retry') }}</button>
      </div>

      <div v-else-if="!filtered.length && !loading" class="list-empty">
        <AppIcon name="clock" :size="28" class="empty-ic" />
        <p class="empty-title">{{ serverQuery ? t('sessions.noMatchTitle') : t('sessions.emptyTitle') }}</p>
        <p class="empty-hint">{{ serverQuery ? t('sessions.noMatchHint') : t('sessions.emptyHint') }}</p>
        <!-- 空态给一条出去的路：没会话的人从这里直接开始第一句。
             对话页当场开新会话；会话页没有 newChat，跳去对话页。 -->
        <button
          v-if="!serverQuery"
          class="wb-btn sm primary list-empty-act"
          type="button"
          :disabled="props.showNew && chat.switching === 'new'"
          @click="props.showNew ? emit('newChat') : emit('open', '')"
        >
          <AppIcon name="plus" :size="13" />
          {{ t('chat.newChat') }}
        </button>
      </div>

      <p v-if="loading && !filtered.length" class="list-note">{{ t('common.loading') }}</p>

      <template v-for="group in groups" :key="group.key">
        <div class="group-head">{{ group.label }}</div>
        <!-- 列表增删接统一淡出（.wb-list-*）：删除动作有 undo toast，配淡出才完整 -->
        <TransitionGroup name="wb-list" tag="div" class="group-rows">
          <SessionCardRow
            v-for="row in group.rows"
            :key="row.session_path"
            :row="row"
            :active="active"
            :compact="compact"
            :segments="highlight(row.session_title)"
            :relative="relative(row.updated_at)"
            :stamp="stamp(row.updated_at)"
            :removing="removing === row.session_path"
            :renaming="renaming === row.session_path"
            @open="emit('open', $event)"
            @rename="rename"
            @remove="remove"
          />
        </TransitionGroup>
      </template>

      <!-- 还有没放开的：一行「显示更多」，点了再补两页 -->
      <button type="button" v-if="hiddenCount > 0" class="wb-btn sm show-more" @click="showMore">
        {{ t('sessions.showMore', { n: hiddenCount }) }}
      </button>
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

/* 行本体（.session-row 起）搬进了 SessionCardRow：行卡片与它的样式同住一个文件 */

/* TransitionGroup 的包装元素：沿用列表纵向 2px 行距，视觉与直排时一致 */
.group-rows {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.list-empty {
  padding: var(--wb-sp-6) var(--wb-sp-3);
  text-align: center;
}

.empty-ic {
  color: var(--wb-faint);
  margin-bottom: var(--wb-sp-2);
}

.list-note {
  margin: 0;
  padding: var(--wb-sp-3);
  text-align: center;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
}

.empty-title {
  margin: 0 0 var(--wb-sp-1);
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
}

.list-empty-act {
  margin-top: var(--wb-sp-3);
}

/* 显示更多：一条居中的窄按钮，不与列表行抢视觉 */
.show-more {
  align-self: center;
  margin: var(--wb-sp-2) 0 var(--wb-sp-1);
}

.empty-hint {
  margin: 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
  line-height: 1.6;
}
</style>
