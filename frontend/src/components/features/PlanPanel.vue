<script setup lang="ts">
// 计划面板：模型提交的计划 + 步骤勾选 + 用户标注。
// 状态机 draft / approved / rejected / done 决定可做的动作；驳回意见要显眼，
// 因为它是下一轮修订的唯一依据。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import type { PlanVO } from '../../types/api'
import { errorText } from '../../utils/errors'
import { fmtAbs } from '../../utils/time'
import EmptyState from '../common/EmptyState.vue'
import AppIcon from '../common/AppIcon.vue'

const { t } = useI18n()

const plan = ref<PlanVO | null>(null)
const loading = ref(true)
const failed = ref('')

const totalSteps = computed(() => plan.value?.plan_steps.length ?? 0)
const pct = computed(() => {
  if (!totalSteps.value) return 0
  return Math.round(((plan.value?.plan_done_steps ?? 0) / totalSteps.value) * 100)
})

/** 状态对应的语义色：草稿待批 / 已批准 / 被驳回 / 已完成 */
const tone = computed(() => {
  switch (plan.value?.plan_status) {
    case 'approved':
      return 'ok'
    case 'rejected':
      return 'bad'
    case 'done':
      return 'mute'
    default:
      return 'warn'
  }
})

/** 整体标注与步骤标注分开渲染：步骤标注要贴着对应步骤读 */
const planAnnotations = computed(
  () => plan.value?.plan_annotations.filter((a) => a.annotation_scope !== 'step') ?? [],
)
const stepNote = (seq: number) =>
  plan.value?.plan_annotations.filter(
    (a) => a.annotation_scope === 'step' && a.annotation_step === seq,
  ) ?? []

async function reload() {
  loading.value = true
  failed.value = ''
  try {
    const resp = await api.plan.get()
    plan.value = resp?.resp_plan ?? null
  } catch (e) {
    failed.value = errorText(e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void reload()
})

defineExpose({ reload })
</script>

<template>
  <div class="plan-wrap">
    <div v-if="loading && !plan" class="wb-card pad">
      <el-skeleton animated :rows="3" />
    </div>

    <div v-else-if="failed" class="wb-card pad err">
      <span class="err-tx">{{ t('common.loadFailed', { msg: failed }) }}</span>
      <el-button size="small" @click="reload">{{ t('common.retry') }}</el-button>
    </div>

    <EmptyState
      v-else-if="!plan"
      icon="flag"
      :title="t('featurePanel.planEmpty')"
      :desc="t('featurePanel.planEmptyHint')"
    />

    <template v-else>
      <div class="head">
        <span class="badge" :class="`is-${tone}`">
          {{ t(`featurePanel.planStatus.${plan.plan_status}`) }}
        </span>
        <span class="time">{{ fmtAbs(plan.plan_updated_at) }}</span>
      </div>

      <h3 class="title">{{ plan.plan_title }}</h3>

      <div v-if="plan.plan_reject_note" class="reject">
        <AppIcon name="alert" size="ic-xs" />
        <span>{{ t('featurePanel.planRejected', { note: plan.plan_reject_note }) }}</span>
      </div>

      <template v-if="totalSteps">
        <div class="cap">
          <span class="wb-label">
            {{ t('featurePanel.planProgress', { done: plan.plan_done_steps, total: totalSteps }) }}
          </span>
          <el-progress
            :percentage="pct"
            :stroke-width="6"
            :show-text="false"
            :status="plan.plan_status === 'done' ? 'success' : undefined"
            class="bar"
          />
        </div>
        <ol class="steps">
          <li
            v-for="(s, i) in plan.plan_steps"
            :key="i"
            class="step"
            :class="{ 'is-done': s.step_done }"
          >
            <div class="step-row">
              <span class="seq">{{ i + 1 }}</span>
              <span class="step-mark" :class="{ 'is-done': s.step_done }">
                <AppIcon v-if="s.step_done" name="check" size="ic-xs" />
              </span>
              <span class="step-tx">{{ s.step_text }}</span>
            </div>
            <p v-for="a in stepNote(i + 1)" :key="a.annotation_id" class="step-note">
              {{ t('featurePanel.planStepNote', { text: a.annotation_text }) }}
            </p>
          </li>
        </ol>
      </template>

      <div v-if="planAnnotations.length" class="notes">
        <span class="wb-label">{{ t('featurePanel.planNotes') }}</span>
        <p v-for="a in planAnnotations" :key="a.annotation_id" class="note">
          {{ a.annotation_text }}
        </p>
      </div>
    </template>
  </div>
</template>

<style scoped>
.plan-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  min-height: 0;
}
.head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.badge {
  padding: 1px var(--wb-sp-2);
  border-radius: var(--wb-radius-full);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  border: 1px solid transparent;
}
.badge.is-warn {
  background: var(--wb-warning-soft, var(--wb-accent-tint));
  color: var(--wb-warning);
  border-color: var(--wb-warning);
}
.badge.is-ok {
  background: var(--wb-success-soft, var(--wb-accent-tint));
  color: var(--wb-success);
  border-color: var(--wb-success);
}
.badge.is-bad {
  background: var(--wb-danger-soft);
  color: var(--wb-danger);
  border-color: var(--wb-danger);
}
.badge.is-mute {
  background: var(--wb-elev-hover);
  color: var(--wb-muted);
  border-color: var(--wb-border);
}
.time {
  margin-left: auto;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.title {
  margin: 0;
  font-size: var(--wb-fs-md);
  font-weight: 700;
  line-height: 1.5;
  color: var(--wb-ink);
  overflow-wrap: anywhere;
}
.reject {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-danger-soft);
  border: 1px solid var(--wb-danger);
  color: var(--wb-danger);
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
}
.cap {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
}
.bar {
  flex: 1;
  min-width: 0;
}
.steps {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  margin: 0;
  padding: 0;
  list-style: none;
}
.step {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--wb-sp-1) var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  transition: background-color var(--wb-dur) var(--wb-ease);
}
.step:hover {
  background: var(--wb-elev-hover);
}
.step-row {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-2);
}
.seq {
  flex: none;
  width: calc(16px * var(--wb-vw-scale));
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
  padding-top: 1px;
}
.step-mark {
  flex: none;
  width: calc(14px * var(--wb-vw-scale));
  height: calc(14px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  border: 1.5px solid var(--wb-border-strong);
  display: grid;
  place-items: center;
  margin-top: 2px;
}
.step-mark.is-done {
  background: var(--wb-success);
  border-color: var(--wb-success);
  color: #fff;
}
.step-tx {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
  overflow-wrap: anywhere;
}
.step.is-done .step-tx {
  color: var(--wb-muted);
  text-decoration: line-through;
  text-decoration-color: var(--wb-border-strong);
}
.step-note {
  margin: 0 0 0 calc(16px * var(--wb-vw-scale) + var(--wb-sp-2) + 14px * var(--wb-vw-scale));
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-primary);
  overflow-wrap: anywhere;
}
.notes {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  padding-top: var(--wb-sp-2);
  border-top: 1px solid var(--wb-border);
}
.note {
  margin: 0;
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-ink-2);
  overflow-wrap: anywhere;
}
.pad {
  padding: var(--wb-sp-4);
}
.err {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-3);
}
.err-tx {
  font-size: var(--wb-fs-sm);
  color: var(--wb-danger);
}
</style>
