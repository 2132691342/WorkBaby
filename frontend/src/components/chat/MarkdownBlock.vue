<script setup lang="ts">
// 助手正文块：流式时走块级增量渲染，稳定后一次性渲染。
// v-html 的内容全部经过 utils/md.ts，是唯一出口。
import { computed } from 'vue'
import { handleCopyClick, renderMarkdown, renderStreaming } from '@/utils/md'

const props = defineProps<{
  text: string
  streaming?: boolean
  cacheKey: string
}>()

const html = computed(() =>
  props.streaming ? renderStreaming(props.text, props.cacheKey) : renderMarkdown(props.text),
)
</script>

<template>
  <div class="md" v-html="html" @click="handleCopyClick" />
</template>
