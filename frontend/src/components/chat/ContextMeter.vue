<script setup lang="ts">
// 上下文水位：输入框底栏常驻的圆环 + 读数。
// 无论是否开始对话、窗口是否已知，这里都必须有内容——什么都不显示等于用户没有这个信息。
import { computed } from 'vue'
import { fmtCount } from '../../utils/num'

const props = defineProps<{
  used: number
  win: number
  ratio: number
  known?: boolean
  /** 给模型输出留的余量：环的分母是「窗口 - 余量」，与助手开始整理的位置一致 */
  reserve?: number
}>()

// known=false 表示窗口来自缺省估算：读数加「约」前缀，数字照常给。
// 不给读数（老实现的做法）用户只看到「窗口未知」，等于这个功能不存在。
const exact = computed(() => props.known !== false)
const pct = computed(() => Math.max(0, Math.min(100, props.ratio || 0)))
const level = computed(() => (pct.value >= 95 ? 'is-danger' : pct.value >= 80 ? 'is-warn' : ''))

// 圆环：r=6.5 的周长，按占用比例画弧。
const RING_R = 6.5
const RING_C = 2 * Math.PI * RING_R
const dash = computed(() => `${((RING_C * pct.value) / 100).toFixed(2)} ${RING_C.toFixed(2)}`)

const reading = computed(() => {
  const used = fmtCount(props.used || 0)
  if (!props.win || props.win <= 0) return used
  return `${exact.value ? '' : '约 '}${used} / ${fmtCount(props.win)}`
})
// 整理线：分母与后端 ratioOf 一致（窗口减余量）。数字给出来，用户才知道「该开新会话了」
// 这句话具体对应到多少 token。
const slimLine = computed(() => {
  const win = props.win || 0
  const reserve = props.reserve || 0
  if (win <= 0) return 0
  return win - reserve > 0 ? win - reserve : Math.floor(win / 2)
})
const tip = computed(() => {
  const line =
    slimLine.value > 0
      ? `到 ${fmtCount(slimLine.value)} 会自动整理较早的内容`
      : '快满时会自动整理较早的内容'
  if (!exact.value) {
    return `上下文已用 ${fmtCount(props.used || 0)}（约），窗口是估算值；可在「设置 · 行为」里手填真实窗口。${line}`
  }
  return `这轮对话已占可用预算的 ${pct.value}%（已用 ${fmtCount(props.used || 0)} / 窗口 ${fmtCount(props.win || 0)}）；${line}`
})
const aria = computed(() =>
  props.win > 0 ? `上下文已用 ${pct.value}%` : `上下文已用 ${fmtCount(props.used || 0)}`,
)
</script>

<template>
  <span class="ctx" :class="[level, { 'is-est': !exact }]" :title="tip" role="img" :aria-label="aria">
    <svg class="ring" viewBox="0 0 18 18" aria-hidden="true">
      <circle class="ring-b" cx="9" cy="9" :r="RING_R" />
      <circle class="ring-f" cx="9" cy="9" :r="RING_R" :style="{ strokeDasharray: dash }" />
    </svg>
    <span class="txt">{{ reading }}</span>
  </span>
</template>

<style scoped>
.ctx {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  flex: none;
  height: var(--wb-ctl-h-sm);
  padding: 0 8px 0 3px;
  border-radius: var(--wb-radius-full);
  border: 1px solid transparent;
  color: var(--wb-ink-2);
  cursor: default;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease);
}
/* 只给底色提示、不做缩放：它是读数不是控件，悬停放大等于骗用户去点 */
.ctx:hover {
  background: var(--wb-tint);
  border-color: var(--wb-line);
}
.ring {
  width: 18px;
  height: 18px;
  flex: none;
  transform: rotate(-90deg);
}
.ring-b {
  fill: none;
  stroke: var(--wb-line-2);
  stroke-width: 2.4;
}
.ring-f {
  fill: none;
  stroke: var(--wb-primary);
  stroke-width: 2.4;
  stroke-linecap: round;
  transition:
    stroke-dasharray var(--wb-dur) var(--wb-ease),
    stroke var(--wb-dur) var(--wb-ease);
}
.is-warn .ring-f {
  stroke: var(--wb-warning);
}
.is-danger .ring-f {
  stroke: var(--wb-danger);
}
.is-warn .txt,
.is-danger .txt {
  color: var(--wb-ink);
}
.txt {
  font-size: var(--wb-fs-hint);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
/* 窗口是估算值时读数降一档亮度：数字给全，但别当成实测值看 */
.is-est .txt {
  opacity: 0.72;
}
</style>
