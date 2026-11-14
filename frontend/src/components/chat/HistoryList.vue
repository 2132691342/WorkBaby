// 历史消息列表：把后端投影好的轮次排成一列，并算出「哪几行该并成一坨」。
// 并坨的判据放这一层：列表里有「切引擎分隔」这类非消息行，index 相邻
// 不代表同一段对话；直播轮以「历史里有没有它」去重，两处都画就是回复画两遍。
<script setup lang="ts">
import { computed } from 'vue'
import MessageTurn from '@/components/chat/MessageTurn.vue'
import { turnPresence, type TurnReading } from '@/composables/useMessageFlow'
import type { ChatBlock, HistoryTurn } from '@/types/api'

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

/** 每一行与其上一行是否同角色（分隔行永远不并坨）。 */
const continuedFlags = computed<boolean[]>(() =>
  props.turns.map((turn, i) => {
    if (i === 0) return false
    const prev = props.turns[i - 1]
    if (turn.turn_role === 'engine-mark' || prev.turn_role === 'engine-mark') return false
    return prev.turn_role === turn.turn_role
  }),
)

// 直播的用户那句要不要画：服务器历史里已有就不画，否则刚发的话出现两次；
// 判据只看最后一行是不是它，可跟着每个 delta 重算。
const liveUserVisible = computed(() => {
  if (!props.live) return false
  const tail = props.turns.at(-1)
  if (!tail) return true
  return !(tail.turn_role === 'user' && (tail.turn_blocks?.[0]?.block_text ?? '') === props.live.userMessage)
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

// 这一行是不是它那一轮的最后一条：读数只挂最后一条（引擎每条助手消息都带累计用量，
// 每条都摆会像花了八次钱）；判据是下一条是不是换角色。
function isRoundEnd(index: number): boolean {
  const next = props.turns[index + 1]
  return !next || next.turn_role !== props.turns[index].turn_role
}
</script>

<template>
  <div class="msg-col">
    <MessageTurn
      v-for="(item, index) in props.turns"
      :key="item.turn_id || `h-${index}`"
      :role="item.turn_role"
      :blocks="item.turn_blocks"
      :run-id="`h-${index}`"
      :model="item.turn_model"
      :continued="continuedFlags[index]"
      :history-turn="item"
      :hid="item.turn_id"
      :show-metrics="isRoundEnd(index)"
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
</style>
