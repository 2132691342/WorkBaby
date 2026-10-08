<script setup lang="ts">
// 聊天主视图：侧栏 + 消息流 + 输入区；没有会话时给引导页。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as api from '../api'
import { useSse } from '../composables/useSse'
import { intentFile, intentNewSession, SHELL_INTENT_EVENT } from '../composables/shellIntent'
import { useChatStore } from '../stores/chat'
import { useSessionStore } from '../stores/session'
import { useSettingsStore } from '../stores/settings'
import { useToastStore } from '../stores/toast'
import type { AttachmentREQ } from '../types/api'
import AppSidebar from '../components/common/AppSidebar.vue'
import AppIcon from '../components/common/AppIcon.vue'
import ChatInput from '../components/chat/ChatInput.vue'
import ExampleCards from '../components/chat/ExampleCards.vue'
import MessageList from '../components/chat/MessageList.vue'

const session = useSessionStore()
const chat = useChatStore()
const settings = useSettingsStore()
const toast = useToastStore()
const inputRef = ref<InstanceType<typeof ChatInput> | null>(null)
const creating = ref(false)

const empty = computed(() => !session.currentId || (!session.messages.length && !chat.running))

async function newSession() {
  if (creating.value) return
  creating.value = true
  try {
    chat.reset()
    await session.create({})
    inputRef.value?.focus()
  } catch (e) {
    toast.bad(`新建对话失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    creating.value = false
  }
}

const sending = ref(false)
const stopping = ref(false)

async function send(text: string, attachments: AttachmentREQ[]) {
  if (!session.currentId) await newSession()
  if (!session.currentId) return
  const sid = session.currentId
  sending.value = true
  let sent
  try {
    sent = await chat.send(sid, text, attachments)
  } catch (e) {
    // 发送被拒（会话忙 / 没配模型服务）必须立刻说清楚，不能让消息无声消失
    toast.bad(`发送失败：${(e as Error)?.message || '请重试'}`)
    return
  } finally {
    sending.value = false
  }
  // 发送后立刻回显这条消息：等 chat:done 拉快照才显示的话，
  // 用户会以为消息没发出去（助手还没回，观感上就是「什么都没发生」）。
  if (sent?.entry_id) session.echoUserMessage(sent.entry_id, text)
  inputRef.value?.focus()
}

async function steer(text: string) {
  if (!session.currentId) return
  try {
    await chat.steer(session.currentId, text)
    toast.ok('已插话，助手会在下一轮响应')
  } catch (e) {
    toast.bad(`插话失败：${(e as Error)?.message || '请重试'}`)
  }
}

async function stop() {
  if (!session.currentId || stopping.value) return
  stopping.value = true
  try {
    await chat.stop(session.currentId)
  } catch (e) {
    toast.bad(`停止失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    stopping.value = false
  }
}

// 收尾后拉权威快照。拉失败必须说出来：静默失败的表现是用户盯着一条
// 已经结束的旧对话，以为助手没写完，而界面上没有任何地方提示过。
async function reload() {
  try {
    await session.refresh()
  } catch (e) {
    chat.notify(`刷新对话失败：${(e as Error)?.message || '稍后重试'}`)
  }
}

// /clear：把叶子指回第一条消息，历史保留，聊天从头上重来
async function clearSession() {
  if (!session.currentId) return
  const first = session.messages[0]
  if (!first) {
    chat.notify('这个会话还没有历史')
    return
  }
  await api.sessions.branch(session.currentId, first.id)
  await session.refresh()
  chat.notify('已回到本会话开头')
}

function pickExample(text: string) {
  void send(text, [])
}

// SSE 事件 → store；结束后拉一次权威快照，前端不做复杂合并。
useSse(
  () => session.currentId,
  async (env) => {
    switch (env.event) {
      case 'chat:start':
        chat.onStart(env.data)
        break
      case 'chat:delta':
        chat.onDelta(env.data)
        break
      case 'chat:tool_start':
        chat.onToolStart(env.data)
        break
      case 'chat:tool_end':
        chat.onToolEnd(env.data)
        break
      case 'chat:approval':
        chat.onApproval(env.data)
        break
      case 'chat:compressed':
        chat.onCompressed(env.data)
        break
      case 'chat:user':
        // 插话 / 排队消息在注入时刻落库后广播：把它补进时间线，
        // 位置与真实对话一致（运行中落在上一轮工具结果之后）。
        session.echoUserMessage(env.data.entry_id, env.data.content)
        break
      case 'chat:context':
        chat.onContext(env.data)
        break
      case 'chat:done':
        chat.onDone(env.data)
        await reload()
        break
      case 'chat:stopped':
        chat.onStopped()
        await reload()
        break
      case 'chat:error':
        chat.onError(env.data)
        await reload()
        break
      case 'chat:gap':
        chat.onGap()
        await chat.syncApprovals(session.currentId || '')
        await reload()
        break
    }
  },
)

// 桌面壳意图：托盘「新建对话」与「外部打开的文件」在这里落地。
// 挂载时先消费一次（意图可能在视图挂载前就到了，用户当时在别的页），
// 之后靠事件即时响应。
function consumeShellIntent() {
  if (intentNewSession.value) {
    intentNewSession.value = false
    void newSession()
  }
  if (intentFile.value) {
    const p = intentFile.value
    intentFile.value = ''
    inputRef.value?.attachPath(p)
  }
}

onMounted(async () => {
  await session.loadList()
  if (!session.list.length) {
    try {
      await session.create({})
    } catch (e) {
      // 打开应用就失败时给一句人话，别让它以「界面出现异常」的面目冒出来
      toast.bad(`新建对话失败：${(e as Error)?.message || '请重试'}`)
    }
  } else if (!session.currentId) {
    await session.open(session.list[0].id)
  }
  window.addEventListener(SHELL_INTENT_EVENT, consumeShellIntent)
  consumeShellIntent()
})

onBeforeUnmount(() => {
  window.removeEventListener(SHELL_INTENT_EVENT, consumeShellIntent)
})

// 切会话必须把流式状态清干净：上一段的正文 / 工具卡 / 审批卡 / 压缩提示
// 都挂在 chat store 上，不清就会盖在新会话的历史上面。
watch(
  () => session.currentId,
  () => chat.reset(),
)

// 快照到手就回填上下文水位：打开 / 切换 / 新建 / 收尾刷新后输入框立刻有读数，
// 不必等下一轮 chat:context——那之前底栏是空的，用户以为这个功能不存在。
function seedContext() {
  if (!chat.running) void chat.seedContext(session.current, session.messages)
}

watch(() => [session.currentId, session.messages] as const, seedContext, { immediate: true })
// 引导数据晚于视图挂载：默认模型一到就补一次，空态也显示窗口刻度。
watch(() => settings.boot?.default_model, seedContext)
</script>

<template>
  <div class="chat wb-ui">
    <AppSidebar @new-session="newSession" />

    <div class="chat-main">
      <header class="chat-head">
        <h1>{{ session.current?.title || '新对话' }}</h1>
      </header>

      <template v-if="empty && !chat.running">
        <div class="welcome">
          <div class="hero-aura" aria-hidden="true">
            <span class="aura aura-top" />
            <span class="aura aura-left" />
            <span class="aura aura-right" />
            <span class="aura-mesh" />
          </div>

          <div class="hero-in">
            <div class="hero-badge rise">
              <AppIcon name="sparkles" size="ic-xs" />
              <span>本地运行 · 会干活的 AI 助手</span>
            </div>
            <h2 class="hero-title rise">你好，我是 <span class="hero-name">WorkBaby</span></h2>
            <p class="hero-sub rise">会读文件、跑代码、查资料。用大白话说需求就行。</p>
            <div class="rise">
              <ExampleCards @pick="pickExample" />
            </div>
            <p class="hero-foot rise">对话与文件都留在这台机器上 · 动手之前会先问你一句</p>
          </div>
        </div>
      </template>
      <template v-else>
        <MessageList />
      </template>

      <ChatInput
        ref="inputRef"
        :running="chat.running"
        :sending="sending"
        :stopping="stopping"
        @send="send"
        @steer="steer"
        @stop="stop"
        @new-session="newSession"
        @clear-session="clearSession"
      />
    </div>
  </div>
</template>

<style scoped>
/* .chat 是 .win-body 的 flex 子项。不写 flex:1 它就塌成内容宽度，
   整页右移出窗口、按钮和文字被裁掉——这不是「窄屏适配问题」，是必现 bug。 */
.chat {
  display: flex;
  height: 100%;
  min-height: 0;
  flex: 1;
  min-width: 0;
}
.chat-main {
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
  min-width: 0;
}
.chat-head {
  position: relative;
}
.chat-head h1 {
  min-width: 0;
}
/* 欢迎页是唯一的装饰性背景：三枚同源光晕 + 一层点阵，只出现在空态。 */
.welcome {
  flex: 1;
  position: relative;
  overflow: hidden;
  display: grid;
  place-items: center;
  padding: var(--wb-sp-8) var(--wb-sp-6);
}
.hero-aura {
  position: absolute;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
}
.aura {
  position: absolute;
  border-radius: 50%;
  filter: blur(90px);
  animation: aura-breathe var(--wb-ease) infinite;
}
.aura-top {
  /* 居中靠 calc 而不是 translateX(-50%)：位移会让 breathe 的 transform 把它抢回去 */
  top: -26%;
  left: calc(50% - 320px);
  width: 640px;
  height: 420px;
  background: radial-gradient(closest-side, var(--wb-glow-1), transparent);
  animation-duration: 14s;
}
.aura-left {
  bottom: -18%;
  left: -10%;
  width: 420px;
  height: 420px;
  background: radial-gradient(closest-side, var(--wb-glow-2), transparent);
  animation-duration: 18s;
  animation-delay: -6s;
}
.aura-right {
  bottom: -22%;
  right: -8%;
  width: 380px;
  height: 380px;
  background: radial-gradient(closest-side, var(--wb-glow-3), transparent);
  animation-duration: 22s;
  animation-delay: -11s;
}
/* 点阵：给纯色底一层秩序感。ellipse mask 让它从中心向外淡出，避免出现硬边 */
.aura-mesh {
  position: absolute;
  inset: 0;
  background-image: radial-gradient(var(--wb-grid-dot) 1px, transparent 0);
  background-size: 22px 22px;
  -webkit-mask-image: radial-gradient(ellipse 60% 55% at 50% 42%, #000 40%, transparent 100%);
  mask-image: radial-gradient(ellipse 60% 55% at 50% 42%, #000 40%, transparent 100%);
}
@keyframes aura-breathe {
  0%,
  100% {
    opacity: 0.8;
    transform: scale(1);
  }
  50% {
    opacity: 1;
    transform: scale(1.06);
  }
}
.hero-in {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-3);
  width: 100%;
  max-width: 660px;
}
.hero-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-2);
  height: var(--wb-ctl-h-sm);
  padding: 0 var(--wb-sp-3);
  border-radius: var(--wb-radius-full);
  background: var(--wb-surface);
  border: 1px solid var(--wb-border);
  box-shadow: var(--wb-shadow-1);
  color: var(--wb-primary);
  font-size: var(--wb-fs-xs);
  font-weight: 600;
}
.hero-title {
  font-family: var(--font-display);
  font-size: var(--wb-fs-3xl);
  font-weight: 700;
  letter-spacing: -0.01em;
  line-height: var(--wb-lh-tight);
  color: var(--wb-ink);
  text-align: center;
}
.hero-name {
  color: var(--wb-primary);
}
.hero-sub {
  color: var(--wb-muted);
  font-size: var(--wb-fs-md);
  margin-bottom: var(--wb-sp-2);
  max-width: 520px;
  text-align: center;
}
.hero-foot {
  margin-top: var(--wb-sp-3);
  color: var(--wb-muted);
  font-size: var(--wb-fs-xs);
}
/* 入场错峰：徽章 → 标题 → 副题 → 例句卡 → 脚注，一拍 60ms，只演一次 */
.rise {
  animation: hero-rise var(--wb-dur-slow) var(--wb-ease) backwards;
}
.hero-title.rise {
  animation-delay: 60ms;
}
.hero-sub.rise {
  animation-delay: 120ms;
}
.hero-in > .rise:nth-child(4) {
  animation-delay: 180ms;
}
.hero-foot.rise {
  animation-delay: 240ms;
}
@keyframes hero-rise {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
}
/* 装饰层动效不吃「减少动态效果」的系统设置：用户关了它，就该真的停 */
@media (prefers-reduced-motion: reduce) {
  .rise,
  .aura {
    animation: none;
  }
}
</style>
