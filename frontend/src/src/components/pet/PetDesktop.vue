<script setup lang="ts">
/**
 * 桌宠形态窗口：渲染 sprite（失败回退内置 SVG）、轮询状态驱动动画、拖拽移动窗口、
 * 双击收起，以及复用 chat store 的迷你聊天面板；开启点击穿透时上报命中区域。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import { getServerPort } from '@/api/http'
import { invokeShell } from '@/api/shellBridge'
import { useChatStore } from '@/stores/chat'
import { MessageSquare, Send, X } from '@/components/common/icons'
import AssistantAvatar from '@/components/chat/AssistantAvatar.vue'

interface PetConfigResp {
  sprite_id: string | null
  bubble_enabled: boolean | null
  scale: number | null
  click_through: boolean | null
}

const chat = useChatStore()

const state = ref('IDLE')
const spriteID = ref<string | null>(null)
const bubbleEnabled = ref(true)
const scale = ref(1)
const online = ref(false)
const spriteFailed = ref(false)
/** 透明区域点击穿透（Windows 原生 SetWindowRgn；由后端按上报矩形裁剪窗口区域）。 */
const clickThrough = ref(false)

// sprite 走 gin 同源全 URL：dev（vite）与生产（wails 嵌入）都能加载；相对路径在 dev 下会 404
const spriteUrl = computed(() => {
  if (!spriteID.value) return ''
  const port = getServerPort()
  const base = port ? `http://127.0.0.1:${port}` : ''
  return `${base}/files/sprites/${spriteID.value}`
})

const MOODS: Record<string, { label: string; text: string }> = {
  IDLE: { label: '发呆中', text: '我在陪你呀 ~' },
  WALKING: { label: '散步中', text: '溜达溜达…' },
  CLICKED: { label: '好开心', text: '嘿嘿，戳到我啦！' },
  THINKING: { label: '思考中', text: '让我想想…' },
  SPEAKING: { label: '说话中', text: '你听我说 ~' }
}
const mood = computed(() => (MOODS[state.value] ? state.value : 'IDLE'))
const stateLabel = computed(() => MOODS[mood.value]?.label ?? '发呆中')
const bubbleText = computed(() => {
  // 流式输出中：气泡直接「说」出正在回复的内容片段，桌宠与聊天真正联动
  if (chatOpen.value && chat.streaming && chat.streamingContent) {
    const tail = chat.streamingContent.trim()
    return tail.length > 24 ? '…' + tail.slice(-24) : tail
  }
  return MOODS[mood.value]?.text ?? '我在陪你呀 ~'
})

// ===== 迷你聊天 =====
const PET_SESSION_NAME = '桌宠对话'
const chatOpen = ref(false)
const draft = ref('')
const chatScrollEl = ref<HTMLElement | null>(null)
let prevSessionID: string | null = null
let petSessionID: string | null = null

/** 进入桌宠态时切到专属会话；退出时还原主窗之前选中的会话。 */
async function setupPetSession(): Promise<void> {
  prevSessionID = chat.currentID
  try {
    await chat.loadSessions()
    let pet = chat.sessions.find((s) => s.name === PET_SESSION_NAME)
    if (!pet) {
      pet = await chat.createSession()
      await chat.renameSession(pet.id, PET_SESSION_NAME)
      pet.name = PET_SESSION_NAME
    }
    petSessionID = pet.id
    if (chat.currentID !== pet.id) await chat.selectSession(pet.id)
  } catch {
    /* 会话不可用时桌宠仍可展示，仅聊天不可用 */
  }
}

function toggleChat(): void {
  chatOpen.value = !chatOpen.value
  if (chatOpen.value) void nextTick(scrollChatBottom)
}

async function sendPet(): Promise<void> {
  const text = draft.value.trim()
  if (!text || chat.streaming) return
  draft.value = ''
  try {
    await chat.sendMessage(text)
  } catch {
    /* 错误经 store.error 展示在面板 */
  }
  scrollChatBottom()
}

function scrollChatBottom(): void {
  const el = chatScrollEl.value
  if (el) el.scrollTop = el.scrollHeight
}

watch(
  () => [chat.messages.length, chat.streamingContent.length],
  () => {
    if (chatOpen.value) void nextTick(scrollChatBottom)
  }
)

/** 最近 6 条历史消息（迷你面板只露尾部，完整历史回主窗看）。 */
const recentMessages = computed(() => chat.messages.slice(-6))

// ===== 轮询状态 =====
let pollTimer: number | undefined

async function refreshState(): Promise<void> {
  try {
    // 后端返回裸状态字符串（如 "idle"/"thinking"）；统一转大写匹配本地 mood 表。
    const s = await apiGet<string>('/api/v1/pet/state')
    state.value = (s || 'idle').toUpperCase()
    online.value = true
  } catch {
    online.value = false
  }
}

async function loadConfig(): Promise<void> {
  try {
    const r = await apiGet<PetConfigResp>('/api/v1/pet/config')
    spriteID.value = r.sprite_id || null
    spriteFailed.value = false
    bubbleEnabled.value = r.bubble_enabled !== false
    if (r.scale && r.scale > 0.3 && r.scale <= 2) scale.value = r.scale
    clickThrough.value = r.click_through === true
  } catch {
    /* 未登录/无配置时用内置形象兜底 */
  }
}

// ===== 点击穿透：可见元素包围盒 → 命中区域 =====
interface HitRect {
  x: number
  y: number
  w: number
  h: number
}

/** 命中区域外扩像素：卡片有漂浮动画（±6px），不留裕量会让边缘点击被裁掉。 */
const HIT_PADDING_PX = 8

/** 采集可见交互元素的包围盒（物理像素；SetWindowRgn 用的是窗口物理坐标）。 */
function collectHitRects(): HitRect[] {
  const dpr = window.devicePixelRatio || 1
  const els = Array.from(document.querySelectorAll<HTMLElement>('.pet-card, .pet-bubble, .pet-chat'))
  const rects: HitRect[] = []
  for (const el of els) {
    const r = el.getBoundingClientRect()
    if (r.width <= 0 || r.height <= 0) continue
    rects.push({
      x: Math.max(0, Math.round(r.left * dpr) - HIT_PADDING_PX),
      y: Math.max(0, Math.round(r.top * dpr) - HIT_PADDING_PX),
      w: Math.round(r.width * dpr) + HIT_PADDING_PX * 2,
      h: Math.round(r.height * dpr) + HIT_PADDING_PX * 2
    })
  }
  return rects
}

let maskTimer: number | undefined

/** 上报命中区域；空列表时跳过（后端对空列表会整窗放开，避免瞬时状态误伤交互）。 */
function pushHitMask(): void {
  if (!clickThrough.value) return
  const rects = collectHitRects()
  if (rects.length === 0) return
  void apiPost('/api/v1/pet/window/click-through', { enabled: true, rects }).catch(() => {
    /* 原生层未就绪时静默降级为整窗可交互 */
  })
}

/** 去抖重算：聊天气泡/面板的出现消失有过渡动画，立即取 rect 会拿到中间态。 */
function scheduleHitMask(): void {
  if (!clickThrough.value) return
  if (maskTimer) window.clearTimeout(maskTimer)
  maskTimer = window.setTimeout(pushHitMask, 120)
}

/** 关闭穿透（恢复整窗可交互）。 */
function clearHitMask(): void {
  void apiPost('/api/v1/pet/window/click-through', { enabled: false, rects: [] }).catch(() => {})
}

// ===== 拖拽移动窗口（rAF 节流 IPC） =====
let dragging = false
let lastX = 0
let lastY = 0
let pendingDx = 0
let pendingDy = 0
let rafID = 0

function onDragStart(e: PointerEvent): void {
  dragging = true
  lastX = e.clientX
  lastY = e.clientY
  window.addEventListener('pointermove', onDragMove)
  window.addEventListener('pointerup', onDragEnd)
}

function onDragMove(e: PointerEvent): void {
  if (!dragging) return
  pendingDx += e.clientX - lastX
  pendingDy += e.clientY - lastY
  lastX = e.clientX
  lastY = e.clientY
  if (!rafID) rafID = requestAnimationFrame(flushMove)
}

function flushMove(): void {
  rafID = 0
  if (pendingDx === 0 && pendingDy === 0) return
  const dx = pendingDx
  const dy = pendingDy
  pendingDx = 0
  pendingDy = 0
  void invokeShell('pet.move', { dx, dy })
}

function onDragEnd(): void {
  dragging = false
  flushMove()
  window.removeEventListener('pointermove', onDragMove)
  window.removeEventListener('pointerup', onDragEnd)
}

/** 双击收起桌宠窗口（切回主形态；Go 侧会 emit pet:hide，App 层负责切路由）。 */
function hide(): void {
  void invokeShell('pet.toggle')
}

onMounted(() => {
  void loadConfig().then(() => scheduleHitMask())
  void refreshState()
  void setupPetSession()
  pollTimer = window.setInterval(() => void refreshState(), 2500)
  window.addEventListener('resize', scheduleHitMask)
})

onBeforeUnmount(() => {
  if (pollTimer) window.clearInterval(pollTimer)
  if (maskTimer) window.clearTimeout(maskTimer)
  if (rafID) cancelAnimationFrame(rafID)
  window.removeEventListener('resize', scheduleHitMask)
  clearHitMask()
  // 还原主窗会话选择，避免桌宠专属会话「劫持」主聊天上下文
  if (prevSessionID && prevSessionID !== petSessionID) {
    void chat.selectSession(prevSessionID)
  }
})

// 穿透开关 / 可见元素变化（聊天气泡、面板、缩放、心情）→ 重算命中区域
watch(clickThrough, (on) => {
  if (on) scheduleHitMask()
  else clearHitMask()
})
watch([chatOpen, scale, bubbleEnabled, mood], () => scheduleHitMask())
</script>

<template>
  <div class="pet-root" @pointerdown="onDragStart" @dblclick="hide">
    <!-- 迷你聊天面板（聊天面板内禁止拖拽/双击收起） -->
    <Transition name="pet-chat-pop">
      <div v-if="chatOpen" class="pet-chat" @pointerdown.stop @dblclick.stop>
        <div class="pet-chat-head">
          <AssistantAvatar class="pet-chat-avatar" :speaking="chat.streaming" />
          <span class="pet-chat-title">WorkBaby</span>
          <button type="button" class="pet-chat-close" title="收起" @click="chatOpen = false">
            <X class="h-3 w-3" />
          </button>
        </div>

        <div ref="chatScrollEl" class="pet-chat-body">
          <p v-if="recentMessages.length === 0 && !chat.streaming" class="pet-chat-empty">
            和我说点什么吧 ~
          </p>
          <div
            v-for="m in recentMessages"
            :key="m.id"
            class="pet-chat-msg"
            :class="m.role === 'user' ? 'pet-chat-msg-user' : 'pet-chat-msg-ai'"
          >
            {{ m.content }}
          </div>
          <div v-if="chat.streaming" class="pet-chat-msg pet-chat-msg-ai pet-chat-streaming">
            {{ chat.streamingContent ? chat.streamingContent.slice(-140) : '…' }}
          </div>
          <p v-if="chat.error" class="pet-chat-err">{{ chat.error }}</p>
        </div>

        <div class="pet-chat-input">
          <input
            v-model="draft"
            type="text"
            placeholder="说点什么…"
            @keydown.enter.prevent="sendPet"
          />
          <button
            type="button"
            class="pet-chat-send"
            :disabled="!draft.trim() || chat.streaming"
            @click="sendPet"
          >
            <Send class="h-3 w-3" />
          </button>
        </div>
      </div>
    </Transition>

    <div class="pet-card" :data-mood="mood" :class="{ 'pet-card-mini': chatOpen }">
      <!-- 说话气泡 -->
      <Transition name="pet-pop">
        <div v-if="bubbleEnabled && online" class="pet-bubble">{{ bubbleText }}</div>
      </Transition>

      <!-- 用户自定义形象（加载失败回退内置 SVG，绝不裂图） -->
      <img
        v-if="spriteUrl && !spriteFailed"
        :src="spriteUrl"
        class="pet-sprite"
        :style="{ transform: `scale(${scale})` }"
        draggable="false"
        alt="pet"
        @error="spriteFailed = true"
      />

      <!-- 内置 SVG 形象（无自定义 sprite 时） -->
      <svg
        v-else
        class="pet-svg"
        :style="{ transform: `scale(${scale})` }"
        viewBox="0 0 120 120"
        fill="none"
      >
        <defs>
          <radialGradient id="petBody" cx="35%" cy="30%" r="80%">
            <stop offset="0%" stop-color="var(--pet-c1)" />
            <stop offset="100%" stop-color="var(--pet-c2)" />
          </radialGradient>
        </defs>
        <!-- 影子 -->
        <ellipse class="pet-shadow" cx="60" cy="108" rx="30" ry="6" fill="rgba(0,0,0,.18)" />
        <!-- 触角 -->
        <path d="M60 26 C 58 16, 66 12, 64 4" stroke="var(--pet-c2)" stroke-width="3" stroke-linecap="round" />
        <circle cx="64" cy="4" r="4" fill="var(--pet-c3)" />
        <!-- 身体 -->
        <ellipse class="pet-body" cx="60" cy="66" rx="38" ry="36" fill="url(#petBody)" />
        <!-- 高光 -->
        <ellipse cx="46" cy="50" rx="12" ry="8" fill="rgba(255,255,255,.35)" />
        <!-- 眼睛 -->
        <g class="pet-eyes">
          <g class="pet-eye">
            <ellipse cx="46" cy="62" rx="7" ry="8.5" fill="#2b2b3a" />
            <circle cx="48.5" cy="59" r="2.4" fill="#fff" />
          </g>
          <g class="pet-eye">
            <ellipse cx="74" cy="62" rx="7" ry="8.5" fill="#2b2b3a" />
            <circle cx="76.5" cy="59" r="2.4" fill="#fff" />
          </g>
          <g class="pet-eyes-happy" opacity="0">
            <path d="M39 62 Q46 55 53 62" stroke="#2b2b3a" stroke-width="3.4" stroke-linecap="round" fill="none" />
            <path d="M67 62 Q74 55 81 62" stroke="#2b2b3a" stroke-width="3.4" stroke-linecap="round" fill="none" />
          </g>
        </g>
        <!-- 腮红 -->
        <ellipse cx="36" cy="74" rx="6" ry="3.6" fill="rgba(255,120,150,.45)" />
        <ellipse cx="84" cy="74" rx="6" ry="3.6" fill="rgba(255,120,150,.45)" />
        <!-- 嘴 -->
        <path class="pet-mouth" d="M53 80 Q60 86 67 80" stroke="#2b2b3a" stroke-width="3" stroke-linecap="round" fill="none" />
        <ellipse class="pet-mouth-open" cx="60" cy="82" rx="7" ry="5.5" fill="#2b2b3a" opacity="0" />
        <!-- 思考泡泡 -->
        <g class="pet-think" opacity="0">
          <circle cx="98" cy="34" r="4" fill="#fff" opacity=".9" />
          <circle cx="106" cy="24" r="6" fill="#fff" opacity=".9" />
          <circle cx="114" cy="12" r="8" fill="#fff" opacity=".9" />
        </g>
      </svg>

      <!-- 状态徽章 + 聊天入口 -->
      <div class="pet-chip">
        <span class="pet-dot" :class="{ offline: !online }" />
        {{ online ? stateLabel : '连接中…' }}
        <button
          type="button"
          class="pet-chat-toggle"
          :class="{ active: chatOpen }"
          title="聊天"
          @pointerdown.stop
          @click.stop="toggleChat"
        >
          <MessageSquare class="h-3 w-3" />
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pet-root {
  width: 100vw;
  height: 100vh;
  overflow: hidden;
  background: transparent;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding: 10px;
  cursor: grab;
  user-select: none;
}
.pet-root:active {
  cursor: grabbing;
}

/* ===== 迷你聊天面板 ===== */
.pet-chat {
  width: 100%;
  max-width: 240px;
  display: flex;
  flex-direction: column;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.97);
  border: 1px solid rgba(129, 140, 248, 0.25);
  box-shadow: 0 12px 32px rgba(30, 34, 90, 0.2);
  overflow: hidden;
  cursor: default;
}
.pet-chat-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 10px;
  border-bottom: 1px solid rgba(129, 140, 248, 0.15);
}
.pet-chat-avatar {
  width: 22px;
  height: 22px;
}
.pet-chat-avatar img,
.pet-chat-avatar svg {
  border-radius: 7px;
}
.pet-chat-title {
  font-size: 12px;
  font-weight: 700;
  color: var(--wb-ink);
}
.pet-chat-close {
  margin-left: auto;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 6px;
  color: #8a8fb8;
  transition: all 0.15s;
}
.pet-chat-close:hover {
  background: rgba(129, 140, 248, 0.12);
  color: #4f5388;
}
.pet-chat-body {
  max-height: 150px;
  min-height: 64px;
  overflow-y: auto;
  padding: 8px 10px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.pet-chat-empty {
  margin: auto;
  font-size: 11px;
  color: #a0a4c8;
}
.pet-chat-msg {
  max-width: 85%;
  padding: 5px 8px;
  border-radius: 10px;
  font-size: 11px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}
.pet-chat-msg-ai {
  align-self: flex-start;
  background: var(--wb-primary-soft);
  color: var(--wb-ink);
  border-bottom-left-radius: 3px;
}
.pet-chat-msg-user {
  align-self: flex-end;
  background: var(--wb-primary);
  color: #fff;
  border-bottom-right-radius: 3px;
}
.pet-chat-streaming::after {
  content: '▍';
  animation: pet-cursor 1s steps(2) infinite;
}
@keyframes pet-cursor {
  50% {
    opacity: 0;
  }
}
.pet-chat-err {
  font-size: 10px;
  color: #e05b5b;
}
.pet-chat-input {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 10px;
  border-top: 1px solid rgba(129, 140, 248, 0.15);
}
.pet-chat-input input {
  flex: 1;
  min-width: 0;
  border: 1px solid rgba(129, 140, 248, 0.25);
  border-radius: 8px;
  padding: 5px 8px;
  font-size: 11px;
  color: var(--wb-ink);
  background: #fff;
  outline: none;
}
.pet-chat-input input:focus {
  border-color: var(--wb-primary);
  box-shadow: 0 0 0 2px var(--wb-primary-soft);
}
.pet-chat-send {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border-radius: 8px;
  background: var(--wb-primary);
  color: #fff;
  flex-shrink: 0;
  transition: all 0.15s;
}
.pet-chat-send:disabled {
  opacity: 0.45;
}
.pet-chat-send:not(:disabled):hover {
  background: var(--wb-primary-strong);
}
.pet-chat-pop-enter-active {
  transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
}
.pet-chat-pop-leave-active {
  transition: all 0.12s ease-in;
}
.pet-chat-pop-enter-from,
.pet-chat-pop-leave-to {
  opacity: 0;
  transform: translateY(8px) scale(0.96);
}

/* 玻璃卡片：随心情换色 */
.pet-card {
  position: relative;
  width: 216px;
  padding: 18px 14px 14px;
  border-radius: 28px;
  background: linear-gradient(160deg, rgba(255, 255, 255, 0.94), rgba(248, 250, 255, 0.88));
  border: 1px solid rgba(255, 255, 255, 0.7);
  box-shadow:
    0 18px 40px rgba(30, 34, 90, 0.22),
    0 2px 8px rgba(30, 34, 90, 0.12),
    inset 0 1px 0 rgba(255, 255, 255, 0.8);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  animation: pet-float 3.6s ease-in-out infinite;
  --pet-c1: #a5b4fc;
  --pet-c2: #818cf8;
  --pet-c3: #f472b6;
}
/* 聊天展开时桌宠缩小，给面板让位 */
.pet-card-mini {
  padding: 10px 12px 10px;
  gap: 6px;
}
.pet-card-mini .pet-svg,
.pet-card-mini .pet-sprite {
  width: 84px;
  height: 84px;
}
.pet-card[data-mood='WALKING'] {
  --pet-c1: #7dd3fc;
  --pet-c2: #38bdf8;
  --pet-c3: #fbbf24;
  animation: pet-float 2.2s ease-in-out infinite, pet-sway 1.6s ease-in-out infinite;
}
.pet-card[data-mood='THINKING'] {
  --pet-c1: #fcd34d;
  --pet-c2: #f59e0b;
  --pet-c3: #f472b6;
}
.pet-card[data-mood='SPEAKING'] {
  --pet-c1: #86efac;
  --pet-c2: #34d399;
  --pet-c3: #fbbf24;
}
.pet-card[data-mood='CLICKED'] {
  --pet-c1: #f9a8d4;
  --pet-c2: #f472b6;
  --pet-c3: #c084fc;
}

@keyframes pet-float {
  0%,
  100% {
    transform: translateY(0);
  }
  50% {
    transform: translateY(-6px);
  }
}
@keyframes pet-sway {
  0%,
  100% {
    rotate: -2deg;
  }
  50% {
    rotate: 2deg;
  }
}

/* 气泡 */
.pet-bubble {
  position: absolute;
  top: -14px;
  left: 50%;
  translate: -50% 0;
  max-width: 190px;
  padding: 7px 12px;
  border-radius: 14px 14px 14px 4px;
  background: rgba(255, 255, 255, 0.97);
  border: 1px solid color-mix(in srgb, var(--wb-primary) 25%, transparent);
  box-shadow: 0 6px 18px rgba(23, 66, 133, 0.16);
  font-size: 12px;
  font-weight: 600;
  color: var(--wb-ink);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  z-index: 2;
}
.pet-pop-enter-active {
  transition: all 0.18s ease-out;
}
.pet-pop-leave-active {
  transition: all 0.12s ease-in;
}
.pet-pop-enter-from,
.pet-pop-leave-to {
  opacity: 0;
  translate: -50% 6px;
}

/* 形象 */
.pet-svg,
.pet-sprite {
  width: 132px;
  height: 132px;
  transition: transform 0.2s ease;
}
.pet-sprite {
  object-fit: contain;
  filter: drop-shadow(0 10px 16px rgba(30, 34, 90, 0.25));
}

/* SVG 动画细节 */
.pet-body {
  animation: pet-breathe 2.8s ease-in-out infinite;
  transform-origin: 60px 66px;
}
@keyframes pet-breathe {
  0%,
  100% {
    transform: scale(1);
  }
  50% {
    transform: scale(1.035);
  }
}
.pet-eye {
  animation: pet-blink 4.2s infinite;
  transform-origin: center;
  transform-box: fill-box;
}
.pet-eye:nth-of-type(2) {
  animation-delay: 0.08s;
}
@keyframes pet-blink {
  0%,
  94%,
  100% {
    transform: scaleY(1);
  }
  96%,
  98% {
    transform: scaleY(0.08);
  }
}
.pet-card[data-mood='THINKING'] .pet-eyes {
  translate: 0 -3px;
}
.pet-card[data-mood='THINKING'] .pet-think {
  opacity: 1;
  animation: pet-think-pop 1.8s ease-in-out infinite;
}
@keyframes pet-think-pop {
  0%,
  100% {
    opacity: 0.55;
  }
  50% {
    opacity: 1;
  }
}
.pet-card[data-mood='CLICKED'] .pet-eyes-happy {
  opacity: 1;
}
.pet-card[data-mood='CLICKED'] .pet-eye {
  opacity: 0;
}
.pet-card[data-mood='SPEAKING'] .pet-mouth {
  opacity: 0;
}
.pet-card[data-mood='SPEAKING'] .pet-mouth-open {
  opacity: 1;
  animation: pet-talk 0.5s ease-in-out infinite;
  transform-origin: 60px 82px;
  transform-box: fill-box;
}
@keyframes pet-talk {
  0%,
  100% {
    transform: scaleY(0.6);
  }
  50% {
    transform: scaleY(1);
  }
}

/* 状态徽章 */
.pet-chip {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 12px;
  border-radius: 999px;
  background: rgba(129, 140, 248, 0.12);
  border: 1px solid rgba(129, 140, 248, 0.22);
  font-size: 11px;
  font-weight: 700;
  color: #4f5388;
}
.pet-dot {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  background: #34d399;
  box-shadow: 0 0 0 3px rgba(52, 211, 153, 0.18);
}
.pet-dot.offline {
  background: #fbbf24;
  box-shadow: 0 0 0 3px rgba(251, 191, 36, 0.18);
}
/* 聊天入口按钮 */
.pet-chat-toggle {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  margin-left: 2px;
  border-radius: 999px;
  color: var(--wb-primary);
  background: rgba(255, 255, 255, 0.8);
  border: 1px solid color-mix(in srgb, var(--wb-primary) 30%, transparent);
  transition: all 0.15s;
}
.pet-chat-toggle:hover {
  background: var(--wb-primary);
  color: #fff;
}
.pet-chat-toggle.active {
  background: var(--wb-primary);
  color: #fff;
}
</style>
