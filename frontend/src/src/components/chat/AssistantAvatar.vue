<script setup lang="ts">
import { Bot, User } from '@/components/common/icons'

/**
 * 聊天头像（主/子 Agent 身份感）：assistant 用品牌图标，user 用中性人形。
 *
 * <p>占位组件：可后续接入用户头像或子 Agent 自定义形象；当前回退到品牌图标，绝不裂图。
 */
withDefaults(
  defineProps<{
    role?: 'assistant' | 'user'
    /** 流式输出中：头像外圈呼吸环，强化「它正在说话」的感知。 */
    speaking?: boolean
  }>(),
  { role: 'assistant', speaking: false }
)
</script>

<template>
  <div
    class="wb-avatar"
    :class="[role === 'user' ? 'wb-avatar-user' : 'wb-avatar-assistant', { 'wb-avatar-speaking': speaking }]"
  >
    <Bot v-if="role === 'assistant'" class="h-4 w-4" />
    <User v-else class="h-3.5 w-3.5" />
  </div>
</template>

<style scoped>
.wb-avatar {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: var(--wb-radius-sm, 6px);
  overflow: visible;
  flex-shrink: 0;
}
.wb-avatar-assistant {
  background: color-mix(in srgb, var(--wb-primary) 10%, transparent);
  color: var(--wb-primary);
}
.wb-avatar-user {
  background: color-mix(in srgb, var(--wb-ink) 8%, transparent);
  color: var(--wb-muted);
}
/* 流式说话态：外圈呼吸环（不遮挡头像本体） */
.wb-avatar-speaking::after {
  content: '';
  position: absolute;
  inset: -3px;
  border-radius: 13px;
  border: 2px solid color-mix(in srgb, var(--wb-primary) 45%, transparent);
  animation: wb-avatar-pulse 1.6s ease-out infinite;
  pointer-events: none;
}
@keyframes wb-avatar-pulse {
  0% {
    opacity: 0.9;
    transform: scale(0.92);
  }
  70% {
    opacity: 0;
    transform: scale(1.12);
  }
  100% {
    opacity: 0;
    transform: scale(1.12);
  }
}
@media (prefers-reduced-motion: reduce) {
  .wb-avatar-speaking::after {
    animation: none;
    opacity: 0.5;
    transform: none;
  }
}
</style>