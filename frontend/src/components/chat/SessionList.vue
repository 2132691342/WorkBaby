<script setup lang="ts">
// 侧栏会话列表：分组展示 + 相对时间 + 重命名 / 删除。
import { computed, onBeforeUnmount, ref } from 'vue'
import * as api from '../../api'
import { useSessionStore } from '../../stores/session'
import { useToastStore } from '../../stores/toast'
import { fmtRel } from '../../utils/time'
import { SaveFileDialog } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'

const emit = defineEmits<{ newSession: [] }>()

const session = useSessionStore()
const toast = useToastStore()
const editingId = ref<string | null>(null)
const draft = ref('')

async function openSession(id: string) {
  // 打开会话要 await 并兜住失败：不接住就是一个未捕获的 rejection，
  // 点了没反应也没有任何提示，用户只能反复点。
  try {
    await session.open(id)
  } catch (e) {
    toast.bad(`打开对话失败：${(e as Error)?.message || '请重试'}`)
  }
}

// 分页展示：默认只渲染最新 10 条，「更多」每次追加 20 条。
// 展开状态在面板会话期内保持，不随重渲染丢得只剩 10 条。
const PAGE = 10
const STEP = 20
const shown = ref(PAGE)
const groups = computed(() => {
  const today: typeof session.list = []
  const week: typeof session.list = []
  const earlier: typeof session.list = []
  const dayStart = new Date()
  dayStart.setHours(0, 0, 0, 0)
  const weekAgo = dayStart.getTime() - 6 * 86400_000
  // 列表本就按更新时间倒序：先取前 N 条再分组，分组标题如实反映时间分布
  for (const s of session.list.slice(0, shown.value)) {
    if (s.updated_at >= dayStart.getTime()) today.push(s)
    else if (s.updated_at >= weekAgo) week.push(s)
    else earlier.push(s)
  }
  const out: Array<{ name: string; items: typeof session.list }> = []
  if (today.length) out.push({ name: '今天', items: today })
  if (week.length) out.push({ name: '本周', items: week })
  if (earlier.length) out.push({ name: '更早', items: earlier })
  return out
})
const hiddenCount = computed(() => Math.max(0, session.list.length - shown.value))
function showMore() {
  shown.value += STEP
}

function startEdit(id: string, title: string) {
  editingId.value = id
  draft.value = title
}

async function commitEdit() {
  const id = editingId.value
  const title = draft.value.trim()
  editingId.value = null
  if (!id || !title) return
  try {
    await session.rename(id, title)
  } catch (e) {
    toast.bad(`重命名失败：${(e as Error)?.message || '请重试'}`)
  }
}

// 两段式确认的还原定时器必须随组件卸载清掉：留着会在已卸载的组件上写 ref，
// 面板切走后再回来状态是脏的。
onBeforeUnmount(() => clearTimeout(confirmTimer))

// 删除走两段式确认，与模型服务 / 知识库 / 技能同一模式：第一次点进入确认态
// （图标换成对勾 + 危险底），3 秒不点自动还原。误触是删掉整段历史，值得多一次点击。
const confirmingId = ref('')
let confirmTimer = 0
function askRemove(s: { id: string; title: string }) {
  if (busyId.value) return
  if (confirmingId.value === s.id) {
    confirmingId.value = ''
    clearTimeout(confirmTimer)
    void doRemove(s)
    return
  }
  confirmingId.value = s.id
  clearTimeout(confirmTimer)
  confirmTimer = window.setTimeout(() => (confirmingId.value = ''), 3000)
}

// 导出：把整段对话写成一份自带样式的 HTML，双击就能看，不依赖本应用。
// 先选保存位置再发请求：导出在 PI 里做，路径必须由用户定。
const exportingId = ref('')
async function exportSession(s: { id: string; title: string }) {
  if (exportingId.value) return
  const name = (s.title || 'WorkBaby 对话').replace(/[\\/:*?"<>|]/g, '-').slice(0, 60)
  const target = await SaveFileDialog('导出对话', `${name}.html`, '*.html')
  if (!target) return
  exportingId.value = s.id
  try {
    const path = await api.sessions.exportHTML(s.id, target)
    toast.ok(`已导出到 ${path}`)
  } catch (e) {
    toast.bad(`导出失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    exportingId.value = ''
  }
}

const busyId = ref('')
async function doRemove(s: { id: string; title: string }) {
  if (busyId.value) return
  busyId.value = s.id
  try {
    await session.remove(s.id)
    toast.ok(`已删除「${s.title || '未命名对话'}」`)
  } catch (e) {
    toast.bad(`删除失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busyId.value = ''
  }
}
</script>

<template>
  <div class="sess-scroll">
    <div v-for="g in groups" :key="g.name" class="sess-group">
      <div class="n">{{ g.name }}</div>
      <!-- TransitionGroup：删掉一条会话时行是淡出+缩掉，不是一瞬间消失。
           列表只有几十行且按天分组，删哪一条要看得见。 -->
      <TransitionGroup name="slist">
        <button
          v-for="s in g.items"
          :key="s.id"
          class="sess-item"
          :class="{ 'is-on': s.id === session.currentId, 'is-loading': session.openingId === s.id }"
          type="button"
          :disabled="!!session.openingId && session.openingId !== s.id"
          @click="openSession(s.id)"
        >
          <template v-if="editingId === s.id">
            <input v-model="draft" class="input rename" autofocus @keydown.enter="commitEdit" @blur="commitEdit" />
          </template>
          <template v-else>
            <span class="si-run" :class="{ on: s.running }" :title="s.running ? '这个会话正在跑' : ''" aria-hidden="true" />
            <span class="si-name">{{ s.title || '未命名对话' }}</span>
            <span class="si-time">{{ fmtRel(s.updated_at) }}</span>
            <span class="si-act">
              <button
                class="icon-btn"
                type="button"
                title="重命名"
                :aria-label="`重命名「${s.title || '未命名对话'}」`"
                :disabled="!!busyId"
                @click.stop="startEdit(s.id, s.title)"
              >
                <AppIcon name="pencil" size="ic-sm" />
              </button>
              <button
                class="icon-btn"
                type="button"
                title="导出为 HTML"
                :aria-label="`导出「${s.title || '未命名对话'}」为 HTML`"
                :class="{ 'is-loading': exportingId === s.id }"
                :disabled="!!busyId || !!exportingId"
                @click.stop="exportSession(s)"
              >
                <AppIcon name="download" size="ic-sm" />
              </button>
              <button
                class="icon-btn danger"
                type="button"
                :title="confirmingId === s.id ? '再点一次确认删除' : '删除'"
                :aria-label="`删除「${s.title || '未命名对话'}」`"
                :class="{ 'is-loading': busyId === s.id, 'is-confirm': confirmingId === s.id }"
                :disabled="!!busyId"
                @click.stop="askRemove(s)"
              >
                <AppIcon :name="confirmingId === s.id ? 'check' : 'trash'" size="ic-sm" />
              </button>
            </span>
          </template>
        </button>
      </TransitionGroup>
    </div>
    <!-- 还有更多：每次追加一页，直到全部展示 -->
    <button v-if="hiddenCount > 0" class="more" type="button" @click="showMore">
      更多（还有 {{ hiddenCount }} 条）
    </button>
    <!-- 一条会话都没有（数据被清空 / 首次进入还没自动建会话）：给一个能往下走的出口 -->
    <div v-if="!session.list.length" class="sess-empty">
      <span class="se-ic"><AppIcon name="chat" size="ic-lg" /></span>
      <b>还没有对话</b>
      <p>说句话就能开一段新的。</p>
      <button class="btn btn-sm" type="button" @click="emit('newSession')">新建对话</button>
    </div>
  </div>
</template>

<style scoped>
/* 「更多」按钮：窄条铺满，淡底不抢会话行；五态与 .btn 同纪律 */
.more {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--wb-sp-2);
  height: var(--wb-ctl-h);
  border-radius: var(--wb-radius);
  border: 1px dashed var(--wb-border);
  color: var(--wb-muted);
  font-size: var(--wb-fs-xs);
  transition:
    color var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease),
    background var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.more:hover {
  color: var(--wb-primary);
  border-color: var(--wb-primary-line);
  background: var(--wb-primary-soft);
  transform: scale(1.02);
}
.more:active {
  transform: scale(0.97);
}

/* 会话行增删：淡出 + 缩掉，与全站列表增删同一语言。
   会话行是满宽的，悬停不放大（会溢出侧栏被裁），按下缩小 0.97 由基元负责。 */
.slist-enter-active,
.slist-leave-active {
  transition:
    opacity var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur) var(--wb-ease);
}
.slist-enter-from {
  opacity: 0;
  transform: translateX(-6px);
}
.slist-leave-to {
  opacity: 0;
  transform: scale(0.96);
}
/* 空态：图标瓦片 + 一句人话，与全站空态同一语言 */
.sess-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8) var(--wb-sp-4);
  text-align: center;
}
.sess-empty .se-ic {
  display: grid;
  place-items: center;
  width: var(--wb-tile);
  height: var(--wb-tile);
  border-radius: var(--wb-radius);
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
}
.sess-empty b {
  font-size: var(--wb-fs-md);
  font-weight: 600;
  color: var(--wb-ink);
}
.sess-empty p {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.rename {
  height: var(--wb-ctl-h-sm);
  font-size: var(--wb-fs-sm);
}
.more {
  display: block;
  width: 100%;
  margin: 6px 0 2px;
  padding: 6px 0;
  border: none;
  background: none;
  color: var(--wb-c-ink-3);
  font-size: var(--wb-fs-hint);
  cursor: pointer;
}
.more:hover:not(:disabled) {
  color: var(--wb-ink);
}
.more:active:not(:disabled) {
  transform: scale(0.97);
  color: var(--wb-ink);
}
.more:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}
/* 加载态：转圈 + 文字保留（只换文案不转圈不算数） */
.more.is-loading {
  opacity: 1;
  pointer-events: none;
  cursor: progress;
  position: relative;
}
.more.is-loading::before {
  content: '';
  display: inline-block;
  width: 10px;
  height: 10px;
  margin-right: 5px;
  border-radius: var(--wb-radius-full);
  border: 1.5px solid currentColor;
  border-top-color: transparent;
  animation: wb-ic-spin 0.8s linear infinite;
}
.icon-btn.danger:hover {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
/* 确认态：对勾 + 危险底，一眼看出「再点就真删了」 */
.icon-btn.danger.is-confirm {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
</style>
