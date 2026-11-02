<script setup lang="ts">
// 工具执行卡：折叠态是一行摘要（状态 + 工具名 + 参数摘要 + 耗时），展开看参数与输出。
// 写文件类工具的后端会带回改动文件与差异行：改动就地着色，不用切到工作区页。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { summarizeArgs } from '../../utils/md'
import { fmtMs } from '../../utils/num'
import type { DiffLine } from '../../types/chatEvents'
import { openFilePeek } from '../../composables/filePeek'
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
  /** 后端回报的改动文件（相对工作区） */
  filePath?: string
  /** 后端回报的差异行 */
  diff?: DiffLine[]
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

/** 差异行统计：折叠态也显示，一眼看出这次动了多少。 */
const diffStat = computed(() => {
  const rows = props.diff
  if (!rows || !rows.length) return null
  let add = 0
  let del = 0
  for (const r of rows) {
    if (r.line_kind === 'add') add++
    else if (r.line_kind === 'del') del++
  }
  return add || del ? { add, del } : null
})

// 改了哪些文件是这一轮最该看的：路径就在参数里，点一下就地看改动，不用切到工作区。
const filePath = computed(() => {
  // 后端回报的改动文件优先：参数里的路径可能是绝对路径或相对别处
  if (props.filePath) return props.filePath
  const a = props.args || {}
  const raw = a.path ?? a.file_path ?? a.filePath
  return typeof raw === 'string' ? raw : ''
})
const canPeek = computed(
  () => !!filePath.value && ['edit', 'write', 'read', 'ffread'].includes(props.tool || ''),
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
      <!-- 改了文件的工具：增删行数直接写在折叠态上，不必展开就知道动了多少 -->
      <span v-if="diffStat" class="tool-stat">
        <span class="is-add">+{{ diffStat.add }}</span>
        <span class="is-del">-{{ diffStat.del }}</span>
      </span>
      <span class="tool-meta">
        <span v-if="duration_ms">{{ fmtMs(duration_ms) }}</span>
        <AppIcon class="tool-chev" :name="open ? 'chevron-down' : 'chevron-right'" size="ic-xs" />
      </span>
    </button>

    <!-- 文件类工具：就地看改动，不必切走工作区再切回来 -->
    <div v-if="canPeek" class="tool-acts">
      <button class="tool-peek" type="button" :title="filePath" @click.stop="openFilePeek(filePath)">
        <AppIcon name="git-diff" size="ic-2xs" />
        <span>{{ t('chat.toolPeek') }}</span>
      </button>
    </div>

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
.tool-acts {
  display: flex;
  justify-content: flex-end;
  padding: 0 var(--wb-sp-2) var(--wb-sp-1);
}
.tool-peek {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  height: calc(20px * var(--wb-vw-scale));
  padding: 0 var(--wb-sp-2);
  border-radius: var(--wb-radius-xs);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-primary);
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.tool-peek:hover {
  background: var(--wb-accent-tint);
  transform: scale(1.06);
}
.tool-peek:active {
  transform: scale(var(--wb-btn-scale-press));
}
.tool-stat {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  flex: none;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
}
.tool-stat .is-add {
  color: var(--wb-success);
}
.tool-stat .is-del {
  color: var(--wb-danger);
}
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
