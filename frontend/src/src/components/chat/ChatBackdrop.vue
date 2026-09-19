<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { apiGet } from '@/api/client'
import type { PetConfig } from '@/types/api'

/**
 * 聊天区背景：复用桌宠 sprite（同一张形象，两处用途，避免用户各传一张）。
 *
 * <p>仅在桌宠配置开启 chat_background 时渲染。纯展示层：
 * pointer-events: none 保证不拦截消息区任何交互；z-index: 0 让内容层压在上面。
 */
const cfg = ref<PetConfig | null>(null)

onMounted(async () => {
  try {
    cfg.value = await apiGet<PetConfig>('/api/v1/pet/config')
  } catch {
    // 桌宠未配置 / 服务未就绪：静默不渲染背景，不影响聊天主链路
  }
})

const style = computed(() => {
  const c = cfg.value
  if (!c?.chat_background || !c.sprite_id) return null
  const opacity = Math.min(Math.max(c.background_opacity ?? 18, 0), 100) / 100
  return {
    backgroundImage: `url(/files/sprites/${c.sprite_id})`,
    opacity: String(opacity),
    filter: c.background_blur_px > 0 ? `blur(${c.background_blur_px}px)` : ''
  }
})
</script>

<template>
  <div v-if="style" class="chat-backdrop" :style="style" aria-hidden="true" />
</template>

<style scoped>
.chat-backdrop {
  position: absolute;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  background-position: center;
  background-size: cover;
  background-repeat: no-repeat;
}
</style>
