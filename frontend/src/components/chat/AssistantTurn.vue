<script setup lang="ts">
// 一条助手消息的内容段：按后端内容块的原始顺序就地渲染思考 / 正文 / 工具卡。
// 不按字段类型分组——分组会把这一轮所有工具卡顶到正文前面，顺序就丢了。
import { computed } from 'vue'
import type { MessageBlock, MessageVO } from '../../types/api'
import { renderMarkdown } from '../../utils/md'
import ThinkingBlock from './ThinkingBlock.vue'
import ToolCard from './ToolCard.vue'

const props = defineProps<{
  msg: MessageVO
  /** 紧随其后的 tool 消息：按 tool_call_id 对上号 */
  tools: MessageVO[]
}>()

/** 正文里可能混着 <think> 标签（模型直接吐标签）：拆成独立的思考段，正文保持干净 */
function splitInlineThink(list: MessageBlock[]): MessageBlock[] {
  const out: MessageBlock[] = []
  for (const b of list) {
    const text = b.block_text || ''
    if (b.block_kind !== 'text' || !text.includes('<think>')) {
      out.push(b)
      continue
    }
    const re = /<think>([\s\S]*?)<\/think>/g
    let last = 0
    let m = re.exec(text)
    while (m) {
      const before = text.slice(last, m.index).trim()
      if (before) out.push({ block_kind: 'text', block_text: before })
      if (m[1].trim()) out.push({ block_kind: 'thinking', block_text: m[1].trim() })
      last = m.index + m[0].length
      m = re.exec(text)
    }
    const rest = text.slice(last).replace(/<\/?think>/g, '').trim()
    if (rest) out.push({ block_kind: 'text', block_text: rest })
  }
  return out
}

// 老响应（缓存里没有 blocks）按 thinking → 正文 → 工具 的旧口径兜底
const blocks = computed<MessageBlock[]>(() => {
  const m = props.msg
  if (m.message_blocks?.length) return splitInlineThink(m.message_blocks)
  const out: MessageBlock[] = []
  if (m.message_thinking?.trim()) out.push({ block_kind: 'thinking', block_text: m.message_thinking })
  if (m.message_content?.trim()) out.push({ block_kind: 'text', block_text: m.message_content })
  for (const tc of m.message_tool_calls ?? []) {
    out.push({
      block_kind: 'tool',
      tool_call_id: tc.tool_call_id,
      tool_name: tc.tool_name,
      tool_label: tc.tool_label,
      tool_args: tc.tool_args,
    })
  }
  return splitInlineThink(out)
})

function resultOf(id?: string): MessageVO | undefined {
  if (!id) return undefined
  return props.tools.find((t) => t.message_tool_call_id === id)
}

function argsOf(b: MessageBlock): Record<string, unknown> | undefined {
  if (b.tool_args && Object.keys(b.tool_args).length) return b.tool_args
  const hit = (props.msg.message_tool_calls ?? []).find((tc) => tc.tool_call_id === b.tool_call_id)
  return hit?.tool_args
}

function htmlOf(b: MessageBlock): string {
  return b.block_text ? renderMarkdown(b.block_text) : ''
}

// 兵底：结果消息存在、但内容块里找不到它的调用（老数据 / 不完整的投影）时，
// 宁可按顺序补在末尾，也不要把一次真的工具调用藏起来。
const orphans = computed(() => {
  const declared = new Set(blocks.value.filter((b) => b.tool_call_id).map((b) => b.tool_call_id))
  return props.tools.filter((t) => !declared.has(t.message_tool_call_id || ''))
})
</script>

<template>
  <template v-for="(b, i) in blocks" :key="`${msg.message_id}-${i}`">
    <ThinkingBlock v-if="b.block_kind === 'thinking'" :text="b.block_text || ''" />
    <div v-else-if="b.block_kind === 'text'" class="prose" :class="{ 'is-error': msg.message_is_error }" v-html="htmlOf(b)" />
    <ToolCard
      v-else
      class="turn-tool"
      :label="b.tool_label || b.tool_name || 'tool'"
      :tool="b.tool_name || ''"
      :args="argsOf(b)"
      :running="false"
      :ok="!resultOf(b.tool_call_id)?.message_is_error"
      :output="resultOf(b.tool_call_id)?.message_content || ''"
    />
  </template>
  <ToolCard
    v-for="t in orphans"
    :key="`orphan-${t.message_id}`"
    class="turn-tool"
    :label="t.message_tool_name || 'tool'"
    :tool="t.message_tool_name || ''"
    :running="false"
    :ok="!t.message_is_error"
    :output="t.message_content || ''"
  />
</template>

<style scoped>
.prose.is-error {
  color: var(--wb-danger);
}
</style>
