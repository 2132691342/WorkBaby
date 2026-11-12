<script setup lang="ts">
// 一个运行时的健康卡：探测结果、四个落盘计数、进程事实、缺引擎时的下一步。
// 纪律：探测中不说「未安装」；计数是真扫出来的（不是零值占位）。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { useAppStore } from '@/stores/app'
import type { EngineHealth } from '@/types/api'

const props = defineProps<{
  engine: EngineHealth
  /** 后台装配还在跑：此时「没装」这个结论尚未成立 */
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

/** 版本号那一行：装了就显示版本，没装且还在装配就显示「准备中」。 */
const versionLabel = computed(() => {
  if (props.engine.engine_version) return props.engine.engine_version
  if (props.detecting) return t('overview.detecting')
  return t('overview.notInstalled')
})

/** 引擎家目录：布局解出来才有值。 */
const agentDir = computed(() => app.engines?.layouts[0]?.agent_dir ?? '')

function uptime(): string {
  // 引擎在进程内，没有启动时刻；忙的那一轮只由会话自己计时，这里不再读进程时长。
  return props.engine.busy ? t('overview.running') : ''
}
</script>

<template>
  <article class="wb-card card-pad engine-card" :class="{ 'is-missing': !props.engine.installed && !props.detecting }">
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

    <!-- 还在装：不能显示「未安装」并给安装入口 -->
    <div v-if="props.detecting && !props.engine.installed" class="engine-missing">
      <p class="engine-hint">
        <span class="dot busy" />
        {{ t('overview.detecting') }}
      </p>
    </div>

    <div v-else-if="!props.engine.installed" class="engine-missing">
      <p class="engine-hint">{{ t('overview.installHint') }}</p>
      <div class="engine-actions">
        <button class="wb-btn sm primary" @click="emit('install')">{{ t('overview.goInstall') }}</button>
        <button class="wb-btn sm" @click="app.openSettings()">{{ t('nav.settings') }}</button>
      </div>
    </div>

    <template v-else>
      <div class="engine-stats">
        <div class="stat">
          <span class="stat-num">{{ props.engine.session_count }}</span>
          <span class="stat-label">{{ t('nav.sessions') }}</span>
        </div>
        <div class="stat">
          <span class="stat-num">{{ props.engine.skill_count }}</span>
          <span class="stat-label">{{ t('nav.skills') }}</span>
        </div>
        <div class="stat">
          <span class="stat-num">{{ props.engine.mcp_count }}</span>
          <span class="stat-label">{{ t('nav.mcp') }}</span>
        </div>
      </div>

      <dl class="engine-meta">
        <div class="wb-kv">
          <dt>{{ t('overview.process') }}</dt>
          <dd>
            <template v-if="props.engine.busy">
              {{ t('overview.running') }}
              <template v-if="uptime()"> · {{ uptime() }}</template>
            </template>
            <template v-else>{{ t('overview.idle') }}</template>
          </dd>
        </div>
        <div v-if="agentDir" class="wb-kv">
          <dt>{{ t('overview.agentDir') }}</dt>
          <dd class="wb-mono dir">{{ agentDir }}</dd>
        </div>
      </dl>
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
