<script setup lang="ts">
// 侧栏会话列表：分组展示 + 重命名 / 删除。
import { computed, ref } from 'vue'
import { useSessionStore } from '../../stores/session'
import { useToastStore } from '../../stores/toast'
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

// 单击直接删：图标按钮上做两段式确认，用户只会觉得「点了没反应」。
// 删除结果必须立刻可见——当前会话被删时由 store 自动切到下一个。
async function doRemove(s: { id: string; title: string }) {
  try {
    await session.remove(s.id)
    toast.ok(`已删除「${s.title || '未命名对话'}」`)
  } catch (e) {
    toast.bad(`删除失败：${(e as Error)?.message || '请重试'}`)
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
          <span class="si-act">
            <span class="icon-btn" title="重命名" @click.stop="startEdit(s.id, s.title)">
              <AppIcon name="pencil" size="ic-sm" />
            </span>
            <span class="icon-btn danger" title="删除" @click.stop="doRemove(s)">
              <AppIcon name="trash" size="ic-sm" />
            </span>
          </span>
        </template>
      </button>
    </div>
  </div>
</template>

<style scoped>
.rename {
  height: var(--wb-ctl-h-sm);
  font-size: var(--wb-fs-sm);
}
.icon-btn.danger:hover {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
</style>
