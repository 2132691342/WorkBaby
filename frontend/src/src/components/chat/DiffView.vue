<script setup lang="ts">
/**
 * 带行号 gutter 的 unified diff 视图：旧/新两侧行号 + 红绿底色行。
 *
 * <p>行号解析在 blocks.parseDiffRows（hunk 头取种子逐行推进；非标准 diff 只标新侧序号）。
 * InlineDiffCard（文件变更卡）与 MessageBlocksRenderer（工具展开体）共用，保证同一份 diff
 * 在消息流两个入口长一个样。
 */
import { computed } from 'vue'
import { parseDiffRows } from '@/chat/models/blocks'

const props = defineProps<{
  diff: string
  /** 最大高度（px）；超限内滚。 */
  maxHeight?: number
}>()

const rows = computed(() => parseDiffRows(props.diff))
const style = computed(() => (props.maxHeight ? { maxHeight: props.maxHeight + 'px' } : undefined))
</script>

<template>
  <div class="wb-diffview" :style="style">
    <div
      v-for="(r, i) in rows"
      :key="i"
      class="wb-diffview-row"
      :class="`is-${r.type}`"
    >
      <span class="wb-diffview-no">{{ r.oldNo ?? '' }}</span>
      <span class="wb-diffview-no">{{ r.newNo ?? '' }}</span>
      <span class="wb-diffview-sign">{{ r.type === 'add' ? '+' : r.type === 'del' ? '-' : '' }}</span>
      <span class="wb-diffview-txt">{{ r.type === 'hunk' ? r.text : r.text.replace(/^[+-]/, '') }}</span>
    </div>
  </div>
</template>

<style scoped>
.wb-diffview {
  overflow: auto;
  overscroll-behavior: contain;
  font-family: var(--font-mono);
  font-size: 11px;
  line-height: 1.6;
}
.wb-diffview-row {
  display: flex;
  align-items: baseline;
  min-width: max-content;
  padding: 0 8px 0 0;
}
.wb-diffview-row.is-add {
  background: color-mix(in srgb, var(--wb-mint) 10%, transparent);
}
.wb-diffview-row.is-del {
  background: color-mix(in srgb, var(--wb-danger) 10%, transparent);
}
.wb-diffview-no {
  flex: none;
  width: 34px;
  padding-right: 6px;
  text-align: right;
  color: var(--wb-muted);
  opacity: 0.65;
  font-variant-numeric: tabular-nums;
  user-select: none;
}
.wb-diffview-sign {
  flex: none;
  width: 12px;
  text-align: center;
  user-select: none;
}
.is-add .wb-diffview-sign,
.is-add .wb-diffview-txt {
  color: var(--wb-mint);
}
.is-del .wb-diffview-sign,
.is-del .wb-diffview-txt {
  color: var(--wb-danger);
}
.is-hunk .wb-diffview-txt {
  color: var(--wb-primary-strong);
}
.wb-diffview-txt {
  white-space: pre;
}
</style>
