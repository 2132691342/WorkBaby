<script setup lang="ts">
// 思考块：默认收起，正文一出现就自动收；流式期只渲染尾部 4000 字（全塞进 DOM 会压住主线程）。
// 展开态记在模块级表里：块组件随轮次反复挂载销毁，组件内 ref 存不住。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

// 记忆上限 400 条，超了从头上丢：它只是「用户上次怎么摆的」，不是要完整保留的账本。
const OPEN_CAP = 400
const openMemo = new Map<string, boolean>()
function memoKey(turnId: string, text: string): string {
  // 文本前 64 字是这个思考段最稳定的身份：块下标会随分组变化，跨重排不可靠。
  return `${turnId}:${text.slice(0, 64)}`
}

const props = defineProps<{ text: string; streaming?: boolean; turnId?: string }>()
const { t } = useI18n()

const touched = ref(false)
const key = computed(() => memoKey(props.turnId ?? '', props.text))
const open = ref(openMemo.get(key.value) ?? Boolean(props.streaming))

watch(
  key,
  (next) => {
    if (!touched.value) open.value = openMemo.get(next) ?? Boolean(props.streaming)
  },
)

watch(
  () => props.streaming,
  (live) => {
    if (live) open.value = true
    else if (!touched.value && !openMemo.has(key.value)) open.value = false
  },
)

function toggle(): void {
  touched.value = true
  open.value = !open.value
  // 记忆写入即记账：Map 当 LRU 用，超量先删最早的一条。
  openMemo.delete(key.value)
  openMemo.set(key.value, open.value)
  if (openMemo.size > OPEN_CAP) {
    const oldest = openMemo.keys().next().value
    if (oldest !== undefined) openMemo.delete(oldest)
  }
}

const shown = computed(() => {
  if (!props.streaming) return props.text
  const limit = 4000
  return props.text.length > limit ? props.text.slice(-limit) : props.text
})

const chars = computed(() => props.text.length)
</script>

<template>
  <div class="think" :class="{ live: streaming }">
    <button class="think-head" type="button" :aria-expanded="open" @click="toggle">
      <span class="dot" :class="streaming ? 'busy' : ''" />
      <span>{{ streaming ? t('chat.thinkingLive') : t('chat.thinking') }}</span>
      <span class="caret-toggle">
        {{ t('chat.chars', { n: chars }) }} · {{ open ? t('chat.collapse') : t('chat.expand') }}
      </span>
    </button>
    <div v-if="open" class="think-body selectable">{{ shown }}</div>
  </div>
</template>
