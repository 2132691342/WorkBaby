// 历史消息列表：把后端投影好的轮次排成一列，并算出「哪几行该并成一坨」。
// 并坨的判据放这一层：列表里有「切引擎分隔」这类非消息行，index 相邻
// 不代表同一段对话；直播轮以「历史里有没有它」去重，两处都画就是回复画两遍。
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import MessageTurn from '@/components/chat/MessageTurn.vue'
import { turnPresence, type TurnReading } from '@/composables/useMessageFlow'
import type { ChatBlock, HistoryTurn } from '@/types/api'

const { t } = useI18n()

const props = defineProps<{
  turns: HistoryTurn[]
  /** 拼在历史后面的当前轮（流式进行中）；头要不要出，取决于历史最后一行是不是同一个角色 */
  live?: {
    userMessage: string
    blocks: ChatBlock[]
    runId: string
    model?: string
    streaming: boolean
    errored: boolean
    /** 直播这一轮的开始时刻：传给脚标秒表 */
    startedAt?: number
    usage?: TurnReading
  } | null
}>()

// 直播的用户那句要不要画：历史里已有就不画，否则刚发的话出现两次。
// 收尾后历史最后一行是助手回复、用户那句退到倒数第二行——只盯最后一行
// 会漏判，把用户气泡在直播位再画一遍。所以收尾后往前找最后一条用户消息来比。
const liveUserVisible = computed(() => {
  if (!props.live) return false
  const tail = props.turns.at(-1)
  if (tail?.turn_role === 'user' && (tail.turn_blocks?.[0]?.block_text ?? '') === props.live.userMessage) return false
  if (!props.live.streaming) {
    for (let i = props.turns.length - 1; i >= 0; i--) {
      const turn = props.turns[i]
      if (turn.turn_role === 'user') return (turn.turn_blocks?.[0]?.block_text ?? '') !== props.live.userMessage
    }
  }
  return true
})

// 直播的助手回复要不要画：整轮已在服务器历史里就不画；逐块比对代价与历史长度成正比，
// 只在流停了之后才算——流式期间历史不可能收下正在长的那一轮。
const liveAssistantVisible = computed(() => {
  if (!props.live) return false
  if (props.live.streaming) return true
  return turnPresence(props.turns, props.live.userMessage, props.live.blocks) !== 2
})

/**
 * 当前轮要不要出头：历史最后一行是同一个角色就不出。
 */
const liveContinued = computed(() => {
  const last = props.turns.at(-1)
  return last?.turn_role === 'assistant'
})

// 长会话一次画全会把主线程压死：首屏只画最后这么多轮，往上滚动再补。
// 窗口不是懒加载的替代——content-visibility 管的是「屏外不参与渲染」，
// 而几百轮的 DOM 本身就要建，这两件事必须同时做。
const PAGE = 40
const shownCount = ref(PAGE)

// 数据换了（换会话 / 隐藏一轮）就从尾屏重新开始：
// 留着旧的 shownCount 会让新会话凭空少几轮。
watch(
  () => props.turns.length,
  () => {
    shownCount.value = PAGE
  },
)

// 真正要画的轮：从尾部取 shownCount 条。
const visibleTurns = computed(() => {
  const total = props.turns.length
  if (total <= shownCount.value) return props.turns
  return props.turns.slice(total - shownCount.value)
})

/** 还有没有更早的轮没画出来。 */
const hasMore = computed(() => props.turns.length > shownCount.value)

/** 往上补一页。 */
function loadMore(): void {
  shownCount.value += PAGE
}

// 可见轮的并坨判据：第一行永远不并（它上面可能还有没画出来的轮）。
const visibleFlags = computed<boolean[]>(() => {
  const list = visibleTurns.value
  return list.map((turn, i) => {
    if (i === 0) return false
    const prev = list[i - 1]
    if (turn.turn_role === 'engine-mark' || prev.turn_role === 'engine-mark') return false
    return prev.turn_role === turn.turn_role
  })
})

// 可见轮的轮尾判据：读数只挂每一坨的最后一条。
function isVisibleRoundEnd(index: number): boolean {
  const list = visibleTurns.value
  const next = list[index + 1]
  return !next || next.turn_role !== list[index].turn_role
}
</script>

<template>
  <div class="msg-col">
    <!-- 更早的轮次没画出来时给一个入口：
         一次画全几百轮会把主线程压死，用户看到的是「打开会话就卡住」。 -->
    <button type="button" v-if="hasMore" class="more-btn wb-btn text sm" @click="loadMore">
      {{ t('chat.loadEarlier', { n: Math.min(PAGE, props.turns.length - shownCount) }) }}
    </button>

    <MessageTurn
      v-for="(item, index) in visibleTurns"
      :key="item.turn_id || `h-${index}`"
      :role="item.turn_role"
      :blocks="item.turn_blocks"
      :run-id="`h-${index}`"
      :model="item.turn_model"
      :continued="visibleFlags[index]"
      :history-turn="item"
      :hid="item.turn_id"
      :show-metrics="isVisibleRoundEnd(index)"
    />

    <template v-if="props.live">
      <MessageTurn
        v-if="liveUserVisible"
        role="user"
        :blocks="[{ block_kind: 'text', block_text: props.live.userMessage }]"
        :run-id="`${props.live.runId}:user`"
        :continued="false"
      />
      <MessageTurn
        v-if="liveAssistantVisible"
        role="assistant"
        :blocks="props.live.blocks"
        :run-id="props.live.runId"
        :model="props.live.model"
        :continued="liveContinued"
        :streaming="props.live.streaming"
        live
        :errored="props.live.errored"
        :started-at="props.live.startedAt"
        :usage="props.live.usage"
        show-metrics
      />
    </template>
  </div>
</template>

<style scoped>
/* 列宽、间距、屏外不参与 layout 都归 content.css 的 .msg-col（唯一落点）。 */

/* 「加载更早」入口：贴在列首，居中一行。
   它是这一列的第一个元素，用户往下滚到底部之前不会注意到它——
   所以做成常驻可见的一行文字按钮，不藏在滚动事件里。 */
.more-btn {
  align-self: center;
  margin-bottom: var(--wb-sp-2);
  border-radius: var(--wb-r-full);
}
</style>
