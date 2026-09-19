<script setup lang="ts">
import { computed } from 'vue'

/**
 * 加载动画组件：多种 CSS 动画替代 loading 文字。
 * 用法：`<LoadingAnimation variant="pet" size="md" />`
 */

export type LoadingVariant = 'dots' | 'pet' | 'flower' | 'bounce' | 'wave'

const props = withDefaults(
  defineProps<{
    variant?: LoadingVariant
    size?: 'sm' | 'md' | 'lg'
    color?: string
  }>(),
  {
    variant: 'dots',
    size: 'md',
    color: 'var(--wb-primary)'
  }
)

const sizeMap = { sm: 16, md: 24, lg: 36 }
const dotSize = computed(() => sizeMap[props.size])
</script>

<template>
  <!-- 跳动圆点 -->
  <div v-if="variant === 'dots'" class="inline-flex items-center gap-1">
    <span
      v-for="i in 3"
      :key="i"
      class="rounded-full"
      :style="{
        width: dotSize + 'px',
        height: dotSize + 'px',
        backgroundColor: color,
        animation: `wb-bounce-dots 1.4s ease-in-out ${(i - 1) * 0.16}s infinite`
      }"
    />
  </div>

  <!-- 宠物跑步 -->
  <div v-else-if="variant === 'pet'" class="inline-block" :style="{ width: dotSize * 2 + 'px', height: dotSize * 2 + 'px' }">
    <svg :width="dotSize * 2" :height="dotSize * 2" viewBox="0 0 40 40" fill="none">
      <!-- 身体 -->
      <ellipse cx="20" cy="24" rx="10" ry="8" :fill="color" />
      <!-- 头 -->
      <circle cx="20" cy="14" r="7" :fill="color" />
      <!-- 耳朵 -->
      <polygon points="14,8 16,14 11,12" :fill="color" />
      <polygon points="26,8 24,14 29,12" :fill="color" />
      <!-- 眼睛 -->
      <circle cx="17" cy="13" r="1.5" fill="white" />
      <circle cx="23" cy="13" r="1.5" fill="white" />
      <circle cx="17" cy="13" r="0.8" fill="#4a3f45" />
      <circle cx="23" cy="13" r="0.8" fill="#4a3f45" />
      <!-- 鼻子 -->
      <circle cx="20" cy="16" r="1" fill="#ff8fab" />
      <!-- 腿（动画） -->
      <g class="wb-legs">
        <rect x="14" y="30" width="3" height="6" rx="1.5" :fill="color" />
        <rect x="23" y="30" width="3" height="6" rx="1.5" :fill="color" />
      </g>
      <!-- 尾巴 -->
      <path d="M30,22 Q34,18 32,14" :stroke="color" stroke-width="2.5" fill="none" stroke-linecap="round" />
    </svg>
  </div>

  <!-- 旋转花朵 -->
  <div v-else-if="variant === 'flower'" class="inline-block" :style="{ width: dotSize * 2 + 'px', height: dotSize * 2 + 'px' }">
    <svg :width="dotSize * 2" :height="dotSize * 2" viewBox="0 0 40 40" fill="none" class="wb-spin" style="transform-origin: center">
      <circle cx="20" cy="10" r="5" :fill="color" />
      <circle cx="20" cy="30" r="5" :fill="color" />
      <circle cx="10" cy="20" r="5" :fill="color" />
      <circle cx="30" cy="20" r="5" :fill="color" />
      <circle cx="13" cy="13" r="5" :fill="color" opacity="0.7" />
      <circle cx="27" cy="13" r="5" :fill="color" opacity="0.7" />
      <circle cx="13" cy="27" r="5" :fill="color" opacity="0.7" />
      <circle cx="27" cy="27" r="5" :fill="color" opacity="0.7" />
      <circle cx="20" cy="20" r="6" fill="white" />
    </svg>
  </div>

  <!-- 弹跳 -->
  <div v-else-if="variant === 'bounce'" class="inline-block" :style="{ width: dotSize + 'px', height: dotSize + 'px' }">
    <div
      class="h-full w-full rounded-full"
      :style="{
        backgroundColor: color,
        animation: 'wb-bounce-ball 0.6s ease-in-out infinite'
      }"
    />
  </div>

  <!-- 波浪 -->
  <div v-else-if="variant === 'wave'" class="inline-flex items-center gap-0.5">
    <span
      v-for="i in 5"
      :key="i"
      class="rounded-full"
      :style="{
        width: (dotSize / 3) + 'px',
        height: dotSize + 'px',
        backgroundColor: color,
        animation: `wb-wave 1.2s ease-in-out ${(i - 1) * 0.1}s infinite`
      }"
    />
  </div>
</template>

<style scoped>
.wb-spin {
  animation: spin 2s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.wb-bounce-dots {
  animation: bounce-dots 1.4s ease-in-out infinite;
}

@keyframes bounce-dots {
  0%, 80%, 100% { transform: translateY(0); }
  40% { transform: translateY(-8px); }
}

.wb-bounce-ball {
  animation: bounce-ball 0.6s ease-in-out infinite;
}

@keyframes bounce-ball {
  0%, 100% { transform: translateY(0) scaleY(1); }
  50% { transform: translateY(-12px) scaleY(0.9); }
}

.wb-legs {
  animation: run 0.3s ease-in-out infinite alternate;
  transform-origin: 20px 24px;
}

@keyframes run {
  from { transform: rotate(-10deg); }
  to { transform: rotate(10deg); }
}

.wb-wave {
  animation: wave 1.2s ease-in-out infinite;
}

@keyframes wave {
  0%, 100% { transform: scaleY(0.5); }
  50% { transform: scaleY(1); }
}
</style>
