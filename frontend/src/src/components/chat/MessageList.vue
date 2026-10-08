<script setup lang="ts">
// 消息流：历史消息 + 正在进行的回合（思考 / 工具 / 流式正文）。
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useChatStore, type ToolRun } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import type { ApprovalVO, MessageVO } from '../../types/api'
import { renderMarkdown, renderMarkdownStream } from '../../utils/md'
import { fmtInt } from '../../utils/num'
import { groupTurns } from '../../utils/turns'
import AppIcon from '../common/AppIcon.vue'
import ApprovalCard from './ApprovalCard.vue'
import MessageItem from './MessageItem.vue'
import ToolRunRow from './ToolRunRow.vue'

type RunBlock =
  | { key: string; kind: 'run'; run: ToolRun }
  | { key: string; kind: 'approval'; approval: ApprovalVO }

// 收尾提示条上的动作：截断续写 / 出错重试都由视图实现，这里只上报意图。
const emit = defineEmits<{ (e: 'continue'): void; (e: 'retry'): void }>()

const session = useSessionStore()
const chat = useChatStore()
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

// 流式正文渲染节流：每个 delta 都全量重解析 markdown 会把主线程打满，
// EventSource 的消息因此积压、被服务端判为慢消费者断连（现象是「卡住」）。
// 数据照单全收，只把渲染放慢到 120ms 一拍；期间跳过高亮，收尾再补完整渲染。
const renderedStream = ref('')
let renderTimer = 0
watch(
  () => chat.streaming,
  () => {
    if (renderTimer) return
    renderTimer = window.setTimeout(() => {
      renderTimer = 0
      renderedStream.value = renderMarkdownStream(chat.streaming)
    }, 120)
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
  const el = scroller.value
  if (!el) return
  stick.value = el.scrollHeight - el.scrollTop - el.clientHeight < 80
}

function callIds(m: MessageVO): string[] {
  if (m.role === 'tool') return m.tool_call_id ? [m.tool_call_id] : []
  return (m.tool_calls || []).map((t) => t.id)
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

// 一次「提问 → 回答」是一整块：用户发一句话，助手可能思考、连着跑几个工具、
// 分几段把话说回来。分组逻辑在 utils/turns.ts（纯函数，可独立测试），
// 审批由回调挂到触发它的回合上。
type TurnBlock = import('../../utils/turns').TurnBlock<ApprovalVO[]>

// 审批要认领它的 tool_call：声明在这条 assistant 上，结果在紧随的 tool 上，任一边命中即可。
function approvalsFor(msg: MessageVO, tools: MessageVO[]): ApprovalVO[] {
  const ids = new Set(callIds(msg))
  for (const t of tools) if (t.tool_call_id) ids.add(t.tool_call_id)
  return chat.approvals.filter((a) => a.tool_call_id && ids.has(a.tool_call_id))
}

const turnBlocks = computed<TurnBlock[]>(() =>
  groupTurns(session.messages, (msg, tools) => approvalsFor(msg, tools)),
)

// 已经收进某一回合的审批：剩下的才算孤儿，避免同一张卡在两处都渲染。
const placedApprovalIds = computed(() => {
  const out = new Set<string>()
  for (const b of turnBlocks.value) {
    if (b.kind !== 'assistant') continue
    for (const p of b.parts) for (const a of p.approvals ?? []) out.add(a.id)
  }
  return out
})

// 运行中的工具卡留在助手列里（缩进对齐），审批卡跟在其后
const runBlocks = computed<RunBlock[]>(() => {
  const out: RunBlock[] = []
  for (const r of chat.runs) {
    out.push({ key: `r-${r.tool_call_id}`, kind: 'run', run: r })
    const hit = chat.approvals.find((a) => a.tool_call_id === r.tool_call_id)
    if (hit) out.push({ key: `ra-${hit.id}`, kind: 'approval', approval: hit })
  }
  return out
})

// 对不上触发点的审批（新 run 刚到、消息还没落库）兜底排在流末尾
const orphanApprovals = computed(() => {
  const placed = new Set<string>(placedApprovalIds.value)
  for (const b of runBlocks.value) if (b.kind === 'approval') placed.add(b.approval.id)
  return chat.approvals.filter((a) => !placed.has(a.id))
})

watch(
  // approvals 也要盯：审批卡是等在正文下面的，不滚过去就被输入框盖住，
  // 用户只看到「助手停了」，看不到该点什么。
  () => [session.messages.length, chat.streaming, chat.thinking, chat.runs.length, chat.approvals.length] as const,
  async () => {
    if (!stick.value) return
    await nextTick()
    const el = scroller.value
    if (el) el.scrollTop = el.scrollHeight
  },
  { deep: true },
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
                <div v-if="p.tools.length || (p.approvals?.length ?? 0)" class="turn-tools">
                  <ToolRunRow
                    v-for="t in p.tools"
                    :key="t.id"
                    :label="t.tool_name || '工具'"
                    :args="toolArgsMap.get(t.tool_call_id || '')"
                    :running="false"
                    :ok="!t.is_error"
                    :output="t.content || ''"
                    :duration_ms="t.latency_ms"
                  />
                  <ApprovalCard
                    v-for="a in p.approvals"
                    :key="a.id"
                    :approval="a"
                    @decide="(id, ok, scope) => chat.decide(id, ok, scope)"
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
              <button class="think-hd" type="button" @click="liveThinkOpen = !liveThinkOpen">
                <span class="think-dot" aria-hidden="true" />
                <AppIcon name="brain" size="ic-xs" />
                <span>正在思考</span>
                <span class="think-hint">{{ chat.thinking.length }} 字</span>
                <AppIcon class="chev" :name="liveThinkOpen ? 'chevron-down' : 'chevron-right'" size="ic-xs" />
              </button>
              <div v-if="liveThinkOpen" class="wb-think-body">{{ liveThink }}</div>
            </div>

            <div v-if="runBlocks.length" class="turn-tools">
              <template v-for="b in runBlocks" :key="b.key">
                <ToolRunRow
                  v-if="b.kind === 'run'"
                  :label="b.run.label"
                  :args="b.run.args"
                  :running="b.run.running"
                  :ok="b.run.ok"
                  :title="b.run.title"
                  :output="b.run.output"
                  :duration_ms="b.run.duration_ms"
                />
                <ApprovalCard v-else :approval="b.approval" @decide="(id, ok, scope) => chat.decide(id, ok, scope)" />
              </template>
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

      <!-- 对不上触发点的审批（新 run 刚到、消息还没落库）：仍然走竖轨和对齐，
           贴到容器左边缘只会让它看着不属于任何一回合。 -->
      <div v-if="orphanApprovals.length" class="turn turn-appr">
        <ApprovalCard
          v-for="a in orphanApprovals"
          :key="a.id"
          :approval="a"
          @decide="(id, ok, scope) => chat.decide(id, ok, scope)"
        />
      </div>

      <div v-if="chat.notice" class="wb-hint">
        <AppIcon name="info" />
        <span>{{ chat.notice }}</span>
      </div>
      <!-- 被截断：正文只写了一半，工具调用作废。给一键续写，别让用户自己猜要怎么办 -->
      <div v-if="chat.truncated" class="alert a-warn tail-act">
        <AppIcon name="alert" />
        <div class="grow">
          <div class="a-t">回答被截断了</div>
          <div>{{ truncText }}</div>
          <div class="a-sub">想让它一次写更长，去「设置 → 模型」把这个模型的输出上限调大。</div>
        </div>
        <button class="btn btn-sm" type="button" :disabled="chat.running" @click="emit('continue')">
          继续
        </button>
      </div>
      <div v-if="chat.lastError" class="alert a-danger tail-act">
        <AppIcon name="alert" />
        <div class="grow">
          <div class="a-t">出错了</div>
          <div>{{ chat.lastError }}</div>
        </div>
        <button class="btn btn-sm" type="button" :disabled="chat.running" @click="emit('retry')">
          重试
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.avatar.is-live {
  box-shadow: 0 0 0 3px var(--wb-live-soft);
}
/* 思考块：流式期间带一条亮紫竖线（活动色），与正文气泡明确分开。
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
</style>
