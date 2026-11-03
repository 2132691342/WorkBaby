<script setup lang="ts">
// 差异行表：只有渲染，没有任何交互；工具卡与工作区差异视图共用
// （行结构已由后端归一，着色这类细节写两遍必有出入）。
// 内联三列 / 并排四列两种摆法；并排的配对由调用方完成——只有它知道谁配谁。
import type { DiffLine } from '@/types/api'

defineProps<{
  /** 内联要画的行；与 pairs 二选一 */
  lines?: DiffLine[]
  /** 并排要画的行对；每对最多两项（左 / 右），缺的一侧留空 */
  pairs?: DiffLine[][]
}>()

/** 一行属于哪一种：决定底色与字色。 */
function rowClass(line: DiffLine | undefined): string {
  if (!line) return ''
  if (line.line_kind === 'add') return 'is-add'
  if (line.line_kind === 'del') return 'is-del'
  if (line.line_kind === 'hunk') return 'is-hunk'
  if (line.line_kind === 'file') return 'is-file'
  return ''
}

/** 并排的一对：删除在左、新增在右；上下文行两侧同文。 */
function sides(pair: DiffLine[]): { left?: DiffLine; right?: DiffLine } {
  if (pair.length === 2) return { left: pair[0], right: pair[1] }
  const only = pair[0]
  if (!only) return {}
  if (only.line_kind === 'add') return { right: only }
  if (only.line_kind === 'del') return { left: only }
  return { left: only, right: only }
}
</script>

<template>
  <div class="diff-rows">
    <template v-if="pairs">
      <div v-for="(pair, i) in pairs" :key="`p-${i}`" class="pair-row">
        <span class="ln">{{ sides(pair).left?.old_no || '' }}</span>
        <span class="tx" :class="rowClass(sides(pair).left)">{{ sides(pair).left?.line_text ?? '' }}</span>
        <span class="ln">{{ sides(pair).right?.new_no || '' }}</span>
        <span class="tx" :class="rowClass(sides(pair).right)">{{ sides(pair).right?.line_text ?? '' }}</span>
      </div>
    </template>
    <template v-else>
      <div v-for="(line, i) in lines ?? []" :key="`i-${i}`" class="diff-row" :class="rowClass(line)">
        <span class="ln">{{ line.old_no || '' }}</span>
        <span class="ln">{{ line.new_no || '' }}</span>
        <span class="tx">{{ line.line_text }}</span>
      </div>
    </template>
  </div>
</template>

<style scoped>
.diff-rows {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  line-height: 1.55;
}

/* 内联：三列 —— 旧行号 / 新行号 / 正文 */
.diff-row {
  display: grid;
  grid-template-columns: 44px 44px 1fr;
  padding: 0 var(--wb-sp-2);
  white-space: pre;
}

/* 并排：四列 —— 旧行号 / 旧正文 / 新行号 / 新正文。
   第三个格子带一条左竖线，是「改动前 / 改动后」的分界。 */
.pair-row {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr) 44px minmax(0, 1fr);
  padding: 0 var(--wb-sp-1);
  white-space: pre;
}
.pair-row > .ln:nth-child(3) {
  border-left: 1px solid var(--wb-line);
}

.ln {
  color: var(--wb-faint);
  text-align: right;
  padding-right: var(--wb-sp-2);
  user-select: none;
  font-variant-numeric: tabular-nums;
}

.tx {
  word-break: break-all;
  white-space: pre-wrap;
  padding-right: var(--wb-sp-2);
}

/* 增删底色：10% 混色，浅色与深色主题下都只是一层薄薄的提示 */
.is-add {
  background: color-mix(in srgb, var(--wb-ok) 10%, transparent);
  color: var(--wb-ink);
}
.is-del {
  background: color-mix(in srgb, var(--wb-bad) 10%, transparent);
  color: var(--wb-ink);
}
.is-hunk {
  color: var(--wb-brand);
  background: var(--wb-hover);
}
.is-file {
  color: var(--wb-faint);
}
</style>
