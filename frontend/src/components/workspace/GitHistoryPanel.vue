<script setup lang="ts">
// 历史面板：提交列表 → 单次提交差异抽屉 → 文件历史抽屉。
// 历史不缓存：实时 git log 是唯一真相源，换分支后历史整块变正是缓存不该存在的原因（specs/09）。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import { useDrawer } from '../../composables/useDrawer'
import type { GitCommitDetailVO, GitCommitVO, GitFileVO } from '../../types/api'
import { fmtCount } from '../../utils/num'
import AppIcon from '../common/AppIcon.vue'
import DiffView from './DiffView.vue'

const props = defineProps<{ sessionId: string }>()
const toast = useToastStore()

const commits = ref<GitCommitVO[]>([])
const loading = ref(true)
const error = ref('')

// 深链：从提交面板的「已提交 xxxx」跳进来时带 hash，进页就展开那一条。
const focusHash = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    commits.value = await api.git.history(props.sessionId, 100)
  } catch (e) {
    commits.value = []
    error.value = (e as Error)?.message || '读不到提交历史'
  } finally {
    loading.value = false
  }
}
onMounted(() => {
  void load()
  if (props.sessionId) focusHash.value = ''
})

function fmtWhen(ts: number): string {
  if (!ts) return '—'
  const d = new Date(ts)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}/${d.getMonth() + 1}/${d.getDate()} ${p(d.getHours())}:${p(d.getMinutes())}`
}

// ---- 单次提交抽屉 ----
const open = ref(false)
const detail = ref<GitCommitDetailVO | null>(null)
const detailLoading = ref(false)
const { drawerRef } = useDrawer(open, () => (open.value = false))

async function show(hash: string) {
  open.value = true
  detail.value = null
  detailLoading.value = true
  try {
    detail.value = await api.git.commit(hash, props.sessionId)
  } catch (e) {
    toast.bad(`读不到这次提交：${(e as Error)?.message || '请重试'}`)
    open.value = false
  } finally {
    detailLoading.value = false
  }
}

// ---- 文件历史抽屉 ----
const fileOpen = ref(false)
const fileCommits = ref<GitCommitVO[]>([])
const fileLoading = ref(false)
const filePath = ref('')
const { drawerRef: fileDrawerRef } = useDrawer(fileOpen, () => (fileOpen.value = false))

async function showFileHistory(path: string) {
  filePath.value = path
  fileOpen.value = true
  fileCommits.value = []
  fileLoading.value = true
  try {
    fileCommits.value = await api.git.fileHistory(path, props.sessionId, 50)
  } catch (e) {
    toast.bad(`读不到这个文件的历史：${(e as Error)?.message || '请重试'}`)
    fileOpen.value = false
  } finally {
    fileLoading.value = false
  }
}

// 从文件历史里点一条 → 关掉文件抽屉、打开那次提交的差异。
async function openFromFile(hash: string) {
  fileOpen.value = false
  await show(hash)
}

const STATUS_TEXT: Record<string, string> = {
  M: '改过',
  A: '新增',
  D: '删除',
  R: '改名',
}
function tagText(f: GitFileVO) {
  return STATUS_TEXT[f.status] || f.status
}
const totalAdd = computed(() => commits.value.reduce((n, c) => n + (c.additions || 0), 0))
const totalDel = computed(() => commits.value.reduce((n, c) => n + (c.deletions || 0), 0))
</script>

<template>
  <div class="gh">
    <div class="gh-hd">
      <span class="cnt">{{ commits.length }} 个提交</span>
      <span v-if="commits.length" class="sum mono">+{{ fmtCount(totalAdd) }} −{{ fmtCount(totalDel) }}</span>
      <span class="sp" />
      <button class="btn btn-sm" type="button" :class="{ 'is-loading': loading }" :disabled="loading" @click="load()">
        <AppIcon v-if="!loading" name="refresh" size="ic-xs" />
        刷新
      </button>
    </div>

    <p v-if="loading" class="none">正在读取…</p>
    <p v-else-if="error" class="none is-bad">{{ error }}</p>
    <p v-else-if="!commits.length" class="none">这个仓库还没有提交。</p>
    <ol v-else class="gh-list">
      <li v-for="c in commits" :key="c.hash">
        <button class="gh-row" type="button" @click="show(c.hash)">
          <span class="gh-dot" />
          <span class="gh-body">
            <span class="gh-subject">{{ c.subject || '（没有提交信息）' }}</span>
            <span class="gh-meta dim">
              <b class="mono">{{ c.short }}</b>
              <span>{{ c.author }}</span>
              <span>{{ fmtWhen(c.when) }}</span>
              <span>{{ c.files }} 个文件</span>
            </span>
          </span>
          <span class="gh-num mono">+{{ c.additions }} −{{ c.deletions }}</span>
        </button>
      </li>
    </ol>

    <!-- 单次提交差异 -->
    <div v-if="open" ref="drawerRef" class="mask" @click.self="open = false">
      <aside class="drawer" role="dialog" aria-label="提交内容">
        <header class="dr-hd">
          <b>{{ detail?.subject || '提交内容' }}</b>
          <span class="sp" />
          <button class="btn btn-sm btn-ghost" type="button" @click="open = false">关闭</button>
        </header>
        <div v-if="detailLoading" class="dr-bd">
          <p class="none">正在读取…</p>
        </div>
        <div v-else-if="detail" class="dr-bd">
          <div class="kv">
            <div><span>提交</span><b class="mono">{{ detail.short }}</b></div>
            <div><span>作者</span><b>{{ detail.author }}</b></div>
            <div><span>时间</span><b>{{ fmtWhen(detail.when) }}</b></div>
          </div>
          <p v-if="detail.body" class="gh-body-text">{{ detail.body }}</p>

          <section class="sec">
            <h3>这次改了 {{ detail.files.length }} 个文件</h3>
            <ul class="gh-files">
              <li v-for="f in detail.files" :key="f.path">
                <span class="nm mono" :title="f.path">{{ f.path }}</span>
                <span class="tag">{{ tagText(f) }}</span>
                <span class="pct mono">+{{ f.additions }} −{{ f.deletions }}</span>
                <button class="btn btn-sm btn-ghost" type="button" @click="showFileHistory(f.path)">
                  它的历史
                </button>
              </li>
            </ul>
          </section>

          <section v-if="detail.diff?.lines.length" class="sec">
            <DiffView
              :lines="detail.diff.lines"
              :path="detail.diff.path"
              :truncated="!!detail.diff.truncated"
              :dropped="detail.diff.dropped || 0"
              :loading="false"
            />
          </section>
        </div>
      </aside>
    </div>

    <!-- 文件历史 -->
    <div v-if="fileOpen" ref="fileDrawerRef" class="mask" @click.self="fileOpen = false">
      <aside class="drawer" role="dialog" aria-label="文件历史">
        <header class="dr-hd">
          <b class="mono">{{ filePath }}</b>
          <span class="sp" />
          <button class="btn btn-sm btn-ghost" type="button" @click="fileOpen = false">关闭</button>
        </header>
        <div class="dr-bd">
          <p v-if="fileLoading" class="none">正在读取…</p>
          <p v-else-if="!fileCommits.length" class="none">这个文件还没有提交记录。</p>
          <ol v-else class="gh-list">
            <li v-for="c in fileCommits" :key="c.hash">
              <button class="gh-row" type="button" @click="openFromFile(c.hash)">
                <span class="gh-dot" />
                <span class="gh-body">
                  <span class="gh-subject">{{ c.subject || '（没有提交信息）' }}</span>
                  <span class="gh-meta dim">
                    <b class="mono">{{ c.short }}</b>
                    <span>{{ fmtWhen(c.when) }}</span>
                  </span>
                </span>
                <span class="gh-num mono">+{{ c.additions }} −{{ c.deletions }}</span>
              </button>
            </li>
          </ol>
        </div>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.gh {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.gh-hd {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.gh-hd .sp {
  flex: 1;
}
.gh-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.gh-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: var(--wb-sp-2) var(--wb-sp-1);
  border: 0;
  border-radius: var(--wb-radius-sm);
  background: none;
  text-align: left;
  cursor: pointer;
  transition: background var(--wb-dur) var(--wb-ease);
}
.gh-row:hover {
  background: var(--wb-tint);
}
.gh-dot {
  flex: none;
  width: 7px;
  height: 7px;
  border-radius: var(--wb-radius-full);
  border: 2px solid var(--wb-primary);
  background: var(--wb-surface);
}
.gh-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.gh-subject {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.gh-meta {
  display: flex;
  gap: var(--wb-sp-3);
  font-size: var(--wb-fs-2xs);
}
.gh-meta b {
  color: var(--wb-primary);
}
.gh-num {
  flex: none;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.gh-body-text {
  margin: 0;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-left: 2px solid var(--wb-line-2);
  border-radius: var(--wb-radius-xs);
  background: var(--wb-surface-2);
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
  white-space: pre-wrap;
}
.gh-files {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.gh-files li {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-1) var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  font-size: var(--wb-fs-xs);
}
.gh-files li:hover {
  background: var(--wb-tint);
}
.gh-files .nm {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--wb-ink-2);
}
.tag {
  flex: none;
  padding: 1px 6px;
  border-radius: var(--wb-radius-full);
  border: 1px solid var(--wb-line-2);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.pct {
  flex: none;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.mask {
  position: fixed;
  inset: 0;
  z-index: 40;
  background: rgb(15 23 42 / 24%);
  display: flex;
  justify-content: flex-end;
}
.drawer {
  width: min(620px, 92vw);
  display: flex;
  flex-direction: column;
  background: var(--wb-surface);
  border-left: 1px solid var(--wb-line);
  box-shadow: var(--wb-shadow-2);
  animation: drawer-in var(--wb-dur) var(--wb-ease);
}
@keyframes drawer-in {
  from {
    transform: translateX(24px);
    opacity: 0;
  }
}
.dr-hd {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3) var(--wb-sp-4);
  border-bottom: 1px solid var(--wb-line);
}
.dr-hd b {
  min-width: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dr-bd {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: var(--wb-sp-4);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.kv {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: var(--wb-sp-2) var(--wb-sp-3);
}
.kv > div {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.kv span {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.kv b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.sec h3 {
  margin: 0 0 var(--wb-sp-2);
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.none {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.none.is-bad {
  color: var(--wb-danger);
}
.mono {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}
.dim {
  color: var(--wb-muted);
}
.cnt,
.sum {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
</style>
