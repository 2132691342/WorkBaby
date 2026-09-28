<script setup lang="ts">
// 消息流：历史消息 + 正在进行的回合（思考 / 工具 / 流式正文）。
import { computed, nextTick, ref, watch } from 'vue'
import { useChatStore, type ToolRun } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import type { ApprovalVO, MessageVO } from '../../types/api'
import { renderMarkdown } from '../../utils/md'
import AppIcon from '../common/AppIcon.vue'
import ApprovalCard from './ApprovalCard.vue'
import MessageItem from './MessageItem.vue'
import ToolRunRow from './ToolRunRow.vue'

type MsgBlock =
  | { key: string; kind: 'msg'; msg: MessageVO }
  | { key: string; kind: 'approval'; approval: ApprovalVO }

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

// 审批卡钉在触发它的那条消息下面，用户不用往上翻找「刚才发生了什么」
const msgBlocks = computed<MsgBlock[]>(() => {
  const out: MsgBlock[] = []
  for (const m of session.messages) {
    out.push({ key: `m-${m.id}`, kind: 'msg', msg: m })
    const ids = callIds(m)
    const hit = chat.approvals.find((a) => ids.includes(a.tool_call_id))
    if (hit) out.push({ key: `a-${hit.id}`, kind: 'approval', approval: hit })
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
  const placed = new Set<string>()
  for (const b of msgBlocks.value) if (b.kind === 'approval') placed.add(b.approval.id)
  for (const b of runBlocks.value) if (b.kind === 'approval') placed.add(b.approval.id)
  return chat.approvals.filter((a) => !placed.has(a.id))
})

watch(
  () => [session.messages.length, chat.streaming, chat.thinking, chat.runs.length] as const,
  async () => {
    if (!stick.value) return
    await nextTick()
    const el = scroller.value
    if (el) el.scrollTop = el.scrollHeight
  },
  { deep: true },
)
</script>

<template>
  <div ref="scroller" class="msgs" @scroll.passive="onScroll">
    <div class="msgs-inner">
      <template v-for="b in msgBlocks" :key="b.key">
        <MessageItem v-if="b.kind === 'msg'" :msg="b.msg" />
        <ApprovalCard v-else :approval="b.approval" @decide="(id, ok, scope) => chat.decide(id, ok, scope)" />
      </template>

      <!-- 正在进行的回合 -->
      <div v-if="chat.running" class="turn">
        <div class="msg-a">
          <div class="avatar is-live"><span>WB</span></div>
          <div class="msg-a-body">
            <div v-if="chat.thinking" class="think">
              <div class="wb-think-body">{{ chat.thinking }}</div>
            </div>
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
            <div v-if="chat.streaming" class="bubble" v-html="renderMarkdown(chat.streaming)" />
            <div v-else-if="!chat.runs.length && !chat.thinking" class="bubble is-typing">
              <span class="dot" /><span class="dot" /><span class="dot" />
            </div>
          </div>
        </div>
      </div>

      <ApprovalCard
        v-for="a in orphanApprovals"
        :key="a.id"
        :approval="a"
        @decide="(id, ok, scope) => chat.decide(id, ok, scope)"
      />

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
.is-typing {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  padding: 10px 14px;
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
