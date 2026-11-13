<script setup lang="ts">
// 工具卡：一行摘要 + 展开后的参数 / 输出；改动看右侧差异抽屉，引擎带 diff 却没给路径时才就地铺差异行。
// 头压一行，挤压时截摘要不截工具名。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import DiffRows from '@/components/common/DiffRows.vue'
import { openFilePeek } from '@/composables/useFilePeek'
import type { ChatBlock, DiffLine } from '@/types/api'

const props = defineProps<{ block: ChatBlock }>()
const { t } = useI18n()

const open = ref(false)
watch(
  () => props.block.tool_status,
  (status, prev) => {
    // 只有出错才自动展开：差异到货就展开会让流式中的版面不停跳动，
    // 「它改了什么」由头一行的「看改动」负责，那是抽屉的入口。
    if (prev === 'running' && status === 'error') open.value = true
  },
)

const status = computed(() => props.block.tool_status ?? 'pending')
const label = computed(() => {
  if (status.value === 'running') return t('chat.toolRunning')
  if (status.value === 'error') return t('chat.toolFailed')
  if (status.value === 'done') return t('chat.toolDone')
  return ''
})

const args = computed(() => {
  const raw = props.block.tool_args
  if (raw === undefined || raw === null) return ''
  if (typeof raw === 'string') return raw
  try {
    return JSON.stringify(raw, null, 2)
  } catch {
    return String(raw)
  }
})

const headline = computed(() => summarise(props.block.tool_args))
/** 一行摘要：从参数里挑最像「在做什么」的那个字段。 */
function summarise(raw: unknown): string {
  if (!raw || typeof raw !== 'object') return typeof raw === 'string' ? raw : ''
  const record = raw as Record<string, unknown>
  for (const key of ['command', 'path', 'file_path', 'pattern', 'query', 'url', 'i']) {
    const value = record[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  const first = Object.values(record).find((v) => typeof v === 'string')
  return typeof first === 'string' ? first : ''
}

const duration = computed(() => {
  const ms = props.block.duration_ms
  if (!ms || ms < 0) return ''
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(ms < 10000 ? 2 : 1)}s`
})

const output = computed(() => props.block.tool_output ?? '')

/** 差异行：引擎改了什么。没有差异的工具（读文件、跑命令）没有这一段。 */
const diffLines = computed<DiffLine[]>(() => props.block.diff_lines ?? [])

/** 差异的增删行数。 */
const diffStat = computed(() => {
  let adds = 0
  let dels = 0
  for (const line of diffLines.value) {
    if (line.line_kind === 'add') adds++
    else if (line.line_kind === 'del') dels++
  }
  return { adds, dels }
})

/** 被改动的文件：只取末段，路径通常很长。 */
const fileName = computed(() => {
  const path = props.block.file_path ?? ''
  if (!path) return ''
  const parts = path.split(/[\\/]/).filter(Boolean)
  return parts.length ? parts[parts.length - 1] : path
})

/**
 * 能不能「看改动」（打开差异抽屉）。
 *
 * 只要知道文件路径就行——抽屉问的是工作区里的真实差异，
 * 不依赖引擎这一帧带没带 diff，也不要求文件落在当前工作区内：
 * 引擎常在别的项目目录里改东西，按工作区过滤会把最常见的情形挡掉。
 */
const canPeek = computed(() => Boolean(props.block.file_path))

/**
 * 就地铺差异行的条件：引擎带了 diff 却没说文件路径。
 * 有路径的一律走抽屉——两处看同一份改动只会让人怀疑哪份是真的。
 */
const inlineDiff = computed(() => diffLines.value.length > 0 && !props.block.file_path)

/** 摘要那一行显示什么：优先文件名（用户认文件，不认命令）。 */
const summaryText = computed(() => fileName.value || headline.value)
</script>

<template>
  <div class="tool" :class="[status, { 'is-open': open }]">
    <div class="tool-head">
      <button class="tool-toggle" type="button" :aria-expanded="open" @click="open = !open">
        <span class="tool-dot" :class="status" :title="label">
          <AppIcon v-if="status === 'running'" name="refresh" :size="11" class="spin" />
          <AppIcon v-else-if="status === 'error'" name="close" :size="11" />
          <AppIcon v-else-if="status === 'done'" name="check" :size="11" />
          <span v-else class="idle-dot" />
        </span>

        <span class="tool-name">{{ block.tool_name || 'tool' }}</span>

        <span class="tool-summary" :title="summaryText">{{ summaryText }}</span>

        <span v-if="diffLines.length" class="tool-stat">
          <b class="is-add">+{{ diffStat.adds }}</b>
          <b class="is-del">−{{ diffStat.dels }}</b>
        </span>

        <span v-if="duration" class="tool-time">{{ duration }}</span>

        <AppIcon name="chevron" :size="11" class="tool-chevron" :class="{ open }" />
      </button>

      <!-- 「看改动」在头这一行里，不另起一行：
           另起一行的话每个工具调用都占两行，一轮十几个调用就多出一屏空白。 -->
      <button
        v-if="canPeek"
        class="tool-peek"
        type="button"
        :title="block.file_path"
        @click="openFilePeek(block.file_path ?? '', fileName)"
      >
        <AppIcon name="git" :size="11" />
        {{ t('chat.viewChanges') }}
      </button>
    </div>

    <div v-if="open" class="tool-body">
      <!-- 没有文件路径的 diff 只能就地铺：抽屉不知道去问哪个文件。 -->
      <div v-if="inlineDiff">
        <div class="tool-label">{{ t('chat.toolChanges') }}</div>
        <div class="tool-diff">
          <DiffRows :lines="diffLines" />
        </div>
      </div>
      <template v-if="args">
        <div>
          <div class="tool-label">{{ t('chat.toolArgs') }}</div>
          <pre class="tool-pre selectable">{{ args }}</pre>
        </div>
      </template>
      <template v-if="output">
        <div>
          <div class="tool-label">{{ t('chat.toolOutput') }}</div>
          <pre class="tool-pre selectable">{{ output }}</pre>
        </div>
      </template>
      <div v-if="!args && !output && !inlineDiff" class="tool-label">{{ t('common.empty') }}</div>
    </div>
  </div>
</template>

<style scoped>
/* 头是严格一行：除摘要以外每一段都 flex: none，被挤的只能是摘要。
   卡片化之后头有自己的上下留白，按钮负责整卡的展开/收起。 */
.tool-toggle {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: 1;
  min-width: 0;
  padding: 6px var(--wb-sp-2) 6px var(--wb-sp-3);
  border: none;
  background: transparent;
  text-align: left;
  color: var(--wb-ink);
  font-size: var(--wb-fs-xs);
  font-family: inherit;
  cursor: pointer;
}

/* 状态点：16px 的圈。运行中转，结束变绿，出错变红；未开始是一个灰点。 */
.tool-dot {
  display: grid;
  place-items: center;
  width: 16px;
  height: 16px;
  flex: none;
  border-radius: var(--wb-r-full);
  background: var(--wb-hover);
  color: var(--wb-mute);
}
.tool-dot.done {
  background: var(--wb-ok-soft);
  color: var(--wb-ok);
}
.tool-dot.error {
  background: var(--wb-bad-soft);
  color: var(--wb-bad);
}
.tool-dot.running {
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
}
.spin {
  animation: tool-spin 900ms linear infinite;
}
@keyframes tool-spin {
  to {
    transform: rotate(360deg);
  }
}
.idle-dot {
  width: 5px;
  height: 5px;
  border-radius: var(--wb-r-full);
  background: var(--wb-faint);
}

/* 工具名：等宽、不换行、不参与收缩 */
.tool-name {
  flex: none;
  max-width: 34%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 摘要：唯一允许被挤压的一段 */
.tool-summary {
  flex: 1;
  min-width: 0;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 增删统计：跟在摘要后面，是用户扫一眼就想知道的数字 */
.tool-stat {
  display: flex;
  gap: var(--wb-sp-1);
  flex: none;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  font-variant-numeric: tabular-nums;
}
.tool-stat .is-add {
  color: var(--wb-ok);
}
.tool-stat .is-del {
  color: var(--wb-bad);
}

.tool-time {
  flex: none;
}

.tool-chevron {
  flex: none;
  color: var(--wb-faint);
  transition: transform var(--wb-dur-fast) var(--wb-ease);
}
.tool-chevron.open {
  transform: rotate(90deg);
}

/* 「看改动」是头这一行的最后一个动作：带文字的链接式按钮，
   进的是差异抽屉。不另起一行，空间紧张时先缩摘要再缩它。 */
.tool-peek {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex: none;
  padding: 0 var(--wb-sp-2) 0 0;
  border: none;
  background: transparent;
  color: var(--wb-brand);
  font-size: var(--wb-fs-3xs);
  font-weight: 600;
  font-family: inherit;
  cursor: pointer;
  transition:
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.tool-peek:hover {
  color: var(--wb-brand-strong);
  transform: scale(var(--wb-press-hover));
}
.tool-peek:active {
  transform: scale(var(--wb-press-active));
}

/* 差异表：三列（旧行号 / 新行号 / 正文），与工作区的差异视图同一套列宽。 */
.tool-diff {
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-sm);
  overflow: hidden;
  background: var(--wb-code-bg);
  max-height: 320px;
  overflow-y: auto;
}
</style>
