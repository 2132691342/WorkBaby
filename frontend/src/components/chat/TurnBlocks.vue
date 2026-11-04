<script setup lang="ts">
// 一轮回答：把思考 / 正文 / 工具卡按引擎吐出的**原始顺序**摆在一起。
// 不按类型分组——分组会把所有工具卡顶到正文前面，顺序一丢就读不出先干了什么。
import { useI18n } from 'vue-i18n'
import MarkdownBlock from './MarkdownBlock.vue'
import ThinkingBlock from './ThinkingBlock.vue'
import ToolCallCard from './ToolCallCard.vue'
import type { ChatBlock } from '@/types/api'

const props = defineProps<{
  blocks: ChatBlock[]
  runId: string
  streaming?: boolean
}>()

const { t } = useI18n()

/** 流式只对「最后一块且是正文」走得动增量渲染，别的一律静态渲染。 */
function isLiveTail(index: number, block: ChatBlock): boolean {
  return Boolean(props.streaming) && index === props.blocks.length - 1 && block.block_kind === 'text'
}
</script>

<template>
  <div class="turn-blocks">
    <template v-for="(block, index) in blocks" :key="`${runId}-${index}-${block.block_kind}-${block.tool_call_id ?? ''}`">
      <ThinkingBlock v-if="block.block_kind === 'thinking'" :text="block.block_text ?? ''" :streaming="isLiveTail(index, block)" />
      <ToolCallCard v-else-if="block.block_kind === 'tool'" :block="block" />
      <MarkdownBlock
        v-else
        :text="block.block_text ?? ''"
        :streaming="isLiveTail(index, block) || isLiveTail(index, block)"
        :cache-key="`${runId}:${index}`"
      />
    </template>
    <div v-if="!blocks.length && streaming" class="waiting">
      <span class="dot busy" />
      <span>{{ t('chat.thinkingLive') }}</span>
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
</style>
