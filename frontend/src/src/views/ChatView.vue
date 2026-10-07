<script setup lang="ts">
// 聊天主视图：侧栏 + 消息流 + 输入区；没有会话时给引导页。
import { computed, onMounted, ref, watch } from 'vue'
import * as api from '../api'
import { useSse } from '../composables/useSse'
import { useChatStore } from '../stores/chat'
import { useSessionStore } from '../stores/session'
import { useToastStore } from '../stores/toast'
import type { AttachmentREQ } from '../types/api'
import AppSidebar from '../components/common/AppSidebar.vue'
import AppIcon from '../components/common/AppIcon.vue'
import ChatInput from '../components/chat/ChatInput.vue'
import ExampleCards from '../components/chat/ExampleCards.vue'
import MessageList from '../components/chat/MessageList.vue'

const session = useSessionStore()
const chat = useChatStore()
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

onMounted(async () => {
  await session.loadList()
  if (!session.list.length) {
    await session.create({})
  } else if (!session.currentId) {
    await session.open(session.list[0].id)
  }
})

// 切会话必须把流式状态清干净：上一段的正文 / 工具卡 / 审批卡 / 压缩提示
// 都挂在 chat store 上，不清就会盖在新会话的历史上面。
watch(
  () => session.currentId,
  () => chat.reset(),
)
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
          <div class="hero-glow g1" aria-hidden="true" />
          <div class="hero-glow g2" aria-hidden="true" />
          <div class="hero-badge rise">
            <AppIcon name="sparkles" size="ic-xs" />
            <span>本地运行 · 会干活的 AI 助手</span>
          </div>
          <h2 class="hero-title rise">你好，我是 <span class="hero-name">WorkBaby</span></h2>
          <p class="hero-sub rise">会读文件、跑代码、查资料。用大白话说需求就行。</p>
          <div class="rise">
            <ExampleCards @pick="pickExample" />
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
.welcome {
  flex: 1;
  position: relative;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-8);
}
/* 光晕只在空态引导出现（案例的 blur 圆）：工作界面仍是纯色 */
.hero-glow {
  position: absolute;
  border-radius: 50%;
  filter: blur(90px);
  pointer-events: none;
}
.hero-glow.g1 {
  width: 480px;
  height: 480px;
  top: -22%;
  left: -12%;
  background: var(--wb-glow-1);
}
.hero-glow.g2 {
  width: 400px;
  height: 400px;
  bottom: -18%;
  right: -10%;
  background: var(--wb-glow-2);
}
.hero-badge,
.hero-title,
.hero-sub,
.welcome > :last-child {
  position: relative;
  z-index: 1;
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
  color: var(--wb-primary);
  font-size: var(--wb-fs-xs);
  font-weight: 600;
}
.hero-title {
  font-family: var(--font-display);
  font-size: var(--wb-fs-3xl);
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--wb-ink);
  text-align: center;
}
.hero-name {
  color: var(--wb-primary);
}
.hero-sub {
  color: var(--wb-muted);
  margin-bottom: var(--wb-sp-4);
  max-width: 460px;
  text-align: center;
}
/* 入场错峰：徽章 → 标题 → 副题 → 例句卡，一拍 60ms，只演一次 */
.rise {
  animation: hero-rise var(--wb-dur-slow) var(--wb-ease) backwards;
}
.hero-title.rise {
  animation-delay: 60ms;
}
.hero-sub.rise {
  animation-delay: 120ms;
}
.welcome > :last-child.rise {
  animation-delay: 180ms;
}
@keyframes hero-rise {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
}
</style>
