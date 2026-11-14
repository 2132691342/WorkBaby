// 卡片基元：抬升的面 + 可选标题行与标题行动作，收口到处都是的「一张卡 + 一行小标题」
// 结构——标题字号与间距只对齐一次，不会这处大一号那处小一号。卡内布局走默认插槽。
<script setup lang="ts">
import { computed, useSlots } from 'vue'
import AppIcon from '@/components/common/AppIcon.vue'
import type { IconName } from '@/components/common/AppIcon.vue'
import type { CardPad } from '@/types/ui'

/** 卡头是否带下边框。 */
const props = defineProps<{
  /** 标题文字 */
  title?: string
  /** 标题左侧的小图标 */
  icon?: IconName
  /** 标题右侧的辅助文字（计数、路径等读数） */
  meta?: string
  /** 卡头是否带下边框：小节标题带，卡头不带 */
  rule?: boolean
  /** 内边距档 */
  pad?: CardPad
  /** 悬停时轻微抬升（可点的卡用） */
  lift?: boolean
}>()

const slots = useSlots()

/** 默认值在 computed 里兜底：defineProps 的类型字面量不配 withDefaults（项目惯例）。
 *  computed 名一律不与 props 同名：同名会让编译器生成两份冲突的 setup 返回。 */
const padSize = computed<CardPad>(() => props.pad ?? 'md')
const withRule = computed(() => props.rule ?? true)
const liftable = computed(() => props.lift ?? false)

const hasTitle = computed(() => Boolean(props.title) || Boolean(slots.title))
const hasActions = computed(() => Boolean(slots.actions))
</script>

<template>
  <section class="wb-card" :class="[`wb-card--pad-${padSize}`, { 'wb-card--lift': liftable }]">
    <header v-if="hasTitle || hasActions" class="card-head" :class="{ 'is-ruled': withRule }">
      <div v-if="hasTitle" class="card-title">
        <AppIcon v-if="icon" :name="icon" :size="14" class="card-title-ic" />
        <span class="wb-sec-title">{{ title }}</span>
        <slot name="title" />
        <span v-if="meta" class="card-meta">{{ meta }}</span>
      </div>
      <div v-if="hasActions" class="card-actions">
        <slot name="actions" />
      </div>
    </header>
    <div class="card-body">
      <slot />
    </div>
  </section>
</template>

<style scoped>
/* 壳与阴影沿用全局 .wb-card（content-surfaces.css）：唯一定义处。
   这里只补标题行与内边距档。 */
.wb-card--pad-sm {
  padding: var(--wb-sp-3) var(--wb-sp-4);
}
.wb-card--pad-md {
  padding: var(--wb-sp-4) var(--wb-sp-5);
}
.wb-card--pad-none {
  padding: 0;
}

.card-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin-bottom: var(--wb-sp-3);
}

.card-head.is-ruled {
  padding-bottom: var(--wb-sp-2);
  border-bottom: 1px solid var(--wb-border);
}

.card-title {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
}

.card-title-ic {
  color: var(--wb-faint);
  flex: none;
}

.card-meta {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-actions {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin-left: auto;
  flex: none;
}

/* 可点的卡：悬停时轻微抬升。整卡不动缩放——一屏卡片一起放大会互相碰撞 */
.wb-card--lift {
  transition:
    box-shadow var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur) var(--wb-ease);
}
.wb-card--lift:hover {
  border-color: var(--wb-border-strong);
  box-shadow: var(--wb-card-shadow-hover);
  transform: translateY(-1px);
}
.wb-card--lift:active {
  transform: scale(0.99);
  transition-duration: 60ms;
}

/* 无内边距时卡头自己补边距：内容区（表格 / 列表）要顶到卡边 */
.wb-card--pad-none .card-head {
  margin: 0;
  padding: var(--wb-sp-3) var(--wb-sp-4);
}
.wb-card--pad-none .card-body {
  padding: 0 var(--wb-sp-4) var(--wb-sp-4);
}
</style>
