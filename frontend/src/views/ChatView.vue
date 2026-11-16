<script setup lang="ts">
// 对话页：左栏会话列表 + 主区消息流 + 输入区。
// 滚动贴底由 JS 管（流式时元素高度在变，浏览器锚定会乱跳）。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ChatComposer from '@/components/chat/ChatComposer.vue'
import TodoPanel from '@/components/chat/TodoPanel.vue'
import ChatNotices from '@/components/chat/ChatNotices.vue'
import ChatHeader from '@/components/chat/ChatHeader.vue'
import HistoryList from '@/components/chat/HistoryList.vue'
import AskCard from '@/components/chat/AskCard.vue'
import WorkPanel from '@/components/chat/WorkPanel.vue'
import AppIcon from '@/components/common/AppIcon.vue'
import SessionList from '@/components/sessions/SessionList.vue'
import ChatLanding from '@/components/chat/ChatLanding.vue'
import ChatEmpty from '@/components/chat/ChatEmpty.vue'
import { useAppStore } from '@/stores/app'
import { useChatStore } from '@/stores/chat'
import { useToasts } from '@/composables/useToasts'
import { useComposerActions } from '@/composables/useComposerActions'
import type { UIResponse } from '@/types/api'

const app = useAppStore()
const chat = useChatStore()
const toasts = useToasts()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const composerActions = useComposerActions()

/** 侧栏顶部的「新对话」：它从输入框底栏搬过来之后住在这里。 */
async function onNewChat(): Promise<void> {
  await composerActions.startNewChat()
  // 地址同步成「无会话」：否则刷新窗口会按旧 query 又跳回上一条会话。
  await router.replace({ name: 'chat', query: {} })
}

/** 落地区的「选工作目录」：重建引擎要十几秒，等待期锁按钮，失败出声。 */
const choosingWorkspace = ref(false)
async function pickWorkspace(): Promise<void> {
  if (choosingWorkspace.value) return
  choosingWorkspace.value = true
  try {
    await app.chooseWorkspace()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    choosingWorkspace.value = false
  }
}

// 切会话的加载态：refresh 要走一次后端投影，长会话要几百毫秒。
// 没有它，点一条会话之后消息区会先空一下——那一下看起来就是「这条会话没了」。
const loadingSession = ref(false)
const sessionError = ref('')
// 首屏那一次 refresh 是否已经落定：落定前说「这条会话是空的」是错的断言。
const loaded = ref(false)

const scroller = ref<HTMLElement | null>(null)
const composer = ref<{ focus: () => void } | null>(null)
const pinned = ref(true)
// 底部栈（待办 + 输入区 + 提示条）的实际高度写进 CSS 变量：
// 悬浮元素（回到底部、过程提示）贴着它算偏移，textarea 撑高也不会叠进输入区。
const mainEl = ref<HTMLElement | null>(null)
const bottomStack = ref<HTMLElement | null>(null)
let bottomRo: ResizeObserver | null = null

// 全局快捷键只留 Ctrl+J（回输入框）与 Esc（收起授权提示）：
// Ctrl+K 归命令面板，其余交给控件自己的按键，不在全局抢。
function onKeydown(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'j') {
    event.preventDefault()
    composer.value?.focus()
  }
}

const hasTurn = computed(() => Boolean(chat.turn))

// 拼在历史后面的当前轮：单独算出来交给 HistoryList——那一层才知道
// 历史最后一行是什么角色，决定这一轮要不要出头。
const liveTurn = computed(() => {
  const turn = chat.turn
  if (!turn) return null
  return {
    userMessage: turn.user_message,
    blocks: turn.blocks,
    runId: turn.run_id,
    model: chat.model?.model_id,
    streaming: chat.streaming,
    errored: Boolean(chat.runError),
    // 脚标秒表的起点：一轮从「发送成功」那一刻起就有耗时，不用等引擎报。
    startedAt: chat.startedAt || undefined,
    // 用量与耗时：收尾之后这一轮会从直播位退到历史位，
    // 那时读数还得留着——所以它按 run id 记在 store 里，不在 turn 上。
    usage: chat.usageOf(turn.run_id),
  }
})

function nearBottom(): boolean {
  const el = scroller.value
  if (!el) return true
  return el.scrollHeight - el.scrollTop - el.clientHeight < 120
}

function scrollToBottom(smooth = false) {
  const el = scroller.value
  if (!el) return
  el.scrollTo({ top: el.scrollHeight, behavior: smooth ? 'smooth' : 'auto' })
}

// 连续钉底几次：流式时正文一阵一阵长高，只钉一次会被后续高度变化抛在后面
// （打开就在中间）；只在真的贴底时才钉。
async function pinBottom(rounds = 3): Promise<void> {
  for (let i = 0; i < rounds; i++) {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
    if (!pinned.value) return
    scrollToBottom()
  }
}

function onScroll() {
  pinned.value = nearBottom()
  // 滚动事件本身也当一次排程信号：贴底状态下滚动条抖动（内容长高把
  // 位置顶上去）没有伴随 store 变化，靠 watch 抓不到。
  schedulePin()
}

// 流式高频补丁下的贴底：同一帧只滚一次。watch 每帧触发几次时
// scrollTo 连发会让滚动条抖动、掉帧；rAF 合帧是唯一的解。
let pinQueued = false
function schedulePin(): void {
  if (!pinned.value || pinQueued) return
  pinQueued = true
  requestAnimationFrame(() => {
    pinQueued = false
    if (!pinned.value) return
    const el = scroller.value
    if (!el) return
    // 距底不到 2px 就不再推：连发 scrollTo 反而让滚动条抖。
    if (el.scrollHeight - el.scrollTop - el.clientHeight < 2) return
    el.scrollTop = el.scrollHeight
  })
}

watch(
  () => [chat.turn?.blocks.length, chat.turn?.blocks.at(-1)?.block_text?.length, chat.history.length],
  () => {
    schedulePin()
  },
)

const answering = ref(false)

async function answer(response: UIResponse) {
  if (answering.value) return
  answering.value = true
  try {
    await chat.answer(response)
  } catch (err) {
    chat.pushNotice('error', err instanceof Error ? err.message : String(err))
  } finally {
    answering.value = false
  }
}

async function openSession(path: string) {
  chat.reset()
  // 切会话就回到贴底：用户点一条历史，看的是它最后那一句。
  pinned.value = true
  loadingSession.value = true
  sessionError.value = ''
  try {
    await chat.refresh(path)
  } catch (err) {
    // 打不开要说清是哪一条、为什么：静默留一个空消息区，
    // 用户会以为会话被清空了。
    sessionError.value = err instanceof Error ? err.message : String(err)
    return
  } finally {
    loadingSession.value = false
  }
  // 会话页点「继续」会带 query.session 跳过来：把地址同步成当前会话，
  // 刷新窗口时才不会又回到默认会话。
  await router.replace({ name: 'chat', query: path ? { session: path } : {} })
  await nextTick()
  await pinBottom()
}

// 侧栏里的会话被删 / 改名了：删的是正看着的这个时，后端已把引擎切到新会话，
// 这里跟着 refresh 对齐——不能调 newChat，那会多开一条空会话。
async function onSessionChanged(path?: string) {
  if (path && path === chat.sessionFile) {
    // 显式传空串：refresh 的缺省参数会带上还没更新的旧路径（刚被删的那个）。
    await chat.refresh('')
  }
}

/** 会话页跳过来时带的会话路径。 */
function sessionFromQuery(): string {
  const value = route.query.session
  return typeof value === 'string' ? value : ''
}

onMounted(async () => {
  window.addEventListener('keydown', onKeydown)
  // 输入区 autosize、待办面板展开收起都会改底部高度：跟着量，悬浮元素不叠压。
  bottomRo = new ResizeObserver(() => {
    const stack = bottomStack.value
    const main = mainEl.value
    if (!stack || !main) return
    main.style.setProperty('--wb-bottom-h', `${stack.offsetHeight}px`)
  })
  if (bottomStack.value) bottomRo.observe(bottomStack.value)
  try {
    const path = sessionFromQuery()
    await chat.refresh(path)
    await nextTick()
    pinned.value = true
    await pinBottom()
    loaded.value = true
  } catch {
    // query 里的会话可能刚被删掉：退回默认会话，地址也一并清掉。
    await router.replace({ name: 'chat', query: {} })
    try {
      await chat.refresh()
    } catch {
      /* 引擎没起来时对话页仍是可用的空态 */
    } finally {
      loaded.value = true
    }
  }
  // 模型与思考档清单不阻塞首屏：它要问引擎，而引擎冷启动可能要十几秒。
  // 但它**必须发**：清单不随引擎事件推送，进对话页不拉一次，下拉就一直是空的。
  void chat.loadModels()
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  bottomRo?.disconnect()
  bottomRo = null
})

watch(
  () => route.query.session,
  async (value) => {
    const path = typeof value === 'string' ? value : ''
    if (!path || path === chat.sessionFile) return
    await openSession(path)
  },
)

// 记住当前会话供下次启动恢复：后端 State 不再代写（只读接口不写盘），这里补上。
// rememberSession 自己吞失败——记不住不影响本次使用。
watch(
  () => chat.sessionFile,
  (file) => {
    if (file) void app.rememberSession(file)
  },
)

</script>

<template>
  <div class="chat-page">
    <aside class="chat-side">
      <SessionList
        :active="chat.sessionFile"
        :compact="true"
        show-new
        @open="openSession"
        @new-chat="onNewChat"
        @changed="onSessionChanged"
      />
    </aside>

    <section ref="mainEl" class="chat-main">
      <ChatHeader />

      <div ref="scroller" class="chat-scroll" @scroll="onScroll">
        <!-- 缺引擎 / 切会话中 / 打不开：三块落地区在 ChatLanding 里 -->
        <ChatLanding
          v-if="!loaded || !app.workspace || !app.hasEngine || loadingSession || Boolean(sessionError)"
          :no-engine="!app.hasEngine"
          :no-workspace="!app.workspace"
          :loading="loadingSession || !loaded"
          :error="sessionError"
          :busy="choosingWorkspace"
          @open-latest="openSession('')"
          @open-settings="router.push({ name: 'settings' })"
          @pick-workspace="pickWorkspace"
        />

        <ChatEmpty v-else-if="!hasTurn && !chat.history.length" />

        <div v-else class="msg-col-wrap">
          <HistoryList :turns="chat.history" :live="liveTurn" />
          <AskCard v-if="chat.activeAsk" :ask="chat.activeAsk" :answering="answering" @answer="answer" />
        </div>
      </div>

      <button type="button" v-if="!pinned" class="jump-bottom" :title="t('chat.jumpBottom')" :aria-label="t('chat.jumpBottom')" @click="scrollToBottom(true)">
        <AppIcon name="chevron" :size="13" />
      </button>

      <div ref="bottomStack" class="chat-bottom">
        <TodoPanel :todos="chat.todos" />
        <ChatComposer ref="composer" />
        <ChatNotices />
      </div>
    </section>

    <WorkPanel />
  </div>
</template>

<style scoped>
.chat-page {
  display: flex;
  flex: 1;
  min-height: 0;
}

.chat-side {
  width: var(--wb-side);
  flex: none;
  border-right: 1px solid var(--wb-line);
  background: var(--wb-window);
  display: flex;
  flex-direction: column;
  min-height: 0;
}

/* 窄窗口（800×600 的默认尺寸）下侧栏收窄：244px 在 800px 的窗口里会把
   主区压到放不下一行代码。用容器查询而不是媒体查询——窗口是最外层容器，
   两者的断点在这里恰好一致，但以后嵌进别的布局时容器查询仍然正确。 */
@container app (max-width: 960px) {
  .chat-side {
    width: var(--wb-side-narrow);
  }
}

.chat-main {
  position: relative;
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
}

/* 页头与授权链接条都搬进 ChatHeader 了：它们只在对话页出现，
   而这一页自己只管三栏的排布与滚动。 */
.chat-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-anchor: none;
  scrollbar-gutter: stable;
}

/* 消息列：HistoryList 自己管列宽与间距，这里只给它一个居中的包裹层 */
.msg-col-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}

/* 底部栈只负责把待办 / 输入区 / 提示条堆在一起并报出总高度（--wb-bottom-h） */
.chat-bottom {
  flex: none;
}

.jump-bottom {
  position: absolute;
  left: 50%;
  bottom: calc(var(--wb-bottom-h, 150px) + var(--wb-sp-3));
  transform: translateX(-50%) rotate(90deg);
  display: grid;
  place-items: center;
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border: 1px solid var(--wb-line-strong);
  border-radius: 50%;
  background: var(--wb-raised);
  color: var(--wb-mute);
  box-shadow: var(--wb-shadow-lift);
  transition:
    transform var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}

.jump-bottom:hover {
  color: var(--wb-brand);
}

.jump-bottom:active {
  transform: translateX(-50%) rotate(90deg) scale(var(--wb-press-active));
}
</style>
