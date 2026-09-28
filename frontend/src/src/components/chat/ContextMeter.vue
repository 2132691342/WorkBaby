<script setup lang="ts">
// 上下文水位：贴输入框底边的一条细线 + 一行极小读数，平时几乎看不见。
import { computed } from 'vue'
import { fmtCount } from '../../utils/num'

const props = defineProps<{ used: number; win: number; ratio: number }>()

// ratio 为 0 表示这个模型没告诉我们窗口多大，这时不猜、不画比例。
const known = computed(() => props.ratio > 0 && props.win > 0)
const level = computed(() => (props.ratio >= 95 ? 'is-danger' : props.ratio >= 80 ? 'is-warn' : ''))
const reading = computed(() => (known.value ? `${fmtCount(props.used)} / ${fmtCount(props.win)}` : '—'))
const tip = '这轮对话已经占了多少模型窗口，超了会自动整理较早的内容'
</script>

<template>
  <span class="ctx-txt" :title="tip">{{ reading }}</span>
  <span
    class="ctx-bar"
    :class="[level, { 'is-on': known && ratio > 0 }]"
    role="img"
    :aria-label="known ? `上下文已用 ${ratio}%` : '上下文占用未知'"
  >
    <i :style="{ width: `${known ? ratio : 0}%` }" />
  </span>
</template>

<style scoped>
.ctx-txt {
  flex: none;
  font-size: var(--wb-fs-hint);
  font-variant-numeric: tabular-nums;
  color: var(--wb-muted);
  opacity: 0.55;
  transition: opacity var(--wb-dur) var(--wb-ease);
  white-space: nowrap;
}
.ctx-bar {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 3px;
  overflow: hidden;
  border-radius: 0 0 var(--wb-radius-lg) var(--wb-radius-lg);
  background: var(--wb-line);
  opacity: 0.35;
  transition: opacity var(--wb-dur) var(--wb-ease);
}
.ctx-bar > i {
  display: block;
  height: 100%;
  background: var(--wb-line-2);
  transition: width var(--wb-dur) var(--wb-ease), background var(--wb-dur) var(--wb-ease);
}
.ctx-bar.is-warn > i {
  background: var(--wb-warning);
}
.ctx-bar.is-danger > i {
  background: var(--wb-danger);
}
.ctx-bar.is-warn,
.ctx-bar.is-danger {
  opacity: 1;
}
.composer:hover .ctx-txt,
.composer:focus-within .ctx-txt {
  opacity: 1;
}
.composer:hover .ctx-bar,
.composer:focus-within .ctx-bar {
  opacity: 1;
}
</style>
