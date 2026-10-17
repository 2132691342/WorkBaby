<script setup lang="ts">
// 差异视图：把后端折好的行结构渲染成等宽两栏（旧 / 新行号 + 正文）。
// 只做展示——提交与回滚都交给版本工具，界面不替用户决定不可逆操作。
import { computed } from 'vue'
import type { GitDiffLineVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{
  lines: GitDiffLineVO[]
  path: string
  truncated: boolean
  dropped: number
  loading: boolean
}>()

const adds = computed(() => props.lines.filter((l) => l.kind === 'add').length)
const dels = computed(() => props.lines.filter((l) => l.kind === 'del').length)
// 方向用行首符号说（+ / -）：语义交给符号与淡底，描边继续只承担分层
const SIGN: Record<string, string> = { add: '+', del: '-' }
const blank = computed(() => !props.loading && props.lines.length === 0)
// 「整个工作区」与「某个文件」的空态不是一回事：前者是没改动，后者是与上一次提交一致
const blankText = computed(() =>
  props.path ? '这个文件现在和上一次提交一致' : '工作区还没有改动',
)
</script>

<template>
  <section class="diff card">
    <div class="df-head">
      <span class="df-ic"><AppIcon name="git-diff" size="ic-sm" /></span>
      <span class="df-path mono" :title="path || '整个工作区'">{{ path || '整个工作区' }}</span>
      <span class="df-num">
        <b class="up">+{{ adds }}</b>
        <b class="down">-{{ dels }}</b>
      </span>
      <span v-if="loading" class="df-spin" aria-live="polite"><AppIcon name="loader" size="ic-xs" spin /></span>
    </div>

    <div v-if="loading" class="df-wait">正在读改动…</div>
    <div v-else-if="blank" class="df-wait">{{ blankText }}</div>
    <div v-else class="df-body scroll">
      <div
        v-for="(l, i) in lines"
        :key="i"
        class="dl"
        :class="l.kind"
      >
        <template v-if="l.kind === 'file'">
          <span class="dl-file mono">{{ l.text }}</span>
        </template>
        <template v-else-if="l.kind === 'hunk'">
          <span class="dl-hunk mono">{{ l.text }}</span>
        </template>
        <template v-else>
          <span class="no">{{ l.old_no || '' }}</span>
          <span class="no">{{ l.new_no || '' }}</span>
          <span class="sg" :class="l.kind">{{ SIGN[l.kind] || ' ' }}</span>
          <span class="tx mono">{{ l.text }}</span>
        </template>
      </div>
    </div>

    <p v-if="!loading && truncated" class="df-cut">
      还有 {{ dropped }} 行没显示：改动太大，只取前面这些
    </p>
  </section>
</template>

<style scoped>
.diff {
  display: flex;
  flex-direction: column;
  min-width: 0;
  padding: var(--wb-sp-4);
}
.df-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin-bottom: var(--wb-sp-3);
  min-width: 0;
}
.df-ic {
  display: grid;
  place-items: center;
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  flex: none;
  border-radius: var(--wb-radius-sm);
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
}
.df-path {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
}
.df-num {
  margin-left: auto;
  flex: none;
  display: flex;
  gap: var(--wb-sp-2);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
}
.df-num .up {
  color: var(--wb-success);
}
.df-num .down {
  color: var(--wb-danger);
}
.df-spin {
  flex: none;
  color: var(--wb-live);
}

.df-body {
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius);
  background: var(--wb-surface-2);
  overflow: auto;
  /* 差异自己滚：几百行的改动不撑长整页，列表与分支条始终在视野里 */
  max-height: 62vh;
  min-height: 0;
}
.dl {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-xs);
  line-height: 1.55;
}
.dl .no {
  flex: none;
  width: 40px;
  padding: 0 var(--wb-sp-1);
  text-align: right;
  color: var(--wb-muted);
  user-select: none;
  opacity: 0.75;
}
.dl .tx {
  flex: 1 1 auto;
  min-width: 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  color: var(--wb-ink);
}
/* 新增 / 删除：淡底 + 行首符号。描边是分层符号，不能被语义色占掉 */
.dl.add {
  background: var(--wb-success-soft);
}
.dl.del {
  background: var(--wb-danger-soft);
}
.dl .sg {
  flex: none;
  width: 10px;
  text-align: center;
  color: var(--wb-muted);
  user-select: none;
}
.dl .sg.add {
  color: var(--wb-success);
}
.dl .sg.del {
  color: var(--wb-danger);
}
.dl.hunk {
  background: var(--wb-tint);
  color: var(--wb-muted);
}
.dl-hunk {
  padding: 2px var(--wb-sp-2);
}
.dl.file {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--wb-surface);
  border-top: 1px solid var(--wb-line);
}
.dl.file:first-child {
  border-top: none;
}
.dl-file {
  padding: 4px var(--wb-sp-2);
  font-size: var(--wb-fs-xs);
  font-weight: 600;
  color: var(--wb-ink-2);
}

.df-wait {
  padding: var(--wb-sp-6) var(--wb-sp-4);
  text-align: center;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.df-cut {
  margin-top: var(--wb-sp-2);
  font-size: var(--wb-fs-hint);
  color: var(--wb-muted);
}

</style>
