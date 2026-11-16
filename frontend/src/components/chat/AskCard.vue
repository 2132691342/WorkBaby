<script setup lang="ts">
// 提问卡片：只渲染引擎给的四个基元（confirm / select / input / editor）。
// 不发明第五种控件——引擎认不出我们编出来的回答形态，重画一套漂亮控件反而是假支持。
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AskRequest, UIResponse } from '@/types/api'

const props = defineProps<{
  ask: AskRequest
  /** 回答正在提交：由父组件在 await 期间持有——emit 不等异步，
      在卡片内部 finally 复位等于「转圈 0ms」，用户能连点两次发两份答案。 */
  answering?: boolean
}>()
const emit = defineEmits<{ answer: [response: UIResponse] }>()

const { t } = useI18n()

const text = ref(props.ask.ask_prefill ?? '')
const picked = ref<number[]>([])
/** 本卡片是否已经按下过一次：连点守卫，避免同一条答案发两遍。 */
const sent = ref(false)
const submitting = computed(() => Boolean(props.answering) || sent.value)

// 引擎给提问带超时（ask_timeout_ms）时显示剩余时间。后端不强制：
// 归零后仍可提交，引擎收不收是它的事，所以只提示不锁按钮。
const receivedAt = Date.now()
const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | null = null

const remainingMs = computed(() => (props.ask.ask_timeout_ms ? props.ask.ask_timeout_ms - (now.value - receivedAt) : 0))
const timedOut = computed(() => Boolean(props.ask.ask_timeout_ms) && remainingMs.value <= 0)
const remainingLabel = computed(() => {
  if (!props.ask.ask_timeout_ms || remainingMs.value <= 0) return ''
  const total = Math.ceil(remainingMs.value / 1000)
  const m = Math.floor(total / 60)
  const s = total % 60
  return m > 0 ? `${m}:${String(s).padStart(2, '0')}` : `${s}s`
})

watch(
  () => props.ask.ask_id,
  () => {
    text.value = props.ask.ask_prefill ?? ''
    picked.value = []
    sent.value = false
  },
)

/** 提问形态的显示名。 */
const methodLabel = computed(() => {
  switch (props.ask.ask_method) {
    case 'confirm':
      return t('chat.askConfirm')
    case 'select':
      return t('chat.askSelect')
    case 'editor':
      return t('chat.askEditor')
    default:
      return t('chat.askInput')
  }
})

/** 长得像「自己写」的那一行当哨兵处理：展开输入框，而不是当成选项。 */
function isCustomOption(label: string): boolean {
  return /^(type something|other|其他|自定义|自己写)/i.test(label.trim())
}

// select 是否多选：只在选项里出现「全选 / 都要 / all」这类明示词时才多选。
// 没有线索就单选——多选会让模型收到一串它没问的答案。
const multiSelect = computed(() =>
  (props.ask.ask_options ?? []).some((o) => /^(全选|都要|all|multiple|多选)/i.test(o.trim())),
)

async function submit() {
  if (submitting.value) return
  sent.value = true
  if (props.ask.ask_method === 'confirm') {
    emit('answer', { ask_id: props.ask.ask_id, confirmed: true })
    return
  }
  if (props.ask.ask_method === 'select') {
    // 多选时把选中的几条按原顺序拼回去；单选取那一条。
    // 走 answers 通道：后端按 SelectedOptions 还原成一句话。
    const options = props.ask.ask_options ?? []
    const chosen = picked.value
      .slice()
      .sort((a, b) => a - b)
      .map((i) => options[i])
      .filter((label): label is string => Boolean(label))
    if (chosen.length) {
      emit('answer', {
        ask_id: props.ask.ask_id,
        value: chosen.join('、'),
        answers: [{ question_id: props.ask.ask_id, selected_options: chosen }],
      })
      return
    }
    emit('answer', { ask_id: props.ask.ask_id, value: text.value })
    return
  }
  emit('answer', { ask_id: props.ask.ask_id, value: text.value })
}

function decline() {
  if (submitting.value) return
  sent.value = true
  emit('answer', { ask_id: props.ask.ask_id, cancelled: true })
}

function pick(index: number) {
  if (multiSelect.value) {
    const at = picked.value.indexOf(index)
    if (at >= 0) picked.value = picked.value.filter((i) => i !== index)
    else picked.value = [...picked.value, index]
    return
  }
  picked.value = [index]
  text.value = ''
}

/** 选项里带「自己写」时，选中它就该展开输入框。 */
const customSelected = computed(() => {
  const option = props.ask.ask_options?.[picked.value[0] ?? -1]
  return option !== undefined && isCustomOption(option)
})

const canSubmit = computed(() => {
  if (props.ask.ask_method === 'confirm') return true
  if (props.ask.ask_method === 'select') return picked.value.length > 0 || text.value.trim().length > 0
  return text.value.trim().length > 0
})

/** 数字键 1-9 快选：选项多的时候比鼠标快。 */
function pickByNumber(key: string): boolean {
  const n = Number(key)
  if (!Number.isInteger(n) || n < 1 || n > 9) return false
  const options = props.ask.ask_options ?? []
  if (n > options.length) return false
  pick(n - 1)
  return true
}

/** 卡片挂上后接管数字键：输入框有焦点时不接管，免得打字打不出数字。 */
function onCardKeydown(event: KeyboardEvent): void {
  const target = event.target
  if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement) return
  if (pickByNumber(event.key)) event.preventDefault()
}

onMounted(() => {
  if (props.ask.ask_timeout_ms) {
    timer = setInterval(() => {
      now.value = Date.now()
    }, 1000)
  }
  document.addEventListener('keydown', onCardKeydown)
})
onUnmounted(() => {
  if (timer) clearInterval(timer)
  document.removeEventListener('keydown', onCardKeydown)
})
</script>

<template>
  <!-- 进出都有过渡：提问卡是打断式的，无声出现/无声消失都会让人疑心「点了没反应」 -->
  <Transition name="ask-in" appear>
    <div class="ask">
      <div class="ask-head">
        <span class="tag brand">{{ methodLabel }}</span>
        <span v-if="ask.ask_title" class="ask-title">{{ ask.ask_title }}</span>
        <span class="ask-head-end">
          <span v-if="timedOut" class="ask-timer expired">{{ t('chat.askExpired') }}</span>
          <span v-else-if="remainingLabel" class="ask-timer">{{ remainingLabel }}</span>
        </span>
      </div>
      <p v-if="ask.ask_message" class="ask-message selectable">{{ ask.ask_message }}</p>

      <!-- confirm -->
      <template v-if="ask.ask_method === 'confirm'">
        <div class="ask-actions">
          <button type="button" class="wb-btn primary" :class="{ 'is-loading': submitting }" :disabled="submitting" @click="submit">
            {{ t('chat.yes') }}
          </button>
          <button type="button" class="wb-btn" :disabled="submitting" @click="decline">{{ t('chat.no') }}</button>
        </div>
      </template>

    <!-- select -->
    <template v-else-if="ask.ask_method === 'select'">
      <div class="ask-options">
        <button
          v-for="(option, index) in ask.ask_options ?? []"
          :key="option"
          class="ask-option wb-pressable-row"
          :class="{ selected: picked.includes(index) }"
          type="button"
          @click="pick(index)"
        >
          <span class="ask-num">{{ index + 1 }}</span>
          <span class="ask-label">{{ option }}</span>
          <span v-if="multiSelect" class="ask-box" :class="{ on: picked.includes(index) }" />
        </button>
      </div>
      <p v-if="multiSelect" class="ask-hint">
        {{ picked.length ? t('chat.askPicked', { n: picked.length }) : t('chat.askMulti') }}
      </p>
      <div v-if="customSelected || !ask.ask_options?.length" class="ask-input">
        <input v-model="text" class="wb-input" :placeholder="ask.ask_placeholder || t('chat.typeAnswer')" @keydown.enter="submit" />
      </div>
      <div class="ask-actions">
        <button type="button" class="wb-btn primary" :class="{ 'is-loading': submitting }" :disabled="submitting || !canSubmit" @click="submit">
          {{ t('chat.submit') }}
        </button>
        <button type="button" class="wb-btn" :disabled="submitting" @click="decline">{{ t('chat.cancel') }}</button>
      </div>
    </template>

    <!-- input / editor -->
    <template v-else>
      <div class="ask-input">
        <textarea
          v-if="ask.ask_method === 'editor'"
          v-model="text"
          class="wb-input ask-editor"
          rows="8"
          :placeholder="ask.ask_placeholder || ''"
        />
        <input
          v-else
          v-model="text"
          class="wb-input"
          :placeholder="ask.ask_placeholder || t('chat.typeAnswer')"
          @keydown.enter="submit"
        />
      </div>
      <div class="ask-actions">
        <button type="button" class="wb-btn primary" :class="{ 'is-loading': submitting }" :disabled="submitting || !canSubmit" @click="submit">
          {{ t('chat.submit') }}
        </button>
        <button type="button" class="wb-btn" :disabled="submitting" @click="decline">{{ t('chat.cancel') }}</button>
      </div>
    </template>
    </div>
  </Transition>
</template>

<style scoped>
.ask {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-4);
  border: 1px solid var(--wb-line-strong);
  border-radius: var(--wb-r-lg);
  background: var(--wb-raised);
  box-shadow: var(--wb-shadow-1);
}

.ask-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}

.ask-title {
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}

.ask-head-end {
  margin-left: auto;
  flex: none;
}

.ask-timer {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
}

.ask-timer.expired {
  color: var(--wb-warn);
}

.ask-message {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
  line-height: var(--wb-lh-base);
}

.ask-options {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}

/* .ask-option 只留一份样式：在 content-ask.css（含悬停/选中态）。
   缩放刻度由模板上的 .wb-pressable-row 给，这里不要再抄。 */

.ask-num {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: var(--wb-r-sm);
  background: var(--wb-sunken);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
}

.ask-label {
  flex: 1;
  user-select: text;
}

/* 多选的方框：选中才填实。单选不显示它——单选已经有整行的选中底色。 */
.ask-box {
  flex: none;
  width: 15px;
  height: 15px;
  border: 1.5px solid var(--wb-line-strong);
  border-radius: var(--wb-r-xs);
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease);
}

.ask-box.on {
  border-color: var(--wb-brand);
  background: var(--wb-brand);
  box-shadow: inset 0 0 0 2px var(--wb-surface);
}

.ask-hint {
  margin: 0;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
}

.ask-input {
  display: flex;
}

.ask-editor {
  resize: vertical;
  font-family: var(--wb-font-mono);
  line-height: var(--wb-lh-base);
}

.ask-actions {
  display: flex;
  gap: var(--wb-sp-2);
  justify-content: flex-end;
}
</style>
