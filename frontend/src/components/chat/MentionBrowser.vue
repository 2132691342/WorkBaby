<script setup lang="ts">
// @ 引用的文件选择：逐级浏览 + 全工作区按文件名搜索。
// 搜索放后端：前端维护几十万文件的索引不现实，后端限界遍历与工作区页
// 同一份目录，不会「这里搜得到、那里打不开」。键盘导航自己实现。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import type { FileEntry, FileHit, SessionRow } from '@/types/api'
import AppIcon from '@/components/common/AppIcon.vue'

const emit = defineEmits<{
  pick: [relPath: string, name: string]
  pickSession: [sessionPath: string, title: string]
  close: []
}>()

const { t } = useI18n()
const app = useAppStore()

// 最近会话：@ 不只引文件，也能把最近一段对话当上下文。条数可配，默认 5。
const sessions = ref<SessionRow[]>([])
const mentionCount = computed(() => {
  const n = Number(app.settings['ui.mention_sessions'])
  return Number.isFinite(n) && n > 0 ? Math.min(n, 20) : 5
})
async function loadSessions(): Promise<void> {
  try {
    sessions.value = (await api.sessions()).sessions ?? []
  } catch {
    sessions.value = []
  }
}

const dirPath = ref('')
const entries = ref<FileEntry[]>([])
const filter = ref('')
const loading = ref(false)
const failed = ref('')
// 当前高亮的下标。-1 表示还没高亮（列表为空或正在加载）。
const index = ref(-1)

// 搜索模式：过滤框非空时进入，清空时回到目录浏览。
const searching = ref(false)
const hits = ref<FileHit[]>([])
const truncated = ref(false)

/** 列表行：浏览与搜索统一成一种形状，键盘导航就不用分叉。 */
interface Row {
  name: string
  path: string
  isDir: boolean
}

/** 面包屑：根 + 每一级。点任意一级跳过去。 */
const crumbs = computed(() => {
  const parts = dirPath.value ? dirPath.value.split('/').filter(Boolean) : []
  return [
    { name: t('chat.mentionRoot'), path: '' },
    ...parts.map((part, i) => ({ name: part, path: parts.slice(0, i + 1).join('/') })),
  ]
})

/** 浏览模式下按过滤词筛当前目录；目录始终显示——用户是在找路，不是在做精确搜索。 */
const shown = computed<Row[]>(() => {
  if (searching.value) {
    return hits.value.map((h) => ({ name: h.entry_name, path: h.file_path, isDir: false }))
  }
  const q = filter.value.trim().toLowerCase()
  const list = q
    ? entries.value.filter((e) => e.entry_is_dir || e.entry_name.toLowerCase().includes(q))
    : entries.value
  return list.map((e) => ({
    name: e.entry_name,
    path: dirPath.value ? `${dirPath.value}/${e.entry_name}` : e.entry_name,
    isDir: e.entry_is_dir,
  }))
})

// 连点两个目录时两次请求会并发返回：只靠 loading 判断会让列表错位。
// 用序号判新旧，旧响应直接丢掉。
let seq = 0

async function load(path: string): Promise<void> {
  const mine = ++seq
  loading.value = true
  failed.value = ''
  dirPath.value = path
  filter.value = ''
  searching.value = false
  hits.value = []
  index.value = -1
  try {
    const list = await api.files(path)
    if (mine !== seq) return
    entries.value = list.entries ?? []
  } catch (err) {
    if (mine !== seq) return
    entries.value = []
    failed.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (mine === seq) loading.value = false
  }
}

// 搜索防抖：每敲一个字都全工作区扫一遍，后端再快也扛不住。
let searchTimer: ReturnType<typeof setTimeout> | undefined
let searchSeq = 0

watch(filter, (value) => {
  const query = value.trim()
  clearTimeout(searchTimer)
  if (!query) {
    searching.value = false
    hits.value = []
    index.value = -1
    return
  }
  searchTimer = setTimeout(() => void runSearch(query), 250)
})

async function runSearch(query: string): Promise<void> {
  const mine = ++searchSeq
  loading.value = true
  failed.value = ''
  index.value = -1
  try {
    const res = await api.filesSearch(query)
    if (mine !== searchSeq || filter.value.trim() !== query) return
    hits.value = res.hits ?? []
    truncated.value = res.truncated
    searching.value = true
  } catch (err) {
    if (mine !== searchSeq) return
    hits.value = []
    searching.value = true
    failed.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (mine === searchSeq) loading.value = false
  }
}

function open(row: Row): void {
  if (!searching.value && row.isDir) {
    void load(row.path)
    return
  }
  emit('pick', row.path, row.name)
}

function up(): void {
  const idx = dirPath.value.lastIndexOf('/')
  void load(idx > 0 ? dirPath.value.slice(0, idx) : '')
}

// 把第 i 项带进可视区：用容器 scrollTop 自己算，scrollIntoView 会连带滚动所有可滚动祖先。
function ensureVisible(i: number): void {
  const box = listEl.value
  if (!box) return
  const el = box.querySelector<HTMLElement>(`[data-idx="${i}"]`)
  if (!el) return
  const top = el.offsetTop - box.offsetTop
  const bottom = top + el.offsetHeight
  if (top < box.scrollTop) box.scrollTop = top
  else if (bottom > box.scrollTop + box.clientHeight) box.scrollTop = bottom - box.clientHeight
}

const listEl = ref<HTMLElement | null>(null)

/** 上下移动高亮。到底部后回到顶部（循环）。 */
function move(delta: number): boolean {
  const total = shown.value.length
  if (!total) return false
  index.value = (index.value + delta + total) % total
  void Promise.resolve().then(() => ensureVisible(index.value))
  return true
}

/** 选中当前高亮项。 */
function pickCurrent(): boolean {
  const entry = shown.value[index.value]
  if (!entry) return false
  open(entry)
  return true
}

const listElRef = listEl

onMounted(() => void load(''))

// 暴露给父组件的重载入口（@ 菜单再次打开时刷新当前目录）。
defineExpose({ reload: () => load(dirPath.value), move, pickCurrent, listEl: listElRef })

onMounted(() => {
  void loadSessions()
})
</script>

<template>
  <div class="mention">
    <header class="mention-head">
      <button
        class="wb-btn icon ghost"
        :disabled="!dirPath"
        :title="t('chat.mentionUp')"
        @click="up"
      >
        <AppIcon name="arrow-left" :size="13" />
      </button>
      <nav class="crumbs">
        <template v-for="(crumb, i) in crumbs" :key="crumb.path">
          <span v-if="i > 0" class="sep">/</span>
          <button class="crumb" @click="load(crumb.path)">{{ crumb.name }}</button>
        </template>
      </nav>
    </header>

    <input
      v-model="filter"
      class="wb-input mention-filter"
      :placeholder="t('chat.mentionFilter')"
      spellcheck="false"
      @input="index = -1"
    />

    <div ref="listEl" class="mention-body">
      <!-- 最近会话：@ 除了引文件，也能把最近一段对话当上下文；只在非搜索态显示 -->
      <template v-if="!searching && sessions.length">
        <p class="mention-note">{{ t('chat.mentionSessions') }}</p>
        <button
          v-for="row in sessions.slice(0, mentionCount)"
          :key="row.session_id"
          class="mention-row"
          @click="emit('pickSession', row.session_path, row.session_title || t('common.untitled'))"
        >
          <AppIcon name="chat" :size="13" />
          <span class="mention-name">{{ row.session_title || t('common.untitled') }}</span>
        </button>
      </template>
      <p v-if="loading" class="mention-note">{{ t('common.loading') }}</p>
      <p v-else-if="failed" class="mention-note bad">{{ failed }}</p>
      <p v-else-if="!shown.length" class="mention-note">{{ t('chat.mentionEmpty') }}</p>
      <p v-else-if="searching && truncated" class="mention-note">{{ t('chat.mentionTruncated') }}</p>
      <button
        v-for="(row, i) in shown"
        :key="row.path"
        :data-idx="i"
        class="mention-row"
        :class="{ dir: row.isDir, 'is-hi': i === index }"
        :title="searching ? row.path : undefined"
        @mouseenter="index = i"
        @click="open(row)"
      >
        <AppIcon :name="row.isDir ? 'folder' : 'file'" :size="13" />
        <span class="mention-name">{{ row.name }}</span>
        <span v-if="searching && row.path.includes('/')" class="mention-dir">{{ row.path }}</span>
        <span v-else-if="row.isDir" class="mention-go">›</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
/* 填满 .pop 的可用高度：菜单面板自己定了上限，这里只负责把内容铺进去，
   滚动交给内部列表。min-height: 0 是 flex 子项能真正收缩的前提。 */
.mention {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  min-height: 0;
  flex: 1;
  padding: 0 var(--wb-sp-1) var(--wb-sp-1);
}

.mention-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
}

.crumbs {
  display: flex;
  align-items: center;
  gap: 2px;
  min-width: 0;
  overflow-x: auto;
  white-space: nowrap;
  scrollbar-width: none;
}

.crumbs::-webkit-scrollbar {
  display: none;
}

.crumb {
  border: 0;
  background: transparent;
  padding: 1px 4px;
  border-radius: var(--wb-r-xs);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
  cursor: pointer;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

/* 面包屑是小控件：悬停放大、按下缩小，与全站按钮同一组刻度 */
.crumb:hover {
  background: var(--wb-hover);
  color: var(--wb-brand);
  transform: scale(var(--wb-press-hover));
}
.crumb:active {
  transform: scale(var(--wb-press-active));
  transition-duration: 60ms;
}

.sep {
  color: var(--wb-faint);
  font-size: var(--wb-fs-2xs);
}

.mention-filter {
  height: var(--wb-ctl-sm);
  font-size: var(--wb-fs-2xs);
}

.mention-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.mention-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: 4px 6px;
  border: 0;
  border-radius: var(--wb-r-xs);
  background: transparent;
  color: var(--wb-ink);
  font-size: var(--wb-fs-2xs);
  text-align: left;
  cursor: pointer;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

/* 整行可点项：按下只缩不放，缩放刻度复用全站令牌 */
.mention-row:active {
  transform: scale(var(--wb-press-row-active));
  transition-duration: 60ms;
}

.mention-row:hover,
.mention-row.is-hi {
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
}

.mention-row.dir {
  color: var(--wb-ink-2);
}

.mention-name {
  flex: none;
  max-width: 45%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--wb-font-mono);
}

/* 搜索命中带出来的所在目录：比名字淡一档，让用户知道会在哪儿改这个文件 */
.mention-dir {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  direction: rtl;
  text-align: left;
  color: var(--wb-faint);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
}

.mention-go {
  color: var(--wb-faint);
}

.mention-note {
  margin: 0;
  padding: var(--wb-sp-2);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
}

.mention-note.bad {
  color: var(--wb-bad);
}
</style>
