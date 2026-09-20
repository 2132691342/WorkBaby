<script setup lang="ts">
/**
 * 任务中心面板（聊天右栏 tasks tab）：后台任务的提交、进度、取消与结果查看。
 *
 * 数据流：REST 权威列表（挂载即拉）+ SSE scope=task 实时事件（store 统一订阅）。
 * 提交即返回 pending；运行中可取消；终态可展开查看结果 / 失败原因。
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useTasksStore } from '@/stores/tasks'
import { useAgentProfilesStore } from '@/stores/agents'
import { useChatStore } from '@/stores/chat'
import { t } from '@/i18n'
import type { BackgroundTask } from '@/types/api'

const tasks = useTasksStore()
const agentsStore = useAgentProfilesStore()
const chat = useChatStore()
const { items, loading, submitting } = storeToRefs(tasks)

// ===== 提交表单 =====

const newAgent = ref('default')
const newPrompt = ref('')

/** Agent 选项：内置 default / explore + 自定义档案。 */
const agentOptions = computed(() => {
  const opts = [
    { value: 'default', label: t('tasks.agentDefault') },
    { value: 'explore', label: t('tasks.agentExplore') }
  ]
  for (const p of agentsStore.profiles) {
    if (!p.enabled) continue
    opts.push({ value: p.name, label: p.name })
  }
  return opts
})

async function submit(): Promise<void> {
  const prompt = newPrompt.value.trim()
  if (!prompt) return
  const row = await tasks.submit(chat.currentID ?? '', newAgent.value, prompt)
  if (row) newPrompt.value = ''
}

// ===== 展示辅助 =====

const expanded = ref<Record<string, boolean>>({})

function toggleExpand(id: string): void {
  expanded.value[id] = !expanded.value[id]
}

function stateLabel(state: BackgroundTask['state']): string {
  return t(`tasks.state.${state}`)
}

function stateClass(state: BackgroundTask['state']): string {
  switch (state) {
    case 'running': return 'badge b-info'
    case 'completed': return 'badge b-success'
    case 'failed': return 'badge b-danger'
    case 'cancelled': return 'badge b-neutral'
    default: return 'badge b-neutral'
  }
}

/** 已运行时长（running 实时性要求低：打开面板时刷新一次即可，不引入定时器）。 */
function elapsed(row: BackgroundTask): string {
  const end = row.finished_at || Date.now()
  const start = row.started_at || row.created_at
  const ms = Math.max(0, end - start)
  if (ms < 60_000) return `${Math.round(ms / 1000)}s`
  return `${Math.floor(ms / 60_000)}m${Math.round((ms % 60_000) / 1000)}s`
}

function fmtTime(ts: number): string {
  return ts ? new Date(ts).toLocaleTimeString() : '—'
}

onMounted(() => {
  tasks.ensureConnected()
  void tasks.load()
  if (agentsStore.profiles.length === 0) void agentsStore.load().catch(() => undefined)
})

onBeforeUnmount(() => {
  // SSE 由 store 常驻（通知不依赖面板）；这里只做组件级清理
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col text-xs">
    <!-- 提交表单 -->
    <div class="shrink-0 space-y-2 border-b border-wb-border p-3">
      <div class="flex items-center gap-2">
        <select v-model="newAgent" class="input h-7 flex-1 text-xs">
          <option v-for="o in agentOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
        <button
          type="button"
          class="btn btn-primary btn-sm shrink-0"
          :disabled="submitting || !newPrompt.trim()"
          @click="submit"
        >
          {{ t('tasks.submit') }}
        </button>
      </div>
      <textarea
        v-model="newPrompt"
        class="input h-16 resize-none text-xs"
        :placeholder="t('tasks.promptPlaceholder')"
        @keydown.enter.exact.prevent="submit"
      />
    </div>

    <!-- 列表 -->
    <div class="min-h-0 flex-1 overflow-y-auto p-2">
      <p v-if="loading && items.length === 0" class="px-2 py-6 text-center text-wb-muted">{{ t('ui.status.loading') }}</p>
      <p v-else-if="items.length === 0" class="px-2 py-6 text-center text-wb-muted">{{ t('tasks.empty') }}</p>

      <div v-for="tk in items" :key="tk.id" class="mb-2 rounded-md border border-wb-border bg-wb-surface p-2">
        <div class="mb-1 flex items-center gap-2">
          <span :class="stateClass(tk.state)">{{ stateLabel(tk.state) }}</span>
          <span class="font-medium text-wb-ink">{{ tk.agent }}</span>
          <span class="sp" />
          <span class="tabular-nums text-2xs text-wb-muted">{{ fmtTime(tk.created_at) }}</span>
          <button
            v-if="tk.state === 'pending' || tk.state === 'running'"
            type="button"
            class="btn btn-sm"
            :title="t('tasks.cancel')"
            @click="tasks.cancel(tk.id)"
          >
            {{ t('tasks.cancel') }}
          </button>
        </div>

        <p class="line-clamp-2 whitespace-pre-wrap break-all text-wb-ink">{{ tk.prompt }}</p>

        <!-- 运行中：已运行时长 -->
        <p v-if="tk.state === 'running'" class="mt-1 text-2xs text-wb-muted">
          {{ t('tasks.elapsed', elapsed(tk)) }}
        </p>

        <!-- 失败原因 / 结果：可展开 -->
        <template v-if="tk.state === 'failed' && tk.error">
          <p class="mt-1 whitespace-pre-wrap break-all text-xs2 text-wb-warning">{{ tk.error }}</p>
        </template>
        <template v-else-if="tk.result">
          <button
            type="button"
            class="mt-1 text-xs2 text-wb-primary"
            @click="toggleExpand(tk.id)"
          >
            {{ expanded[tk.id] ? t('tasks.collapse') : t('tasks.expandResult') }}
          </button>
          <p
            v-if="expanded[tk.id]"
            class="mt-1 max-h-48 overflow-y-auto whitespace-pre-wrap break-all rounded bg-wb-surface-2 p-2 text-xs2 text-wb-ink"
          >{{ tk.result }}</p>
        </template>
      </div>
    </div>
  </div>
</template>
