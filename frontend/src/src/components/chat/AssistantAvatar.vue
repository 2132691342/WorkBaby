<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Bot, User } from '@/components/common/icons'
import { apiGet } from '@/api/client'
import { getServerPort } from '@/api/http'

/**
 * 聊天头像（主/子 Agent 身份感）：assistant 用当前桌宠 sprite，user 用中性人形。
 *
 * <p>桌宠与聊天共用同一形象来源（pet_configs.sprite_id → /files/sprites/{id}），
 * 首启由后端播种内置吉祥物，开箱即有头像。sprite 走 gin 同源全 URL，
 * dev（vite）与生产（wails 嵌入）均可加载；加载失败回退品牌图标，绝不裂图。
 */
const props = withDefaults(
  defineProps<{
    role?: 'assistant' | 'user'
    /** 流式输出中：头像外圈呼吸环，强化「它正在说话」的感知。 */
    speaking?: boolean
  }>(),
  { role: 'assistant', speaking: false }
)

const spriteUrl = ref('')
const failed = ref(false)

// 全组件共享一次配置拉取（头像很多，避免每条消息各请求一次）
let configPromise: Promise<string> | null = null
function fetchSpriteUrl(): Promise<string> {
  if (!configPromise) {
    configPromise = apiGet<{ sprite_id: string | null }>('/api/v1/pet/config')
      .then((c) => {
        if (!c.sprite_id) return ''
        const port = getServerPort()
        const base = port ? `http://127.0.0.1:${port}` : ''
        return `${base}/files/sprites/${c.sprite_id}`
      })
      .catch(() => '')
  }
  return configPromise
}

async function loadSprite(): Promise<void> {
  failed.value = false
  spriteUrl.value = await fetchSpriteUrl()
}

onMounted(() => {
  if (props.role !== 'assistant') return
  void loadSprite()
  // 桌宠形象变更（PetSpaceView 保存配置）后广播，全部头像即时跟随
  window.addEventListener('wb-avatar-refresh', () => {
    configPromise = null
    void loadSprite()
  })
})
</script>

<template>
  <div
    class="wb-avatar"
    :class="[role === 'user' ? 'wb-avatar-user' : 'wb-avatar-assistant', { 'wb-avatar-speaking': speaking }]"
  >
    <img
      v-if="role === 'assistant' && spriteUrl && !failed"
      :src="spriteUrl"
      alt="WorkBaby"
      draggable="false"
      @error="failed = true"
    />
    <Bot v-else-if="role === 'assistant'" class="h-4 w-4" />
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
  border-radius: var(--wb-radius-sm, 6px); /* 原型 .av 同款小圆角 */
  overflow: visible;
  flex-shrink: 0;
}
.wb-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  border-radius: var(--wb-radius-sm, 6px);
  background: var(--wb-surface);
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
