<script setup lang="ts">
// 消息流：历史消息 + 正在进行的回合（思考 / 工具 / 流式正文）。
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useChatStore, type ToolRun } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import type { ApprovalVO, MessageVO } from '../../types/api'
import { renderMarkdown } from '../../utils/md'
import AppIcon from '../common/AppIcon.vue'
import ApprovalCard from './ApprovalCard.vue'
import MessageItem from './MessageItem.vue'
import ToolRunRow from './ToolRunRow.vue'

type RunBlock =
  | { key: string; kind: 'run'; run: ToolRun }
  | { key: string; kind: 'approval'; approval: ApprovalVO }

const session = useSessionStore()
const chat = useChatStore()
const scroller = ref<HTMLElement | null>(null)
const stick = ref(true)

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
// 分几段把话说回来。数据里那是好几条 assistant 消息，但界面上必须是一个头像、
// 一根不断的竖轨——每条 assistant 各配一个头像的话，一次回答就被切成几段流水账，
// 读者根本看不出「这是同一个动作的几个步骤」。
type AssistantPart = { msg: MessageVO; tools: MessageVO[]; approvals: ApprovalVO[] }

type TurnBlock =
  | { key: string; kind: 'user'; msg: MessageVO }
  | { key: string; kind: 'assistant'; parts: AssistantPart[] }

// 审批要认领它的 tool_call：声明在这条 assistant 上，结果在紧随的 tool 上，任一边命中即可。
function approvalsFor(msg: MessageVO, tools: MessageVO[]): ApprovalVO[] {
  const ids = new Set(callIds(msg))
  for (const t of tools) if (t.tool_call_id) ids.add(t.tool_call_id)
  return chat.approvals.filter((a) => a.tool_call_id && ids.has(a.tool_call_id))
}

const turnBlocks = computed<TurnBlock[]>(() => {
  const out: TurnBlock[] = []
  const msgs = session.messages
  for (let i = 0; i < msgs.length; i++) {
    const m = msgs[i]
    if (m.role !== 'user') {
      continue // 回合开头的 user 之后才有助手内容；游离消息在下面被收进那一块
    }
    out.push({ key: `t-${m.id}`, kind: 'user', msg: m })

    // 收下这条提问之后的全部助手内容，直到下一条 user 为止
    const parts: AssistantPart[] = []
    let end = i
    for (let j = i + 1; j < msgs.length && msgs[j].role !== 'user'; j++) {
      const cur = msgs[j]
      if (cur.role !== 'assistant') {
        continue // 工具结果由声明它的那条 assistant 收纳
      }
      const tools: MessageVO[] = []
      let k = j + 1
      while (k < msgs.length && msgs[k].role === 'tool') {
        tools.push(msgs[k])
        k++
      }
      parts.push({ msg: cur, tools, approvals: approvalsFor(cur, tools) })
      j = k - 1
      end = k
    }
    if (parts.length) {
      out.push({ key: `t-${m.id}-a`, kind: 'assistant', parts })
    }
    i = end
  }
  return out
})

// 已经收进某一回合的审批：剩下的才算孤儿，避免同一张卡在两处都渲染。
const placedApprovalIds = computed(() => {
  const out = new Set<string>()
  for (const b of turnBlocks.value) {
    if (b.kind !== 'assistant') continue
    for (const p of b.parts) for (const a of p.approvals) out.add(a.id)
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
                <div v-if="p.tools.length || p.approvals.length" class="turn-tools">
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
            <!-- 思考：流式期间默认展开，跑完就折起来 -->
            <div v-if="chat.thinking" class="think is-live">
              <div class="think-hd">
                <AppIcon name="brain" size="ic-xs" />
                <span>正在思考</span>
                <span class="think-hint">{{ chat.thinking.length }} 字</span>
              </div>
              <div class="wb-think-body">{{ chat.thinking }}</div>
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
            <div v-if="chat.streaming" class="bubble is-streaming" v-html="renderMarkdown(chat.streaming)" />
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
      <div v-if="chat.lastError" class="alert a-danger">
        <AppIcon name="alert" />
        <div>
          <div class="a-t">出错了</div>
          <div>{{ chat.lastError }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.avatar.is-live {
  box-shadow: 0 0 0 3px var(--wb-primary-soft);
}
/* 思考块：流式期间带一条左侧竖线，与正文气泡明确分开。
   展开时占满助手列，字数提示才能顶到行尾；收起时按内容收窄。 */
.think.is-live {
  align-self: stretch;
  align-items: stretch;
  border-left: 2px solid var(--wb-primary-line);
  padding-left: var(--wb-sp-3);
  margin-bottom: var(--wb-sp-2);
}
.think-hd {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  margin-bottom: var(--wb-sp-1);
}
.think-hint {
  margin-left: auto;
  font-variant-numeric: tabular-nums;
  opacity: 0.7;
}
/* 流式正文末尾的光标：持续输出时最重要的一个视觉信号 */
.bubble.is-streaming::after {
  content: '';
  display: inline-block;
  width: 2px;
  height: 1em;
  margin-left: 2px;
  vertical-align: -0.15em;
  background: var(--wb-primary);
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
  background: var(--wb-muted);
  animation: blink 1.1s ease-in-out infinite;
}
.dot:nth-child(2) { animation-delay: 0.18s; }
.dot:nth-child(3) { animation-delay: 0.36s; }
@keyframes blink {
  50% { opacity: 0.2; transform: translateY(-2px); }
}
</style>
