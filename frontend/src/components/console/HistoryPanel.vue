<script setup lang="ts">
// 历史页签：只读翻看别处聊过的天 + 接着聊（specs/12）。
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import * as api from '../../api'
import type { HistoryEntry } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import PageState from '../common/PageState.vue'
import { useToastStore } from '../../stores/toast'
import { useDrawer } from '../../composables/useDrawer'

const toast = useToastStore()
const router = useRouter()
const entries = ref<HistoryEntry[]>([])
const skipped = ref(0)
const timedOut = ref(false)
const loading = ref(true)
const error = ref('')
const q = ref('')
const cur = ref<HistoryEntry | null>(null)
const openingId = ref('')
const reading = ref(false)

const shown = computed(() => {
  const kw = q.value.trim().toLowerCase()
  if (!kw) return entries.value
  return entries.value.filter(
    (e) =>
      e.title.toLowerCase().includes(kw) ||
      (e.project || '').toLowerCase().includes(kw),
  )
})

async function scan() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.history.scan()
    // 后端出口已保证空数组；这里再兜一次，防的是缓存里的旧响应与 hand-written 假后端。
    entries.value = res.entries ?? []
    skipped.value = res.skipped
    timedOut.value = res.timed_out
  } catch (e) {
    error.value = (e as Error)?.message || '扫不到历史会话'
  } finally {
    loading.value = false
  }
}
onMounted(scan)

async function open(e: HistoryEntry) {
  // openingId 区分「正在打开哪一条」：只用面板级 reading 的话，连点不同条目时
  // 后到的响应会覆盖先到的内容——用户读到的是另一条的快照，还以为这一条坏了。
  if (openingId.value) return
  openingId.value = e.id
  reading.value = true
  try {
    cur.value = await api.history.read(e.id)
  } catch (err) {
    toast.bad(`读不出来：${(err as Error)?.message || '可能被移动或删除了'}`)
  } finally {
    openingId.value = ''
    reading.value = false
  }
}

// 接着聊：补一句「接下来干嘛」，建新会话并注入上下文，然后跳对话页。
const nextOpen = ref(false)
const nextText = ref('')
const continuing = ref(false)
function startContinue() {
  if (!cur.value) return
  nextText.value = ''
  nextOpen.value = true
}
async function doContinue() {
  if (!cur.value) return
  continuing.value = true
  try {
    const sess = await api.history.continueChat(cur.value.id, nextText.value.trim())
    nextOpen.value = false
    router.push(`/?session=${encodeURIComponent(sess.id)}`)
  } catch (e) {
    toast.bad(`接着聊失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    continuing.value = false
  }
}

// 自定义文件夹
const folderOpen = ref(false)
const folderPath = ref('')
const folders = ref<Record<string, string>>({})
async function loadFolders() {
  try {
    folders.value = await api.history.folders()
  } catch {
    /* 读不到不挡主流程 */
  }
}
onMounted(loadFolders)
const addingFolder = ref(false)
// 两个抽屉各持一份焦点管理：Esc 关闭 + 焦点圈在抽屉内，关闭后还给触发钮
const { drawerRef: folderDrawerRef } = useDrawer(folderOpen, () => (folderOpen.value = false))
const { drawerRef: nextDrawerRef } = useDrawer(nextOpen, () => (nextOpen.value = false))
async function addFolder() {
  if (addingFolder.value) return
  addingFolder.value = true
  try {
    await api.history.addFolder('custom', folderPath.value.trim())
    folderPath.value = ''
    folderOpen.value = false
    await loadFolders()
    // 触发一次全量重扫要几秒到几十秒，按钮必须转到底：
    // 只等一个 toast 的表现是「点了没反应」，用户必然再点一次。
    await scan()
  } catch (e) {
    toast.bad(`添加失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    addingFolder.value = false
  }
}

const ROLE: Record<string, string> = { user: '我', assistant: '助手', tool: '工具' }
</script>

<template>
  <PageState :loading="loading" :error="error">
    <header class="pane-head">
      <div>
        <h2>历史</h2>
        <p>在 Claude Code / Codex 里聊过的天，这里翻看；挑一段就能搬到 WorkBaby 接着聊。</p>
      </div>
      <div class="head-ops">
        <button class="btn btn-sm btn-ghost" type="button" @click="folderOpen = true">加文件夹</button>
        <button
          class="btn btn-sm btn-ghost"
          type="button"
          :class="{ 'is-loading': loading }"
          :disabled="loading"
          @click="scan"
        >
          <AppIcon v-if="!loading" name="refresh" size="ic-xs" /> 刷新
        </button>
      </div>
    </header>

    <p v-if="timedOut" class="note warn">扫描超时了，下面是已扫到的部分。</p>
    <p v-else-if="skipped" class="note">有 {{ skipped }} 个文件读不出来，已跳过。</p>

    <div class="hist-body">
      <div class="hist-list-wrap">
        <input v-model="q" class="input hist-search" placeholder="搜标题或项目" />
        <div v-if="!shown.length" class="card blank small">
          <b>没有找到会话</b>
          <p>换个关键词，或确认那些工具确实在这台机器上用过。</p>
        </div>
        <ul v-else class="hist-list">
          <!-- 用 button 而不是 li@click：历史条目是「点开一段」的独立动作，
               原生 button 才有 Tab 可达与回车/空格触发，键盘用户整块功能才用得上 -->
          <li v-for="e in shown" :key="e.id">
            <button
              class="hist-item"
              :class="{ on: cur?.id === e.id, 'is-loading': openingId === e.id }"
              type="button"
              :disabled="!!openingId"
              :aria-current="cur?.id === e.id"
              @click="open(e)"
            >
              <b>{{ e.title }}</b>
              <span class="dim">{{ e.project }} · {{ e.message_count }} 条</span>
            </button>
          </li>
        </ul>
      </div>

      <div class="hist-read">
        <PageState :loading="reading" :error="''">
          <div v-if="!cur" class="card blank small">
            <span class="bt"><AppIcon name="clock" size="ic-lg" /></span>
            <b>选一段看看</b>
            <p>左边点一条，这里显示当时的对话（只读）。</p>
          </div>
          <template v-else>
            <header class="read-head">
              <div>
                <h3>{{ cur.title }}</h3>
                <span class="dim">{{ cur.project }}</span>
              </div>
              <button class="btn btn-primary btn-sm" type="button" @click="startContinue">接着聊</button>
            </header>
            <div class="msgs">
              <div v-for="(m, i) in cur.messages" :key="i" class="msg" :class="m.role">
                <span class="who">{{ ROLE[m.role] || m.role }}</span>
                <p>{{ m.text }}</p>
              </div>
            </div>
          </template>
        </PageState>
      </div>
    </div>

    <Transition name="scrim">
      <div v-if="folderOpen" class="scrim-side no-scrim-anim" @click.self="folderOpen = false">
        <Transition name="drawer" appear>
          <section v-if="folderOpen" ref="folderDrawerRef" class="drawer-side is-narrow" role="dialog" aria-modal="true" aria-label="加文件夹">
            <header class="drawer-head">
              <h3>加一个会话文件夹</h3>
              <button class="btn btn-sm btn-ghost" type="button" @click="folderOpen = false">关闭</button>
            </header>
            <p class="dim">别的工具存的聊天记录文件夹（里面是 .jsonl 文件）。扫到时按通用格式解析。</p>
            <label class="fld">
              <span>文件夹路径</span>
              <input v-model="folderPath" class="input" placeholder="D:\backups\chats" />
            </label>
            <footer class="drawer-foot">
              <button
                class="btn btn-primary"
                :class="{ 'is-loading': addingFolder }"
                :disabled="addingFolder || !folderPath.trim()"
                type="button"
                @click="addFolder"
              >
                添加并重新扫描
              </button>
            </footer>
          </section>
        </Transition>
      </div>
    </Transition>

    <Transition name="scrim">
      <div v-if="nextOpen" class="scrim-side no-scrim-anim" @click.self="nextOpen = false">
        <Transition name="drawer" appear>
          <section v-if="nextOpen" ref="nextDrawerRef" class="drawer-side is-narrow" role="dialog" aria-modal="true" aria-label="接着聊">
            <header class="drawer-head">
              <h3>接着聊</h3>
              <button class="btn btn-sm btn-ghost" type="button" @click="nextOpen = false">关闭</button>
            </header>
            <p class="dim">会把这段历史作为上下文带进新对话。想接着说什么，写在这里：</p>
            <textarea v-model="nextText" class="input" rows="3" placeholder="比如：按这个思路，把第二版写出来" />
            <footer class="drawer-foot">
              <button
                class="btn btn-primary"
                :class="{ 'is-loading': continuing }"
                :disabled="continuing"
                type="button"
                @click="doContinue"
              >
                开始
              </button>
            </footer>
          </section>
        </Transition>
      </div>
    </Transition>
  </PageState>
</template>

<style scoped>
.pane-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--wb-sp-4);
  margin-bottom: var(--wb-sp-4);
}
.pane-head h2 { font-size: var(--wb-fs-xl); color: var(--wb-ink); }
.pane-head p { font-size: var(--wb-fs-sm); color: var(--wb-muted); margin-top: var(--wb-sp-1); max-width: var(--wb-w-desc); }
.head-ops { display: flex; gap: var(--wb-sp-2); }
.note { font-size: var(--wb-fs-hint); color: var(--wb-muted); margin-bottom: var(--wb-sp-3); }
.note.warn { color: var(--wb-warning); }
.hist-body { display: grid; grid-template-columns: 300px 1fr; gap: var(--wb-sp-4); min-height: 420px; }
.hist-list-wrap { display: flex; flex-direction: column; gap: var(--wb-sp-3); min-width: 0; }
.hist-search { flex-shrink: 0; }
.hist-list { display: flex; flex-direction: column; gap: var(--wb-sp-2); overflow: auto; }
.hist-list { display: flex; flex-direction: column; gap: var(--wb-sp-2); overflow: auto; }
.hist-list li { min-width: 0; }
.hist-item {
  width: 100%;
  padding: var(--wb-sp-3);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-surface);
  cursor: pointer;
  text-align: left;
  font: inherit;
  color: inherit;
  display: flex;
  flex-direction: column;
  gap: 2px;
  transition:
    border-color var(--wb-dur-fast) var(--wb-ease),
    background var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.hist-item:hover:not(:disabled) { border-color: var(--wb-line-2); }
/* 整行元素不悬停放大（宽度 100% 放大必溢出），按下 0.97 保留点击感 */
.hist-item:active:not(:disabled) { transform: scale(0.97); }
.hist-item.on { border-color: var(--wb-primary-line); background: var(--wb-primary-soft); }
.hist-item b { font-size: var(--wb-fs-sm); color: var(--wb-ink); }
.hist-read { min-width: 0; }
.blank {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8);
  text-align: center;
  color: var(--wb-muted);
}
.blank.small { padding: var(--wb-sp-6); }
.blank b { color: var(--wb-ink); }
.read-head { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: var(--wb-sp-4); }
.read-head h3 { font-size: var(--wb-fs-lg); color: var(--wb-ink); }
.msgs { display: flex; flex-direction: column; gap: var(--wb-sp-3); }
.msg { display: flex; gap: var(--wb-sp-3); font-size: var(--wb-fs-sm); }
.msg .who { flex-shrink: 0; width: 32px; color: var(--wb-muted); font-weight: 600; }
.msg p { color: var(--wb-ink-2); white-space: pre-wrap; }
.msg.user .who { color: var(--wb-primary); }
.drawer-head { display: flex; justify-content: space-between; align-items: center; }
.drawer-head h3 { font-size: var(--wb-fs-lg); color: var(--wb-ink); }
.fld { display: flex; flex-direction: column; gap: var(--wb-sp-2); }
.fld > span { font-size: var(--wb-fs-label); font-weight: 600; color: var(--wb-ink-2); }
.drawer-foot { display: flex; justify-content: flex-end; }
.dim { color: var(--wb-muted); font-size: var(--wb-fs-hint); }
</style>
