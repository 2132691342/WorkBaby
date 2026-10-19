<script setup lang="ts">
// 读数卡：主读数 + 标签 + 一行注解 + 右上角水印图标。
// 悬停只做轻微抬升与描边加深——它是展示块，不做「可点」的缩放误导。
import AppIcon from '../common/AppIcon.vue'

withDefaults(
  defineProps<{
    icon: string
    label: string
    value: string
    note?: string
    /** 命中 / 正向指标：数值用成功色 */
    hit?: boolean
    /** 强调卡：淡靛底 + 主色读数（整屏最多一张） */
    accent?: boolean
  }>(),
  { note: '', hit: false, accent: false },
)
</script>

<template>
  <div class="kpi" :class="{ 'is-accent': accent }">
    <AppIcon class="kpi-mark" :name="icon" size="ic-xl" />
    <div class="kpi-top">
      <span class="kpi-label">{{ label }}</span>
      <span class="kpi-tile">
        <AppIcon :name="icon" size="ic-sm" />
      </span>
    </div>
    <div class="kpi-value" :class="{ 'is-hit': hit }">{{ value }}</div>
    <div v-if="note" class="kpi-note">{{ note }}</div>
  </div>
</template>

<style scoped>
.kpi {
  position: relative;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-4) var(--wb-sp-5);
  min-height: calc(112px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xl);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  box-shadow: var(--wb-shadow-1);
  transition:
    box-shadow var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur) var(--wb-ease);
}
.kpi:hover {
  box-shadow: var(--wb-card-shadow-hover);
  border-color: var(--wb-border-strong);
  transform: translateY(-1px);
}
.kpi.is-accent {
  background: var(--wb-accent-tint-soft);
  border-color: var(--wb-accent-border);
}
/* 水印图标：右上角，透明度极低，悬停轻微放大 */
.kpi-mark {
  position: absolute;
  right: calc(-8px * var(--wb-vw-scale));
  top: calc(-8px * var(--wb-vw-scale));
  color: var(--wb-primary);
  opacity: 0.08;
  width: calc(76px * var(--wb-vw-scale)) !important;
  height: calc(76px * var(--wb-vw-scale)) !important;
  transition: transform var(--wb-dur-slow) var(--wb-ease);
  pointer-events: none;
}
.kpi:hover .kpi-mark {
  transform: scale(1.08) rotate(-4deg);
}
.kpi-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-2);
  position: relative;
  z-index: 1;
}
.kpi-label {
  font-size: var(--wb-fs-xs);
  font-weight: 700;
  color: var(--wb-muted);
}
.kpi-tile {
  display: grid;
  place-items: center;
  width: calc(26px * var(--wb-vw-scale));
  height: calc(26px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-sm);
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
}
.kpi-value {
  font-family: var(--font-display);
  font-size: var(--wb-fs-2xl);
  font-weight: 700;
  letter-spacing: -0.02em;
  line-height: 1.1;
  color: var(--wb-ink);
  font-variant-numeric: tabular-nums;
  position: relative;
  z-index: 1;
}
.kpi-value.is-hit {
  color: var(--wb-success);
}
.kpi-note {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  position: relative;
  z-index: 1;
}
</style>
