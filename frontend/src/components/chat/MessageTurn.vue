<script setup lang="ts">
// 一轮消息：角色头 + 若干块（正文 / 思考 / 工具卡）+ 轮尾读数。
// 同一角色连续几轮只出一次头——一次回答被切成几块，读的人得自己拼。
// 轮尾给读数（输入 / 输出 / 缓存 / 耗时）与复制、删除，答案不藏进菜单。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import { fmtCount, fmtMs } from '@/utils/format'
import TurnBlocks from '@/components/chat/TurnBlocks.vue'
import MarkdownBlock from '@/components/chat/MarkdownBlock.vue'
import TurnChanges from '@/components/chat/TurnChanges.vue'
import AppIcon from '@/components/common/AppIcon.vue'
import type { TurnReading } from '@/composables/useMessageFlow'
import { useTurnClock } from '@/composables/useTurnClock'
import { useChatStore } from '@/stores/chat'
import { useToasts } from '@/composables/useToasts'
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
  /**
   * 上一轮是不是同一个角色。
   *
   * 为真时这一轮不出头、也不给上外边距——它与上一轮在视觉上是同一坨。
   * 由父组件算而不是自己看 index==0：列表里还有「切引擎分隔」这种
   * 不是消息的行，index 相邻的未必属于同一段对话。
   */
  continued: boolean
  /** 流式进行中的当前轮 */
  live?: boolean
  /** 这一轮出错了 */
  errored?: boolean
  /**
   * 这一轮的用量与耗时（助手轮才有）。
   *
   * 两个来源：流式那一轮从流里取，历史那一轮从后端投影里取。
   * 两者字段名不一样（驼峰 vs 下划线），usageFrom 两种都认。
   */
  usage?: TurnReading
  /** 历史轮自带的用量（后端投影带下来的） */
  historyTurn?: HistoryTurn
  /** 这一轮的隐藏标识：有它才能「删除」（直播轮没有，因此不可删） */
  hid?: string
  /** 直播这一轮的开始时刻（毫秒时间戳）：脚标的计时从它起走 */
  startedAt?: number
  /**
   * 要不要显示用量读数。
   *
   * 只在最后一轮上显示：引擎每条助手消息都带一份累计用量，
   * 而后端投影已经把一次回答里的多条助手消息并成了一轮。
   */
  showMetrics?: boolean
}>()

const { t } = useI18n()
const chat = useChatStore()
const toasts = useToasts()

const isUser = computed(() => props.role === 'user')

/**
 * 直播脚标：轮尾一行「正在回答 · 12s」。
 *
 * 流式的增量是一阵一阵到的——两阵之间界面纹丝不动，
 * 用户分不清「在执行」还是「卡住了」。脚标常驻在正文下面，
 * 秒表一秒一跳，哪怕半分钟没有新字，也能看见时间在走。
 */
const liveActive = computed(() => Boolean(props.live) && Boolean(props.streaming))

// 秒表与活动条共用同一份实现与同一个起点：两处显示的时间必须一致。
const { label: liveElapsed } = useTurnClock(
  () => liveActive.value,
  () => props.startedAt ?? 0,
)

/**
 * 脚标上写什么。
 *
 * 优先级：扩展设的状态文字 > 实时阶段 > 一句兜底。
 * 扩展的文字更具体（它会说「正在跑测试」），而阶段是引擎自己的事实，不依赖任何扩展。
 */
const liveLabel = computed(() => {
  if (chat.statusText) return chat.statusText
  const live = chat.live
  if (live.phase === 'tool' && live.tool_name) return t('chat.liveTool', { name: live.tool_name })
  if (live.phase === 'thinking') return t('chat.thinkingLive')
  return t('chat.liveAnswering')
})

/** 已经吐出的输出 token：实时读数优先，它比用量帧更早到。 */
const liveOut = computed(() => chat.live.output_tokens || props.usage?.output_tokens || 0)

/** 引擎报的生成速度：给就显示，不给不占位。 */
const liveSpeed = computed(() => '')

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

const copied = ref(false)

/**
 * 复制这一轮的回答。
 *
 * 放在轮尾而不是每一段上：用户要的通常是「这次回答的全文」，
 * 而逐段复制会让他按段拼一遍。按钮只在悬停时出现，平时不占视觉重量。
 */
async function copyTurn(): Promise<void> {
  if (!plainText.value.trim()) return
  try {
    await navigator.clipboard.writeText(plainText.value)
    copied.value = true
    window.setTimeout(() => {
      copied.value = false
    }, 1400)
  } catch {
    /* 剪贴板不可用时不打扰：用户可以手动选中 */
  }
}

/**
 * 隐藏这一轮（界面上的「删除」）。
 *
 * 语义是隐藏：引擎的会话文件与上下文一个字不动，被摘掉的轮次
 * 在会话详情页仍然查得到。所以确认框把话说清楚，撤销入口就在
 * 删完的那条提示上——界面层的误删必须一键能回。
 */
async function removeTurn(): Promise<void> {
  const hid = props.hid
  if (!hid) return
  try {
    await ElMessageBox.confirm(t('chat.hideTurnHint'), t('chat.hideTurnTitle'), {
      confirmButtonText: t('chat.hideTurnConfirm'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    await chat.hideTurn(hid)
    toasts.push('ok', t('chat.turnHidden'), undefined, {
      label: t('common.undo'),
      run: () => {
        chat.unhideTurn(hid).catch((err) => toasts.push('error', err instanceof Error ? err.message : String(err)))
      },
    })
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

/**
 * 这一轮是不是只有工具调用、没有任何文字。
 *
 * 只有工具的时候，轮尾的「复制」没有东西可复制——不渲染它，
 * 一轮里就少一个点不动的按钮。
 */
const hasText = computed(() => plainText.value.trim().length > 0)

/**
 * 用量读数：提示词总量（命中 + 未命中 + 写入）/ 输出 / 缓存命中率 / 耗时。
 *
 * 缓存命中率单独标绿：它是「这一轮几乎没花钱」的那件事，
 * 也是用户最想知道的那个数字（为什么这次这么快、这么便宜）。
 */
const metrics = computed(() => {
  if (props.showMetrics === false) return []
  // 直播中的轮次不摆静态读数：轮尾的脚标在替它计时，
  // 两份时间戳并存必然有一份是停着的——停着的那份就是「是不是卡了」的误报来源。
  if (liveActive.value) return []
  const u = props.usage ?? props.historyTurn?.turn_usage
  if (!u) return []
  const prompt = u.input_tokens + u.cache_read_tokens + u.cache_write_tokens
  const out: { key: string; value: string; hit?: boolean }[] = []
  if (prompt > 0) out.push({ key: '↓', value: fmtCount(prompt) })
  if (u.output_tokens > 0) out.push({ key: '↑', value: fmtCount(u.output_tokens) })
  if (prompt > 0) {
    const rate = Math.round((u.cache_read_tokens / prompt) * 100)
    out.push({ key: 'cache', value: `${rate}%`, hit: rate > 0 })
  }
  if (u.cost_usd > 0) out.push({ key: '$', value: u.cost_usd.toFixed(u.cost_usd < 0.01 ? 4 : 3) })
  if (props.usage && props.usage.elapsed_ms > 0) {
    out.push({ key: '⏱', value: fmtMs(props.usage.elapsed_ms) })
  }
  return out
})

/** token 数按量级压缩：界面上要能一眼比较，不是要精确到个位。 */
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
      <span class="msg-badge" :class="{ user: isUser }">{{ isUser ? 'you' : 'assistant' }}</span>
      <span v-if="props.model" class="msg-meta">{{ props.model }}</span>
    </div>
    <div class="msg-body">
      <!-- 用户轮只有一段纯文本，不需要 TurnBlocks 那套工具卡分派 -->
      <MarkdownBlock
        v-if="isUser"
        :text="props.blocks[0]?.block_text ?? ''"
        :streaming="false"
        :cache-key="`${props.runId}:user`"
      />
      <TurnBlocks v-else :blocks="props.blocks" :run-id="props.runId" :streaming="props.streaming" />

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
      <div class="msg-acts">
        <span v-if="metrics.length" class="msg-metrics">
          <span v-for="m in metrics" :key="m.key" class="msg-metric" :class="{ 'is-hit': m.hit }">
            <b>{{ m.key }}</b>{{ m.value }}
          </span>
        </span>
        <button v-if="hasText" class="msg-act" type="button" :disabled="copied" @click="copyTurn">
          <AppIcon :name="copied ? 'check' : 'file'" :size="11" />
          {{ copied ? t('common.copied') : t('common.copy') }}
        </button>
        <button v-if="props.hid" class="msg-act danger" type="button" @click="removeTurn">
          <AppIcon name="trash" :size="11" />
          {{ t('common.delete') }}
        </button>
      </div>
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

/* 轮尾动作：默认透明，悬停整个回合才浮现 */
.msg-acts {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--wb-sp-3);
  min-height: 18px;
  opacity: 0;
  transition: opacity var(--wb-dur-fast) var(--wb-ease);
}

.msg:hover .msg-acts,
.msg-acts:has(.msg-metrics) {
  opacity: 1;
}

/* 用量读数：等宽数字 + 极淡的分隔线。
   它是读数不是控件——不套描边胶囊、不做悬停缩放。 */
.msg-metrics {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding-right: var(--wb-sp-2);
  border-right: 1px solid var(--wb-line);
}

.msg-metric {
  display: inline-flex;
  align-items: baseline;
  gap: 2px;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  font-variant-numeric: tabular-nums;
  font-family: var(--wb-font-mono);
}

.msg-metric b {
  font-weight: 600;
  opacity: 0.7;
}

/* 缓存命中率：命中就标绿——它是「这次几乎没花钱」的那件事 */
.msg-metric.is-hit {
  color: var(--wb-ok);
}

.msg-act {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 6px;
  border: none;
  background: transparent;
  color: var(--wb-faint);
  font-size: var(--wb-fs-3xs);
  font-weight: 600;
  transition:
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.msg-act:hover:not(:disabled) {
  color: var(--wb-brand);
  transform: scale(var(--wb-press-hover));
}

.msg-act:active:not(:disabled) {
  transform: scale(var(--wb-press-active));
}

/* 删除是破坏性动作：常规态就要与复制分开一点颜色，悬停转危险色 */
.msg-act.danger {
  color: var(--wb-mute);
}

.msg-act.danger:hover:not(:disabled) {
  color: var(--wb-bad);
}

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
