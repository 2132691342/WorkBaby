<script setup lang="ts">
// 思考段：就地折叠在消息流里，不单独浮到回合顶部。
// 流式期间默认展开、只渲染尾部；正文一出现就自动收起（用户要看的是答案）。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{
  text: string
  /** 正在流式产出：带呼吸点、默认展开 */
  live?: boolean
  /** 正文已经开始了：自动收一次，之后用户自己决定 */
  collapse?: boolean
}>()

const { t } = useI18n()
const open = ref(!!props.live)

watch(
  () => props.live,
  (on) => {
    if (on) open.value = true
  },
)
watch(
  () => props.collapse,
  (on) => {
    if (on) open.value = false
  },
)

// 思考可能几万字：全量塞进 DOM 会让每帧重排压住主线程，也会把正文顶出屏幕。
// 流式期间只渲染尾部——推理是流式的，用户要看的是「现在想到哪」。
const TAIL = 4000
const shown = computed(() => {
  if (!props.live || props.text.length <= TAIL) return props.text
  return `…\n${props.text.slice(-TAIL)}`
})
</script>

<template>
  <div class="think" :class="{ 'is-live': live }">
    <button class="think-head" type="button" :aria-expanded="open" @click="open = !open">
      <span v-if="live" class="live-dot" />
      <AppIcon name="brain" size="ic-xs" />
      <span>{{ live ? t('chat.thinkingLive') : t('chat.thinkingBlock') }}</span>
      <span class="think-count">{{ t('chat.thinkChars', { n: text.length }) }}</span>
      <AppIcon class="think-chev" :name="open ? 'chevron-down' : 'chevron-right'" size="ic-xs" />
    </button>
    <div v-if="open" class="think-body">{{ shown }}</div>
  </div>
</template>

<style scoped>
.think-count {
  font-variant-numeric: tabular-nums;
  opacity: 0.75;
  font-weight: 500;
}
.think-chev {
  margin-left: auto;
  transition: transform var(--wb-dur-fast) var(--wb-ease);
}
.live-dot {
  width: 6px;
  height: 6px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-live);
  animation: wb-pulse 1.4s var(--wb-ease) infinite;
}
</style>
