<script setup lang="ts">
// 活动条：输入框上方常驻一行，回答「现在到底在执行，还是在卡着」。
// 流式增量一阵一阵到，两阵之间界面纹丝不动；轮内脚标会随消息滚走，
// 用户往上翻内容时屏幕上就只剩一个停止按钮。这一条位置固定，读数与轮内脚标同源。
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import AppIcon from '../common/AppIcon.vue'

const { t } = useI18n()
const chat = useChatStore()
const session = useSessionStore()

/** 只有真的在跑时才出现：空闲时它不占一行高度。 */
const active = computed(() => chat.running || chat.finalizing)

/** 「安静很久」的阈值：一个工具跑几十秒是常态，把它当卡住会让提示变成噪声。 */
const STALL_MS = 45_000

const now = ref(Date.now())
let timer = 0
watch(
  active,
  (on) => {
    clearInterval(timer)
    timer = 0
    if (!on) return
    now.value = Date.now()
    timer = window.setInterval(() => (now.value = Date.now()), 1000)
  },
  { immediate: true },
)
onBeforeUnmount(() => clearInterval(timer))

/** 已经安静了多久（毫秒）：没有起点或还没动过时为 0。 */
const quietMs = computed(() => {
  if (!chat.lastActivityAt) return 0
  return Math.max(0, now.value - chat.lastActivityAt)
})

const stalled = computed(() => active.value && !chat.finalizing && quietMs.value > STALL_MS)

/** 当前正在跑的工具：取最后一个还在 running 的记录。 */
const liveTool = computed(() => {
  const running = chat.runs.filter((r) => r.running)
  return running.length ? running[running.length - 1] : null
})

/**
 * 阶段文案。优先级：收尾中 > 安静提示 > 引擎状态文字 > 正在跑的工具 > 流式正文。
 * 安静提示排在阶段文字前面：它是唯一一条「现在可能不对」的话，被盖住就没人看见。
 */
const phase = computed(() => {
  if (chat.finalizing) return t('chat.phaseFinalizing')
  if (stalled.value) return t('chat.phaseStalled')
  const statuses = Object.values(chat.engineStatus)
  if (statuses.length) return statuses[0]
  if (liveTool.value) return t('chat.phaseTool', { name: liveTool.value.label || liveTool.value.tool })
  if (chat.streaming.length) return t('chat.phaseStreaming')
  if (chat.thinking.length) return t('chat.phaseThinking')
  return t('chat.phaseWaiting')
})

const elapsed = computed(() => {
  const s = chat.elapsedSec
  if (!s) return ''
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m${s % 60}s`
})

/** 这一轮已经吐了多少字：读数给「它确实在干活」一个具体证据。 */
const chars = computed(() => chat.streaming.length + chat.thinking.length)

const model = computed(() => session.current?.session_model || '')
</script>

<template>
  <Transition name="act-slide">
    <div v-if="active" class="act" :class="{ 'is-stalled': stalled }">
      <span class="act-dot" aria-hidden="true" />
      <span class="act-phase">{{ phase }}</span>
      <span v-if="model" class="act-sep">·</span>
      <span v-if="model" class="act-model" :title="model">{{ model }}</span>
      <span class="act-sp" />
      <span v-if="chars" class="act-num" :title="t('chat.phaseChars')">
        <AppIcon name="sparkles" size="ic-2xs" />
        {{ chars }}
      </span>
      <span v-if="elapsed" class="act-num">
        <AppIcon name="clock" size="ic-2xs" />
        {{ elapsed }}
      </span>
    </div>
  </Transition>
</template>

<style scoped>
.act {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: none;
  height: calc(26px * var(--wb-vw-scale));
  padding: 0 var(--wb-sp-5);
  border-top: 1px solid var(--wb-border);
  background: var(--wb-elev-workspace);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
  min-width: 0;
}
.act-dot {
  width: 6px;
  height: 6px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-live);
  flex: none;
  animation: act-breathe 1.5s var(--wb-ease) infinite;
}
.is-stalled .act-dot {
  background: var(--wb-warning);
  animation-duration: 3s;
}
.is-stalled .act-phase {
  color: var(--wb-warning);
}
@keyframes act-breathe {
  0%,
  100% {
    opacity: 0.35;
  }
  50% {
    opacity: 1;
  }
}
.act-phase {
  font-weight: 700;
  color: var(--wb-ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.act-sep {
  opacity: 0.5;
}
.act-model {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: calc(220px * var(--wb-vw-scale));
}
.act-sp {
  flex: 1;
  min-width: var(--wb-sp-2);
}
.act-num {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex: none;
}
.act-slide-enter-active,
.act-slide-leave-active {
  transition:
    opacity var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur) var(--wb-ease);
}
.act-slide-enter-from,
.act-slide-leave-to {
  opacity: 0;
  transform: translateY(4px);
}
</style>
