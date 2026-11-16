<script setup lang="ts">
// 引擎运行状态：当前在不在、忙不忙、上一次出了什么错。
// 引擎是进程内的原生实现，没有 PID 与启动时刻——
// 用户能感受到的是「它此刻在不在干活」，所以这里给一句状态与一条可读的失败原话。
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
const busy = computed(() => Boolean(props.instance?.busy))

/** 最近一次失败：后端 omitempty，空串表示从没失败过——那时整行都不该占位。 */
const lastError = computed(() => (props.instance?.last_error ?? '').trim())

/** 状态词：就绪 / 正在回答 / 未就绪。徽章只说这一件事，不印数字。 */
const stateLabel = computed(() => {
  if (!props.ready) return t('overview.collectingShort')
  if (!running.value) return t('overview.idle')
  return busy.value ? t('overview.busy') : t('overview.running')
})

const stateTone = computed(() => {
  if (!props.ready) return ''
  if (!running.value) return 'warn'
  return busy.value ? 'brand' : 'ok'
})
</script>

<template>
  <section class="wb-card card-pad">
    <div class="wb-section-title">
      {{ t('overview.processes') }}
      <span class="tag" :class="stateTone">{{ stateLabel }}</span>
      <button class="wb-btn text sm push" type="button" :title="t('common.refresh')" @click="emit('refresh')">
        <AppIcon name="refresh" :size="12" />
      </button>
    </div>

    <p class="wb-hint">{{ t('overview.processesHint') }}</p>

    <p v-if="!props.ready" class="proc-empty">
      <span class="dot busy" />
      {{ t('overview.collecting') }}
    </p>

    <p v-else-if="!running" class="proc-empty">{{ t('overview.processesEmpty') }}</p>

    <div v-else class="proc-row">
      <span class="dot" :class="busy ? 'busy' : 'ok'" />
      <span class="proc-kind">{{ props.instance?.engine_label }}</span>

      <!-- 忙的那一个不给重启：重启等于把用户正在看的那一轮打断。
           不渲染成灰按钮，而是换成一句话——一句话只说一次。 -->
      <span v-if="busy" class="proc-busy">{{ t('overview.busy') }}</span>
      <button
        v-else
        class="wb-btn text sm danger"
        type="button"
        :class="{ 'is-loading': props.stopping }"
        :disabled="props.stopping"
        :title="t('overview.restartProcess')"
        @click="emit('restart')"
      >
        {{ t('overview.restartProcess') }}
      </button>
    </div>

    <!-- 最近一次失败：它是「这个引擎刚才为什么不听话」的唯一线索。
         没有失败过就不渲染这一行——空盒子会让人以为在等一条永远不会来的消息。 -->
    <div v-if="lastError" class="proc-error">
      <AppIcon name="alert" :size="13" class="proc-error-ic" />
      <span class="proc-error-label">{{ t('overview.lastError') }}</span>
      <span class="proc-error-text mono selectable">{{ lastError }}</span>
    </div>
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

.proc-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin-top: var(--wb-sp-1);
  padding: var(--wb-sp-2) 0;
  font-size: var(--wb-fs-2xs);
}

.proc-kind {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--wb-ink);
  font-weight: 600;
}

.proc-busy {
  flex: none;
  color: var(--wb-brand);
  font-weight: 600;
}

/* 失败原话：淡危险底色 + 等宽字，允许换行——里面常带路径，
   截断掉就等于把最关键的一半信息藏起来。 */
.proc-error {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-2);
  margin-top: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border: 1px solid var(--wb-bad-soft);
  border-radius: var(--wb-r-sm);
  background: var(--wb-bad-soft);
  font-size: var(--wb-fs-3xs);
}

.proc-error-ic {
  flex: none;
  margin-top: 1px;
  color: var(--wb-bad);
}

.proc-error-label {
  flex: none;
  color: var(--wb-bad);
  font-weight: 600;
}

.proc-error-text {
  flex: 1;
  min-width: 0;
  color: var(--wb-ink-2);
  line-height: 1.6;
  overflow-wrap: anywhere;
}
</style>
