<script setup lang="ts">
// 助手正文块：流式时按平滑节奏揭示 + 块级增量渲染，稳定后一次性渲染。
// v-html 的内容全部经过 utils/md.ts，是唯一出口。
import { computed } from 'vue'
import { handleCopyClick, renderMarkdown, renderStreaming } from '@/utils/md'
import { useSmoothText } from '@/composables/useSmoothText'

const props = defineProps<{
  text: string
  streaming?: boolean
  cacheKey: string
}>()

// 平滑揭示只作用于流式增长：delta 改的是 source，DOM 只按节奏更新，
// 缓存键不变（`${runId}:${块下标}`），已稳定的块照旧复用 HTML。
const smooth = useSmoothText(computed(() => props.text), computed(() => Boolean(props.streaming)))

const html = computed(() =>
  props.streaming ? renderStreaming(smooth.value, props.cacheKey) : renderMarkdown(props.text),
)
</script>

<template>
  <div class="md" :class="{ 'md-streaming': streaming }" v-html="html" @click="handleCopyClick" />
</template>
