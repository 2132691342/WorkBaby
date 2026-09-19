<script setup lang="ts">
/**
 * 全局背景层。
 *
 * <p>默认不渲染任何内容：页面底色与柔光晕由 body 的 {@code --wb-page-gradient} 承担
 * （light 蓝调 / dark 紫调），玻璃卡的透光层次依赖它。
 * 仅当用户上传了自定义背景图时叠加一层模糊图 + 主题色遮罩，保证正文对比度。
 */
defineProps<{
  /** 可选：用户上传背景图 URL（未接时纯色）。 */
  backgroundUrl?: string | null
}>()
</script>

<template>
  <div class="pointer-events-none fixed inset-0 -z-10 overflow-hidden">
    <!-- 用户上传背景（高斯模糊 + 主题色遮罩保对比度；遮罩走 token，跟随亮/暗主题） -->
    <template v-if="backgroundUrl">
      <div
        class="absolute inset-0 bg-cover bg-center"
        :style="{ backgroundImage: `url(${backgroundUrl})`, filter: 'blur(18px)' }"
      />
      <div class="absolute inset-0" :style="{ background: 'var(--wb-bg)', opacity: 0.72 }" />
    </template>
  </div>
</template>
