<script setup lang="ts">
import { Layers, Pencil, Trash2, Pause, Play, Send, ArrowUp, ArrowDown } from '@/components/common/icons'
import { t } from '@/i18n'

/**
 * 轮次中队列：助手流式回复期间用户可继续输入，消息入队并在当前轮结束后自动出队发送。
 *
 * <p>队列持久化（导航 / 刷新不丢，由 ChatInput 负责 store 化），支持暂停自动出队、
 * 逐条编辑、上下排序与立即发送；排序用上下键而非拖拽，零额外依赖。
 */
export interface QueuedMessage {
  id: string
  text: string
}

const props = defineProps<{
  queue: QueuedMessage[]
  /** true = 已暂停自动出队（手动「继续」才恢复 flush）。 */
  paused?: boolean
}>()

const emit = defineEmits<{
  remove: [id: string]
  edit: [id: string, text: string]
  move: [id: string, dir: -1 | 1]
  'toggle-pause': []
  flush: []
}>()

function canMove(idx: number, dir: -1 | 1): boolean {
  const j = idx + dir
  return j >= 0 && j < props.queue.length
}
</script>

<template>
  <div v-if="queue.length > 0" class="flex flex-col gap-1.5">
    <div class="flex items-center justify-between px-1 text-[10px] text-wb-muted">
      <span class="inline-flex items-center gap-1">
        <el-icon :size="11"><Layers /></el-icon>
        {{ t('queue.label', queue.length) }}
        <span v-if="paused" class="rounded border border-wb-border px-1 py-0 text-[9px]">{{ t('queue.paused') }}</span>
      </span>
      <span class="inline-flex items-center gap-1">
        <el-button text size="small" @click="emit('toggle-pause')">
          <el-icon class="mr-0.5"><component :is="paused ? Play : Pause" /></el-icon>
          {{ paused ? t('queue.resume') : t('queue.pause') }}
        </el-button>
        <el-button v-if="!paused" text size="small" @click="emit('flush')">
          <el-icon class="mr-0.5"><Send /></el-icon>
          {{ t('queue.sendNow') }}
        </el-button>
      </span>
    </div>
    <div class="flex flex-col gap-1 px-1">
      <div
        v-for="(m, idx) in queue"
        :key="m.id"
        class="group/q flex items-center gap-1.5 rounded-md border border-wb-border bg-wb-surface px-2 py-1"
      >
        <span class="min-w-0 flex-1 truncate text-xs text-wb-ink" :title="m.text">{{ m.text }}</span>
        <!-- 编辑 / 排序 / 删除：hover 显现（与消息 hover 操作同模式） -->
        <span class="hidden shrink-0 items-center gap-1 group-hover/q:flex">
          <button type="button" class="cursor-pointer text-wb-muted hover:text-wb-primary" :title="t('queue.edit')" @click="emit('edit', m.id, m.text)">
            <el-icon :size="12"><Pencil /></el-icon>
          </button>
          <button type="button" class="cursor-pointer text-wb-muted hover:text-wb-primary disabled:opacity-30" :disabled="!canMove(idx, -1)" :title="t('queue.moveUp')" @click="canMove(idx, -1) && emit('move', m.id, -1)">
            <el-icon :size="12"><ArrowUp /></el-icon>
          </button>
          <button type="button" class="cursor-pointer text-wb-muted hover:text-wb-primary disabled:opacity-30" :disabled="!canMove(idx, 1)" :title="t('queue.moveDown')" @click="canMove(idx, 1) && emit('move', m.id, 1)">
            <el-icon :size="12"><ArrowDown /></el-icon>
          </button>
          <button type="button" class="cursor-pointer text-wb-muted hover:text-wb-danger" :title="t('common.delete')" @click="emit('remove', m.id)">
            <el-icon :size="12"><Trash2 /></el-icon>
          </button>
        </span>
      </div>
    </div>
  </div>
</template>
