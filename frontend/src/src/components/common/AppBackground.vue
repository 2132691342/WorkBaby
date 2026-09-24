<script setup lang="ts">
/**
 * 全局背景层：只负责「用户自定义背景图」，其余不画。
 *
 * <p>页面底色是纯色（themes.css 的 --wb-bg）。此前的三色漂浮光球 + 噪点已移除：
 * 背景里同时出现蓝 / 青 / 紫三团光会让整屏发花，也让「卡片浮起来」这件事失去参照。
 * 用户上传背景图时，叠一层主题色遮罩以保正文对比度。
 */
defineProps<{
  /** 可选：用户上传背景图 URL（未接时纯色）。 */
  backgroundUrl?: string | null
}>()
</script>

<template>
  <div v-if="backgroundUrl" class="pointer-events-none fixed inset-0 -z-10 overflow-hidden">
    <div
      class="absolute inset-0 bg-cover bg-center"
      :style="{ backgroundImage: `url(${backgroundUrl})`, filter: 'blur(18px)' }"
    />
    <div class="absolute inset-0" :style="{ background: 'var(--wb-bg)', opacity: 0.72 }" />
  </div>
</template>
