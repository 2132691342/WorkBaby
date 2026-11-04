<script setup lang="ts">
// 引擎进程：当前那一个实例的明细。
//
// 为什么需要这一块：引擎是常驻子进程，用户能感受到的是「机器变卡」，
// 而界面上一句解释都没有。这里有明细：PID、跑了多久、忙不忙、调过几次，
// 以及重启它。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { EngineHealth } from '@/types/api'

const props = defineProps<{
  /** 引擎健康快照；没拿到时为 undefined */
  instance?: EngineHealth
  /** 正在重启 */
  stopping: boolean
  /** 这一屏的数字还没数完：此时不把列表当成结论 */
  ready: boolean
}>()

const emit = defineEmits<{ restart: []; refresh: [] }>()

const { t } = useI18n()

/** 存活时长：分钟以内说「刚刚」，超过一天说天数。 */
function uptime(startedAt: number): string {
  if (!startedAt) return ''
  const min = Math.floor((Date.now() - startedAt) / 60000)
  if (min < 1) return t('overview.justNow')
  if (min < 60) return `${min}${t('overview.minutes')}`
  const hours = Math.floor(min / 60)
  if (hours < 24) return `${hours}${t('overview.hours')}`
  return `${Math.floor(hours / 24)}${t('overview.days')}`
}

const running = computed(() => Boolean(props.instance?.running))
</script>

<template>
  <section class="wb-card card-pad">
    <div class="wb-section-title">
      {{ t('overview.processes') }}
      <span class="tag" :class="running ? 'ok' : ''">
        {{ props.ready ? (running ? 1 : 0) : '—' }}
      </span>
      <button class="wb-btn text sm push" :title="t('common.refresh')" @click="emit('refresh')">
        <AppIcon name="refresh" :size="12" />
      </button>
    </div>

    <p class="wb-hint">{{ t('overview.processesHint') }}</p>

    <p v-if="!props.ready" class="proc-empty">
      <span class="dot busy" />
      {{ t('overview.collecting') }}
    </p>

    <p v-else-if="!running" class="proc-empty">{{ t('overview.processesEmpty') }}</p>

    <div v-else class="proc-table">
      <div class="proc-row">
        <span class="dot" :class="props.instance?.busy ? 'busy' : 'ok'" />
        <span class="proc-kind">{{ props.instance?.engine_label }}</span>
        <span class="proc-meta mono">
          <span v-if="props.instance?.process_id">pid {{ props.instance?.process_id }}</span>
          <span v-if="uptime(props.instance?.started_at ?? 0)">{{ uptime(props.instance?.started_at ?? 0) }}</span>
          <span v-if="props.instance?.call_count">{{ t('overview.calls', { n: props.instance?.call_count }) }}</span>
        </span>

        <!-- 忙的那一个不给重启：重启等于把用户正在看的那一轮打断。
             不渲染成灰按钮，而是换成一句话。 -->
        <span v-if="props.instance?.busy" class="proc-busy">{{ t('overview.busy') }}</span>
        <button
          v-else
          class="wb-btn text sm danger"
          :class="{ 'is-loading': props.stopping }"
          :disabled="props.stopping"
          @click="emit('restart')"
        >
          {{ t('overview.restartProcess') }}
        </button>
      </div>
    </div>

    <!-- 最近一次命令失败：它是「这个引擎刚才为什么不听话」的唯一线索 -->
    <p v-if="props.instance?.last_error" class="proc-error">
      <span class="mono">{{ props.instance?.engine_label }}</span> · {{ props.instance?.last_error }}
    </p>
  </section>
</template>

<style scoped>
.push {
  margin-left: auto;
}

.proc-empty {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin: var(--wb-sp-1) 0 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
}

.proc-table {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: var(--wb-sp-1);
}

.proc-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: 3px var(--wb-sp-2);
  border-radius: var(--wb-r-sm);
  transition: background-color var(--wb-dur-fast) var(--wb-ease);
}

.proc-row:hover {
  background: var(--wb-hover);
}

.proc-kind {
  flex: none;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-ink);
}

.proc-meta {
  display: inline-flex;
  flex: 1;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  font-variant-numeric: tabular-nums;
}

.proc-busy {
  flex: none;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-live);
}

.proc-error {
  margin: var(--wb-sp-1) 0 0;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-bad);
  overflow-wrap: anywhere;
}
</style>
