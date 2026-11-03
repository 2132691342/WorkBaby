<script setup lang="ts">
// 一个引擎的健康卡：探测结果、四个落盘计数、进程事实、缺引擎时的下一步。
// 纪律：探测中不说「未安装」；计数没读到不显示 0（零是反的断言）；
// 「没装」要说清哪一种（没装 / 路径错 / 超时），下一步完全不同。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { useAppStore } from '@/stores/app'
import type { EngineHealth } from '@/types/api'

const props = defineProps<{
  engine: EngineHealth
  /** 探测还在跑：此时「没装」这个结论尚未成立 */
  detecting: boolean
}>()

const emit = defineEmits<{ install: [] }>()

const { t } = useI18n()
const app = useAppStore()

const statusLabel = computed(() => {
  if (props.engine.running) return t('overview.running')
  if (props.engine.installed) return t('overview.idle')
  if (props.detecting) return t('overview.detecting')
  return t('overview.notInstalled')
})

const statusTone = computed(() => {
  if (props.engine.running) return 'ok'
  if (props.engine.installed) return ''
  if (props.detecting) return ''
  return 'warn'
})

/** 版本号那一行：装了就显示版本，没装且还在探就显示「检测中」。 */
const versionLabel = computed(() => {
  if (props.engine.version) return props.engine.version
  if (props.detecting) return t('overview.detecting')
  return t('overview.notInstalled')
})

/** 「没装」的三种原因各说一句话。空串表示默认的那句「还没装它」。 */
const reasonLabel = computed(() => {
  switch (props.engine.probe_unknown_reason) {
    case 'bad-path':
      return t('overview.probeBadPath')
    case 'timeout':
      return t('overview.probeTimeout')
    default:
      return ''
  }
})

/** 一个计数怎么显示：没读到的那一格显示「—」。 */
function countLabel(n: number, known: boolean): string {
  return known ? String(n) : '—'
}

function uptime(): string {
  if (!props.engine.started_at) return ''
  const min = Math.floor((Date.now() - props.engine.started_at) / 60000)
  if (min < 1) return t('overview.justNow')
  if (min < 60) return `${min}${t('overview.minutes')}`
  const hours = Math.floor(min / 60)
  if (hours < 24) return `${hours}${t('overview.hours')}`
  return `${Math.floor(hours / 24)}${t('overview.days')}`
}
</script>

<template>
  <article
    class="wb-card card-pad engine-card"
    :class="{ 'is-missing': !props.engine.installed && !props.detecting }"
  >
    <div class="engine-head">
      <span class="wb-tile wb-tile--sm" :class="props.engine.installed ? '' : 'wb-tile--neutral'">
        <AppIcon name="box" :size="16" />
      </span>
      <div class="engine-id">
        <span class="engine-name">{{ props.engine.engine_label }}</span>
        <span class="engine-ver">{{ versionLabel }}</span>
      </div>
      <span class="tag" :class="statusTone">
        <span
          class="wb-led"
          :class="props.engine.running ? 'wb-led--ok' : props.engine.installed || props.detecting ? '' : 'wb-led--warn'"
        />
        {{ statusLabel }}
      </span>
    </div>

    <!-- 还在探测：不能显示「未安装」并给出安装命令 -->
    <div v-if="props.detecting && !props.engine.installed" class="engine-missing">
      <p class="engine-hint">
        <span class="dot busy" />
        {{ t('overview.detecting') }}
      </p>
    </div>

    <div v-else-if="!props.engine.installed" class="engine-missing">
      <p class="engine-hint">{{ reasonLabel || t('overview.installHint') }}</p>
      <code v-if="props.engine.install_hint" class="engine-cmd selectable">
        {{ props.engine.install_hint }}
      </code>
      <div class="engine-actions">
        <button class="wb-btn sm primary" @click="emit('install')">
          {{ t('overview.goInstall') }}
        </button>
        <button
          v-if="props.engine.probe_unknown_reason === 'bad-path'"
          class="wb-btn sm"
          @click="app.openSettings()"
        >
          {{ t('overview.fixEnginePath') }}
        </button>
      </div>
    </div>

    <template v-else>
      <div class="engine-stats">
        <div class="stat">
          <span class="stat-num">{{ countLabel(props.engine.session_count, props.engine.session_count_known) }}</span>
          <span class="stat-label">{{ t('nav.sessions') }}</span>
        </div>
        <div class="stat">
          <span class="stat-num">{{ countLabel(props.engine.skill_count, props.engine.skill_count_known) }}</span>
          <span class="stat-label">{{ t('nav.skills') }}</span>
        </div>
        <div class="stat">
          <span class="stat-num">{{ countLabel(props.engine.plugin_count, props.engine.plugin_count_known) }}</span>
          <span class="stat-label">{{ t('nav.plugins') }}</span>
        </div>
        <div class="stat">
          <span class="stat-num">{{ countLabel(props.engine.mcp_count, props.engine.mcp_count_known) }}</span>
          <span class="stat-label">{{ t('nav.mcp') }}</span>
        </div>
      </div>

      <dl class="engine-meta">
        <div class="wb-kv">
          <dt>{{ t('overview.channel') }}</dt>
          <dd>{{ props.engine.channel_label }}</dd>
        </div>
        <div class="wb-kv">
          <dt>{{ t('overview.process') }}</dt>
          <dd>
            <template v-if="props.engine.running">
              pid {{ props.engine.pid
              }}<template v-if="props.engine.protocol"> · v{{ props.engine.protocol }}</template>
              <template v-if="uptime()"> · {{ uptime() }}</template>
            </template>
            <template v-else>{{ t('overview.idle') }}</template>
          </dd>
        </div>
        <div class="wb-kv">
          <dt>{{ t('overview.agentDir') }}</dt>
          <dd class="wb-mono dir">{{ props.engine.agent_dir }}</dd>
        </div>
      </dl>

      <p v-if="props.engine.last_error" class="engine-error">{{ props.engine.last_error }}</p>
    </template>
  </article>
</template>

<style scoped>
.engine-card {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}

.engine-card.is-missing {
  border-style: dashed;
}

.engine-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}

.engine-id {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.engine-name {
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-base);
  font-weight: 650;
  color: var(--wb-ink);
}

.engine-ver {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
}

.engine-head .tag {
  margin-left: auto;
}

.engine-stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--wb-sp-2);
}

.stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--wb-sp-2);
  border-radius: var(--wb-r-sm);
  background: var(--wb-hover);
}

.stat-num {
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-lg);
  font-weight: 650;
  color: var(--wb-ink);
  font-variant-numeric: tabular-nums;
}

.stat-label {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
}

.engine-meta {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  margin: 0;
}

.dir {
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.engine-missing {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--wb-sp-2);
}

.engine-actions {
  display: flex;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
}

.engine-hint {
  margin: 0;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
}

.engine-cmd {
  width: 100%;
  padding: var(--wb-sp-2);
  border-radius: var(--wb-r-sm);
  background: var(--wb-code-bg);
  border: 1px solid var(--wb-line);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-ink-5);
  overflow-wrap: anywhere;
}

.engine-error {
  margin: 0;
  padding: var(--wb-sp-2);
  border-radius: var(--wb-r-sm);
  background: var(--wb-bad-soft);
  color: var(--wb-bad);
  font-size: var(--wb-fs-2xs);
  overflow-wrap: anywhere;
}
</style>
