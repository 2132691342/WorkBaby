<script setup lang="ts">
// 思考块：默认收起，正文一出现就自动收。
// 流式期只渲染尾部 4000 字——几万字的思考全塞进 DOM 会压住主线程，还把答案顶出屏。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ text: string; streaming?: boolean }>()
const { t } = useI18n()

const open = ref(Boolean(props.streaming))
const touched = ref(false)

watch(
  () => props.streaming,
  (live) => {
    if (live) open.value = true
    else if (!touched.value) open.value = false
  },
)

const shown = computed(() => {
  if (!props.streaming) return props.text
  const limit = 4000
  return props.text.length > limit ? props.text.slice(-limit) : props.text
})

const chars = computed(() => props.text.length)
</script>

<template>
  <div class="think" :class="{ live: streaming }">
    <button
      class="think-head"
      type="button"
      @click="
        open = !open;
        touched = true
      "
    >
      <span class="dot" :class="streaming ? 'busy' : ''" />
      <span>{{ streaming ? t('chat.thinkingLive') : t('chat.thinking') }}</span>
      <span class="caret-toggle">
        {{ t('chat.chars', { n: chars }) }} · {{ open ? t('chat.collapse') : t('chat.expand') }}
      </span>
    </button>
    <div v-if="open" class="think-body selectable">{{ shown }}</div>
  </div>
</template>
