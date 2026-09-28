<script setup lang="ts">
// 单条消息：用户气泡 / 助手回答（markdown + 思考折叠 + 工具卡）。
import { computed, ref } from 'vue'
import type { MessageVO } from '../../types/api'
import { renderMarkdown } from '../../utils/md'
import AppIcon from '../common/AppIcon.vue'
import ToolRunRow from './ToolRunRow.vue'

const props = defineProps<{ msg: MessageVO }>()

const html = computed(() => (props.msg.content ? renderMarkdown(props.msg.content) : ''))
const thinkOpen = ref(false)
const copied = ref(false)

async function copy() {
  if (!props.msg.content) return
  await navigator.clipboard.writeText(props.msg.content)
  copied.value = true
  setTimeout(() => (copied.value = false), 1200)
}

function fmtTime(ms: number) {
  const d = new Date(ms)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}
</script>

<template>
  <div class="turn">
    <!-- 用户消息 -->
    <div v-if="msg.role === 'user'" class="msg-u-wrap">
      <div class="msg-u">
        {{ msg.content }}
      </div>
      <div class="utk">{{ fmtTime(msg.created_at) }}</div>
    </div>

    <!-- 工具结果：收进工具卡 -->
    <div v-else-if="msg.role === 'tool'" class="msg-a">
      <ToolRunRow
        :label="msg.tool_name || '工具'"
        :running="false"
        :ok="!msg.is_error"
        :output="msg.content || ''"
        :duration_ms="msg.latency_ms"
      />
    </div>

    <!-- 助手回答 -->
    <div v-else class="msg-a">
      <div class="avatar"><span>WB</span></div>
      <div class="msg-a-body">
        <div v-if="msg.thinking" class="think">
          <button class="wb-think-toggle" type="button" @click="thinkOpen = !thinkOpen">
            <AppIcon :name="thinkOpen ? 'chevron-down' : 'chevron-right'" size="ic-xs" /> 想了想
          </button>
          <div v-if="thinkOpen" class="wb-think-body">{{ msg.thinking }}</div>
        </div>
        <div class="bubble" :class="{ 'is-error': msg.is_error }" v-html="html" />
        <div class="msg-acts">
          <button class="act" type="button" @click="copy">
            <AppIcon name="copy" size="ic-xs" />
            {{ copied ? '已复制' : '复制' }}
          </button>
          <span v-if="msg.usage" class="sep" />
          <span v-if="msg.usage" class="act is-dim">{{ msg.usage.total }} tokens</span>
          <span v-if="msg.latency_ms" class="sep" />
          <span v-if="msg.latency_ms" class="act is-dim">{{ (msg.latency_ms / 1000).toFixed(1) }}s</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.act.is-dim {
  color: var(--wb-muted);
  cursor: default;
}
.bubble.is-error {
  border-color: var(--wb-danger);
  color: var(--wb-danger);
}
</style>
