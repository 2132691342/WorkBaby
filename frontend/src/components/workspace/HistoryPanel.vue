<script setup lang="ts">
// 历史面板：提交列表 + 单次提交详情（改动文件与逐行差异）+ 单个文件的时间线。
// 单独一个组件：这里同时管「列表 / 详情 / 文件过滤」三种状态与选择态，
// 塞回页面会让那个文件变成第二个 store。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import DiffView from '@/components/workspace/DiffView.vue'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import type { GitCommit, GitCommitDetail, GitDiff } from '@/types/api'

const props = defineProps<{
  /** 当前工作区里的提交列表（由页面加载，切分支后由页面刷新） */
  commits: GitCommit[]
  /** 只看这个文件的历史；空串表示看整个仓库 */
  file?: string
}>()

const emit = defineEmits<{ clearFile: [] }>()

const { t } = useI18n()
const toasts = useToasts()

const rows = ref<GitCommit[]>([])
const loading = ref(false)
// 详情：一次只展开一个提交。openHash 为空表示没展开。
const openHash = ref('')
const detail = ref<GitCommitDetail | null>(null)
const detailLoading = ref(false)
// 详情里选中的文件与它的差异。commitDiff 为整份提交的差异，fileDiff 为单文件的。
const pickedFile = ref('')
const commitDiff = ref<GitDiff | null>(null)
const fileDiff = ref<GitDiff | null>(null)
const diffLoading = ref(false)

const list = computed(() => (props.file ? rows.value : props.commits))

/** 拉文件级历史。file 为空时直接用页面给的全量列表。 */
async function loadFileHistory() {
  if (!props.file) {
    rows.value = []
    return
  }
  loading.value = true
  try {
    const data = await api.gitFileHistory(props.file, 60)
    rows.value = data.commits ?? []
  } catch (err) {
    rows.value = []
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

watch(() => props.file, loadFileHistory, { immediate: true })

/** 展开一个提交：拉元信息与整份差异。 */
async function open(commit: GitCommit) {
  if (openHash.value === commit.commit_hash) {
    openHash.value = ''
    detail.value = null
    return
  }
  openHash.value = commit.commit_hash
  detail.value = null
  commitDiff.value = null
  fileDiff.value = null
  pickedFile.value = ''
  detailLoading.value = true
  try {
    const data = await api.gitCommitDetail(commit.commit_hash)
    detail.value = data
    commitDiff.value = data.diff ?? null
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    detailLoading.value = false
  }
}

/**
 * 在展开的提交里选一个文件。
 *
 * 只改选中状态并顺手拉单文件差异：整份提交的差异已经拿到了，
 * 但几千行一起渲染会卡，而用户通常只想看当前选中的那个。
 */
async function pickFile(filePath: string) {
  if (!detail.value) return
  pickedFile.value = filePath
  diffLoading.value = true
  try {
    fileDiff.value = await api.gitFileDiff(detail.value.commit.commit_hash, filePath)
  } catch {
    fileDiff.value = null
  } finally {
    diffLoading.value = false
  }
}

/** 详情里展示的差异：选中文件用单文件的，否则用整份的。 */
const shownDiff = computed<GitDiff | null>(() => fileDiff.value ?? commitDiff.value)
const shownPath = computed(() => pickedFile.value)

/** 时间戳到「多久以前」：历史列表里精确到秒没有意义。 */
function relative(ms: number): string {
  if (!ms) return ''
  const diff = Date.now() - ms
  const min = Math.floor(diff / 60_000)
  if (min < 1) return t('history.justNow')
  if (min < 60) return t('history.minutesAgo', { n: min })
  const hour = Math.floor(min / 60)
  if (hour < 24) return t('history.hoursAgo', { n: hour })
  const day = Math.floor(hour / 24)
  if (day < 30) return t('history.daysAgo', { n: day })
  return new Date(ms).toLocaleDateString()
}

defineExpose({ reload: loadFileHistory })
</script>

<template>
  <div class="history">
    <!-- 文件过滤条：只在「看这个文件的历史」时出现，带一个退出入口 -->
    <div v-if="file" class="file-filter">
      <AppIcon name="file" :size="12" />
      <span class="file-filter-path mono">{{ file }}</span>
      <span class="readout">{{ rows.length }}</span>
      <button class="wb-btn text sm" @click="emit('clearFile')">{{ t('history.showAll') }}</button>
    </div>

    <div v-if="loading" class="empty-mini">{{ t('common.loading') }}</div>
    <div v-else-if="!list.length" class="empty-mini">
      {{ file ? t('history.noFileHistory') : t('workspace.noHistory') }}
    </div>

    <template v-else>
      <div v-for="item in list" :key="item.commit_hash" class="commit-block">
        <button
          class="commit-line"
          :class="{ open: openHash === item.commit_hash }"
          @click="open(item)"
        >
          <span class="hash mono">{{ item.short_hash }}</span>
          <span class="subject">{{ item.commit_subject }}</span>
          <span class="readout">{{ item.author_name }} · {{ relative(item.commit_time) }}</span>
          <span class="num">
            <b class="add">+{{ item.insertions }}</b>
            <b class="del">−{{ item.deletions }}</b>
          </span>
        </button>

        <div v-if="openHash === item.commit_hash" class="commit-detail">
          <div v-if="detailLoading" class="empty-mini">{{ t('common.loading') }}</div>
          <template v-else-if="detail">
            <p v-if="detail.commit_body" class="commit-body selectable">{{ detail.commit_body }}</p>
            <div class="detail-meta">
              <span class="readout">{{ detail.commit.commit_hash }}</span>
              <span v-if="detail.parent_hashes.length" class="readout">
                {{ t('history.parent') }} {{ detail.parent_hashes[0]?.slice(0, 7) }}
              </span>
            </div>

            <div class="detail-files">
              <button
                v-for="changed in detail.changed_files"
                :key="changed.file_path"
                class="detail-file"
                :class="{ active: pickedFile === changed.file_path }"
                @click="pickFile(changed.file_path)"
              >
                <span class="path">{{ changed.file_path }}</span>
                <span class="num">
                  <b class="add">+{{ changed.insertions }}</b>
                  <b class="del">−{{ changed.deletions }}</b>
                </span>
              </button>
            </div>

            <DiffView :diff="shownDiff" :path="shownPath" :loading="diffLoading" />
          </template>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.history {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow-y: auto;
  /* 列表本身不逐条画框线：40 条提交就是 40 个框，读起来像一堆卡片。
     整列靠留白分组，悬停才给底色。 */
  gap: 0;
  padding-right: var(--wb-sp-1);
}

.file-filter {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) 0;
  border-bottom: 1px solid var(--wb-line);
  flex: none;
}

.file-filter-path {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.commit-block {
  display: flex;
  flex-direction: column;
}

/* 一条提交：hash · 标题 · 作者时间 · 增删。整行是一个可点的对象，
   所以给整行的悬停底色与按下缩放，但不给描边。 */
.commit-line {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: 5px var(--wb-sp-2);
  border: 0;
  border-radius: var(--wb-r-sm);
  background: transparent;
  color: var(--wb-ink);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

/* 相邻两条之间一条极淡的分隔：比每块套框安静，又不至于糊成一片 */
.commit-block + .commit-block .commit-line {
  box-shadow: inset 0 1px 0 var(--wb-line);
}

.commit-line:hover {
  background: var(--wb-hover);
  box-shadow: none;
}

.commit-line:active {
  transform: scale(var(--wb-press-row-active));
  transition-duration: 60ms;
}

/* 展开的那一条：淡强调底 + 左缘主色竖条。
   竖条而不是整圈描边：描边会把这一行框成一个盒子，列表就散了。 */
.commit-line.open {
  background: var(--wb-selected);
  box-shadow: inset 2px 0 0 var(--wb-brand);
}

.hash {
  flex: none;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-brand);
}

.subject {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 提交时间与作者：次级读数，与标题拉开一级灰度 */
.readout {
  flex: none;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.num {
  display: flex;
  gap: var(--wb-sp-2);
  flex: none;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
}

.add {
  color: var(--wb-ok);
  font-weight: 500;
}

.del {
  color: var(--wb-bad);
  font-weight: 500;
}

/* 详情区：与列表同底色，只靠缩进与间距分开。
   不再套一层描边：下面已经有一份差异视图（它自带圆角描边），
   再加上去就是三层框叠在一起，而这三层表达的是同一件事。 */
.commit-detail {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  margin: var(--wb-sp-1) 0 var(--wb-sp-3) var(--wb-sp-2);
  padding: var(--wb-sp-2) 0 var(--wb-sp-2) var(--wb-sp-3);
  border-left: 1px solid var(--wb-line);
  animation: detail-unfold var(--wb-dur) var(--wb-ease);
}

@keyframes detail-unfold {
  from {
    opacity: 0;
    transform: translateY(-4px);
  }
}

.commit-body {
  margin: 0;
  padding: var(--wb-sp-2);
  border-radius: var(--wb-r-sm);
  background: var(--wb-code-bg);
  font-size: var(--wb-fs-2xs);
  line-height: 1.65;
  color: var(--wb-ink-5);
  white-space: pre-wrap;
}

.detail-meta {
  display: flex;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
}

.detail-files {
  display: flex;
  flex-direction: column;
  gap: 1px;
  max-height: 200px;
  overflow-y: auto;
}

.detail-file {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: 3px var(--wb-sp-2);
  border: 0;
  border-radius: var(--wb-r-xs);
  background: transparent;
  color: var(--wb-ink-2);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}

.detail-file:hover {
  background: var(--wb-surface);
}

.detail-file.active {
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
}

.path {
  flex: 1;
  min-width: 0;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-ink-5);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.empty-mini {
  padding: var(--wb-sp-4);
  text-align: center;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
}
</style>
