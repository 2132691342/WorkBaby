<script setup lang="ts">
/**
 * 流式中的 assistant 气泡：数据源为 chat store 的 streamingBlocks，渲染交给 MessageBlocksRenderer；
 * 本组件补光标与实时用量徽标。
 */
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { Loader2 } from '@/components/common/icons'
import MessageBlocksRenderer from '@/components/chat/MessageBlocksRenderer.vue'
import UsageBadge from '@/components/chat/UsageBadge.vue'
import MarkdownRenderer from '@/components/chat/MarkdownRenderer.vue'
import { useChatStore } from '@/stores/chat'
import { useFocusMode } from '@/composables/useFocusMode'
import { t } from '@/i18n'

const chat = useChatStore()
const {
  streamingBlocks,
  streamingStats,
  streamingTurn,
  lastCheckpointTurn,
  streamingRetry
} = storeToRefs(chat)
const { enabled: focusMode } = useFocusMode()

/** 流式 running tick：所有 running 块共享一个 1s 心跳（工具行内显示经过时长）。 */
const now = ref(Date.now())
let tickTimer: ReturnType<typeof setInterval> | null = null
watch(
  () => streamingBlocks.value.some((b) => b.state === 'running'),
  (running) => {
    if (running && tickTimer === null) {
      tickTimer = setInterval(() => {
        now.value = Date.now()
      }, 1000)
    } else if (!running && tickTimer !== null) {
      clearInterval(tickTimer)
      tickTimer = null
    }
  },
  { immediate: true }
)

/** 最后一条 text 块的内容（光标挂在它后面）；若无 text 块则光标放最末。 */
const lastText = computed(() => {
  const blocks = streamingBlocks.value
  for (let i = blocks.length - 1; i >= 0; i--) {
    if (blocks[i].kind === 'text') return blocks[i].text
  }
  return ''
})

/** 是否还有任何内容（决定是否显示光标）。 */
const hasAnyContent = computed(() => {
  const blocks = streamingBlocks.value
  return blocks.length > 0 && blocks.some((b) =>
    (b.kind === 'text' && b.text) ||
    b.kind === 'tool_call' ||
    b.kind === 'tool_result' ||
    b.kind === 'artifact' ||
    b.kind === 'genui' ||
    b.kind === 'skill'
  )
})
</script>

<template>
  <!-- 流式块序列：按事件到达顺序，文本与工具穿插呈现（恢复真实时序） -->
  <div class="bubble">
    <!-- 建流瞬时错误自动重试提示（限流/5xx；正文恢复自动消失） -->
    <div
      v-if="streamingRetry"
      class="mb-2 flex items-center gap-2 rounded-lg bg-wb-lemon/10 px-2 py-1 text-[11px] text-wb-lemon"
    >
      <Loader2 class="h-3 w-3 animate-spin" />
      {{ t('chat.retryHint', streamingRetry.attempt, Math.max(1, Math.round(streamingRetry.delay_ms / 1000))) }}
    </div>

    <!-- 轮次标记：多轮任务里「第 N 轮」是「还在干活」的关键信号；
         单轮（turn<=1）不显示，避免给简单问答加噪声。检查点位点仅在多轮时一并展示。 -->
    <div
      v-if="!focusMode && streamingTurn > 1"
      class="mb-1.5 flex items-center gap-1.5 px-1 text-[11px] text-wb-muted"
    >
      <span class="font-medium text-wb-ink/70">{{ t('chat.turnLabel', streamingTurn) }}</span>
      <span v-if="lastCheckpointTurn" class="text-wb-muted/70">
        {{ t('chat.checkpointLabel', lastCheckpointTurn) }}
      </span>
    </div>

    <!-- 流式块序列：MessageBlocksRenderer 直接消费 streamingBlocks。
         最后一条 text 块的内容由本组件用 MarkdownRenderer + cursor 接管（不受 v-html 重写影响）。 -->
    <MessageBlocksRenderer
      v-if="!focusMode && streamingBlocks.length > 0"
      :streaming="streamingBlocks"
      :streaming_mode="true"
      :now="now"
    >
      <!-- 文本块：流式 Markdown 渲染 + 光标 -->
      <template #text="{ content, streaming }">
        <div class="flex items-end gap-1">
          <MarkdownRenderer :content="content" :streaming="streaming" />
          <span v-if="streaming" class="wb-cursor" />
        </div>
      </template>
    </MessageBlocksRenderer>

    <!-- 无任何内容但流式中：仅显示光标 -->
    <span v-else-if="!hasAnyContent" class="wb-cursor" />

    <!-- 焦点模式：仅显示最后一条 text 内容 + 简化光标（其他块全部隐藏） -->
    <template v-else-if="focusMode && lastText">
      <div class="flex items-end gap-1">
        <MarkdownRenderer :content="lastText" :streaming="true" />
        <span class="wb-cursor" />
      </div>
    </template>
  </div>

  <!-- 流式用量（后端 chat:stats 事件；每轮结束刷新累计值） -->
  <UsageBadge
    v-if="streamingStats"
    :usage="{
      input_tokens: streamingStats.input_tokens,
      output_tokens: streamingStats.output_tokens,
      total_tokens: streamingStats.total_tokens,
      latency_ms: streamingStats.latency_ms,
      cost: streamingStats.cost
    }"
    live
  />
</template>