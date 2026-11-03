<script setup lang="ts">
// 输入区上方的活动条：一行回答「到底在执行、还是在卡着」。
//
// 为什么要有它：流式的增量一阵一阵到，两阵之间界面纹丝不动；
// 而轮内脚标会随消息滚走——用户往上翻代码时，屏幕上就只剩一个「停止」按钮。
// 这一条常驻在输入框上方，位置固定，读数与轮内脚标同一份数据。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { useChatStore } from '@/stores/chat'
import { useTurnClock } from '@/composables/useTurnClock'

const props = defineProps<{
  /** 停止按钮的 loading 态由输入区持有 */
  stopping: boolean
}>()

const emit = defineEmits<{ stop: [] }>()

const chat = useChatStore()
const { t } = useI18n()

// 只有真的在跑时才出现：空闲时它不占一行高度。
const active = computed(() => chat.streaming || chat.compacting)

const { now, label: elapsed } = useTurnClock(
  () => chat.streaming,
  () => chat.startedAt,
)

/** 安静多久了（毫秒）。没有起点或还没动过时为 0。 */
const quietMS = computed(() => {
  if (!chat.lastActivityAt || !now.value) return 0
  return Math.max(0, now.value - chat.lastActivityAt)
})

/**
 * 「安静很久」的阈值。
 *
 * 45 秒选得很保守：一个工具跑上几十秒是常态（测试 / 构建），
 * 把它当成卡住会让这条提示变成噪声；而真的卡住时 45 秒已经足够让人怀疑。
 */
const STALL_MS = 45_000

/** 是不是安静得可疑。 */
const stalled = computed(() => chat.streaming && !chat.compacting && quietMS.value > STALL_MS)

/**
 * 当前阶段写什么。
 *
 * 优先级：压缩 > 安静提示 > 扩展状态文字 > 正在跑的工具 > 引擎阶段。
 * 安静提示排在前头：它是唯一一条「现在可能不对」的话，被阶段文字盖住就没人看得到。
 */
const phaseLabel = computed(() => {
  if (chat.compacting) return t('chat.compacting')
  if (stalled.value) return t('chat.liveStalled')
  if (chat.statusText) return chat.statusText
  const live = chat.live
  if (live.phase === 'tool' && live.tool_name) return t('chat.liveTool', { name: live.tool_name })
  if (live.phase === 'thinking') return t('chat.thinkingLive')
  return t('chat.liveAnswering')
})

/** 正在跑的工具名：单独一段，等宽显示，方便一眼认出是哪条命令。 */
const toolName = computed(() =>
  chat.statusText || chat.compacting || stalled.value ? '' : chat.live.tool_name,
)

/** 已经吐出的 token。0 不显示：刚开局那个 0 没有信息量。 */
const outputTokens = computed(() => chat.live.output_tokens)

/** 生成速度：引擎给就显示，不给不占位。 */
const speed = computed(() => {
  const tps = chat.tokensPerSecond
  return tps && tps > 0 ? `${tps.toFixed(1)} tok/s` : ''
})

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(n >= 100_000 ? 0 : 1)}k`
  return String(n)
}
</script>

<template>
  <Transition name="act">
    <div v-if="active" class="activity" :class="{ 'is-compacting': chat.compacting, 'is-stalled': stalled }">
      <span class="act-dot" :class="{ busy: chat.compacting, stalled }" />

      <span class="act-phase">{{ phaseLabel }}</span>

      <code v-if="toolName" class="act-tool">{{ toolName }}</code>

      <span class="act-read">
        <span v-if="speed">{{ speed }}</span>
        <span v-if="outputTokens">↑ {{ fmtTokens(outputTokens) }}</span>
        <span v-if="elapsed && !chat.compacting" class="act-clock">{{ elapsed }}</span>
      </span>

      <!-- 压缩没有 abort 入口：那是引擎自己的动作，中途停下只会留下半个会话。 -->
      <button
        v-if="chat.streaming"
        class="wb-btn sm ghost act-stop"
        :class="{ 'is-loading': props.stopping }"
        :disabled="props.stopping"
        :title="t('chat.stop')"
        @click="emit('stop')"
      >
        <AppIcon name="stop" :size="12" />
        {{ t('chat.stop') }}
      </button>
    </div>
  </Transition>
</template>

<style scoped>
/* 与输入区同宽同列：它属于「这一句话现在在干什么」，不是消息流的一部分。 */
.activity {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  max-width: var(--wb-col);
  margin: 0 auto var(--wb-sp-2);
  padding: 3px var(--wb-sp-2) 3px var(--wb-sp-3);
  border: 1px solid var(--wb-live-line);
  border-radius: var(--wb-r-full);
  background: var(--wb-live-soft);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  min-height: var(--wb-ctl-sm);
}

/* 压缩不是「一轮在跑」，用中性线：它由引擎自己发起，不能停。 */
.activity.is-compacting {
  border-color: var(--wb-line-strong);
  background: var(--wb-surface);
}

/* 安静得可疑：转警告色。不弹窗、不打断——引擎可能只是在跑一个长构建，
   这一行只负责把「这么久没动静」说出来。 */
.activity.is-stalled {
  border-color: var(--wb-warn);
  background: var(--wb-warn-soft);
}

.act-dot {
  width: 7px;
  height: 7px;
  flex: none;
  border-radius: var(--wb-r-full);
  background: var(--wb-live);
  animation: wb-pulse 1.6s var(--wb-ease) infinite;
}

/* 安静时不呼吸：转成常亮警告色。呼吸是「还在动」的表达，一直呼吸会骗人。 */
.act-dot.stalled {
  background: var(--wb-warn);
  animation: none;
}

.act-phase {
  flex: none;
  max-width: 40%;
  font-weight: 600;
  color: var(--wb-live);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.activity.is-compacting .act-phase {
  color: var(--wb-mute);
}

.activity.is-stalled .act-phase {
  color: var(--wb-warn);
}

/* 工具名是这一行最重要的信息之一：等宽字、允许被挤压、不换行。 */
.act-tool {
  flex: 0 1 auto;
  min-width: 0;
  padding: 1px 6px;
  border-radius: var(--wb-r-xs);
  background: var(--wb-surface);
  border: 1px solid var(--wb-line);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 读数不等于控件：不套描边、不做悬停缩放，只走字色。 */
.act-read {
  display: inline-flex;
  align-items: baseline;
  gap: var(--wb-sp-2);
  margin-left: auto;
  flex: none;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
  font-variant-numeric: tabular-nums;
}

.act-clock {
  min-width: 3em;
  text-align: right;
  color: var(--wb-ink-2);
  font-weight: 600;
}

.act-stop {
  flex: none;
}

/* 入场：从下方浮上来一点，别做整块高度跳变——它上方是消息流。 */
.act-enter-active,
.act-leave-active {
  transition:
    opacity var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur) var(--wb-ease);
}

.act-enter-from,
.act-leave-to {
  opacity: 0;
  transform: translateY(4px);
}
</style>
