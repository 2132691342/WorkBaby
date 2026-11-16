<script setup lang="ts">
// 一轮回答：把思考 / 正文 / 工具卡按引擎吐出的**原始顺序**摆在一起。
// 顺序永不重排——重排会把所有工具卡顶到正文前面，顺序一丢就读不出先干了什么。
// 改的只是「怎么收」：连续的活动段折成一行摘要，正文回到视线主干上。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import MarkdownBlock from './MarkdownBlock.vue'
import ThinkingBlock from './ThinkingBlock.vue'
import ToolCallCard from './ToolCallCard.vue'
import ActivityGroup from './ActivityGroup.vue'
import { groupBlocks } from '@/utils/activity'
import { useTurnClock } from '@/composables/useTurnClock'
import { useReadingHold } from '@/composables/useDisclosure'
import type { ChatBlock } from '@/types/api'

const props = defineProps<{
  blocks: ChatBlock[]
  runId: string
  streaming?: boolean
  /** 这一轮的开始时刻（毫秒）：等待态的秒表从它起走 */
  startedAt?: number
}>()

const { t } = useI18n()

/** 等待态已经等了多久：冷启动时首字能差十几秒，不给读数就像卡住了。 */
const { label: elapsed } = useTurnClock(() => Boolean(props.streaming), () => props.startedAt ?? 0)

const rootEl = ref<HTMLElement | null>(null)
// 阅读保护：焦点 / 选区在卡内时推迟自动折叠——把正在读的内容收走是最糟的自动行为。
const reading = useReadingHold(rootEl)

// 折叠是单向闩：流式中永不折叠（折叠后用户判断不出它是否卡住），
// 流停且没人正在读才折这一次；换一轮（runId 变）重新开闩。
const folded = ref(false)
let foldRun = ''
watch(
  [() => props.runId, () => props.streaming, reading],
  () => {
    if (foldRun !== props.runId) {
      foldRun = props.runId
      folded.value = false
    }
    if (!props.streaming && !reading.value) folded.value = true
  },
  { immediate: true },
)

/** 渲染单元：折叠闩放行时才把连续活动段收成一行摘要。 */
const groups = computed(() => groupBlocks(props.blocks, folded.value))

/** 流式只对「最后一块且是正文」走得动增量渲染，别的一律静态渲染。 */
function isLiveTail(index: number, block: ChatBlock): boolean {
  return Boolean(props.streaming) && index === props.blocks.length - 1 && block.block_kind === 'text'
}
</script>

<template>
  <div ref="rootEl" class="turn-blocks">
    <template v-for="(group, gi) in groups" :key="`${runId}-g${gi}-${group.kind === 'activity' ? group.key : group.index}`">
      <!-- 活动段：一段连续的思考与工具，收成一行摘要 -->
      <ActivityGroup
        v-if="group.kind === 'activity'"
        :blocks="group.blocks"
        :group-key="`${runId}-${group.key}`"
      />
      <!-- 正文：下标沿用平铺时的原下标，缓存键不变 -->
      <template v-else>
        <ThinkingBlock
          v-if="group.block.block_kind === 'thinking'"
          :text="group.block.block_text ?? ''"
          :streaming="isLiveTail(group.index, group.block)"
          :turn-id="runId"
        />
        <ToolCallCard v-else-if="group.block.block_kind === 'tool'" :block="group.block" />
        <MarkdownBlock
          v-else
          :text="group.block.block_text ?? ''"
          :streaming="isLiveTail(group.index, group.block)"
          :cache-key="`${runId}:${group.index}`"
        />
      </template>
    </template>
    <!-- 一个块都还没有：说明引擎受理了但还没吐出任何东西。
         只放一个转圈会被当成卡住——补一句正在做什么与已等多久。 -->
    <div v-if="!blocks.length && streaming" class="waiting">
      <span class="dot busy" />
      <span>{{ t('chat.thinkingLive') }}</span>
      <span v-if="elapsed" class="waiting-clock">{{ elapsed }}</span>
    </div>
  </div>
</template>

<style scoped>
/* 一轮里的块间距。
   工具卡是平铺的一行，不需要它自己的空气；
   正文与工具卡之间要留一点，否则工具行看起来像正文的一部分。 */
.turn-blocks {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}

.waiting {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-sm);
  color: var(--wb-mute);
}

/* 等待读数：与转圈同一行，等宽字。它是读数不是控件，不做悬停反馈。 */
.waiting-clock {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  font-variant-numeric: tabular-nums;
}
</style>
