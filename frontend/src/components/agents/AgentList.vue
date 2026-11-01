<script setup lang="ts">
// 子代理列表：一行一条，跑着的带呼吸灯。选中的那条进右侧详情。
// 列表只回答「有几个、在干什么」，过程细节交给详情面板。
import { useI18n } from 'vue-i18n'
import type { AgentRow } from '../../stores/agents'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{
  /** 待展示的行（已按时间倒序） */
  rows: AgentRow[]
  /** 当前选中 */
  selected: string
  /** 是否还在首次拉取 */
  loading: boolean
}>()

const emit = defineEmits<{ select: [id: string] }>()

const { t } = useI18n()

/** 角色码 → 显示名；未收录的角色原样显示。 */
function roleLabel(code: string): string {
  const key = `agents.role.${code}`
  const text = t(key)
  return text === key ? code : text
}

/** 一行摘要：任务描述压成单行。 */
function brief(task: string): string {
  return task.replace(/\s+/g, ' ').trim()
}

function isLive(row: AgentRow): boolean {
  return !row.agent_finished_at
}

/** 用时读数的量级：跑着的按现在算。 */
function elapsedOf(row: AgentRow): string {
  const ms = row.agent_finished_at ? row.agent_duration_ms : Date.now() - row.agent_started_at
  if (ms < 1000) return `${ms}ms`
  const s = Math.floor(ms / 1000)
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m${s % 60}s`
}

const empty = () => props.rows.length === 0
</script>

<template>
  <div class="agent-list">
    <div v-if="loading && empty()" class="al-skel">
      <div v-for="i in 3" :key="i" class="wb-skeleton al-skel-row" />
    </div>

    <div v-else-if="empty()" class="al-empty">{{ t('agents.listEmpty') }}</div>

    <button
      v-for="row in rows"
      v-else
      :key="row.agent_session_id"
      class="al-row"
      :class="{ 'is-on': row.agent_session_id === selected, 'is-live': isLive(row) }"
      type="button"
      @click="emit('select', row.agent_session_id)"
    >
      <span class="al-led" :class="{ 'is-live': isLive(row), 'is-bad': row.agent_failed }" />
      <span class="al-body">
        <span class="al-top">
          <b class="al-role">{{ roleLabel(row.agent_role_code) }}</b>
          <span class="al-task">{{ brief(row.agent_task) || t('common.empty') }}</span>
        </span>
        <span class="al-meta">
          <span class="al-id">{{ row.agent_session_id }}</span>
          <span class="al-time">{{ elapsedOf(row) }}</span>
        </span>
      </span>
      <AppIcon name="chevron-right" size="ic-xs" class="al-chev" />
    </button>
  </div>
</template>

<style scoped>
.agent-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--wb-sp-2);
  min-height: 0;
  overflow-y: auto;
}
.al-skel {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2);
}
.al-skel-row {
  height: calc(46px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-sm);
}
.al-empty {
  padding: var(--wb-sp-8) var(--wb-sp-4);
  text-align: center;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.al-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  text-align: left;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.al-row:hover {
  background: var(--wb-elev-hover);
  transform: scale(1.01);
}
.al-row:active {
  transform: scale(var(--wb-row-scale-press));
}
.al-row.is-on {
  background: var(--wb-accent-tint);
}
.al-row.is-on .al-task {
  color: var(--wb-primary);
}
.al-led {
  width: 7px;
  height: 7px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-border-strong);
  flex: none;
}
.al-led.is-live {
  background: var(--wb-live);
  animation: al-breathe 1.2s var(--wb-ease) infinite;
}
.al-led.is-bad {
  background: var(--wb-danger);
}
@keyframes al-breathe {
  50% {
    opacity: 0.3;
  }
}
.al-body {
  display: flex;
  flex-direction: column;
  gap: 1px;
  flex: 1;
  min-width: 0;
}
.al-top {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-2);
  min-width: 0;
}
.al-role {
  flex: none;
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-primary);
}
.al-task {
  min-width: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.al-meta {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.al-id,
.al-time {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.al-chev {
  flex: none;
  color: var(--wb-muted);
}
@media (prefers-reduced-motion: reduce) {
  .al-led.is-live {
    animation: none;
  }
  .al-row:hover,
  .al-row:active {
    transform: none;
  }
}
</style>
