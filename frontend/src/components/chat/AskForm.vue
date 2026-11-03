<script setup lang="ts">
// 问卷分支：一次问多题，答完统一提交。答案按题序以换行分隔回传——
// 与后端 ask_user 的 split('\n') 解析约定严格对齐，不能改成 JSON。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PendingAsk } from '../../types/chatEvents'

const props = defineProps<{ ask: PendingAsk; busy: boolean }>()
const emit = defineEmits<{ submit: [lines: string]; cancel: [] }>()

const { t } = useI18n()

/** 每题的选中项 */
const picked = ref<string[]>([])
/** 每题的自由输入 */
const free = ref<string[]>([])

watch(
  () => props.ask.id,
  () => {
    const n = props.ask.questions?.length ?? 0
    picked.value = Array.from({ length: n }, () => '')
    free.value = Array.from({ length: n }, () => '')
  },
  { immediate: true },
)

/** 选了选项或填了自由输入都算答完这题 */
function filled(i: number): boolean {
  return !!picked.value[i] || !!free.value[i]?.trim()
}

const ready = computed(() => {
  const n = props.ask.questions?.length ?? 0
  if (!n) return false
  return Array.from({ length: n }, (_, i) => filled(i)).every(Boolean)
})

/** 自由输入优先：用户写了具体内容就以他写的为准；换行压平成一行 */
function send() {
  const n = props.ask.questions?.length ?? 0
  const lines = Array.from({ length: n }, (_, i) => {
    const text = free.value[i]?.trim() ?? ''
    return (text || picked.value[i] || '').replace(/\r?\n/g, ' ')
  })
  emit('submit', lines.join('\n'))
}
</script>

<template>
  <div class="ask-form">
    <div v-for="(q, qi) in ask.questions" :key="`${ask.id}-q${qi}`" class="form-q">
      <div class="form-q-title">
        <span class="form-q-no">{{ qi + 1 }}</span>
        <span>{{ q.question_title }}</span>
      </div>
      <div v-if="q.question_options?.length" class="form-opts">
        <button
          v-for="(o, oi) in q.question_options"
          :key="`${qi}-o${oi}`"
          type="button"
          class="form-opt"
          :class="{ 'is-on': picked[qi] === o.option_label }"
          :disabled="busy"
          @click="picked[qi] = picked[qi] === o.option_label ? '' : o.option_label"
        >
          <span class="form-opt-t">{{ o.option_label }}</span>
          <span v-if="o.option_description" class="form-opt-d">{{ o.option_description }}</span>
        </button>
      </div>
      <el-input
        v-model="free[qi]"
        size="small"
        class="form-free"
        :placeholder="
          q.question_options?.length ? t('chat.askFormFreeHint') : t('chat.askInputPlaceholder')
        "
      />
    </div>
    <div class="ask-acts">
      <el-button
        type="primary"
        size="small"
        :loading="busy"
        :disabled="busy || !ready"
        @click="send"
      >
        {{ t('chat.askSubmit') }}
      </el-button>
      <el-button size="small" :disabled="busy" @click="emit('cancel')">
        {{ t('chat.askReject') }}
      </el-button>
    </div>
  </div>
</template>

<style scoped>
.ask-form {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.form-q {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  background: var(--wb-elev-hover);
}
.form-q-title {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
  line-height: 1.5;
}
.form-q-no {
  flex: none;
  width: calc(18px * var(--wb-vw-scale));
  height: calc(18px * var(--wb-vw-scale));
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--wb-radius-full);
  background: var(--wb-primary);
  color: var(--wb-primary-ink);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
}
.form-opts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-2);
}
.form-opt {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-width: 100%;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius);
  background: var(--wb-elev-surface);
  color: var(--wb-ink);
  text-align: left;
  font-size: var(--wb-fs-xs);
  cursor: pointer;
}
.form-opt:hover:not(:disabled) {
  border-color: var(--wb-primary);
  background: var(--wb-accent-tint);
}
.form-opt.is-on {
  border-color: var(--wb-primary);
  background: var(--wb-accent-tint);
  box-shadow: inset 0 0 0 1px var(--wb-primary);
}
.form-opt:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.form-opt-t {
  font-weight: 600;
  line-height: 1.4;
}
.form-opt-d {
  color: var(--wb-muted);
  font-size: var(--wb-fs-2xs);
  line-height: 1.45;
  white-space: pre-wrap;
}
.form-free {
  max-width: 100%;
}
.ask-acts {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
}
</style>
