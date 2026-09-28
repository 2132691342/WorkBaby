<script setup lang="ts">
// 聊天主视图：侧栏 + 消息流 + 输入区；没有会话时给引导页。
import { computed, onMounted, ref } from 'vue'
import * as api from '../api'
import { useSse } from '../composables/useSse'
import { useChatStore } from '../stores/chat'
import { useSessionStore } from '../stores/session'
import type { AttachmentREQ } from '../types/api'
import AppSidebar from '../components/common/AppSidebar.vue'
import ChatInput from '../components/chat/ChatInput.vue'
import ExampleCards from '../components/chat/ExampleCards.vue'
import MessageList from '../components/chat/MessageList.vue'

const session = useSessionStore()
const chat = useChatStore()
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
  } finally {
    creating.value = false
  }
}

async function send(text: string, attachments: AttachmentREQ[]) {
  if (!session.currentId) await newSession()
  if (!session.currentId) return
  await chat.send(session.currentId, text, attachments)
  inputRef.value?.focus()
}

async function steer(text: string) {
  if (session.currentId) await chat.steer(session.currentId, text)
}

async function stop() {
  if (session.currentId) await chat.stop(session.currentId)
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
        await session.refresh()
        break
      case 'chat:error':
        chat.onError(env.data)
        await session.refresh()
        break
      case 'chat:gap':
        chat.onGap()
        await session.refresh()
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
