<script setup lang="ts">
// 聊天主视图：会话面板 + 消息流 / 欢迎页 + 输入区。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useSse } from '../composables/useSse'
import { intentFile, intentNewSession, SHELL_INTENT_EVENT } from '../composables/shellIntent'
import { useChatStore } from '../stores/chat'
import { useSessionStore } from '../stores/session'
import { useSettingsStore } from '../stores/settings'
import { useToastStore } from '../stores/toast'
import type { AttachmentREQ } from '../types/api'
import AppIcon from '../components/common/AppIcon.vue'
import ChatInput from '../components/chat/ChatInput.vue'
import ExampleCards from '../components/chat/ExampleCards.vue'
import MessageList from '../components/chat/MessageList.vue'
import SessionPanel from '../components/chat/SessionPanel.vue'
import StatusHud from '../components/chat/StatusHud.vue'

const { t } = useI18n()
const session = useSessionStore()
const chat = useChatStore()
const settings = useSettingsStore()
const toast = useToastStore()
const router = useRouter()
const route = useRoute()
const inputRef = ref<InstanceType<typeof ChatInput> | null>(null)
const creating = ref(false)

const empty = computed(() => !session.messages.length && !chat.running)

// 没配默认模型时，欢迎页先把「去配置」顶到最前——不然第一条消息必然失败
const needsSetup = computed(() => settings.boot?.pi_env?.model_ready === false)

/** 新对话：不建空会话——PI 那边第一条消息才落盘，这里只把界面切到草稿态 */
function newSession() {
  if (creating.value) return
  creating.value = true
  try {
    chat.reset()
    session.draft()
    inputRef.value?.focus()
  } finally {
    creating.value = false
  }
}

const sending = ref(false)
const stopping = ref(false)

async function send(text: string, attachments: AttachmentREQ[]) {
  if (sending.value) return
  // loading 必须挂在最前面：没有会话时要先建会话，反馈晚于点击一秒以上会被当成没点上
  sending.value = true
  try {
    const sent = await chat.send(session.currentId || '', text, attachments)
    // 立刻回显：等 chat:done 拉快照才显示的话，用户会以为消息没发出去
    session.echoUserMessage(text)
    if (!session.currentId && sent?.session_id) {
      const fresh = sent.session_id
      await session.adopt(fresh)
      window.setTimeout(() => {
        if (!chat.running && session.currentId === fresh) void reload()
      }, 1500)
    }
  } catch (e) {
    toast.bad(t('chat.sendFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    sending.value = false
    inputRef.value?.focus()
  }
}

const steering = ref(false)
const queueing = ref(false)

async function steer(text: string) {
  if (!session.currentId || steering.value) return
  steering.value = true
  try {
    await chat.steer(session.currentId, text)
    toast.ok(t('chat.retryEvent'))
  } catch (e) {
    toast.bad(t('chat.steerFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    steering.value = false
  }
}

async function queueUp(text: string) {
  if (!session.currentId || queueing.value) return
  queueing.value = true
  try {
    await chat.followUp(session.currentId, text)
  } catch (e) {
    toast.bad(t('chat.queueFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    queueing.value = false
  }
}

async function stop() {
  if (!session.currentId || stopping.value) return
  stopping.value = true
  try {
    await chat.stop(session.currentId)
  } catch (e) {
    toast.bad(t('chat.stopFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    stopping.value = false
  }
}

// 收尾后拉权威快照。拉失败必须说出来：静默失败的表现是用户盯着一条已经结束的旧对话
async function reload() {
  try {
    await session.refresh()
  } catch (e) {
    chat.notify(t('chat.refreshFailed', { msg: (e as Error)?.message || '' }))
  }
}

function pickExample(text: string) {
  void send(text, [])
}

// 续写：截断后接着写。发一句「继续」是唯一协议安全的方式
async function resume() {
  if (chat.running) return
  await send(t('chat.continue'), [])
}

// 重试：把最后一条用户消息原样再发一次
async function retry() {
  if (chat.running) return
  const last = [...session.messages].reverse().find((m) => m.role === 'user')
  if (!last?.content) return
  await send(last.content, [])
}

// 断线重连后的对账：退出运行态 → 拉权威快照
async function reconcile() {
  chat.onGap()
  await reload()
}

// SSE 事件 → store；结束后拉一次权威快照，前端不做复杂合并
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
      case 'chat:ask':
        chat.onAsk(env.data)
        break
      case 'chat:ask_closed':
        chat.onAskClosed(env.data)
        break
      case 'chat:notify':
        chat.onNotify(env.data)
        break
      case 'chat:status':
        chat.onStatus(env.data)
        break
      case 'chat:widget':
        chat.onWidget(env.data)
        break
      case 'chat:title':
        chat.onTitle(env.data)
        break
      case 'chat:compressed':
        chat.onCompressed(env.data)
        break
      case 'chat:user':
        session.echoUserMessage(env.data.content)
        chat.onDequeued(env.data.content)
        break
      case 'chat:context':
        chat.onContext(env.data)
        break
      case 'chat:retry':
        chat.onRetry(env.data)
        break
      case 'chat:done':
        chat.onDone(env.data)
        await reload()
        void session.loadList()
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
        await reload()
        break
      case 'chat:switched':
        chat.onStopped()
        if (env.data?.session_id && env.data.session_id !== session.currentId) {
          await session.open(env.data.session_id)
        } else {
          await reload()
        }
        break
    }
  },
  reconcile,
)

// 桌面壳意图（托盘「新建对话」/ 外部打开文件）在这里落地
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

async function syncOnMount() {
  chat.reset()
  if (session.currentId) await reload()
}

onMounted(async () => {
  await session.loadList()
  const want = typeof route.query.session === 'string' ? route.query.session : ''
  if (want && want !== session.currentId) {
    await session.open(want)
  } else if (!session.currentId) {
    if (session.list.length) await session.open(session.list[0].id)
    else session.draft()
  }
  if (want) router.replace({ query: { ...route.query, session: undefined } })
  await syncOnMount()
  window.addEventListener(SHELL_INTENT_EVENT, consumeShellIntent)
  consumeShellIntent()
})

onBeforeUnmount(() => {
  window.removeEventListener(SHELL_INTENT_EVENT, consumeShellIntent)
})

// 切会话必须把流式状态清干净
watch(
  () => session.currentId,
  () => chat.reset(),
)

// 快照到手就回填上下文水位
function seedContext() {
  if (chat.running) return
  const provider = session.current?.provider || settings.boot?.default_provider || ''
  const model = session.current?.model || settings.boot?.default_model || ''
  void chat.seedContext(provider, model, session.messages)
}
watch(() => [session.currentId, session.messages] as const, seedContext, { immediate: true })
watch(() => settings.boot?.default_model, seedContext)
</script>

<template>
  <div class="chat">
    <SessionPanel :creating="creating" @new-session="newSession" />

    <div class="chat-main">
      <Transition name="chat-swap" mode="out-in">
        <div v-if="empty && !chat.running" class="hero">
          <div aria-hidden="true" class="hero-aura">
            <span class="wb-aura wb-aura--top" />
            <span class="wb-aura wb-aura--left" />
            <span class="wb-aura wb-aura--right" />
          </div>

          <div class="hero-in">
            <span class="hero-badge rise rise-1">
              <AppIcon name="sparkles" size="ic-xs" />
              {{ t('chat.emptyWelcomeBadge') }}
            </span>
            <h2 class="hero-title rise rise-2">
              {{ t('chat.emptyTitle') }} <em>WorkBaby</em>
            </h2>
            <p class="hero-sub rise rise-3">{{ t('chat.emptySub') }}</p>

            <div v-if="needsSetup" class="setup-card rise rise-4">
              <span class="wb-tile"><AppIcon name="cpu" /></span>
              <div class="setup-tx">
                <b>{{ t('chat.setupTitle') }}</b>
                <p>{{ t('chat.setupSub') }}</p>
              </div>
              <div class="setup-acts">
                <el-button type="primary" size="small" @click="router.push('/settings/providers')">
                  {{ t('chat.setupGo') }}
                </el-button>
                <el-button size="small" @click="router.push('/help/models')">
                  {{ t('chat.setupGuide') }}
                </el-button>
              </div>
            </div>
            <div v-else class="rise rise-4">
              <ExampleCards :busy="sending || chat.running" @pick="pickExample" />
            </div>

            <p class="hero-foot rise rise-5">{{ t('chat.emptyFoot') }}</p>
          </div>
        </div>

        <MessageList v-else :on-continue="resume" :on-retry="retry" />
      </Transition>

      <ChatInput
        ref="inputRef"
        :running="chat.running"
        :sending="sending"
        :stopping="stopping"
        :acting="steering ? 'steer' : queueing ? 'queue' : ''"
        @send="send"
        @steer="steer"
        @queue="queueUp"
        @stop="stop"
        @new-session="newSession"
      />
      <StatusHud />
    </div>
  </div>
</template>

<style scoped>
.chat {
  display: flex;
  flex: 1;
  min-width: 0;
  min-height: 0;
  height: 100%;
}
.chat-main {
  position: relative;
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  min-height: 0;
}
/* 欢迎页 ↔ 消息流切换：out-in 保证两态不重叠 */
.chat-swap-enter-active {
  transition:
    opacity var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur) var(--wb-ease);
}
.chat-swap-leave-active {
  transition: opacity var(--wb-dur-fast) var(--wb-ease);
}
.chat-swap-enter-from {
  opacity: 0;
  transform: translateY(6px);
}
.chat-swap-leave-to {
  opacity: 0;
}
.hero-aura {
  position: absolute;
  inset: 0;
  pointer-events: none;
  overflow: hidden;
}
.hero-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-2);
  height: var(--wb-ctl-h-sm);
  padding: 0 var(--wb-sp-3);
  border-radius: var(--wb-radius-full);
  background: var(--wb-elev-raised);
  border: 1px solid var(--wb-border);
  box-shadow: var(--wb-card-shadow);
  color: var(--wb-primary);
  font-size: var(--wb-fs-xs);
  font-weight: 700;
}
.hero-foot {
  margin-top: var(--wb-sp-2);
  color: var(--wb-muted);
  font-size: var(--wb-fs-xs);
}
.setup-card {
  width: 100%;
  max-width: calc(560px * var(--wb-vw-scale));
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-4);
  border-radius: var(--wb-radius-xl);
  background: var(--wb-elev-raised);
  border: 1px solid var(--wb-border);
  box-shadow: var(--wb-card-shadow);
  text-align: left;
}
.setup-tx {
  min-width: 0;
  flex: 1;
}
.setup-tx b {
  display: block;
  font-size: var(--wb-fs-md);
  font-weight: 700;
  color: var(--wb-ink);
  margin-bottom: 2px;
}
.setup-tx p {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
}
.setup-acts {
  flex: none;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
</style>
