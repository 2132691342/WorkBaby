<script setup lang="ts">
// 一轮消息：角色头 + 若干块（正文 / 思考 / 工具卡）+ 轮尾读数。
// 同一角色连续几轮只出一次头——一次回答被切成几块，读的人得自己拼。
// 轮尾给读数（输入 / 输出 / 缓存 / 耗时）与复制、删除，答案不藏进菜单。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { fmtCount } from '@/utils/format'
import TurnBlocks from '@/components/chat/TurnBlocks.vue'
import MarkdownBlock from '@/components/chat/MarkdownBlock.vue'
import TurnChanges from '@/components/chat/TurnChanges.vue'
import TurnFooter from '@/components/chat/TurnFooter.vue'
import AppIcon from '@/components/common/AppIcon.vue'
import type { TurnReading } from '@/composables/useMessageFlow'
import { useTurnClock } from '@/composables/useTurnClock'
import { useChatStore } from '@/stores/chat'
import type { ChatBlock, EngineKind, HistoryTurn } from '@/types/api'

const props = defineProps<{
  /** 这一轮的角色 */
  role: string
  /** 这一轮的若干块 */
  blocks: ChatBlock[]
  /** 稳定前缀，TurnBlocks 用它做增量渲染的缓存键 */
  runId: string
  /** 模型名：只在出头的那一轮显示，接着往下说时不必重复 */
  model?: string
  /** 流式渲染最后一块 */
  streaming?: boolean
  /** 上一轮是不是同一个角色：为真时不出头、不给上外边距；由父组件算好传入（列表里还有非消息的行） */
  continued: boolean
  /** 流式进行中的当前轮 */
  live?: boolean
  /** 这一轮出错了 */
  errored?: boolean
  /** 这一轮的用量与耗时（助手轮才有）：流式轮从流里取、历史轮从后端投影取，两种字段名 usageFrom 都认 */
  usage?: TurnReading
  /** 历史轮自带的用量（后端投影带下来的） */
  historyTurn?: HistoryTurn
  /** 这一轮的隐藏标识：有它才能「删除」（直播轮没有，因此不可删） */
  hid?: string
  /** 直播这一轮的开始时刻（毫秒时间戳）：脚标的计时从它起走 */
  startedAt?: number
  /** 要不要显示用量读数：只在最后一轮显示，一次回答里的多条助手消息已由后端投影并成一轮 */
  showMetrics?: boolean
}>()

const { t } = useI18n()
const chat = useChatStore()

const isUser = computed(() => props.role === 'user')

// 直播脚标「正在回答 · 12s」：增量是一阵一阵到的，两阵之间秒表还在走，
// 用户才分得清「在执行」不是「卡住了」。
const liveActive = computed(() => Boolean(props.live) && Boolean(props.streaming))

// 秒表与活动条共用同一份实现与同一个起点：两处显示的时间必须一致。
const { label: liveElapsed } = useTurnClock(
  () => liveActive.value,
  () => props.startedAt ?? 0,
)

// 脚标文字的优先级：扩展设的状态文字 > 实时阶段 > 兜底——扩展的更具体，阶段是引擎自己的事实。
const liveLabel = computed(() => {
  if (chat.statusText) return chat.statusText
  const live = chat.live
  if (live.phase === 'tool' && live.tool_name) return t('chat.liveTool', { name: live.tool_name })
  if (live.phase === 'thinking') return t('chat.thinkingLive')
  return t('chat.liveAnswering')
})

/** 已经吐出的输出 token：实时读数优先，它比用量帧更早到。 */
const liveOut = computed(() => chat.live.output_tokens || props.usage?.output_tokens || 0)

/** 引擎报的生成速度：由相邻两帧 token 读数差出来，0 不显示。 */
const liveSpeed = computed(() => {
  const n = chat.live.tokens_per_s
  return n > 0 ? `${fmtCount(n)}${t('chat.tokensPerSec')}` : ''
})

/** 切引擎的分隔行：它不是消息，只是界面层的标记。 */
const handoffEngine = computed<EngineKind | ''>(() =>
  props.role === 'engine-mark' ? ((props.blocks[0]?.block_text ?? '') as EngineKind) : '',
)

/** 这一轮的纯文本（复制用）。 */
const plainText = computed(() =>
  props.blocks
    .filter((b) => b.block_kind === 'text' && b.block_text)
    .map((b) => b.block_text)
    .join('\n\n'),
)

// 出头行的时刻脚标：直播轮从秒表起点取「什么时候开始问的」，
// 历史轮后端投影没带时间戳，留空不编造——悬停里的删除/分叉不依赖它。
const turnStamp = computed(() => {
  if (liveActive.value && props.startedAt) {
    const d = new Date(props.startedAt)
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${pad(d.getHours())}:${pad(d.getMinutes())}`
  }
  return ''
})

</script>

<template>
  <!-- 切引擎分隔线：说明下面的内容来自另一个引擎、另一条会话 -->
  <div v-if="handoffEngine" class="mark-line">
    <span class="mark-text">{{ t('chat.engineHandoff', { engine: handoffEngine }) }}</span>
  </div>

  <article
    v-else
    class="msg"
    :class="{
      user: isUser,
      'is-assistant': !isUser,
      'is-continued': props.continued,
      'msg-streaming': props.live,
      'msg-error': props.errored,
    }"
  >
    <div v-if="!props.continued" class="msg-head">
      <span class="msg-avatar" :class="{ user: isUser }" aria-hidden="true">
        <AppIcon :name="isUser ? 'chat' : 'spark'" :size="14" />
      </span>
      <span class="msg-badge" :class="{ user: isUser }">{{ isUser ? t('chat.roleYou') : t('chat.roleAssistant') }}</span>
      <span v-if="props.model" class="msg-meta">{{ props.model }}</span>
      <span v-if="turnStamp" class="msg-time">{{ turnStamp }}</span>
    </div>
    <div class="msg-body">
      <!-- 用户轮只有一段纯文本，不需要 TurnBlocks 那套工具卡分派 -->
      <MarkdownBlock
        v-if="isUser"
        :text="props.blocks[0]?.block_text ?? ''"
        :streaming="false"
        :cache-key="`${props.runId}:user`"
      />
      <TurnBlocks v-else :blocks="props.blocks" :run-id="props.runId" :streaming="props.streaming" :started-at="props.startedAt" />

      <!-- 这轮改了哪些文件：从工具块就地聚合，点击去工作区页看差异 -->
      <TurnChanges :blocks="props.blocks" />

      <!-- 直播脚标：呼吸点 + 状态 + 秒表。增量一阵一阵到，
           两阵之间就让这一行替流「证明自己还活着」。 -->
      <div v-if="liveActive" class="live-foot">
        <span class="live-dot" />
        <span class="live-label">{{ liveLabel }}</span>
        <span class="live-read">
          <span v-if="liveSpeed">{{ liveSpeed }}</span>
          <span v-if="liveOut">↑ {{ fmtCount(liveOut) }}</span>
          <span v-if="liveElapsed" class="live-elapsed">{{ liveElapsed }}</span>
        </span>
      </div>

      <!-- 轮尾：读数 + 复制 + 删除。悬停整个回合才浮现，平时不占位置。
           用户侧也有复制：用户要拿自己刚发的那句去别处贴，
           不该先去输入框里选中再复制。删除只对历史轮开放（有 hid 才删得掉），
           直播中的轮次先让它把话说完。 -->
      <!-- 轮尾：读数 + 复制 / 分叉 / 隐藏，全在 TurnFooter 里（含它自己的瞬时态） -->
      <TurnFooter
        :text="plainText"
        :usage="props.usage ?? props.historyTurn?.turn_usage"
        :show-metrics="props.showMetrics !== false && !liveActive"
        :hid="props.hid ?? ''"
        :is-user="isUser"
      />
    </div>
  </article>
</template>

<style scoped>
.msg {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}

/* 连续同一角色：取消与上一条之间的间距，视觉上合成一坨。
   头与正文之间的间距留着——那一坨内部还是看得出结构。 */
.msg.is-continued {
  margin-top: calc(-1 * var(--wb-sp-3));
}

.msg-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}

/* 角色徽章：一行人名，不画胶囊。
   长会话里每一轮都顶一个描边胶囊时，一屏会多出十几个相同形状的框。 */
.msg-badge {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--wb-faint);
}

.msg-badge.user {
  color: var(--wb-brand);
}

.msg-meta {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  font-family: var(--wb-font-mono);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.msg-body {
  min-width: 0;
}

/* 直播脚标：读数不是控件——不套描边、不缩放，只走字色 */
.live-foot {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-height: 18px;
  margin-top: var(--wb-sp-1);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
  font-variant-numeric: tabular-nums;
}

.live-dot {
  width: 7px;
  height: 7px;
  flex: none;
  border-radius: var(--wb-r-full);
  background: var(--wb-live);
  animation: wb-pulse 1.6s var(--wb-ease) infinite;
}

.live-label {
  color: var(--wb-live);
  font-weight: 600;
}

.live-read {
  display: inline-flex;
  align-items: baseline;
  gap: var(--wb-sp-3);
  margin-left: auto;
}

.live-elapsed {
  min-width: 3em;
  text-align: right;
  color: var(--wb-ink-2);
  font-weight: 600;
}

/* 轮尾的读数与动作搬进 TurnFooter：样式与它的瞬时态同住一个文件 */

.mark-line {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  margin: var(--wb-sp-4) 0;
  color: var(--wb-faint);
}

.mark-line::before,
.mark-line::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--wb-line);
}

.mark-text {
  flex: none;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  letter-spacing: 0.04em;
}
</style>
