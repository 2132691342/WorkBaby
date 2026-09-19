<script setup lang="ts">
import { computed } from 'vue'
import { t } from '@/i18n'
import type { SessionGoal } from '@/types/api'
import { useChatStore } from '@/stores/chat'
import { useToast } from '@/composables/useToast'

/**
 * 目标模式状态卡：悬于聊天区右上角，展示目标描述、
 * 自动推进轮数与下一步动作；暂停/完成态颜色区分，支持暂停 / 清除。
 * 目标由 /goal 斜杠命令管理，状态经 chat:goal 事件与权威拉取双通道同步。
 */
const props = defineProps<{ goal: SessionGoal }>()

const chat = useChatStore()
const toast = useToast()

const maxRounds = computed(() => props.goal.max_rounds || 20)
/** 状态徽标：active 靛蓝 / paused 琥珀 / done 薄荷绿。 */
const statusKey = computed(() =>
  props.goal.status === 'done'
    ? 'chat.goal.statusDone'
    : props.goal.status === 'paused'
      ? 'chat.goal.statusPaused'
      : 'chat.goal.statusActive'
)
const statusClass = computed(() => props.goal.status)

async function pause(): Promise<void> {
  const id = chat.currentID
  if (!id) return
  try {
    await chat.updateGoal(id, 'pause')
  } catch (e) {
    toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
  }
}

async function clear(): Promise<void> {
  const id = chat.currentID
  if (!id) return
  try {
    await chat.updateGoal(id, 'clear')
    toast.success(t('chat.goal.cleared'))
  } catch (e) {
    toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
  }
}

async function resume(): Promise<void> {
  const id = chat.currentID
  if (!id) return
  try {
    await chat.updateGoal(id, 'resume')
  } catch (e) {
    toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
  }
}
</script>

<template>
  <div class="goal-card" :class="statusClass">
    <div class="flex items-center gap-1.5">
      <span class="text-[13px]">🎯</span>
      <span class="text-[11px] font-semibold text-wb-ink">{{ t('chat.goal.title') }}</span>
      <span class="goal-status" :class="statusClass">{{ t(statusKey) }}</span>
      <span class="ml-auto text-[10.5px] tabular-nums text-wb-muted">
        {{ t('chat.goal.round', goal.round, maxRounds) }}
      </span>
      <button v-if="goal.status === 'active'" class="goal-act" :title="t('chat.goal.pause')" @click="pause">
        ⏸
      </button>
      <button
        v-else-if="goal.status === 'paused'"
        class="goal-act"
        :title="t('chat.goal.resume')"
        @click="resume"
      >
        ▶
      </button>
      <button class="goal-act" :title="t('chat.goal.clear')" @click="clear">✕</button>
    </div>
    <p class="mt-1 text-[12px] leading-[1.6] text-wb-ink">{{ goal.text }}</p>
    <p v-if="goal.status === 'active' && goal.next_step" class="mt-1 text-[11px] leading-[1.6] text-wb-muted">
      {{ t('chat.goal.nextStep') }}{{ goal.next_step }}
    </p>
    <p v-if="goal.status === 'done' && goal.done_because" class="mt-1 text-[11px] leading-[1.6] text-wb-mint">
      {{ goal.done_because }}
    </p>
  </div>
</template>

<style scoped>
.goal-card {
  position: absolute;
  top: 12px;
  right: 16px;
  z-index: 20;
  width: 300px;
  padding: 10px 12px;
  border-radius: var(--wb-radius-lg, 12px);
  border: 1px solid var(--wb-border);
  background: var(--wb-surface);
  box-shadow: var(--wb-shadow-lg);
}
.goal-status {
  padding: 0 6px;
  height: 16px;
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  font-size: 10px;
  font-weight: 600;
}
.goal-status.active {
  background: var(--wb-primary-soft);
  color: var(--wb-primary-strong);
}
.goal-status.paused {
  background: color-mix(in srgb, var(--wb-warning) 15%, transparent);
  color: var(--wb-warning);
}
.goal-status.done {
  background: color-mix(in srgb, var(--wb-mint) 15%, transparent);
  color: var(--wb-mint);
}
.goal-act {
  width: 20px;
  height: 20px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  font-size: 10px;
  color: var(--wb-muted);
}
.goal-act:hover {
  background: var(--wb-surface-hover);
  color: var(--wb-ink);
}
</style>
