<script setup lang="ts">
// 近 N 天 token 柱状图：纯 div 高度百分比，不引任何图表库。
import { computed } from 'vue'
import type { StatsDailyItem } from '../../types/api'
import { fmtCount } from '../../utils/num'

const props = defineProps<{ daily: StatsDailyItem[] }>()

const max = computed(() => Math.max(1, ...props.daily.map((d) => d.total || 0)))
// 天数一多 x 轴就只标首尾与每隔几天，免得挤成一团
const step = computed(() => (props.daily.length <= 7 ? 1 : props.daily.length <= 14 ? 2 : 5))

const bars = computed(() =>
  props.daily.map((d, i) => ({
    key: d.date || String(i),
    label: (d.date || '').slice(5).replace('-', '/'),
    input: `${Math.round(((d.input || 0) / max.value) * 1000) / 10}%`,
    output: `${Math.round(((d.output || 0) / max.value) * 1000) / 10}%`,
    tip: `${d.date}：输入 ${fmtCount(d.input)} · 输出 ${fmtCount(d.output)} · 合计 ${fmtCount(d.total)}`,
    showX: i === 0 || i === props.daily.length - 1 || (i + 1) % step.value === 0,
  })),
)
</script>

<template>
  <div class="plot">
    <div
      v-for="b in bars"
      :key="b.key"
      class="col"
      tabindex="0"
      role="img"
      :title="b.tip"
      :aria-label="b.tip"
    >
      <span class="stack">
        <span class="tip">{{ b.tip }}</span>
        <i class="seg out" :style="{ height: b.output }" />
        <i class="seg in" :style="{ height: b.input }" />
      </span>
      <span class="x" :class="{ 'is-on': b.showX }">{{ b.showX ? b.label : '' }}</span>
    </div>
  </div>
</template>

<style scoped>
/* 绘图区高度不是控件高度，不受 --wb-ctl-h 约束 */
.plot {
  --plot-h: 150px;
  display: flex;
  align-items: flex-end;
  gap: 2px;
}
.col {
  flex: 1 1 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  cursor: default;
  border-radius: var(--wb-radius-xs);
}
.stack {
  position: relative;
  width: 100%;
  max-width: 22px;
  height: var(--plot-h);
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
  border-bottom: 1px solid var(--wb-line);
}
.seg {
  display: block;
  width: 100%;
  min-height: 1px;
  border-radius: 2px 2px 0 0;
}
.seg.in {
  background: var(--wb-primary);
}
.seg.out {
  background: var(--wb-ch-2);
  border-radius: 0 0 2px 2px;
  margin-bottom: -1px;
}
.x {
  height: 1.5em;
  padding-top: 2px;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-hint);
  color: var(--wb-muted);
  white-space: nowrap;
  overflow: hidden;
}
.tip {
  position: absolute;
  bottom: calc(100% + 6px);
  left: 50%;
  transform: translateX(-50%);
  display: none;
  z-index: var(--wb-z-pop);
  padding: 5px var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-surface-solid);
  border: 1px solid var(--wb-border-strong);
  box-shadow: var(--wb-shadow-pop);
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
  white-space: nowrap;
  pointer-events: none;
}
.col:hover .tip,
.col:focus-visible .tip {
  display: block;
}
/* 首尾两根柱子离卡片边缘太近，提示条改成贴边展开，别被裁掉 */
.col:nth-child(-n + 2) .tip {
  left: 0;
  transform: none;
}
.col:nth-last-child(-n + 2) .tip {
  left: auto;
  right: 0;
  transform: none;
}
.col:hover .seg.in,
.col:focus-visible .seg.in {
  background: var(--wb-primary-strong);
}
</style>
