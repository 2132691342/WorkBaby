<script setup lang="ts">
// 引擎进程：活着的每一个实例一行（引擎 + 工作目录是键）。
//
// 为什么需要这一块：切一次目录就新建一个进程，空闲的十分钟后才会被回收。
// 用户能感受到的是「机器越来越卡」，而界面上一句解释都没有。
// 这里有明细：哪个引擎、在哪个目录、PID、跑了多久、忙不忙，以及停掉它。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { EngineRun } from '@/types/api'

const props = defineProps<{
  runs: EngineRun[]
  /** 实例数上限：达到后按最久未用淘汰 */
  maxEngines: number
  /** 正在停的那一行（`kind|work_dir`），空表示没有 */
  stopping: string
  /** 这一屏的数字还没数完：此时不把列表当成结论 */
  ready: boolean
}>()

const emit = defineEmits<{ stop: [run: EngineRun]; refresh: [] }>()

const { t } = useI18n()

/** 一行的键：停止时要用它定位，所以与后端同一个口径。 */
function rowKey(run: EngineRun): string {
  return `${run.engine_kind}|${run.work_dir}`
}

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

/** 只显示目录末段太容易撞名；显示完整路径但可被截断。 */
const hasRuns = computed(() => props.runs.length > 0)
</script>

<template>
  <section class="wb-card card-pad">
    <div class="wb-section-title">
      {{ t('overview.processes') }}
      <span class="tag" :class="hasRuns ? 'ok' : ''">
        {{ props.ready ? props.runs.length : '—' }}
        <template v-if="props.maxEngines">/ {{ props.maxEngines }}</template>
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

    <p v-else-if="!hasRuns" class="proc-empty">{{ t('overview.processesEmpty') }}</p>

    <div v-else class="proc-table">
      <div v-for="run in props.runs" :key="rowKey(run)" class="proc-row">
        <span class="dot" :class="run.busy ? 'busy' : 'ok'" />
        <span class="proc-kind">{{ run.engine_label }}</span>
        <span class="proc-dir mono" :title="run.work_dir">{{ run.work_dir }}</span>
        <span class="proc-meta mono">
          <span v-if="run.process_id">pid {{ run.process_id }}</span>
          <span v-if="run.protocol_version">v{{ run.protocol_version }}</span>
          <span v-if="uptime(run.started_at)">{{ uptime(run.started_at) }}</span>
          <span v-if="run.call_count">{{ t('overview.calls', { n: run.call_count }) }}</span>
        </span>

        <!-- 忙的那一个不给停止：停它等于把用户正在看的那一轮打断。
             不渲染成灰按钮，而是换成一句话。 -->
        <span v-if="run.busy" class="proc-busy">{{ t('overview.busy') }}</span>
        <button
          v-else
          class="wb-btn text sm danger"
          :class="{ 'is-loading': props.stopping === rowKey(run) }"
          :disabled="Boolean(props.stopping)"
          @click="emit('stop', run)"
        >
          {{ t('overview.stopProcess') }}
        </button>
      </div>
    </div>

    <!-- 最近一次命令失败：它是「这个引擎刚才为什么不听话」的唯一线索 -->
    <p v-for="run in props.runs.filter((r) => r.last_error)" :key="`e-${rowKey(run)}`" class="proc-error">
      <span class="mono">{{ run.engine_label }}</span> · {{ run.last_error }}
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

/* 一行一个实例：目录可截断，其余固定宽度——PID 与时长是扫一眼要读的 */
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
  width: 3.2em;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-ink);
}

.proc-dir {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  direction: rtl;
  text-align: left;
}

.proc-meta {
  display: inline-flex;
  gap: var(--wb-sp-2);
  flex: none;
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
