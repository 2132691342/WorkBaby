<script setup lang="ts">
// 聊天主视图：会话面板 + 消息流 + 输入区；没有会话时给引导页。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import * as api from '../api'
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

const session = useSessionStore()
const chat = useChatStore()
const settings = useSettingsStore()
const toast = useToastStore()
const router = useRouter()
const inputRef = ref<InstanceType<typeof ChatInput> | null>(null)
const creating = ref(false)

const empty = computed(() => !session.currentId || (!session.messages.length && !chat.running))

// 没配默认模型时，欢迎页先把「去配置」顶到最前——不然用户的第一条消息必然失败，
// 而失败提示要十几秒后才出现，那一刻他已经认定「这软件是坏的」。
const needsSetup = computed(() => !settings.boot?.default_provider_id || !settings.boot?.default_model)

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
  if (sending.value) return
  // loading 必须挂在最前面：没有会话时要先建会话（建会话 + 拉列表 + 取快照三次请求），
  // 反馈晚于点击一秒以上，用户就会以为没点上而重复点。
  sending.value = true
  try {
    if (!session.currentId) await newSession()
    if (!session.currentId) return
    const sent = await chat.send(session.currentId, text, attachments)
    // 发送后立刻回显这条消息：等 chat:done 拉快照才显示的话，
    // 用户会以为消息没发出去（助手还没回，观感上就是「什么都没发生」）。
    if (sent?.entry_id) session.echoUserMessage(sent.entry_id, text)
  } catch (e) {
    // 发送被拒（会话忙 / 没配模型服务）必须立刻说清楚，不能让消息无声消失
    toast.bad(`发送失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    sending.value = false
    inputRef.value?.focus()
  }
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

// 续写：截断后接着写。发一句「继续」是唯一协议安全的方式——
// 直接把半截回复续上会被上游拒掉，助手也不该看到一条自己没写完的消息。
async function resume() {
  if (chat.running) return
  await send('继续', [])
}

// 重试：把最后一条用户消息原样再发一次。失败轮的半成品已经落库，
// 重发等于让助手在同一段上下文上再答一遍，历史不会因此错乱。
async function retry() {
  if (chat.running) return
  const last = [...session.messages].reverse().find((m) => m.role === 'user')
  if (!last?.content) return
  await send(last.content, [])
}

// 断线重连后的对账：断开期间可能丢了 done / error，重放窗口也不保证兜得住。
// 退出运行态 → 拉权威快照 → 对齐未决审批；少做一件，界面就可能永远停在「运行中」。
async function reconcile() {
  chat.onGap()
  await chat.syncApprovals(session.currentId || '')
  await reload()
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
  reconcile,
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
    <SessionPanel :creating="creating" @new-session="newSession" />

    <div class="chat-main">
      <template v-if="empty && !chat.running">
        <div class="welcome">
          <div class="hero-aura" aria-hidden="true">
            <span class="wb-aura wb-aura-top" />
            <span class="wb-aura wb-aura-left" />
            <span class="wb-aura wb-aura-right" />
            <span class="wb-aura-mesh" />
          </div>

          <div class="hero-in">
            <div class="hero-badge rise">
              <AppIcon name="sparkles" size="ic-xs" />
              <span>本地运行 · 会干活的 AI 助手</span>
            </div>
            <h2 class="hero-title rise">你好，我是 <span class="hero-name">WorkBaby</span></h2>
            <p class="hero-sub rise">会读文件、跑代码、查资料。用大白话说需求就行。</p>

            <!-- 没配模型：例句卡没有意义（发了也收不到回答），换成一步到位的配置引导 -->
            <div v-if="needsSetup" class="setup-card rise">
              <span class="setup-ic"><AppIcon name="cpu" /></span>
              <div class="setup-tx">
                <b>先把模型服务配好，我才能开口说话</b>
                <p>需要一个 API Key（在服务商官网申请），大约 1 分钟。跟着教程走就行。</p>
              </div>
              <div class="setup-acts">
                <button class="btn btn-primary" type="button" @click="router.push('/settings/providers')">
                  去配置
                </button>
                <button class="btn" type="button" @click="router.push('/help/models')">看教程</button>
              </div>
            </div>
            <div v-else class="rise">
              <ExampleCards :busy="sending || chat.running" @pick="pickExample" />
            </div>

            <p class="hero-foot rise">对话与文件都留在这台机器上 · 动手之前会先问你一句</p>
          </div>
        </div>
      </template>
      <template v-else>
        <MessageList :on-continue="resume" :on-retry="retry" />
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
/* 欢迎页的配置引导卡：白底 + 中性描边，与例句卡同族但更「有事要做」 */
.setup-card {
  width: 100%;
  max-width: 520px;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-4);
  border-radius: var(--wb-radius-lg);
  background: var(--wb-surface);
  border: 1px solid var(--wb-border);
  box-shadow: var(--wb-shadow-2);
  text-align: left;
}
.setup-ic {
  flex: none;
  width: var(--wb-tile);
  height: var(--wb-tile);
  display: grid;
  place-items: center;
  border-radius: var(--wb-radius);
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
}
.setup-tx {
  min-width: 0;
  flex: 1;
}
.setup-tx b {
  display: block;
  font-size: var(--wb-fs-md);
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
/* 欢迎页是唯一的装饰性背景：三枚同源光晕 + 一层点阵，只出现在空态。 */
.welcome {
  flex: 1;
  min-height: 0;
  position: relative;
  /* 内容（badge + 标题 + 例句卡）在最小窗口（960×640）下装不下，
     用可滚动 + margin auto 居中：装得下时居中，装不下时从顶部滚，不许裁切 */
  overflow-y: auto;
  overflow-x: hidden;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 0 var(--wb-sp-6);
}
/* 欢迎页的光晕 / 点阵装饰已收进 wb-ui.css 的 .wb-aura*：整套才一份定义，
   组件里只留「放在哪」的位置规则。 */
.hero-in {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-3);
  width: 100%;
  max-width: 660px;
  margin: auto 0;
  padding: var(--wb-sp-6) 0;
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
  .wb-aura {
    animation: none;
  }
}
</style>
