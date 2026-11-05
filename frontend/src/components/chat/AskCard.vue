<script setup lang="ts">
// 提问卡片：只渲染引擎给的四个基元（confirm / select / input / editor）。
// 不发明第五种控件——引擎认不出我们编出来的回答形态，重画一套漂亮控件反而是假支持。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AskRequest, UIResponse } from '@/types/api'

const props = defineProps<{ ask: AskRequest }>()
const emit = defineEmits<{ answer: [response: UIResponse] }>()

const { t } = useI18n()

const text = ref(props.ask.ask_prefill ?? '')
const picked = ref<number[]>([])
const submitting = ref(false)

watch(
  () => props.ask.ask_id,
  () => {
    text.value = props.ask.ask_prefill ?? ''
    picked.value = []
    submitting.value = false
  },
)

/** 长得像「自己写」的那一行当哨兵处理：展开输入框，而不是当成选项。 */
function isCustomOption(label: string): boolean {
  return /^(type something|other|其他|自定义|自己写)/i.test(label.trim())
}

async function submit() {
  submitting.value = true
  try {
    if (props.ask.ask_method === 'confirm') {
      emit('answer', { ask_id: props.ask.ask_id, confirmed: true })
      return
    }
    if (props.ask.ask_method === 'select') {
      const choice = props.ask.ask_options?.[picked.value[0] ?? -1] ?? text.value
      emit('answer', { ask_id: props.ask.ask_id, value: choice })
      return
    }
    emit('answer', { ask_id: props.ask.ask_id, value: text.value })
  } finally {
    submitting.value = false
  }
}

function decline() {
  emit('answer', { ask_id: props.ask.ask_id, cancelled: true })
}

function pick(index: number) {
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
</script>

<template>
  <div class="ask">
    <div class="ask-head">
      <span class="tag brand">{{ methodLabel }}</span>
      <span v-if="ask.ask_title" class="ask-title">{{ ask.ask_title }}</span>
    </div>
    <p v-if="ask.ask_message" class="ask-message selectable">{{ ask.ask_message }}</p>

    <!-- confirm -->
    <template v-if="ask.ask_method === 'confirm'">
      <div class="ask-actions">
        <button class="wb-btn primary" :class="{ 'is-loading': submitting }" :disabled="submitting" @click="submit">
          {{ t('chat.yes') }}
        </button>
        <button class="wb-btn" :disabled="submitting" @click="decline">{{ t('chat.no') }}</button>
      </div>
    </template>

    <!-- select -->
    <template v-else-if="ask.ask_method === 'select'">
      <div class="ask-options">
        <button
          v-for="(option, index) in ask.ask_options ?? []"
          :key="option"
          class="ask-option"
          :class="{ selected: picked.includes(index) }"
          type="button"
          @click="pick(index)"
        >
          <span class="ask-num">{{ index + 1 }}</span>
          <span class="ask-label">{{ option }}</span>
        </button>
      </div>
      <div v-if="customSelected || !ask.ask_options?.length" class="ask-input">
        <input v-model="text" class="wb-input" :placeholder="ask.ask_placeholder || t('chat.typeAnswer')" @keydown.enter="submit" />
      </div>
      <div class="ask-actions">
        <button class="wb-btn primary" :class="{ 'is-loading': submitting }" :disabled="submitting || !canSubmit" @click="submit">
          {{ t('chat.submit') }}
        </button>
        <button class="wb-btn" :disabled="submitting" @click="decline">{{ t('chat.cancel') }}</button>
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
        <button class="wb-btn primary" :class="{ 'is-loading': submitting }" :disabled="submitting || !canSubmit" @click="submit">
          {{ t('chat.submit') }}
        </button>
        <button class="wb-btn" :disabled="submitting" @click="decline">{{ t('chat.cancel') }}</button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.ask {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-4);
  border: 1px solid var(--wb-line-strong);
  border-radius: var(--wb-radius-lg);
  background: var(--wb-surface-raised);
  box-shadow: var(--wb-shadow-sm);
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

.ask-message {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-soft);
  line-height: var(--wb-lh-base);
}

.ask-options {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}

.ask-option {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius-md);
  background: var(--wb-surface);
  color: var(--wb-ink);
  font-size: var(--wb-fs-sm);
  text-align: left;
  cursor: pointer;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.ask-option:hover {
  background: var(--wb-surface-hover);
  border-color: var(--wb-brand-soft);
  transform: scale(1.01);
}

.ask-option:active {
  transform: scale(0.99);
}

.ask-option.selected {
  border-color: var(--wb-brand);
  background: var(--wb-brand-wash);
}

.ask-num {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: var(--wb-radius-sm);
  background: var(--wb-surface-sunken);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-soft);
}

.ask-label {
  flex: 1;
  user-select: text;
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
