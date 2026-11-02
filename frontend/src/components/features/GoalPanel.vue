<script setup lang="ts">
// 目标面板：单个活跃目标 + 步骤清单 + 状态机。
// 目标只有一个（不是列表）——多目标会让「现在在推进哪件事」变得含糊。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import type { GoalVO } from '../../types/api'
import { errorText } from '../../utils/errors'
import EmptyState from '../common/EmptyState.vue'
import AppIcon from '../common/AppIcon.vue'

const { t } = useI18n()

const goal = ref<GoalVO | null>(null)
const resumeCount = ref(0)
const loading = ref(true)
const failed = ref('')

const totalSteps = computed(() => goal.value?.goal_steps.length ?? 0)
const pct = computed(() => {
  if (!totalSteps.value) return 0
  return Math.round(((goal.value?.goal_done_steps ?? 0) / totalSteps.value) * 100)
})

/** 状态灯的三档语义：正在推进 / 卡住 / 已结束 */
const tone = computed(() => {
  switch (goal.value?.goal_status) {
    case 'active':
      return 'live'
    case 'blocked':
      return 'warn'
    case 'achieved':
      return 'ok'
    default:
      return 'mute'
  }
})

async function reload() {
  loading.value = true
  failed.value = ''
  try {
    const resp = await api.goal.get()
    goal.value = resp?.resp_goal ?? null
    resumeCount.value = resp?.resp_resume_count ?? 0
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
  <div class="goal-wrap">
    <div v-if="loading && !goal" class="wb-card pad">
      <el-skeleton animated :rows="2" />
    </div>

    <div v-else-if="failed" class="wb-card pad err">
      <span class="err-tx">{{ t('common.loadFailed', { msg: failed }) }}</span>
      <el-button size="small" @click="reload">{{ t('common.retry') }}</el-button>
    </div>

    <EmptyState
      v-else-if="!goal"
      icon="target"
      :title="t('featurePanel.goalEmpty')"
      :desc="t('featurePanel.goalEmptyHint')"
    />

    <template v-else>
      <div class="head">
        <span class="tone-dot" :class="`is-${tone}`" aria-hidden="true" />
        <span class="status">{{ t(`featurePanel.goalStatus.${goal.goal_status}`) }}</span>
        <span v-if="resumeCount" class="resume">
          {{ t('featurePanel.goalResume', { n: resumeCount }) }}
        </span>
      </div>

      <p class="goal-text">{{ goal.goal_text }}</p>
      <p v-if="goal.goal_note" class="goal-note">{{ goal.goal_note }}</p>

      <template v-if="totalSteps">
        <div class="cap">
          <span class="wb-label">
            {{ t('featurePanel.goalProgress', { done: goal.goal_done_steps, total: totalSteps }) }}
          </span>
          <el-progress
            :percentage="pct"
            :stroke-width="6"
            :show-text="false"
            :status="goal.goal_status === 'achieved' ? 'success' : undefined"
            class="bar"
          />
        </div>
        <ul class="steps">
          <li v-for="(s, i) in goal.goal_steps" :key="i" class="step" :class="{ 'is-done': s.step_done }">
            <span class="step-mark" :class="{ 'is-done': s.step_done }">
              <AppIcon v-if="s.step_done" name="check" size="ic-xs" />
            </span>
            <span class="step-tx">{{ s.step_text }}</span>
          </li>
        </ul>
      </template>
    </template>
  </div>
</template>

<style scoped>
.goal-wrap {
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
.tone-dot {
  width: 7px;
  height: 7px;
  border-radius: var(--wb-radius-full);
  flex: none;
}
.tone-dot.is-live {
  background: var(--wb-live);
  animation: goal-breathe 1.6s var(--wb-ease) infinite;
}
.tone-dot.is-warn {
  background: var(--wb-warning);
}
.tone-dot.is-ok {
  background: var(--wb-success);
}
.tone-dot.is-mute {
  background: var(--wb-muted);
}
@keyframes goal-breathe {
  0%,
  100% {
    opacity: 0.4;
  }
  50% {
    opacity: 1;
  }
}
.status {
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  letter-spacing: 0.02em;
  color: var(--wb-ink-2);
}
.resume {
  margin-left: auto;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.goal-text {
  margin: 0;
  font-size: var(--wb-fs-md);
  font-weight: 700;
  line-height: 1.5;
  color: var(--wb-ink);
  overflow-wrap: anywhere;
}
.goal-note {
  margin: 0;
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-muted);
  padding-left: var(--wb-sp-2);
  border-left: 2px solid var(--wb-border);
  overflow-wrap: anywhere;
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
  align-items: flex-start;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-1) var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  transition: background-color var(--wb-dur) var(--wb-ease);
}
.step:hover {
  background: var(--wb-elev-hover);
}
.step-mark {
  flex: none;
  width: calc(14px * var(--wb-vw-scale));
  height: calc(14px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
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
