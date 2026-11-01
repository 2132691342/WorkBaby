<script setup lang="ts">
// 子代理详情：任务、流式过程（思考 / 正文 / 工具卡）、结论与用量。
// 与对话流的区别只有一条：这里的过程不进任何上下文，纯观测。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AgentRow } from '../../stores/agents'
import { renderMarkdown } from '../../utils/md'
import { fmtMs } from '../../utils/num'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{
  /** 要展示的那条子代理；为空时显示空态 */
  agent: AgentRow | null
}>()

const { t } = useI18n()

const running = computed(() => !!props.agent && !props.agent.agent_finished_at)
const tools = computed(() => Object.values(props.agent?.agent_tools ?? {}))
const html = computed(() => (props.agent?.agent_text ? renderMarkdown(props.agent.agent_text) : ''))

/** 角色码 → 显示名：未知角色直接显示原码，不编造。 */
function roleLabel(code: string): string {
  const key = `agents.role.${code}`
  const text = t(key)
  return text === key ? code : text
}

/** 用时：跑着的算到现在，跑完的用后端给的耗时。 */
const elapsed = computed(() => {
  const a = props.agent
  if (!a) return 0
  if (a.agent_finished_at) return a.agent_duration_ms
  return Date.now() - a.agent_started_at
})

/** 工具卡片的一行摘要：挑最像「在做什么」的字段。 */
function summaryOf(args: Record<string, unknown>): string {
  for (const key of ['command', 'path', 'file_path', 'pattern', 'query', 'url']) {
    const v = args[key]
    if (typeof v === 'string' && v.trim()) return v.trim()
  }
  return ''
}
</script>

<template>
  <div v-if="!agent" class="agent-empty">
    <span class="wb-tile wb-tile--lg"><AppIcon name="cpu" size="ic-xl" /></span>
    <b>{{ t('agents.emptyTitle') }}</b>
    <p>{{ t('agents.emptyHint') }}</p>
  </div>

  <div v-else class="agent-detail">
    <header class="ad-head">
      <span class="ad-role">{{ roleLabel(agent.agent_role_code) }}</span>
      <span class="ad-state" :class="running ? 'is-live' : agent.agent_failed ? 'is-bad' : 'is-ok'">
        <span class="ad-dot" />
        {{ running ? t('agents.running') : agent.agent_failed ? t('agents.failed') : t('agents.done') }}
      </span>
      <span class="ad-id">{{ agent.agent_session_id }}</span>
      <span class="ad-sp" />
      <span class="ad-time">{{ fmtMs(elapsed) }}</span>
    </header>

    <div class="ad-task">
      <span class="wb-label">{{ t('agents.task') }}</span>
      <p class="ad-task-tx">{{ agent.agent_task || t('common.empty') }}</p>
    </div>

    <div v-if="agent.agent_thinking" class="ad-sec">
      <span class="wb-label">{{ t('agents.thinking') }}</span>
      <pre class="ad-pre is-think">{{ agent.agent_thinking }}</pre>
    </div>

    <div v-if="tools.length" class="ad-sec">
      <span class="wb-label">{{ t('agents.tools', { n: tools.length }) }}</span>
      <div class="ad-tools">
        <div
          v-for="tool in tools"
          :key="tool.tool_call_id"
          class="ad-tool"
          :class="{ 'is-bad': tool.is_error, 'is-run': tool.running }"
        >
          <span class="ad-tool-dot" />
          <span class="ad-tool-name">{{ tool.tool_name }}</span>
          <span class="ad-tool-sum">{{ summaryOf(tool.tool_args) }}</span>
          <span class="ad-tool-time">{{ tool.duration_ms ? fmtMs(tool.duration_ms) : '' }}</span>
        </div>
      </div>
    </div>

    <div v-if="agent.agent_text" class="ad-sec">
      <span class="wb-label">{{ t('agents.output') }}</span>
      <div class="prose" v-html="html" />
    </div>

    <div v-if="!running" class="ad-sec">
      <span class="wb-label">{{ t('agents.conclusion') }}</span>
      <pre class="ad-pre" :class="{ 'is-bad': agent.agent_failed }">{{
        agent.agent_conclusion || t('agents.noConclusion')
      }}</pre>
    </div>

    <div v-if="running" class="ad-live">
      <span class="ad-live-dot" />
      <span>{{ t('agents.liveHint') }}</span>
    </div>
  </div>
</template>

<style scoped>
.agent-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-12) var(--wb-sp-6);
  text-align: center;
}
.agent-empty b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.agent-empty p {
  max-width: calc(420px * var(--wb-vw-scale));
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
}
.agent-detail {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
  padding: var(--wb-sp-4);
}
.ad-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
}
.ad-role {
  padding: 1px var(--wb-sp-2);
  border-radius: var(--wb-radius-full);
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
}
.ad-state {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-muted);
}
.ad-dot {
  width: 6px;
  height: 6px;
  border-radius: var(--wb-radius-full);
  background: currentColor;
}
.ad-state.is-live {
  color: var(--wb-live);
}
.ad-state.is-live .ad-dot {
  animation: ad-breathe 1.2s var(--wb-ease) infinite;
}
.ad-state.is-ok {
  color: var(--wb-success);
}
.ad-state.is-bad {
  color: var(--wb-danger);
}
@keyframes ad-breathe {
  50% {
    opacity: 0.35;
  }
}
.ad-id {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.ad-sp {
  flex: 1;
}
.ad-time {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.ad-task-tx {
  margin-top: var(--wb-sp-1);
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
  line-height: var(--wb-lh-base);
  white-space: pre-wrap;
}
.ad-sec {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}
.ad-pre {
  margin: 0;
  padding: var(--wb-sp-3);
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius);
  background: var(--wb-elev-hover);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  line-height: var(--wb-lh-base);
  color: var(--wb-ink-2);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: calc(320px * var(--wb-vw-scale));
  overflow-y: auto;
}
.ad-pre.is-think {
  color: var(--wb-muted);
  font-style: italic;
}
.ad-pre.is-bad {
  color: var(--wb-danger);
}
.ad-tools {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}
.ad-tool {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-1) var(--wb-sp-2);
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-sm);
  font-size: var(--wb-fs-xs);
}
.ad-tool.is-run {
  border-color: var(--wb-live);
  background: var(--wb-live-soft);
}
.ad-tool.is-bad {
  border-color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
.ad-tool-dot {
  width: 6px;
  height: 6px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-success);
  flex: none;
}
.ad-tool.is-run .ad-tool-dot {
  background: var(--wb-live);
  animation: ad-breathe 1s var(--wb-ease) infinite;
}
.ad-tool.is-bad .ad-tool-dot {
  background: var(--wb-danger);
}
.ad-tool-name {
  font-family: var(--font-mono);
  font-weight: 700;
  color: var(--wb-ink);
  flex: none;
}
.ad-tool-sum {
  flex: 1;
  min-width: 0;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ad-tool-time {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  flex: none;
}
.ad-live {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.ad-live-dot {
  width: 6px;
  height: 6px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-live);
  animation: ad-breathe 1.2s var(--wb-ease) infinite;
}
</style>
