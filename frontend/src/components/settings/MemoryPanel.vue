<script setup lang="ts">
// 记忆面板：写一条 → 之后的对话自动带上；助手在对话里说「记住…」也会写到这里。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import type { MemoryVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import PageState from '../common/PageState.vue'

const toast = useToastStore()
const items = ref<MemoryVO[]>([])
const loading = ref(true)
const error = ref('')
// 忙碌标识带动作与目标：转圈只出现在真正在跑的那个按钮上
const busy = ref('')
const acting = computed(() => busy.value !== '')

const draft = ref('')
const editing = ref('')
const editText = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const r = await api.memories.list()
    items.value = r.items ?? []
  } catch (e) {
    error.value = (e as Error)?.message || '读不到记忆，稍后再试'
  } finally {
    loading.value = false
  }
}

async function add() {
  const text = draft.value.trim()
  if (!text || acting.value) return
  busy.value = 'add'
  error.value = ''
  try {
    const m = await api.memories.add(text)
    items.value = [m, ...items.value.filter((x) => x.id !== m.id)]
    draft.value = ''
    toast.ok('记住了，之后的对话会带上')
  } catch (e) {
    error.value = (e as Error)?.message || '没记住，稍后再试'
  } finally {
    busy.value = ''
  }
}

function startEdit(m: MemoryVO) {
  editing.value = m.id
  editText.value = m.content
}

async function commitEdit(id: string) {
  const text = editText.value.trim()
  if (!text) return
  busy.value = `edit:${id}`
  error.value = ''
  try {
    const m = await api.memories.update(id, { content: text })
    replace(m)
    editing.value = ''
    toast.ok('已改好')
  } catch (e) {
    error.value = (e as Error)?.message || '没改成，稍后再试'
  } finally {
    busy.value = ''
  }
}

async function toggle(m: MemoryVO) {
  busy.value = `toggle:${m.id}`
  error.value = ''
  try {
    replace(await api.memories.update(m.id, { enabled: !m.enabled }))
  } catch (e) {
    error.value = (e as Error)?.message || '没改成功，稍后再试'
  } finally {
    busy.value = ''
  }
}

// 删除两段式确认：第一次点进入确认态，3 秒不点自动还原
const confirming = ref('')
let confirmTimer = 0
function askRemove(m: MemoryVO) {
  if (confirming.value === m.id) {
    confirming.value = ''
    void remove(m.id)
    return
  }
  confirming.value = m.id
  clearTimeout(confirmTimer)
  confirmTimer = window.setTimeout(() => (confirming.value = ''), 3000)
}

async function remove(id: string) {
  busy.value = `remove:${id}`
  error.value = ''
  try {
    await api.memories.remove(id)
    items.value = items.value.filter((x) => x.id !== id)
    toast.ok('已忘掉这条')
  } catch (e) {
    error.value = (e as Error)?.message || '没删掉，稍后再试'
  } finally {
    busy.value = ''
  }
}

function replace(m: MemoryVO) {
  const i = items.value.findIndex((x) => x.id === m.id)
  if (i >= 0) items.value[i] = m
}

onMounted(load)
</script>

<template>
  <div class="mem">
    <div class="add">
      <input
        v-model="draft"
        class="input"
        placeholder="写一条想让助手一直记住的事，例如：报表标题都用深蓝色"
        @keydown.enter="add"
      />
      <button
        class="btn btn-primary"
        type="button"
        :class="{ 'is-loading': busy === 'add' }"
        :disabled="!draft.trim() || acting"
        @click="add"
      >
        <AppIcon name="plus" size="ic-xs" /> 添加
      </button>
    </div>

    <p class="hint">
      助手在对话里听到「记住…」也会写到这里。停用的条目不会带进对话，但不会被删掉。
    </p>

    <p v-if="error" class="alert a-warn">
      <AppIcon name="info" />
      <div>{{ error }}</div>
    </p>

    <PageState
      :loading="loading"
      :error="''"
      :empty="!items.length"
      empty-title="还没有记住任何事"
      empty-sub="在上面写一条，或者在对话里对助手说「记住…」"
    >
      <div class="rowlist">
        <div v-for="m in items" :key="m.id" class="rli">
          <div class="grow">
            <input
              v-if="editing === m.id"
              v-model="editText"
              class="input"
              @keydown.enter="commitEdit(m.id)"
              @keydown.esc="editing = ''"
            />
            <p
              v-else
              class="mem-tx"
              :class="{ 'is-off': !m.enabled }"
              title="双击可以改"
              @dblclick="startEdit(m)"
            >
              {{ m.content }}
            </p>
          </div>
          <div class="flex-r nowrap">
            <span class="tag">{{ m.source === 'auto' ? '助手记的' : '我写的' }}</span>
            <span class="tag" :class="{ 'b-success': m.enabled }">
              {{ m.enabled ? '生效中' : '已停用' }}
            </span>
            <button
              class="btn btn-sm btn-ghost"
              type="button"
              :class="{ 'is-loading': busy === `toggle:${m.id}` }"
              :disabled="acting"
              @click="toggle(m)"
            >
              {{ m.enabled ? '停用' : '启用' }}
            </button>
            <button
              class="btn btn-sm btn-danger-ghost"
              type="button"
              :class="{ 'is-loading': busy === `remove:${m.id}` }"
              :disabled="acting"
              @click="askRemove(m)"
            >
              {{ confirming === m.id ? '确认删除' : '删除' }}
            </button>
          </div>
        </div>
      </div>
    </PageState>
  </div>
</template>

<style scoped>
.mem {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  min-width: 0;
}
.add {
  display: flex;
  gap: var(--wb-sp-2);
  min-width: 0;
}
.add .input {
  flex: 1;
  min-width: 0;
}
.hint {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.mem-tx {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
  word-break: break-word;
}
/* 停用态要一眼看出来：只降透明度不够，得落到次级文字色上 */
.mem-tx.is-off {
  color: var(--wb-muted);
  text-decoration: line-through;
}
</style>
