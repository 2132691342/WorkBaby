// 分段控件基元：一组互斥选项的横条，收口五处手写的 .seg 结构——五态与刻度
// 共用一份，不会改了四处漏一处。只管选中态、键盘可达与禁用态，选项由调用方给。
<script setup lang="ts">
import { computed, ref } from 'vue'
import AppIcon from '@/components/common/AppIcon.vue'
import type { IconName } from '@/components/common/AppIcon.vue'
import type { SegItem } from '@/types/ui'

const props = defineProps<{
  /** 受选的项；支持 v-model */
  modelValue: string
  /** 分段清单 */
  items: SegItem[]
  /** 尺寸：sm 用于页头工具条，md 用于主操作区 */
  size?: 'sm' | 'md'
  /** 整组禁用（数据还没到时） */
  disabled?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  change: [value: string]
}>()

/** 默认值在 computed 里兜底：defineProps 的类型字面量不配 withDefaults（项目惯例）。 */
const size = computed(() => props.size ?? 'sm')
const disabled = computed(() => props.disabled ?? false)

// 图标的类型收窄：SegItem.icon 声明成 string 是因为调用方清单常由 computed 生成，
// 写联合类型会让每处显式标注；在唯一的使用点收窄一次。
function iconOf(name: string | undefined): IconName {
  return (name ?? 'box') as IconName
}

/** 可见的分段：hidden 的项不进 DOM，而不是渲染成禁用。 */
const shown = computed(() => props.items.filter((item) => !item.hidden))

/** 键盘导航：方向键与 Home/End 在组内移动焦点。 */
const group = ref<HTMLElement | null>(null)

function onKeydown(event: KeyboardEvent, index: number): void {
  const total = shown.value.length
  if (!total) return
  let next = -1
  if (event.key === 'ArrowRight' || event.key === 'ArrowDown') next = (index + 1) % total
  else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') next = (index - 1 + total) % total
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = total - 1
  if (next < 0) return
  event.preventDefault()
  const buttons = group.value?.querySelectorAll<HTMLButtonElement>('.seg-btn')
  buttons?.[next]?.focus()
}

function pick(item: SegItem): void {
  if (disabled.value || item.value === props.modelValue) return
  emit('update:modelValue', item.value)
  emit('change', item.value)
}
</script>

<template>
  <div ref="group" class="seg" :class="`seg--${size}`" role="tablist" :aria-disabled="disabled">
    <button
      v-for="(item, index) in shown"
      :key="item.value"
      type="button"
      role="tab"
      class="seg-btn"
      :class="{ active: item.value === modelValue }"
      :aria-selected="item.value === modelValue"
      :disabled="disabled"
      :tabindex="item.value === modelValue ? 0 : -1"
      @click="pick(item)"
      @keydown="onKeydown($event, index)"
    >
      <AppIcon v-if="item.icon" :name="iconOf(item.icon)" :size="size === 'sm' ? 13 : 14" />
      <span v-if="item.label">{{ item.label }}</span>
    </button>
  </div>
</template>

<style scoped>
/* 五态与刻度沿用全局 .seg / .seg-btn（base-press.css）：
   那里是唯一定义处，这里只补尺寸档。 */
.seg--md .seg-btn {
  height: var(--wb-ctl);
  padding: 0 var(--wb-sp-4);
  font-size: var(--wb-fs-sm);
}
</style>
