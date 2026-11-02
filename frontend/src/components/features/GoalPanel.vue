<script setup lang="ts">
// 目标面板：单个活跃目标 + 步骤清单 + 状态机。
// 目标只有一个（不是列表）——多目标会让「现在在推进哪件事」变得含糊。
// 用户做裁决：勾步骤、置阻塞/恢复、达成、放弃、记进展；起草目标仍是模型的事。
import { computed, nextTick, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import type { GoalVO } from '../../types/api'
import { errorText } from '../../utils/errors'
import EmptyState from '../common/EmptyState.vue'
import AppIcon from '../common/AppIcon.vue'

const { t } = useI18n()
const toast = useToastStore()

const goal = ref<GoalVO | null>(null)
const resumeCount = ref(0)
const loading = ref(true)
const failed = ref('')
/** 正在写的动作名（空串 = 空闲） */
const busy = ref('')
const creating = ref(false)
const draftText = ref('')
const draftSteps = ref('')
const draftInput = ref<{ focus: () => void } | null>(null)
const noteEditing = ref(false)
const noteDraft = ref('')

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

function adopt(resp: { resp_goal: GoalVO | null; resp_resume_count: number }) {
  goal.value = resp.resp_goal ?? null
  resumeCount.value = resp.resp_resume_count ?? 0
}

async function reload() {
  loading.value = true
  failed.value = ''
  try {
    adopt(await api.goal.get())
  } catch (e) {
    failed.value = errorText(e)
  } finally {
    loading.value = false
  }
}

/** 所有写入走同一出口：统一忙态、统一提示、统一回填。 */
async function write(key: string, body: Parameters<typeof api.goal.mutate>[0]) {
  if (busy.value) return false
  busy.value = key
  try {
    adopt(await api.goal.mutate(body))
    return true
  } catch (e) {
    toast.bad(errorText(e))
    return false
  } finally {
    busy.value = ''
  }
}

/** 勾一个步骤：勾完最后一步时后端自动置 achieved */
async function markStep(seq: number) {
  await write('step-' + seq, { op: 'done_steps', done_steps: [seq] })
}

/** 阻塞 / 恢复：状态机要求恢复必须显式置回 active */
async function toggleBlocked() {
  const next = goal.value?.goal_status === 'blocked' ? 'active' : 'blocked'
  await write('status', { op: 'status', status: next })
}

async function achieve() {
  await write('status', { op: 'status', status: 'achieved' })
}

async function abandon() {
  await write('abandon', { op: 'abandon' })
}

async function startCreate() {
  creating.value = true
  await nextTick()
  draftInput.value?.focus()
}

async function submitCreate() {
  const text = draftText.value.trim()
  if (!text) return
  // 步骤按行拆：用户写清单的习惯就是一行一步
  const steps = draftSteps.value
    .split(/\r?\n/)
    .map((s) => s.trim())
    .filter(Boolean)
  if (await write('create', { op: 'create', text, steps })) {
    creating.value = false
    draftText.value = ''
    draftSteps.value = ''
  }
}

function startNote() {
  noteDraft.value = goal.value?.goal_note ?? ''
  noteEditing.value = true
}

async function saveNote() {
  if (await write('note', { op: 'note', note: noteDraft.value })) noteEditing.value = false
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

    <template v-else-if="!goal">
      <div v-if="creating" class="create-box">
        <el-input
          ref="draftInput"
          v-model="draftText"
          size="small"
          :placeholder="t('featurePanel.goalCreatePlaceholder')"
        />
        <el-input
          v-model="draftSteps"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 6 }"
          :placeholder="t('featurePanel.goalCreateSteps')"
        />
        <div class="create-acts">
          <el-button
            size="small"
            type="primary"
            :loading="busy === 'create'"
            :disabled="!draftText.trim() || !!busy"
            @click="submitCreate"
          >
            {{ t('common.confirm') }}
          </el-button>
          <el-button size="small" :disabled="!!busy" @click="creating = false">
            {{ t('common.cancel') }}
          </el-button>
        </div>
      </div>
      <template v-else>
        <EmptyState
          icon="target"
          :title="t('featurePanel.goalEmpty')"
          :desc="t('featurePanel.goalEmptyHint')"
        />
        <el-button size="small" class="mk-btn" :disabled="!!busy" @click="startCreate">
          <AppIcon name="plus" size="ic-2xs" />
          <span class="ml-1">{{ t('featurePanel.goalCreate') }}</span>
        </el-button>
      </template>
    </template>

    <template v-else>
      <div class="head">
        <span class="tone-dot" :class="`is-${tone}`" aria-hidden="true" />
        <span class="status">{{ t(`featurePanel.goalStatus.${goal.goal_status}`) }}</span>
        <span v-if="resumeCount" class="resume">
          {{ t('featurePanel.goalResume', { n: resumeCount }) }}
        </span>
      </div>

      <p class="goal-text">{{ goal.goal_text }}</p>

      <!-- 状态动作：裁决权在用户，按钮只在当前状态合法时出现 -->
      <div class="acts">
        <template v-if="goal.goal_status === 'active' || goal.goal_status === 'blocked'">
          <el-button size="small" :loading="busy === 'status'" :disabled="!!busy" @click="toggleBlocked">
            {{ goal.goal_status === 'blocked' ? t('featurePanel.goalResumeIt') : t('featurePanel.goalBlock') }}
          </el-button>
          <el-button
            size="small"
            type="primary"
            :loading="busy === 'status'"
            :disabled="!!busy"
            @click="achieve"
          >
            {{ t('featurePanel.goalAchieve') }}
          </el-button>
          <el-button size="small" :loading="busy === 'abandon'" :disabled="!!busy" @click="abandon">
            {{ t('featurePanel.goalAbandon') }}
          </el-button>
        </template>
        <el-button v-if="!noteEditing" size="small" :disabled="!!busy" @click="startNote">
          {{ t('featurePanel.goalNoteEdit') }}
        </el-button>
        <el-button v-else size="small" :loading="busy === 'note'" :disabled="!!busy" @click="saveNote">
          {{ t('common.save') }}
        </el-button>
      </div>

      <div v-if="noteEditing" class="note-edit">
        <el-input
          v-model="noteDraft"
          size="small"
          :placeholder="t('featurePanel.goalNotePlaceholder')"
          @keydown.enter="saveNote"
        />
      </div>
      <p v-else-if="goal.goal_note" class="goal-note">{{ goal.goal_note }}</p>

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
            <button
              class="step-mark"
              :class="{ 'is-done': s.step_done }"
              type="button"
              :title="t('featurePanel.goalMarkStep')"
              :disabled="s.step_done || !!busy"
              @click="markStep(i + 1)"
            >
              <AppIcon v-if="s.step_done" name="check" size="ic-xs" />
            </button>
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
.acts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-2);
}
.note-edit,
.create-box {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.create-acts {
  display: flex;
  gap: var(--wb-sp-2);
}
.mk-btn {
  align-self: flex-start;
}
.step-mark {
  flex: none;
  width: calc(15px * var(--wb-vw-scale));
  height: calc(15px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  border: 1.5px solid var(--wb-border-strong);
  display: grid;
  place-items: center;
  margin-top: 2px;
  transition:
    border-color var(--wb-dur) var(--wb-ease),
    background-color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.step-mark:hover:not(:disabled) {
  border-color: var(--wb-primary);
  transform: scale(1.15);
}
.step-mark:active:not(:disabled) {
  transform: scale(var(--wb-btn-scale-press));
}
.step-mark:disabled {
  cursor: default;
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
