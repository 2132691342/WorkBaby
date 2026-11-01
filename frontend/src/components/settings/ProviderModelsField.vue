<script setup lang="ts">
// 服务商的模型清单编辑区：手动加 / 从上游拉 / 逐条改窗口与能力。
// 抽成组件只有一个理由：这块表单独占一百多行模板，与卡片列表无关。
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '../common/AppIcon.vue'

/** 编辑中的一条模型配置（就地双向绑定，直接改对象字段）。 */
export interface ModelDraft {
  /** 模型 ID（上游的 model 名） */
  model_id: string
  /** 显示名 */
  model_name: string
  /** 是否推理模型 */
  model_reasoning: boolean
  /** 是否支持视觉输入 */
  model_vision: boolean
  /** 上下文窗口（tokens）；0 表示按后端兜底 */
  model_context_window: number
  /** 单次输出上限（tokens）；0 表示按后端兜底 */
  model_max_tokens: number
}

const props = defineProps<{
  /** 当前已选模型（父组件持有，子组件就地改字段） */
  models: ModelDraft[]
  /** 从上游拉回的候选清单；空数组即不展示这一段 */
  fetched: string[]
  /** 字段级错误文案；非空时整块标红 */
  error: string
  /** 整体忙碌态：拉取 / 保存 / 测试期间锁住所有输入 */
  busy: string
  /** 兜底窗口值：留空时的占位提示 */
  defaultContextWindow: number
  /** 兜底输出上限：留空时的占位提示 */
  defaultMaxTokens: number
}>()

const emit = defineEmits<{
  /** 请求父组件去上游拉模型清单 */
  fetch: []
  /** 输入有改动：父组件据此清掉字段错误 */
  changed: []
}>()

const { t } = useI18n()
const manual = ref('')

function addModel(id: string) {
  const name = id.trim()
  if (!name || props.models.some((m) => m.model_id === name)) return
  props.models.push({
    model_id: name,
    model_name: name,
    model_reasoning: false,
    model_vision: true,
    model_context_window: props.defaultContextWindow,
    model_max_tokens: props.defaultMaxTokens,
  })
  emit('changed')
}

function addManual() {
  for (const line of manual.value.split(/[\s,]+/)) addModel(line)
  manual.value = ''
}

function dropModel(id: string) {
  const i = props.models.findIndex((m) => m.model_id === id)
  if (i >= 0) props.models.splice(i, 1)
  emit('changed')
}

const blank = (id: string) => !id.trim()
</script>

<template>
  <div class="models">
    <div class="models-bar">
      <el-input
        v-model="manual"
        size="small"
        :placeholder="t('settings.providers.modelId')"
        @keydown.enter="addManual"
      />
      <el-button size="small" :disabled="!!busy" @click="addManual">{{ t('common.add') }}</el-button>
      <el-button size="small" :loading="busy === 'fetch'" :disabled="!!busy" @click="emit('fetch')">
        {{ t('settings.providers.fetchModels') }}
      </el-button>
    </div>

    <div v-if="fetched.length" class="models-fetched">
      <span class="wb-label">{{ t('settings.providers.fetched', { n: fetched.length }) }}</span>
      <div class="models-chips">
        <button
          v-for="m in fetched"
          :key="m"
          class="model-chip"
          type="button"
          :class="{ 'is-on': models.some((x) => x.model_id === m) }"
          @click="addModel(m)"
        >
          {{ m }}
        </button>
      </div>
    </div>

    <ul v-if="models.length" class="models-list">
      <li v-for="m in models" :key="m.model_id" class="model-card">
        <div class="model-head">
          <el-input
            v-model="m.model_id"
            size="small"
            class="w-id"
            :class="{ 'is-bad': blank(m.model_id) }"
            :placeholder="t('settings.providers.modelId')"
            @input="emit('changed')"
          />
          <el-input
            v-model="m.model_name"
            size="small"
            class="w-name"
            :placeholder="t('settings.providers.modelName')"
          />
          <button class="model-del" type="button" :title="t('common.delete')" @click="dropModel(m.model_id)">
            <AppIcon name="close" size="ic-xs" />
          </button>
        </div>
        <!-- 第二行只放读数：两行分开之后每个控件都有足够宽度，不再被挤掉 -->
        <div class="model-meta">
          <label class="model-field">
            <span class="model-cap">{{ t('settings.providers.contextWindow') }}</span>
            <el-input-number
              v-model="m.model_context_window"
              size="small"
              :min="0"
              :step="1000"
              :controls="false"
              :placeholder="String(defaultContextWindow)"
              class="w-num"
            />
          </label>
          <label class="model-field">
            <span class="model-cap">{{ t('settings.providers.maxTokens') }}</span>
            <el-input-number
              v-model="m.model_max_tokens"
              size="small"
              :min="0"
              :step="1000"
              :controls="false"
              :placeholder="String(defaultMaxTokens)"
              class="w-num"
            />
          </label>
          <el-checkbox v-model="m.model_reasoning" size="small">
            {{ t('settings.providers.reasoning') }}
          </el-checkbox>
          <el-checkbox v-model="m.model_vision" size="small">
            {{ t('settings.providers.vision') }}
          </el-checkbox>
        </div>
      </li>
    </ul>
    <p v-else class="hint">{{ error || t('settings.providers.modelsRequired') }}</p>
  </div>
</template>

<style scoped>
.models {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.models-bar {
  display: flex;
  gap: var(--wb-sp-2);
}
.models-fetched {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}
.models-chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-1);
  max-height: calc(120px * var(--wb-vw-scale));
  overflow-y: auto;
}
.model-chip {
  padding: 2px var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}
.model-chip:hover {
  border-color: var(--wb-primary-line);
  color: var(--wb-primary);
  background: var(--wb-accent-tint);
}
.model-chip.is-on {
  background: var(--wb-accent-tint);
  border-color: var(--wb-primary);
  color: var(--wb-primary);
}
.models-list {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  max-height: calc(320px * var(--wb-vw-scale));
  overflow-y: auto;
}
/* 一条模型 = 一张小卡：上行标识 / 下行读数。挤成一行时名字与四个读数只能互相截断 */
.model-card {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2);
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-elev-hover);
}
.model-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.model-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--wb-sp-2) var(--wb-sp-4);
}
.model-field {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.model-cap {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  white-space: nowrap;
}
.w-id {
  flex: 1.3;
  min-width: 0;
}
/* 空白模型 ID 当场标红：等点保存才报就得多找一遍哪一行漏了 */
.w-id.is-bad :deep(.el-input__wrapper) {
  box-shadow: 0 0 0 1px var(--wb-danger) inset;
}
.w-name {
  flex: 1;
  min-width: 0;
}
.w-num {
  width: calc(112px * var(--wb-vw-scale));
  flex: none;
}
.model-del {
  display: grid;
  place-items: center;
  width: calc(22px * var(--wb-vw-scale));
  height: calc(22px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  color: var(--wb-muted);
  flex: none;
}
.model-del:hover {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
</style>
