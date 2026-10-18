<script setup lang="ts">
// 消息流：历史消息 + 正在进行的回合（思考 / 工具 / 流式正文）。
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useChatStore, type ToolRun } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import { renderMarkdown, renderMarkdownStream } from '../../utils/md'
import { fmtInt } from '../../utils/num'
import { groupTurns } from '../../utils/turns'
import AppIcon from '../common/AppIcon.vue'
import AskCard from './AskCard.vue'
import MessageItem from './MessageItem.vue'
import ToolRunRow from './ToolRunRow.vue'

type RunBlock = { key: string; kind: 'run'; run: ToolRun }

// 收尾提示条上的动作：截断续写 / 出错重试由视图实现。
// 传回调而不是 emit：按钮要 await 到结果才能收回 loading，emit 是同步的拿不到。
const props = defineProps<{
  onContinue?: () => Promise<void> | void
  onRetry?: () => Promise<void> | void
}>()

const acting = ref<'' | 'continue' | 'retry'>('')

// 停止重试：失败已由 store 弹提示，这里只负责把按钮的 loading 收回去
const cancellingRetry = ref(false)
async function stopRetry() {
  if (cancellingRetry.value) return
  cancellingRetry.value = true
  try {
    await chat.cancelRetry(session.currentId || '')
  } catch {
    /* store 层没有 toast，失败时保持提示条与按钮可用即可 */
  } finally {
    cancellingRetry.value = false
  }
}
async function act(kind: 'continue' | 'retry') {
  if (acting.value || chat.running) return
  acting.value = kind
  try {
    if (kind === 'continue') await props.onContinue?.()
    else await props.onRetry?.()
  } finally {
    acting.value = ''
  }
}

const session = useSessionStore()
const chat = useChatStore()
const router = useRouter()

// 模型配置类错误（重试解决不了）：服务不存在 3101、缺服务或模型名 3111、
// 模型不识图 3113、没存 Key 3114 —— 出路都在设置页。
const SETUP_CODES = new Set(['3101', '3111', '3113', '3114'])
const needSetup = computed(() => SETUP_CODES.has(chat.lastErrorCode))

const scroller = ref<HTMLElement | null>(null)
const stick = ref(true)
// 流式思考块的折叠：默认展开（回复中看得见在想什么），可以收起
const liveThinkOpen = ref(true)

// 思考可能几万字：全量塞进 DOM 会让每帧重排压住主线程，也会把正文顶出屏幕。
// 只渲染尾部——推理是流式的，用户要看的是「现在想到哪」。
const THINK_TAIL = 4000
const liveThink = computed(() => {
  const t = chat.thinking
  if (t.length <= THINK_TAIL) return t
  return `…（前文已折叠，共 ${t.length} 字）\n${t.slice(-THINK_TAIL)}`
})

// 正文一出现就自动收起思考：视线该落在答案上，而不是继续跟着推理往下滚。
// 只自动收一次，用户随后手动展开不再被打断。
let autoCollapsed = false
watch(
  () => chat.streaming,
  (text) => {
    if (text && !autoCollapsed) {
      autoCollapsed = true
      liveThinkOpen.value = false
    }
  },
)
watch(
  () => chat.running,
  (running) => {
    if (running) {
      autoCollapsed = false
      liveThinkOpen.value = true
    }
  },
)

// 截断提示必须带出实际预算：撞的是「我们给的额度」还是「厂商硬限制」，
// 光看提示分不出来，而两者该做的事不同（去调额度 vs 换模型或接受）。
const truncText = computed(() =>
  chat.truncBudget
    ? `这一轮用满了 ${fmtInt(chat.truncBudget)} 的输出预算，后半段没有写出来。`
    : '这一轮到了输出上限，后半段没有写出来。',
)

// 流式渲染节流：每个 delta 全量重解析 markdown 会打满主线程，SSE 被服务端判为慢消费者断连；
// 数据照单全收、只把渲染放慢，收尾补完整渲染。长文再降一档（全量重解析是 O(n)）。
const STREAM_TICK_MS = 120
const STREAM_TICK_MS_LONG = 280
const STREAM_LONG_CHARS = 20000
const renderedStream = ref('')
let renderTimer = 0
watch(
  () => chat.streaming,
  () => {
    if (renderTimer) return
    const tick = chat.streaming.length > STREAM_LONG_CHARS ? STREAM_TICK_MS_LONG : STREAM_TICK_MS
    renderTimer = window.setTimeout(() => {
      renderTimer = 0
      renderedStream.value = renderMarkdownStream(chat.streaming)
    }, tick)
  },
  { immediate: true },
)
watch(
  () => chat.running,
  (running) => {
    if (!running && renderTimer) {
      clearTimeout(renderTimer)
      renderTimer = 0
    }
    // 收尾用完整渲染：把流式期间跳过的代码高亮一次性补上
    renderedStream.value = renderMarkdown(chat.streaming)
  },
)

function onScroll() {
  if (jumping) return
  const el = scroller.value
  if (!el) return
  stick.value = el.scrollHeight - el.scrollTop - el.clientHeight < 80
}

// 回到底部：滚上去看历史时流式还在继续，没有这个入口就只能手动拖回去。
// 平滑滚动期间 scroll 事件会把 stick 判回 false、贴底逻辑也会抢着置底，都让位。
let jumping = false
function jumpToLatest() {
  const el = scroller.value
  if (!el) return
  jumping = true
  stick.value = true
  el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
  window.setTimeout(() => {
    jumping = false
  }, 400)
}

// 工具结果的参数不在自己身上，而在声明它的那条 assistant(tool_calls) 里。
// 不做这层关联的话，历史工具卡只剩一个光秃秃的名字，用户完全不知道助手干了什么。
const toolArgsMap = computed(() => {
  const map = new Map<string, Record<string, unknown>>()
  for (const m of session.messages) {
    for (const tc of m.tool_calls || []) {
      if (tc.args) map.set(tc.id, tc.args)
    }
  }
  return map
})

// 一次「提问 → 回答」是一整块（思考 + 多个工具 + 几段正文）；分组逻辑在 utils/turns.ts。
type TurnBlock = import('../../utils/turns').TurnBlock<unknown>

const turnBlocks = computed<TurnBlock[]>(() => groupTurns(session.messages, () => []))

// 运行中的工具卡留在助手列里（缩进对齐）
const runBlocks = computed<RunBlock[]>(() =>
  chat.runs.map((r) => ({ key: `r-${r.tool_call_id}`, kind: 'run' as const, run: r })),
)

// 贴底滚动走 rAF 合并：逐 delta 读 scrollHeight 会把主线程压成强制重排。
// 提问卡也要盯：它等在正文下面，不滚过去就被输入框盖住。
let scrollPending = false
function stickToBottom() {
  if (!stick.value || scrollPending || jumping) return
  scrollPending = true
  requestAnimationFrame(async () => {
    scrollPending = false
    await nextTick()
    const el = scroller.value
    if (el) el.scrollTop = el.scrollHeight
  })
}
watch(
  () => [session.messages.length, chat.streaming.length, chat.thinking.length, chat.runs.length, chat.asks.length] as const,
  stickToBottom,
)

// 打开会话时消息往往已经加载完了才挂载本组件，watch 不会为「首次就有数据」触发，
// 于是永远停在最上面那条。切回一个聊了一半的会话却要从头翻，是很直接的挫败感。
onMounted(async () => {
  await nextTick()
  const el = scroller.value
  if (el) el.scrollTop = el.scrollHeight
})
</script>

<template>
  <div ref="scroller" class="msgs" @scroll.passive="onScroll">
    <div class="msgs-inner">
      <template v-for="b in turnBlocks" :key="b.key">
        <MessageItem v-if="b.kind === 'user'" :msg="b.msg" />
        <div v-else-if="b.kind === 'assistant'" class="turn turn-a">
          <div class="msg-a">
            <div class="avatar"><span>WB</span></div>
            <div class="msg-a-body">
              <template v-for="p in b.parts" :key="p.msg.id">
                <MessageItem :msg="p.msg" plain />
                <!-- 工具卡和审批卡收进同一块：它们本来就是这件事的步骤。
                     审批是这一回合要用户拍板的一步，脱离竖轨就成了悬空的独立元素。 -->
                <div v-if="p.tools.length" class="turn-tools">
                  <ToolRunRow
                    v-for="t in p.tools"
                    :key="t.id"
                    :label="t.tool_name || '工具'"
                    :tool="t.tool_name || ''"
                    :args="toolArgsMap.get(t.tool_call_id || '')"
                    :running="false"
                    :ok="!t.is_error"
                    :output="t.content || ''"
                  />
                </div>
              </template>
            </div>
          </div>
        </div>
      </template>

      <!-- 正在进行的回合 -->
      <div v-if="chat.running" class="turn turn-a">
        <div class="msg-a">
          <div class="avatar is-live"><span>WB</span></div>
          <div class="msg-a-body">
            <!-- 思考：流式期间默认展开，可手动收起；呼吸点标记「还在想」 -->
            <div v-if="chat.thinking" class="think is-live">
              <button
                class="think-hd"
                type="button"
                :aria-expanded="liveThinkOpen"
                aria-controls="live-think-body"
                @click="liveThinkOpen = !liveThinkOpen"
              >
                <span class="think-dot" aria-hidden="true" />
                <AppIcon name="brain" size="ic-xs" />
                <span>正在思考</span>
                <span class="think-hint">{{ chat.thinking.length }} 字</span>
                <AppIcon class="chev" :name="liveThinkOpen ? 'chevron-down' : 'chevron-right'" size="ic-xs" />
              </button>
              <div v-if="liveThinkOpen" id="live-think-body" class="wb-think-body">{{ liveThink }}</div>
            </div>

            <div v-if="runBlocks.length" class="turn-tools">
              <ToolRunRow
                v-for="b in runBlocks"
                :key="b.key"
                :label="b.run.label"
                :tool="b.run.tool"
                :args="b.run.args"
                :running="b.run.running"
                :ok="b.run.ok"
                :title="b.run.title"
                :output="b.run.output"
                :duration_ms="b.run.duration_ms"
              />
            </div>

            <!-- 正文：光标常驻，让「还在写」这件事一眼可见 -->
            <div v-if="chat.streaming" class="bubble is-streaming" v-html="renderedStream" />
            <div v-else-if="!chat.runs.length && !chat.thinking" class="bubble is-typing">
              <span class="dot" /><span class="dot" /><span class="dot" />
              <span class="typing-txt">正在组织回答</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 引擎在问用户：问题来自 PI 进程，不挂在某一条消息上，统一排在流末尾 -->
      <div v-if="chat.asks.length" class="turn turn-appr">
        <AskCard v-for="a in chat.asks" :key="a.id" :ask="a" />
      </div>

      <!-- 重试提示条：重试期间用户唯一的主动选择是「别试了」，所以按钮必须在这一条上 -->
      <div v-if="chat.notice" class="wb-hint" :class="{ 'tail-act': chat.retrying }">
        <AppIcon name="info" />
        <span class="grow">{{ chat.notice }}</span>
        <button
          v-if="chat.retrying"
          class="btn btn-sm"
          type="button"
          :class="{ 'is-loading': cancellingRetry }"
          :disabled="cancellingRetry"
          @click="stopRetry"
        >
          停止重试
        </button>
      </div>
      <!-- 被截断：正文只写了一半，工具调用作废。给一键续写，别让用户自己猜要怎么办 -->
      <div v-if="chat.truncated" class="alert a-warn tail-act">
        <AppIcon name="alert" />
        <div class="grow">
          <div class="a-t">回答被截断了</div>
          <div>{{ truncText }}</div>
          <div class="a-sub">输出预算已按上下文窗口给足（窗口的 1/8）。仍被截断说明撞到了模型自己的硬上限：换一个支持更长输出的模型，或点「继续」接着写。</div>
        </div>
        <button
          class="btn btn-sm"
          type="button"
          :class="{ 'is-loading': acting === 'continue' }"
          :disabled="chat.running || !!acting"
          @click="act('continue')"
        >
          继续
        </button>
      </div>
      <div v-if="chat.lastError" class="alert a-danger tail-act">
        <AppIcon name="alert" />
        <div class="grow">
          <div class="a-t">出错了</div>
          <div>{{ chat.lastError }}</div>
        </div>
        <button v-if="needSetup" class="btn btn-sm" type="button" @click="router.push({ name: 'settings', params: { tab: 'providers' } })">
          去设置
        </button>
        <button
          class="btn btn-sm"
          type="button"
          :class="{ 'is-loading': acting === 'retry' }"
          :disabled="chat.running || !!acting"
          @click="act('retry')"
        >
          重试
        </button>
      </div>
    </div>
    <!-- 回到底部：sticky 钉在滚动口下缘，只有离开底部才出现 -->
    <div v-if="!stick" class="jump-row">
      <button class="jump-btn" type="button" title="回到底部" aria-label="回到底部" @click="jumpToLatest">
        <AppIcon name="chevron-down" size="ic" />
      </button>
    </div>
  </div>
</template>

<style scoped>
/* 新回合入场：自下方轻浮入。历史消息首屏渲染带同一动画——打开会话时整屏一次性
   淡入（一档动效内完成），比「啪」地出现柔和；新增的回合则得到明确的「来了」反馈。
   流式期间的重渲染不重建元素，动画不会重放。 */
.msgs-inner > * {
  animation: turn-in var(--wb-dur) var(--wb-ease);
}
@keyframes turn-in {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
}
.avatar.is-live {
  box-shadow: 0 0 0 3px var(--wb-live-soft);
}
/* 回到底部悬浮钮：零高不占版面；单点目标档 1.02 / 0.97 */
.jump-row {
  position: sticky;
  bottom: var(--wb-sp-3);
  height: 0;
  display: flex;
  justify-content: center;
  pointer-events: none;
}
.jump-btn {
  pointer-events: auto;
  width: var(--wb-ctl-icon);
  height: var(--wb-ctl-icon);
  display: grid;
  place-items: center;
  border-radius: var(--wb-radius-full);
  background: var(--wb-surface-solid);
  border: 1px solid var(--wb-border-strong);
  box-shadow: var(--wb-shadow-pop);
  color: var(--wb-ink-2);
  cursor: pointer;
  transition: all var(--wb-dur-fast) var(--wb-ease);
}
.jump-btn:hover {
  transform: scale(1.02);
  color: var(--wb-primary);
}
.jump-btn:active {
  transform: scale(0.97);
}
/* 思考块：流式期间带一条亮靛竖线（活动色），与正文气泡明确分开。
   展开时占满助手列，字数提示才能顶到行尾；收起时按内容收窄。 */
.think.is-live {
  align-self: stretch;
  align-items: stretch;
  border-left: 2px solid var(--wb-live-line);
  padding-left: var(--wb-sp-3);
  margin-bottom: var(--wb-sp-2);
}
.think-hd {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: 3px 6px;
  margin-left: -6px;
  border: 0;
  border-radius: var(--wb-radius-sm);
  background: transparent;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  cursor: pointer;
  text-align: left;
}
.think-hd:hover {
  background: var(--wb-tint);
  color: var(--wb-ink-2);
}
.think-hd:active {
  transform: scale(0.97);
}
.think-hd .chev {
  margin-left: auto;
}
/* 呼吸点：正文开始前唯一需要盯着的信号，比图标更早被余光捕捉 */
.think-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--wb-live);
  animation: think-pulse 1.2s var(--wb-ease) infinite;
}
@keyframes think-pulse {
  50% {
    opacity: 0.25;
    transform: scale(0.75);
  }
}
.wb-think-body {
  animation: think-fade var(--wb-dur) var(--wb-ease);
}
@keyframes think-fade {
  from {
    opacity: 0;
  }
}
.think-hint {
  font-variant-numeric: tabular-nums;
  opacity: 0.7;
}
/* 流式正文末尾的光标：持续输出时最重要的一个视觉信号（极光青 = 正在发生） */
.bubble.is-streaming::after {
  content: '';
  display: inline-block;
  width: 2px;
  height: 1em;
  margin-left: 2px;
  vertical-align: -0.15em;
  background: var(--wb-live);
  animation: caret 1s step-end infinite;
}
@keyframes caret {
  50% {
    opacity: 0;
  }
}
.is-typing {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  padding: 10px 14px;
}
.typing-txt {
  margin-left: var(--wb-sp-2);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--wb-live);
  animation: blink 1.1s ease-in-out infinite;
}
.dot:nth-child(2) { animation-delay: 0.18s; }
.dot:nth-child(3) { animation-delay: 0.36s; }
@keyframes blink {
  50% { opacity: 0.2; transform: translateY(-2px); }
}
/* 收尾提示条：说明占中间，动作贴右；说明短的时候按钮也不会被拉到中缝 */
.tail-act {
  align-items: center;
}
.tail-act .grow {
  flex: 1 1 auto;
  min-width: 0;
}
.tail-act .btn {
  flex: none;
  margin-left: auto;
}
/* 重试条带按钮时靠左排：居中的一句话旁边 4 个字的按钮会被挤成两行 */
.wb-hint.tail-act {
  justify-content: flex-start;
  text-align: left;
}
</style>
