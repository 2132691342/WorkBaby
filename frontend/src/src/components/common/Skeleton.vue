<script setup lang="ts">
import { computed } from 'vue'

/**
 * 骨架屏：text 变体按行数渲染文本骨架，其余变体渲染单个指定尺寸的块。
 *
 * <p>刻意不引 ElementPlus `el-skeleton`——本组件被多个懒加载路由共享，
 * 引入后会被 Rollup 提成公共 chunk 并拖入整套 EP 依赖（实测多出 ~1.1MB）。
 */
const props = withDefaults(
  defineProps<{
    variant?: 'text' | 'circular' | 'rectangular' | 'card'
    width?: string
    height?: string
    lines?: number
    animated?: boolean
  }>(),
  { variant: 'text', width: undefined, height: undefined, lines: 1, animated: true }
)

const variantClass = computed(() => {
  switch (props.variant) {
    case 'circular':
      return 'rounded-full'
    case 'rectangular':
      return 'rounded-lg'
    case 'card':
      return 'rounded-2xl'
    default:
      return 'rounded-md'
  }
})

const computedWidth = computed(() => props.width ?? '100%')
const computedHeight = computed(() => {
  if (props.height) return props.height
  switch (props.variant) {
    case 'circular':
      return '2.5rem'
    case 'card':
      return '8rem'
    case 'rectangular':
      return '5rem'
    default:
      return '0.875rem'
  }
})
</script>

<template>
  <div v-if="variant === 'text'" class="space-y-2">
    <div
      v-for="i in lines"
      :key="i"
      class="bg-wb-border/40"
      :class="[variantClass, animated ? 'animate-pulse' : '']"
      :style="{ width: i === lines && lines > 1 ? '75%' : computedWidth, height: computedHeight }"
    />
  </div>

  <div
    v-else
    class="bg-wb-border/40"
    :class="[variantClass, animated ? 'animate-pulse' : '']"
    :style="{ width: computedWidth, height: computedHeight }"
  />
</template>
