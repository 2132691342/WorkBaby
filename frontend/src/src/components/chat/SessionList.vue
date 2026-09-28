<script setup lang="ts">
// 侧栏会话列表：分组展示 + 重命名 / 删除。
import { computed, ref } from 'vue'
import { useSessionStore } from '../../stores/session'
import AppIcon from '../common/AppIcon.vue'

const session = useSessionStore()
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
  if (editingId.value && draft.value.trim()) {
    await session.rename(editingId.value, draft.value.trim())
  }
  editingId.value = null
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
            <span class="icon-btn" title="删除" @click.stop="session.remove(s.id)">
              <AppIcon name="close" size="ic-sm" />
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
</style>
