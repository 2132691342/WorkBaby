<script setup lang="ts">
/**
 * 全局背景层。
 *
 * <p>页面底色与柔光晕由 body 的 {@code --wb-page-gradient} 承担；本组件只叠加两层视觉气氛：
 * <ol>
 *   <li>3 团柔和漂浮光球（CSS 动画缓慢漂移），造出"有空气"的纵深感；</li>
 *   <li>极淡噪点（SVG fractalNoise），避免大色块过于"塑料"。</li>
 * </ol>
 * 用户上传背景图时，叠加一层模糊图 + 主题色遮罩以保对比度。
 */
defineProps<{
  /** 可选：用户上传背景图 URL（未接时纯色）。 */
  backgroundUrl?: string | null
}>()
</script>

<template>
  <div class="pointer-events-none fixed inset-0 -z-10 overflow-hidden appbg">
    <!-- 漂浮光球：3 团大尺寸柔和辉光，缓慢漂移 -->
    <div class="orb orb--cyan" />
    <div class="orb orb--blue" />
    <div class="orb orb--violet" />

    <!-- 噪点（极淡，避免画面过于"塑料"） -->
    <div class="noise" />

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

<style scoped>
/* 漂浮光球 —— 大尺寸柔和辉光，缓慢漂移制造纵深感 */
.orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(70px);
  opacity: 0.55;
  will-change: transform;
  animation: wb-orb-float 22s ease-in-out infinite alternate;
  pointer-events: none;
}
.orb--cyan {
  left: -8%;
  top: -10%;
  width: 56vmin;
  height: 56vmin;
  background: radial-gradient(circle at 30% 30%, rgba(34, 183, 245, 0.55), transparent 65%);
}
.orb--blue {
  right: -10%;
  top: 18%;
  width: 64vmin;
  height: 64vmin;
  background: radial-gradient(circle at 70% 40%, rgba(47, 123, 246, 0.55), transparent 65%);
  animation-duration: 28s;
  animation-delay: -6s;
}
.orb--violet {
  left: 28%;
  bottom: -16%;
  width: 72vmin;
  height: 72vmin;
  background: radial-gradient(circle at 50% 50%, rgba(122, 90, 248, 0.45), transparent 65%);
  animation-duration: 36s;
  animation-delay: -12s;
}
@keyframes wb-orb-float {
  0% {
    transform: translate3d(0, 0, 0) scale(1);
  }
  50% {
    transform: translate3d(4vw, -3vh, 0) scale(1.06);
  }
  100% {
    transform: translate3d(-3vw, 4vh, 0) scale(0.97);
  }
}

/* 极淡噪点：避免大面积柔光看起来"塑料" */
.noise {
  position: absolute;
  inset: 0;
  opacity: 0.035;
  pointer-events: none;
  background-image: url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='180' height='180'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='2' stitchTiles='stitch'/></filter><rect width='100%25' height='100%25' filter='url(%23n)' opacity='0.9'/></svg>");
  background-size: 180px 180px;
  mix-blend-mode: overlay;
}
</style>