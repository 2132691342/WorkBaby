<script setup lang="ts">
// 工具执行卡：折叠态显示一行摘要，展开看参数与输出；edit 的输出按 diff 着色。
import { computed, ref } from 'vue'
import { summarizeArgs } from '../../utils/md'
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

const open = ref(false)

// edit 的输出里带着 ```diff 围栏（改动处前后 3 行上下文）：
// 按增删行着色后改动一目了然，纯文本 pre 里只是一堆加减号。
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
</script>

<template>
  <div class="tool" :class="{ open }">
    <button
      class="tool-hd"
      type="button"
      :aria-expanded="open"
      :aria-label="`${label}${open ? '（收起详情）' : '（展开详情）'}`"
      @click="open = !open"
    >
      <span class="st" :class="running ? 'is-run' : ok === false ? 'is-bad' : 'is-ok'">
        <AppIcon v-if="running" name="loader" size="ic-xs" spin />
        <AppIcon v-else-if="ok === false" name="close" size="ic-xs" />
        <AppIcon v-else name="check" size="ic-xs" />
      </span>
      <span class="nm">{{ label }}</span>
      <span class="arg">{{ summarizeArgs(args) }}</span>
      <span v-if="duration_ms" class="ms">{{ Math.round(duration_ms / 100) / 10 }}s</span>
      <AppIcon class="chev" name="chevron-right" size="ic-sm" />
    </button>
    <div v-if="open" class="tool-bd">
      <template v-if="args && Object.keys(args).length">
        <div class="lb">参数</div>
        <pre><code>{{ JSON.stringify(args, null, 2) }}</code></pre>
      </template>
      <template v-if="output">
        <div class="lb">{{ title || '输出' }}</div>
        <div v-if="diffRows" class="diff">
          <div v-for="(r, i) in diffRows" :key="i" :class="'d-' + r.kind">{{ r.text }}</div>
        </div>
        <pre v-else :class="{ 'is-bad': ok === false }"><code>{{ output }}</code></pre>
      </template>
      <div v-if="!output && !running" class="lb is-dim">没有输出</div>
    </div>
  </div>
</template>

<style scoped>
.st {
  width: 16px;
  height: 16px;
  border-radius: var(--wb-radius-full);
  display: grid;
  place-items: center;
  font-size: var(--wb-fs-xs);
  flex: none;
}
.st.is-run {
  color: var(--wb-live);
  background: var(--wb-live-soft);
  animation: pulse 1.1s ease-in-out infinite;
}
.st.is-ok {
  color: var(--wb-success);
  background: var(--wb-success-soft);
}
.st.is-bad {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
@keyframes pulse {
  50% { opacity: 0.35; }
}
.is-dim { color: var(--wb-muted); }
pre.is-bad { color: var(--wb-danger); }
/* diff 视图：加删行各自带极淡底色，超长改动内部滚动不撑爆卡片 */
.diff {
  max-height: 320px;
  overflow-y: auto;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius);
  padding: var(--wb-sp-2) 0;
}
.diff > div {
  padding: 0 var(--wb-sp-2);
  white-space: pre-wrap;
  word-break: break-all;
}
/* 展开：内容自上而下轻轻展开，箭头同步转向——没有这层过渡，折叠条会「跳」 */
.tool-bd {
  animation: tool-unfold var(--wb-dur) var(--wb-ease);
}
@keyframes tool-unfold {
  from {
    opacity: 0;
    transform: translateY(-4px);
  }
}
.chev {
  transition: transform var(--wb-dur-fast) var(--wb-ease);
}
.tool.open .chev {
  transform: rotate(90deg);
}
</style>
