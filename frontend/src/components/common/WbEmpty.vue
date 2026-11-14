// 空态基元：图标瓦片 + 标题 + 提示 + 动作。五处空态共用一份结构，
// 间距、字号、瓦片尺寸只对齐一次；只做结构，图标文案动作由调用方给。
<script setup lang="ts">
import { computed, useSlots } from 'vue'
import AppIcon from '@/components/common/AppIcon.vue'
import type { IconName } from '@/components/common/AppIcon.vue'
import type { TileSize, TileTone } from '@/types/ui'

const props = defineProps<{
  /** 主图标：空态的视觉锚 */
  icon?: IconName
  /** 标题（一句话说清「这里现在没有东西」） */
  title: string
  /** 提示（说清下一步做什么） */
  hint?: string
  /** 瓦片尺寸 */
  tile?: TileSize
  /** 图标瓦片的色调 */
  tone?: TileTone
}>()

const slots = useSlots()

/** 图标缺省用盒子：调用方不给时也有一个可识别的形状。 */
const iconName = computed<IconName>(() => props.icon ?? 'box')
/** 瓦片尺寸缺省 lg：空态是全页面的视觉锚，小了撑不起那一屏空白。 */
const tileSize = computed<TileSize>(() => props.tile ?? 'lg')
/** 色调缺省中性：空态不是错误，用品牌色会像在推销。 */
const tileTone = computed<TileTone>(() => props.tone ?? 'neutral')

/** 有动作插槽才渲染那一行：空着的一行比没有更糟。 */
const hasAction = computed(() => Boolean(slots.action))
</script>

<template>
  <div class="wb-empty">
    <span class="wb-tile" :class="[`wb-tile--${tileSize}`, `wb-tile--${tileTone}`]">
      <AppIcon :name="iconName" :size="tileSize === 'lg' ? 22 : tileSize === 'md' ? 18 : 16" />
    </span>
    <p class="wb-empty-title">{{ title }}</p>
    <p v-if="hint" class="wb-empty-hint">{{ hint }}</p>
    <div v-if="hasAction" class="wb-empty-action">
      <slot name="action" />
    </div>
  </div>
</template>

<style scoped>
/* 结构与间距沿用全局 .wb-empty（content-surfaces.css）：那里是唯一定义处。
   这里只补动作行的上边距——它只在有动作时才出现。 */
.wb-empty-action {
  margin-top: var(--wb-sp-2);
}
</style>
