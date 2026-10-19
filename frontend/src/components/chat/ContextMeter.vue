<script setup lang="ts">
// 上下文水位：输入框底栏常驻的仪表环 + 读数。
// 无论是否开始对话、窗口是否已知，这里都必须有内容——什么都不显示等于用户没有这个信息。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { fmtCount } from '../../utils/num'

const props = defineProps<{
  used: number
  win: number
  ratio: number
  known?: boolean
  /** 给模型输出留的余量：环的分母是「窗口 - 余量」，与助手开始整理的位置一致 */
  reserve?: number
}>()

const { t } = useI18n()

const exact = computed(() => props.known !== false)
const pct = computed(() => Math.max(0, Math.min(100, props.ratio || 0)))
const level = computed(() => (pct.value >= 95 ? 'is-danger' : pct.value >= 80 ? 'is-warn' : ''))

const RING_R = 7
const RING_C = 2 * Math.PI * RING_R
const dash = computed(() => `${((RING_C * pct.value) / 100).toFixed(2)} ${RING_C.toFixed(2)}`)

const reading = computed(() => {
  const used = fmtCount(props.used || 0)
  if (!props.win || props.win <= 0) return used
  return `${used} / ${fmtCount(props.win)}`
})

// 整理线：分母与后端 ratioOf 一致（窗口减余量）
const slimLine = computed(() => {
  const win = props.win || 0
  const reserve = props.reserve || 0
  if (win <= 0) return 0
  return win - reserve > 0 ? win - reserve : Math.floor(win / 2)
})

const tip = computed(() => {
  if (!props.win || props.win <= 0) {
    return `${t('chat.contextMeter')}: ${fmtCount(props.used || 0)}`
  }
  const base = t('chat.contextHint', {
    used: fmtCount(props.used || 0),
    win: fmtCount(props.win || 0),
  })
  const line = slimLine.value > 0 ? ` · ${fmtCount(slimLine.value)}` : ''
  return `${base} · ${pct.value}%${line}`
})
</script>

<template>
  <el-tooltip :content="tip" placement="top" :show-after="250">
    <span class="ctx" :class="[level, { 'is-est': !exact }]" role="img" :aria-label="tip">
      <svg class="ring" viewBox="0 0 18 18" aria-hidden="true">
        <circle class="ring-b" cx="9" cy="9" :r="RING_R" />
        <circle class="ring-f" cx="9" cy="9" :r="RING_R" :style="{ strokeDasharray: dash }" />
      </svg>
      <span class="txt">{{ reading }}</span>
    </span>
  </el-tooltip>
</template>

<style scoped>
/* 裸读数：只有环与数字，不套描边胶囊——底栏已有模型 / 思考两个 chip，
   再给水位加一个盒子就是第三个「看起来能点」的框，而它只读。 */
.ctx {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  flex: none;
  height: var(--wb-ctl-h-sm);
  color: var(--wb-muted);
  cursor: default;
  /* 读数不可选、不画焦点框：拖选正文时扫过它，浏览器会给 16px 的环套一个淡蓝方块，
     比读数本身显眼得多（el-tooltip 会把触发器标成可聚焦）。键盘聚焦时 tooltip 自己会出来。 */
  user-select: none;
  border-radius: var(--wb-radius-full);
  transition: color var(--wb-dur-fast) var(--wb-ease);
}
/* 显式写全：base.css 的 :focus-visible 画全局焦点环，同权重时靠源序赢，不靠猜。 */
.ctx:focus,
.ctx:focus-visible,
.ctx *:focus,
.ctx *:focus-visible {
  outline: none;
}
/* 只换字色、不缩放不铺底：它是读数不是控件，任何「按下去」的暗示都是骗点击。 */
.ctx:hover {
  color: var(--wb-ink-2);
}
/* 仪表环：细轨 + 平头弧。圆头 + 深轨在低占比时会读成一个转圈加载图标，
   这里刻意的平头 + 浅轨就是为了让它一眼是「水位表」不是「spinner」。 */
.ring {
  width: calc(16px * var(--wb-vw-scale));
  height: calc(16px * var(--wb-vw-scale));
  flex: none;
  transform: rotate(-90deg);
}
.ring-b {
  fill: none;
  stroke: var(--wb-border);
  stroke-width: 2;
}
.ring-f {
  fill: none;
  stroke: var(--wb-primary);
  stroke-width: 2;
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
.is-warn .txt {
  color: var(--wb-warning);
}
.is-danger .txt {
  color: var(--wb-danger);
}
.txt {
  font-size: var(--wb-fs-2xs);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  font-weight: 600;
}
/* 窗口是估算值时读数降一档亮度：数字给全，但别当成实测值看 */
.is-est .txt {
  opacity: 0.72;
}
</style>
