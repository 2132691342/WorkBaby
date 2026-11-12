<script setup lang="ts">
// 引擎运行状态：当前在不在、忙不忙。
// 引擎是进程内的原生实现，没有 PID 与启动时刻——
// 用户能感受到的是「它此刻在不在干活」，界面上一句解释都没有，所以这里给个读数。
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
          <span v-if="props.instance?.busy">{{ t('overview.busy') }}</span>
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

    <!-- 最近一次失败：它是「这个引擎刚才为什么不听话」的唯一线索 -->
    <p v-if="props.instance?.engine_label" class="proc-error">
      <span class="mono">{{ props.instance.engine_label }}</span>
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
