<script setup lang="ts">
/**
 * 聊天页的桌宠陪伴体：抠好的形象浮在输入框上方，随会话状态换姿态
 * （idle 浮动 / thinking 加速摆动 / sad 垂头），点击随机台词，可召唤与收起。
 * 形象源与桌宠、聊天头像同源（pet_configs.sprite_id）。
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { apiGet } from '@/api/client'
import { getServerPort } from '@/api/http'
import { t } from '@/i18n'

const props = defineProps<{
  /** 是否正在生成（思考态）。 */
  streaming?: boolean
  /** 上一轮是否失败（垂头态）。 */
  failed?: boolean
}>()

const PREF_KEY = 'wb.petCompanion'
const visiblePref = ref(readPref())
const spriteID = ref<string | null>(null)
const hovered = ref(false)
const bubble = ref('')
let bubbleTimer: ReturnType<typeof setTimeout> | null = null

function readPref(): boolean {
  try {
    return localStorage.getItem(PREF_KEY) !== 'false'
  } catch {
    return true
  }
}

async function loadSprite(): Promise<void> {
  try {
    const c = await apiGet<{ sprite_id: string | null; enabled?: boolean }>('/api/v1/pet/config')
    spriteID.value = c.sprite_id ?? null
  } catch {
    spriteID.value = null
  }
}

const spriteUrl = computed(() => {
  if (!spriteID.value) return ''
  const port = getServerPort()
  const base = port ? `http://127.0.0.1:${port}` : ''
  return `${base}/files/sprites/${spriteID.value}`
})

const mood = computed<'idle' | 'thinking' | 'sad'>(() => {
  if (props.failed) return 'sad'
  return props.streaming ? 'thinking' : 'idle'
})

const LINES = ['pet.bubble.0', 'pet.bubble.1', 'pet.bubble.2', 'pet.bubble.3']

function say(): void {
  const idx = Math.floor(Math.random() * LINES.length)
  bubble.value = t(LINES[idx])
  if (bubbleTimer !== null) clearTimeout(bubbleTimer)
  bubbleTimer = setTimeout(() => {
    bubble.value = ''
  }, 3200)
}

function toggle(): void {
  visiblePref.value = !visiblePref.value
  try {
    localStorage.setItem(PREF_KEY, String(visiblePref.value))
  } catch {
    // 隐私模式下不可写，仅本次会话生效
  }
  if (visiblePref.value && !spriteID.value) void loadSprite()
}

/** 配置页换了形象后刷新（pet store 保存时派发）。 */
function onAvatarRefresh(): void {
  void loadSprite()
}

onMounted(() => {
  void loadSprite()
  window.addEventListener('wb-avatar-refresh', onAvatarRefresh)
})

onBeforeUnmount(() => {
  window.removeEventListener('wb-avatar-refresh', onAvatarRefresh)
  if (bubbleTimer !== null) clearTimeout(bubbleTimer)
})
</script>

<template>
  <!-- 收起态留一个极小入口，避免「想叫回来找不到」 -->
  <button
    v-if="!visiblePref"
    type="button"
    class="mb-1 ml-auto mr-1 flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] text-wb-muted transition-colors hover:bg-wb-surface-hover hover:text-wb-ink"
    :title="t('pet.companionShow')"
    @click="toggle"
  >
    {{ t('pet.companionShow') }}
  </button>

  <div
    v-else
    class="pet-companion"
    :class="[`is-${mood}`]"
    @mouseenter="hovered = true"
    @mouseleave="hovered = false"
  >
    <transition name="pet-bubble">
      <span v-if="bubble" class="pet-companion__bubble">{{ bubble }}</span>
    </transition>

    <button type="button" class="pet-companion__body" :title="t('pet.companionHide')" @click="say">
      <img v-if="spriteUrl" :src="spriteUrl" :alt="t('pet.alt')" />
      <span v-else class="pet-companion__fallback">{{ mood === 'sad' ? '😵' : mood === 'thinking' ? '🤔' : '😺' }}</span>
    </button>

    <button
      v-show="hovered"
      type="button"
      class="pet-companion__hide"
      :title="t('pet.companionHide')"
      @click="toggle"
    >
      ×
    </button>
  </div>
</template>

<style scoped>
.pet-companion {
  position: relative;
  display: flex;
  align-items: flex-end;
  justify-content: flex-end;
  height: 64px;
  padding: 0 18px 2px;
  pointer-events: none;
}

.pet-companion__body {
  pointer-events: auto;
  display: block;
  width: 56px;
  height: 56px;
  border: 0;
  background: transparent;
  cursor: pointer;
  filter: drop-shadow(0 6px 10px rgb(0 0 0 / 22%));
  animation: companion-float 4s ease-in-out infinite;
}

.pet-companion__body img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.pet-companion__fallback {
  display: flex;
  height: 100%;
  align-items: center;
  justify-content: center;
  font-size: 30px;
}

/* 思考态：浮动更快 + 轻微左右摆，传达「在忙」 */
.pet-companion.is-thinking .pet-companion__body {
  animation-duration: 1.6s;
}
/* 失败态：垂头 + 去饱和 */
.pet-companion.is-sad .pet-companion__body {
  animation-duration: 6s;
  filter: drop-shadow(0 4px 8px rgb(0 0 0 / 18%)) grayscale(0.5);
  transform: rotate(-6deg);
}

.pet-companion__bubble {
  pointer-events: none;
  margin-right: 8px;
  max-width: 240px;
  border: 1px solid var(--wb-border);
  border-radius: 14px 14px 4px 14px;
  background: var(--wb-surface);
  padding: 5px 10px;
  font-size: 11.5px;
  line-height: 1.5;
  color: var(--wb-ink);
  box-shadow: var(--wb-shadow);
}

.pet-companion__hide {
  pointer-events: auto;
  position: absolute;
  top: 2px;
  width: 16px;
  height: 16px;
  border: 0;
  border-radius: 9999px;
  background: var(--wb-surface);
  color: var(--wb-muted);
  font-size: 12px;
  line-height: 1;
  cursor: pointer;
  box-shadow: var(--wb-shadow);
}
.pet-companion__hide:hover {
  color: var(--wb-ink);
}

@keyframes companion-float {
  0%,
  100% {
    transform: translateY(0);
  }
  50% {
    transform: translateY(-5px);
  }
}

.pet-bubble-enter-active,
.pet-bubble-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.pet-bubble-enter-from,
.pet-bubble-leave-to {
  opacity: 0;
  transform: translateY(4px);
}

@media (prefers-reduced-motion: reduce) {
  .pet-companion__body {
    animation: none !important;
  }
}
</style>
