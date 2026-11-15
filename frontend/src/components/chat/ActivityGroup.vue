<script setup lang="ts">
// 活动段：一段连续的思考与工具调用，收成一行摘要（「读了 3 个文件 · 搜了 2 次」），展开才见逐个卡片。
// 子卡默认不挂载，展开过一次后保留：一段能有几十个工具，全挂上等于把折叠省下的 DOM 又建回来。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import ThinkingBlock from '@/components/chat/ThinkingBlock.vue'
import ToolCallCard from '@/components/chat/ToolCallCard.vue'
import { hasError, isRunning, summarise, totalMs } from '@/utils/activity'
import type { ChatBlock } from '@/types/api'

const props = defineProps<{
  blocks: ChatBlock[]
  /** 段键：展开状态与缓存键都由它派生 */
  groupKey: string
}>()

const { t } = useI18n()

const open = ref(false)
/** 是否已经展开过。true 之后子卡常驻，收起只藏不销毁。 */
const revealed = ref(false)

function toggle(): void {
  open.value = !open.value
  if (open.value) revealed.value = true
}

const parts = computed(() => summarise(props.blocks))
const failed = computed(() => hasError(props.blocks))
const running = computed(() => isRunning(props.blocks))
const ms = computed(() => totalMs(props.blocks))

/** 耗时读数：合计过千才显示，几十个几十毫秒加起来是个噪声。 */
const msLabel = computed(() => {
  const v = ms.value
  if (v < 1000) return ''
  if (v < 10000) return `${(v / 1000).toFixed(2)}s`
  return `${(v / 1000).toFixed(1)}s`
})
</script>

<template>
  <div class="activity" :class="{ 'is-open': open, 'is-failed': failed, 'is-running': running }">
    <button class="activity-head" type="button" :aria-expanded="open" @click="toggle">
      <AppIcon name="chevron" :size="12" class="activity-chevron" />
      <span v-if="running" class="activity-dot" />
      <span class="activity-summary">
        <template v-for="(part, i) in parts" :key="part.verb">
          <span v-if="i > 0" class="activity-sep">·</span>
          <span class="activity-part">{{ part.verb }} {{ part.count }}</span>
        </template>
        <span v-if="!parts.length" class="activity-part">{{ t('chat.thinking') }}</span>
      </span>
      <span v-if="msLabel" class="activity-ms">{{ msLabel }}</span>
    </button>

    <!-- 展开过的段才建子卡：revealed 之后收起只藏不销毁 -->
    <div v-if="revealed" v-show="open" class="activity-body">
      <template v-for="(block, i) in blocks" :key="`${groupKey}-${i}-${block.tool_call_id ?? ''}`">
        <ThinkingBlock v-if="block.block_kind === 'thinking'" :text="block.block_text ?? ''" :turn-id="groupKey" />
        <ToolCallCard v-else-if="block.block_kind === 'tool'" :block="block" />
      </template>
    </div>
  </div>
</template>

<style scoped>
.activity {
  display: flex;
  flex-direction: column;
}

.activity-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  align-self: flex-start;
  max-width: 100%;
  padding: 3px var(--wb-sp-2);
  border: none;
  border-radius: var(--wb-r-sm);
  background: transparent;
  color: var(--wb-mute);
  font-size: var(--wb-fs-xs);
  font-family: inherit;
  text-align: left;
  cursor: pointer;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}

.activity-head:hover {
  background: var(--wb-hover);
  color: var(--wb-ink-2);
}

.activity-chevron {
  flex: none;
  transition: transform var(--wb-dur-fast) var(--wb-ease);
}

.activity.is-open .activity-chevron {
  transform: rotate(90deg);
}

/* 段还在跑：转圈点。整段只有一个点，不是每个工具一个——
   用户要知道的是「这一段没完」，细节展开自己看。 */
.activity-dot {
  flex: none;
  width: 6px;
  height: 6px;
  border-radius: var(--wb-r-full);
  background: var(--wb-live);
  animation: wb-pulse 1.6s var(--wb-ease) infinite;
}

.activity-summary {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-2);
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.activity-sep {
  color: var(--wb-faint);
}

.activity-part {
  font-weight: 550;
}

.activity-ms {
  flex: none;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  font-variant-numeric: tabular-nums;
}

/* 段里出过错：整行转危险色。逐个卡片找红点太累，
   段级标色让「哪一段出了问题」一眼可见。 */
.activity.is-failed .activity-head {
  color: var(--wb-bad);
}

.activity-body {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  margin: var(--wb-sp-1) 0 var(--wb-sp-1) var(--wb-sp-2);
  padding-left: var(--wb-sp-3);
  border-left: 1px solid var(--wb-line);
}
</style>
