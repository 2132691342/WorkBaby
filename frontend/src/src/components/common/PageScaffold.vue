<script setup lang="ts">
/**
 * 页面骨架：统一 hero header（图标 + 标题 + 副标题 + 操作插槽）与空状态。
 */
defineProps<{
  /** 顶部 logo 字符（emoji / SVG 字符）。 */
  icon: string
  /** 页面标题。 */
  title: string
  /** 副标题（一行说明）。 */
  subtitle?: string
  /** 空状态文案。 */
  emptyText?: string
  /** 状态徽章文本（如 "EMPTY" / "READY" / "5 个"）。 */
  badge?: string
}>()
</script>

<template>
  <div class="flex h-full overflow-y-auto text-wb-ink">
    <div class="mx-auto w-full max-w-5xl space-y-6 px-6 py-8">
      <!-- Hero header -->
      <header class="flex items-center justify-between gap-3">
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-wb-primary/15 text-lg">
            {{ icon }}
          </div>
          <div>
            <h1 class="text-base font-semibold text-wb-ink">{{ title }}</h1>
            <p v-if="subtitle" class="text-xs text-wb-muted">{{ subtitle }}</p>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <span
            v-if="badge"
            class="rounded-md border border-wb-border bg-wb-surface/60 px-2 py-1 text-xs text-wb-muted"
          >
            {{ badge }}
          </span>
          <slot name="actions" />
        </div>
      </header>

      <!-- 主体内容（业务组件直接放在 default slot） -->
      <slot />

      <!-- 空状态（无内容时显示） -->
      <div
        v-if="emptyText !== undefined && ($slots.default === undefined)"
        class="rounded-2xl border border-dashed border-wb-border bg-wb-surface/60 p-12 text-center"
      >
        <div class="text-3xl">{{ icon }}</div>
        <p class="mt-3 text-sm text-wb-muted">{{ emptyText }}</p>
        <p class="mt-1 text-xs text-wb-muted">功能正在开发中，敬请期待。</p>
      </div>
    </div>
  </div>
</template>
