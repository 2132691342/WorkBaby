<script setup lang="ts">
// 提问卡片：只渲染引擎给的四个基元（confirm / select / input / editor）与 ask 工具。
// 不发明第五种控件——PI 认不出我们编出来的回答形态，重画一套漂亮控件反而是假支持。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AskRequest, UIResponse } from '@/types/api'

const props = defineProps<{ ask: AskRequest }>()
const emit = defineEmits<{ answer: [response: UIResponse] }>()

const { t } = useI18n()

const text = ref(props.ask.prefill ?? '')
const picked = ref<number[]>([])
const answers = ref<Record<string, string[]>>({})
const custom = ref<Record<string, string>>({})
const submitting = ref(false)

watch(
  () => props.ask.id,
  () => {
    text.value = props.ask.prefill ?? ''
    picked.value = []
    answers.value = {}
    custom.value = {}
    submitting.value = false
  },
)

const isAskTool = computed(() => props.ask.method === 'ask')

function toggleOption(key: string, label: string, multi: boolean) {
  const current = answers.value[key] ?? []
  if (multi) {
    answers.value = {
      ...answers.value,
      [key]: current.includes(label) ? current.filter((l) => l !== label) : [...current, label],
    }
    return
  }
  answers.value = { ...answers.value, [key]: [label] }
}

function isPicked(key: string, label: string): boolean {
  return (answers.value[key] ?? []).includes(label)
}

/** 长得像「自己写」的那一行当哨兵处理：展开输入框，而不是当成选项。 */
function isCustomOption(label: string): boolean {
  return /^(type something|other|其他|自定义|自己写)/i.test(label.trim())
}

async function submit() {
  submitting.value = true
  try {
    if (isAskTool.value) {
      const payload = (props.ask.questions ?? []).map((q) => ({
        id: q.id,
        selectedOptions: answers.value[q.id] ?? [],
        customInput: custom.value[q.id] || undefined,
      }))
      emit('answer', { id: props.ask.id, answers: payload })
      return
    }
    if (props.ask.method === 'confirm') {
      emit('answer', { id: props.ask.id, confirmed: true })
      return
    }
    if (props.ask.method === 'select') {
      const choice = props.ask.options?.[picked.value[0] ?? -1] ?? text.value
      emit('answer', { id: props.ask.id, value: choice })
      return
    }
    emit('answer', { id: props.ask.id, value: text.value })
  } finally {
    submitting.value = false
  }
}

function decline() {
  emit('answer', { id: props.ask.id, cancelled: true })
}

function pick(index: number) {
  picked.value = [index]
  text.value = ''
}

const canSubmit = computed(() => {
  if (isAskTool.value) {
    return (props.ask.questions ?? []).every(
      (q) => (answers.value[q.id] ?? []).length > 0 || (custom.value[q.id] ?? '').trim().length > 0,
    )
  }
  if (props.ask.method === 'confirm') return true
  if (props.ask.method === 'select') return picked.value.length > 0 || text.value.trim().length > 0
  return text.value.trim().length > 0
})
</script>

<template>
  <div class="ask">
    <div class="ask-head">
      <span class="tag brand">{{ ask.method === 'confirm' ? '确认' : ask.method === 'select' ? '选择' : ask.method === 'editor' ? '编辑' : ask.method === 'ask' ? '提问' : '输入' }}</span>
      <span v-if="ask.title" class="ask-title">{{ ask.title }}</span>
    </div>
    <p v-if="ask.message" class="ask-message selectable">{{ ask.message }}</p>

    <!-- confirm -->
    <template v-if="ask.method === 'confirm'">
      <div class="ask-actions">
        <button class="wb-btn primary" :class="{ 'is-loading': submitting }" :disabled="submitting" @click="submit">
          {{ t('chat.yes') }}
        </button>
        <button class="wb-btn" :disabled="submitting" @click="decline">{{ t('chat.no') }}</button>
      </div>
    </template>

    <!-- select -->
    <template v-else-if="ask.method === 'select'">
      <div class="ask-options">
        <button
          v-for="(option, index) in ask.options ?? []"
          :key="option"
          class="ask-option"
          :class="{ selected: picked.includes(index) }"
          type="button"
          @click="pick(index)"
        >
          <span class="ask-num">{{ index + 1 }}</span>
          <span class="ask-option-text">
            <span class="ask-option-label">{{ option }}</span>
            <span v-if="ask.optionDetails?.[index]?.description" class="ask-option-desc">
              {{ ask.optionDetails[index].description }}
            </span>
          </span>
        </button>
      </div>
      <div class="ask-input-row">
        <input v-model="text" class="wb-input" :placeholder="ask.placeholder || '或者自己写一句'" @input="picked = []" />
        <button class="wb-btn primary" :class="{ 'is-loading': submitting }" :disabled="!canSubmit || submitting" @click="submit">
          {{ t('chat.submit') }}
        </button>
      </div>
      <button class="wb-btn text sm" @click="decline">{{ t('common.cancel') }}</button>
    </template>

    <!-- input / editor -->
    <template v-else-if="ask.method === 'input' || ask.method === 'editor'">
      <textarea
        v-if="ask.method === 'editor'"
        v-model="text"
        class="wb-input"
        rows="6"
        :placeholder="ask.placeholder || ''"
      />
      <input v-else v-model="text" class="wb-input" :placeholder="ask.placeholder || ''" @keyup.enter="canSubmit && submit()" />
      <div class="ask-actions">
        <button class="wb-btn primary" :class="{ 'is-loading': submitting }" :disabled="!canSubmit || submitting" @click="submit">
          {{ t('chat.submit') }}
        </button>
        <button class="wb-btn" :disabled="submitting" @click="decline">{{ t('common.cancel') }}</button>
      </div>
    </template>

    <!-- ask 工具：多题、多选、自定义 -->
    <template v-else-if="ask.method === 'ask'">
      <div v-for="question in ask.questions ?? []" :key="question.id" class="ask-question">
        <p class="ask-title">{{ question.header || question.question }}</p>
        <p v-if="question.header && question.question !== question.header" class="ask-message selectable">
          {{ question.question }}
        </p>
        <div class="ask-options">
          <button
            v-for="option in question.options"
            :key="option.label"
            class="ask-option"
            :class="{ selected: isPicked(question.id, option.label) }"
            type="button"
            @click="toggleOption(question.id, option.label, Boolean(question.multi))"
          >
            <span class="ask-num">{{ isPicked(question.id, option.label) ? '✓' : '' }}</span>
            <span class="ask-option-text">
              <span class="ask-option-label">{{ option.label }}</span>
              <span v-if="option.description" class="ask-option-desc">{{ option.description }}</span>
            </span>
          </button>
        </div>
        <input
          v-model="custom[question.id]"
          class="wb-input"
          :placeholder="isCustomOption('') ? '' : '也可以自己写一句'"
        />
      </div>
      <div class="ask-actions">
        <button class="wb-btn primary" :class="{ 'is-loading': submitting }" :disabled="!canSubmit || submitting" @click="submit">
          {{ t('chat.submit') }}
        </button>
        <button class="wb-btn" :disabled="submitting" @click="decline">{{ t('common.cancel') }}</button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.ask-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}

.ask-options {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.ask-option-text {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.ask-option-label {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
  white-space: pre-wrap;
}

.ask-option-desc {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
}

.ask-actions {
  display: flex;
  gap: var(--wb-sp-2);
}

.ask-input-row {
  display: flex;
  gap: var(--wb-sp-2);
}

.ask-question {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding-top: var(--wb-sp-2);
  border-top: 1px solid var(--wb-line);
}

.ask-question:first-of-type {
  border-top: none;
  padding-top: 0;
}
</style>
