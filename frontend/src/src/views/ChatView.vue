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

async function send(text: string, attachments: AttachmentREQ[]) {
  if (!session.currentId) await newSession()
  if (!session.currentId) return
  const sid = session.currentId
  let sent
  try {
    sent = await chat.send(sid, text, attachments)
  } catch (e) {
    // 发送被拒（会话忙 / 没配模型服务）必须立刻说清楚，不能让消息无声消失
    toast.bad(`发送失败：${(e as Error)?.message || '请重试'}`)
    return
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
  if (!session.currentId) return
  try {
    await chat.stop(session.currentId)
  } catch (e) {
    toast.bad(`停止失败：${(e as Error)?.message || '请重试'}`)
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
          <div class="hero-mark">WB</div>
          <h2>你好，我是 WorkBaby</h2>
          <p>会读文件、跑代码、查资料。用大白话说需求就行。</p>
          <ExampleCards @pick="pickExample" />
        </div>
      </template>
      <template v-else>
        <MessageList />
      </template>

      <ChatInput
        ref="inputRef"
        :running="chat.running"
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
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-8);
}
.hero-mark {
  width: 64px;
  height: 64px;
  border-radius: var(--wb-radius-xl);
  display: grid;
  place-items: center;
  background: var(--wb-primary);
  color: var(--wb-primary-ink);
  font-family: var(--font-display);
  font-size: var(--wb-fs-2xl);
  font-weight: 700;
  letter-spacing: var(--wb-ls-plate);
}
.welcome h2 {
  font-family: var(--font-display);
  font-size: var(--wb-fs-xl);
  color: var(--wb-ink);
}
.welcome p {
  color: var(--wb-muted);
  margin-bottom: var(--wb-sp-5);
}
</style>
