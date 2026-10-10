<script setup lang="ts">
// 侧栏会话列表：分组展示 + 相对时间 + 重命名 / 删除。
import { computed, ref } from 'vue'
import { useSessionStore } from '../../stores/session'
import { useToastStore } from '../../stores/toast'
import { fmtRel } from '../../utils/time'
import AppIcon from '../common/AppIcon.vue'

const session = useSessionStore()
const toast = useToastStore()
const editingId = ref<string | null>(null)
const draft = ref('')

const groups = computed(() => {
  const today: typeof session.list = []
  const week: typeof session.list = []
  const earlier: typeof session.list = []
  const dayStart = new Date()
  dayStart.setHours(0, 0, 0, 0)
  const weekAgo = dayStart.getTime() - 6 * 86400_000
  for (const s of session.list) {
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

// 一次性加载全部旧会话既慢又没必要；超过一页时给一个显式的入口，
// 否则用了几个月后旧对话在界面上凭空消失（数据其实还在）。
const loadingMore = ref(false)
async function more() {
  loadingMore.value = true
  try {
    await session.loadMore()
  } catch (e) {
    toast.bad(`加载更多失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    loadingMore.value = false
  }
}
</script>

<template>
  <div class="sess-scroll">
    <div v-for="g in groups" :key="g.name" class="sess-group">
      <div class="n">{{ g.name }}</div>
      <button
        v-for="s in g.items"
        :key="s.id"
        class="sess-item"
        :class="{ 'is-on': s.id === session.currentId }"
        type="button"
        @click="session.open(s.id)"
      >
        <template v-if="editingId === s.id">
          <input v-model="draft" class="input rename" autofocus @keydown.enter="commitEdit" @blur="commitEdit" />
        </template>
        <template v-else>
          <span class="si-name">{{ s.title || '未命名对话' }}</span>
          <span class="si-time">{{ fmtRel(s.updated_at) }}</span>
          <span class="si-act">
            <button
              class="icon-btn"
              type="button"
              title="重命名"
              :disabled="!!busyId"
              @click.stop="startEdit(s.id, s.title)"
            >
              <AppIcon name="pencil" size="ic-sm" />
            </button>
            <button
              class="icon-btn danger"
              type="button"
              :title="confirmingId === s.id ? '再点一次确认删除' : '删除'"
              :class="{ 'is-loading': busyId === s.id, 'is-confirm': confirmingId === s.id }"
              :disabled="!!busyId"
              @click.stop="askRemove(s)"
            >
              <AppIcon :name="confirmingId === s.id ? 'check' : 'trash'" size="ic-sm" />
            </button>
          </span>
        </template>
      </button>
    </div>
    <button
      v-if="session.hasMore"
      class="more"
      type="button"
      :class="{ 'is-loading': loadingMore }"
      :disabled="loadingMore"
      @click="more"
    >
      {{ loadingMore ? '加载中…' : '加载更早的对话' }}
    </button>
  </div>
</template>

<style scoped>
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
