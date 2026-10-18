<script setup lang="ts">
// 工具执行卡：折叠态是一行摘要（状态 + 工具名 + 参数摘要 + 耗时），展开看参数与输出。
// edit 的输出里带 ```diff 围栏，按增删行着色后改动一目了然。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { summarizeArgs } from '../../utils/md'
import { fmtMs } from '../../utils/num'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{
  label: string
  /** 工具注册名：决定输出用什么方式呈现（edit → diff 视图） */
  tool?: string
  args?: Record<string, unknown>
  running?: boolean
  ok?: boolean
  title?: string
  output?: string
  duration_ms?: number
}>()

const { t } = useI18n()
const open = ref(false)

const summary = computed(() => summarizeArgs(props.args))
const status = computed(() => (props.running ? 'run' : props.ok === false ? 'bad' : 'ok'))
const statusText = computed(() =>
  props.running ? t('chat.toolRunning') : props.ok === false ? t('chat.toolFailed') : t('chat.toolDone'),
)

type DiffRow = { kind: 'add' | 'del' | 'meta'; text: string }
const diffRows = computed<DiffRow[] | null>(() => {
  if (props.tool !== 'edit') return null
  const m = (props.output || '').match(/```diff\n([\s\S]*?)(?:```|$)/)
  if (!m) return null
  const rows = m[1].split('\n')
  while (rows.length && rows[rows.length - 1] === '') rows.pop()
  if (!rows.length) return null
  return rows.map((line) => ({
    kind: line.startsWith('+') ? 'add' : line.startsWith('-') ? 'del' : 'meta',
    text: line === '' ? ' ' : line,
  }))
})

const argsJson = computed(() =>
  props.args && Object.keys(props.args).length ? JSON.stringify(props.args, null, 2) : '',
)
</script>

<template>
  <div class="tool" :class="{ 'is-bad': status === 'bad' }">
    <button class="tool-hd" type="button" :aria-expanded="open" @click="open = !open">
      <span class="tool-dot" :class="`is-${status}`" :title="statusText">
        <AppIcon v-if="running" name="loader" size="ic-2xs" spin />
        <AppIcon v-else-if="status === 'bad'" name="close" size="ic-2xs" />
        <AppIcon v-else name="check" size="ic-2xs" />
      </span>
      <span class="tool-name">{{ label }}</span>
      <span v-if="summary" class="tool-label">{{ summary }}</span>
      <span class="tool-meta">
        <span v-if="duration_ms">{{ fmtMs(duration_ms) }}</span>
        <AppIcon class="tool-chev" :name="open ? 'chevron-down' : 'chevron-right'" size="ic-xs" />
      </span>
    </button>

    <div v-if="open" class="tool-bd">
      <template v-if="argsJson">
        <div class="wb-label tool-sec">{{ t('chat.toolArgs') }}</div>
        <pre class="tool-out">{{ argsJson }}</pre>
      </template>
      <div class="wb-label tool-sec">{{ title || t('chat.toolOutput') }}</div>
      <div v-if="diffRows" class="diff">
        <div
          v-for="(r, i) in diffRows"
          :key="i"
          class="diff-row"
          :class="{ 'is-add': r.kind === 'add', 'is-del': r.kind === 'del' }"
        >
          <span /><span /><span>{{ r.text }}</span>
        </div>
      </div>
      <pre v-else-if="output" class="tool-out" :class="{ 'is-bad': status === 'bad' }">{{ output }}</pre>
      <div v-else-if="!running" class="tool-out is-dim">{{ t('chat.toolEmpty') }}</div>
    </div>
  </div>
</template>

<style scoped>
.tool-dot {
  width: calc(17px * var(--wb-vw-scale));
  height: calc(17px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  display: grid;
  place-items: center;
  flex: none;
}
.tool-dot.is-run {
  color: var(--wb-live);
  background: var(--wb-live-soft);
  animation: tool-breathe 1.2s var(--wb-ease) infinite;
}
.tool-dot.is-ok {
  color: var(--wb-success);
  background: var(--wb-success-soft);
}
.tool-dot.is-bad {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
@keyframes tool-breathe {
  50% {
    opacity: 0.45;
  }
}
.tool-sec {
  margin: var(--wb-sp-2) 0 var(--wb-sp-1);
}
.tool-sec:first-child {
  margin-top: 0;
}
.tool-out.is-dim {
  color: var(--wb-muted);
}
.tool-out.is-bad {
  color: var(--wb-danger);
}
.tool-chev {
  transition: transform var(--wb-dur-fast) var(--wb-ease);
}
.tool-bd {
  animation: tool-unfold var(--wb-dur) var(--wb-ease);
}
@keyframes tool-unfold {
  from {
    opacity: 0;
    transform: translateY(-4px);
  }
}
</style>
