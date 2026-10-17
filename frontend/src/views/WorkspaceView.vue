<script setup lang="ts">
// 工作区视图：助手改了哪些文件、改了什么，在这里一眼看到。
// 只读（status / diff）——提交与回滚是版本操作，交给用户在自己的工具里决定。
import { computed, onMounted, ref, watch } from 'vue'
import * as api from '../api'
import { useSessionStore } from '../stores/session'
import { useToastStore } from '../stores/toast'
import type { GitDiffVO, GitFileVO, GitStatusVO } from '../types/api'
import AppIcon from '../components/common/AppIcon.vue'
import PageState from '../components/common/PageState.vue'
import DiffView from '../components/workspace/DiffView.vue'

const toast = useToastStore()
const session = useSessionStore()

// 看的是当前会话的工作目录：会话自己选过目录时，全局目录不是改动发生的地方
const sessionId = computed(() => session.current?.id || '')

const status = ref<GitStatusVO | null>(null)
const diff = ref<GitDiffVO | null>(null)
const loading = ref(true)
const error = ref('')
// 选中文件与正在读差异的文件分开存：行上的转圈只该出现在被点的那一行
const selected = ref('')
const loadingDiff = ref(false)

const files = computed<GitFileVO[]>(() => status.value?.files || [])
const readable = computed(() => !!status.value?.available && !!status.value?.is_repo)

const STATUS_TEXT: Record<string, string> = {
  M: '改过',
  A: '新增',
  D: '删除',
  R: '改名',
  '?': '未跟踪',
}
const STATUS_CLASS: Record<string, string> = {
  M: 'is-mod',
  A: 'is-add',
  D: 'is-del',
  R: 'is-mod',
  '?': 'is-new',
}

function tagText(s: string) {
  return STATUS_TEXT[s] || s
}
function tagClass(s: string) {
  return STATUS_CLASS[s] || ''
}
// 长路径只留末尾两段：目录前缀在工作区条里已经给过了
function shortPath(p: string) {
  const parts = p.split('/').filter(Boolean)
  return parts.length > 2 ? '…/' + parts.slice(-2).join('/') : p
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    status.value = await api.git.status(sessionId.value)
    await openDiff('')
  } catch (e) {
    status.value = null
    error.value = (e as Error)?.message || '读不到工作区状态，稍后再试试'
  } finally {
    loading.value = false
  }
}

async function openDiff(path: string) {
  selected.value = path
  loadingDiff.value = true
  try {
    diff.value = await api.git.diff(path, sessionId.value)
  } catch (e) {
    // 读不出差异不能静默：列表里那一行还亮着，用户不知道是没改动还是读失败
    toast.bad(`读不出 ${shortPath(path) || '改动'}：${(e as Error)?.message || '请重试'}`)
    diff.value = null
  } finally {
    loadingDiff.value = false
  }
}

onMounted(() => {
  // 直接进这一页时会话列表可能还没拉过：拿不到当前会话就只能看全局目录
  if (!session.current) void session.loadList()
  void load()
})
// 换会话就换目录：不跟着切，这一页会一直显示上一个会话的改动
watch(sessionId, load)
</script>

<template>
  <div class="wk wb-ui">
    <div class="wk-main scroll">
      <div class="wrap">
        <header class="page-head">
          <div class="ph-txt">
            <h1>工作区</h1>
            <p>助手在你的文件夹里改了什么，这里逐行对账</p>
          </div>
          <span class="sp" />
          <button
            class="btn btn-sm"
            type="button"
            :class="{ 'is-loading': loading }"
            :disabled="loading"
            @click="load()"
          >
            <AppIcon v-if="!loading" name="refresh" size="ic-xs" />
            {{ loading ? '刷新中…' : '刷新' }}
          </button>
        </header>

        <PageState :loading="loading" :error="error">
          <!-- 分支与目录：先说清「看的是哪个文件夹的哪个分支」，再看改了什么 -->
          <div v-if="readable" class="wk-bar">
            <span class="br">
              <AppIcon name="git-branch" size="ic-sm" />
              {{ status?.branch || '（无分支）' }}
            </span>
            <span v-if="status?.ahead" class="tag is-mod">领先 {{ status.ahead }} 个提交</span>
            <span v-if="status?.behind" class="tag is-mod">落后 {{ status.behind }} 个提交</span>
            <span class="sp" />
            <span class="pt mono" :title="status?.workspace || ''">{{ status?.workspace }}</span>
          </div>

          <div v-if="!readable" class="card blank">
            <div class="blank-in">
              <span class="bt"><AppIcon name="git-branch" size="ic-lg" /></span>
              <div>
                <b>{{ status?.reason || '这里看不了改动' }}</b>
                <p>
                  工作区是「助手能读写的那个文件夹」，装上 Git 并在这个文件夹里执行一次
                  <span class="wb-code-inline">git init</span> 就能看到改动明细。
                </p>
              </div>
            </div>
          </div>

          <div v-else class="cols">
            <section class="card list-card">
              <div class="card-head">
                <h2>改动文件</h2>
                <span class="cnt">{{ files.length }}</span>
              </div>
              <div v-if="files.length" class="rowlist">
                <button
                  v-for="f in files"
                  :key="f.path"
                  class="rli chg"
                  type="button"
                  :class="{ 'is-on': selected === f.path, 'is-loading': loadingDiff && selected === f.path }"
                  :title="f.path"
                  @click="openDiff(f.path)"
                >
                  <AppIcon name="doc" size="ic-xs" />
                  <span class="nm mono">{{ shortPath(f.path) }}</span>
                  <span class="tag" :class="tagClass(f.status)">{{ tagText(f.status) }}</span>
                  <span class="pct">+{{ f.additions }} −{{ f.deletions }}</span>
                </button>
              </div>
              <p v-else class="none">工作区干干净净，还没有改动</p>
            </section>

            <DiffView
              :lines="diff?.lines || []"
              :path="diff?.path || ''"
              :truncated="!!diff?.truncated"
              :dropped="diff?.dropped || 0"
              :loading="loadingDiff"
            />
          </div>
        </PageState>
      </div>
    </div>
  </div>
</template>

<style scoped>
.wk {
  display: flex;
  height: 100%;
  min-height: 0;
  flex: 1;
  min-width: 0;
}
.wk-main {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
}
.wrap {
  width: 100%;
  max-width: 1180px;
  margin: 0 auto;
  padding: var(--wb-sp-8) var(--wb-sp-6) var(--wb-sp-10);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
  min-width: 0;
}
.wrap > * {
  min-width: 0;
}
.page-head {
  display: flex;
  align-items: flex-end;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
}
.ph-txt {
  min-width: 0;
}
.page-head h1 {
  font-family: var(--font-display);
  font-size: var(--wb-fs-2xl);
  font-weight: 700;
  letter-spacing: -0.01em;
  line-height: var(--wb-lh-tight);
  color: var(--wb-ink);
}
.page-head p {
  margin-top: 2px;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.page-head .sp {
  flex: 1;
}

/* 分支条：一行说清分支、与上游的差距、目录 */
.wk-bar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  background: var(--wb-surface);
  border: 1px solid var(--wb-line);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.wk-bar .br {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  color: var(--wb-ink-2);
}
.wk-bar .sp {
  flex: 1;
}
.wk-bar .pt {
  max-width: 46%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cols {
  display: grid;
  grid-template-columns: minmax(0, 340px) minmax(0, 1fr);
  gap: var(--wb-sp-4);
  align-items: start;
  min-height: 0;
}
.list-card {
  padding: var(--wb-sp-4);
}
.card-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin-bottom: var(--wb-sp-3);
}
.card-head h2 {
  font-size: var(--wb-fs-md);
  font-weight: 600;
  color: var(--wb-ink);
}
.cnt {
  margin-left: auto;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.none {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}

/* 改动行：整行不放大（侧栏里放大 1% 会被裁掉边缘），点击感交给底色与缩小 */
.chg {
  width: 100%;
  text-align: left;
  gap: var(--wb-sp-2);
}
.chg:hover {
  background: var(--wb-tint);
}
.chg:active {
  transform: scale(0.99);
}
.chg.is-on {
  background: var(--wb-sel);
  box-shadow: inset 2px 0 0 var(--wb-primary);
}
.chg:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
/* 读差异中：转圈顶掉图标位，路径与行数保留——只换文案不转圈等于没给反馈 */
.chg.is-loading {
  pointer-events: none;
  cursor: progress;
}
.chg.is-loading .nm {
  position: relative;
  padding-left: 18px;
}
.chg.is-loading .nm::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  width: 11px;
  height: 11px;
  margin-top: -5.5px;
  border-radius: var(--wb-radius-full);
  border: 1.5px solid currentColor;
  border-top-color: transparent;
  animation: wb-ic-spin 0.8s linear infinite;
}
.chg .nm {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
}
.chg .pct {
  flex: none;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.tag {
  flex: none;
  padding: 0 6px;
  border-radius: var(--wb-radius-full);
  border: 1px solid var(--wb-line-2);
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
  color: var(--wb-muted);
}
.tag.is-add {
  color: var(--wb-success);
  background: var(--wb-success-soft);
}
.tag.is-del {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
.tag.is-mod {
  color: var(--wb-primary);
  background: var(--wb-primary-soft);
}
.tag.is-new {
  color: var(--wb-live);
  background: var(--wb-live-soft);
}

/* ---- 空态 ---- */
.blank {
  padding: var(--wb-sp-6);
}
.blank-in {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-4);
  color: var(--wb-muted);
}
.blank-in .bt {
  display: grid;
  place-items: center;
  width: var(--wb-tile);
  height: var(--wb-tile);
  flex: none;
  border-radius: var(--wb-radius);
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
}
.blank-in b {
  color: var(--wb-ink);
  font-size: var(--wb-fs-md);
  font-weight: 600;
}
.blank-in p {
  margin-top: 2px;
  font-size: var(--wb-fs-sm);
  line-height: var(--wb-lh-base);
}

@media (prefers-reduced-motion: reduce) {
  .chg:active {
    transform: none;
  }
}
@media (max-width: 980px) {
  .cols {
    grid-template-columns: minmax(0, 1fr);
  }
}
@media (max-width: 720px) {
  .wrap {
    padding: var(--wb-sp-5) var(--wb-sp-4) var(--wb-sp-8);
  }
  .page-head h1 {
    font-size: var(--wb-fs-xl);
  }
}
</style>
