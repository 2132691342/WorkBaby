<script setup lang="ts">
// 服务商编辑弹窗：模型清单、密钥、接口地址一次配完。
// 从 ProviderPanel 拆出——面板管清单与动作，弹窗管一张表单，两件事的改动节奏不同。
import { useI18n } from 'vue-i18n'
import type { ModelREQ } from '../../types/api'
import ProviderModelsField from './ProviderModelsField.vue'

defineProps<{
  /** 弹窗开关（v-model） */
  modelValue: boolean
  /** 正在编辑的服务商 id；空串 = 新建 */
  editing: string
  /** 表单草稿（父级持有，弹窗只读写它） */
  form: {
    /** 服务商 id，新建时为空 */
    id: string
    /** 显示名 */
    name: string
    /** wire api 类型 */
    api: string
    /** 接口地址 */
    base_url: string
    /** 密钥（留空表示不改） */
    api_key: string
    /** 模型清单 */
    models: ModelREQ[]
  }
  /** 字段级错误文案 */
  errors: { name: string; base_url: string; models: string }
  /** 从上游拉到的模型 id 候选 */
  fetched: string[]
  /** 进行中的动作；非空即禁用全部控件 */
  busy: string
  /** 模型默认上下文窗口 */
  defaultContextWindow: number
  /** 模型默认单轮上限 */
  defaultMaxTokens: number
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'fetch'): void
  (e: 'clear-error', field: 'name' | 'base_url' | 'models'): void
  (e: 'save'): void
}>()

const { t } = useI18n()

/** wire api 清单：后端只认这几条，写错就 3005。 */
const APIS = [
  { key: 'openai-completions', label: 'OpenAI Completions' },
  { key: 'openai-responses', label: 'OpenAI Responses' },
  { key: 'anthropic-messages', label: 'Anthropic Messages' },
  { key: 'google-generative-ai', label: 'Google Generative AI' },
  { key: 'azure-openai-responses', label: 'Azure OpenAI Responses' },
]
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    :title="editing ? t('common.edit') : t('settings.providers.add')"
    :width="`calc(760px * var(--wb-vw-scale))`"
    destroy-on-close
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
  >
    <el-form label-position="top" @submit.prevent>
      <div class="form-grid">
        <el-form-item :label="t('settings.providers.api')">
          <el-select v-model="form.api" :disabled="!!busy">
            <el-option v-for="a in APIS" :key="a.key" :label="a.label" :value="a.key" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('common.name')" required :error="errors.name">
          <el-input
            v-model="form.name"
            :placeholder="form.id || t('settings.providers.namePlaceholder')"
            :disabled="!!busy"
            @input="emit('clear-error', 'name')"
          />
          <p v-if="form.id" class="hint">{{ t('settings.providers.idLocked') }}</p>
        </el-form-item>
      </div>

      <el-form-item :label="t('settings.providers.baseUrl')" required :error="errors.base_url">
        <el-input
          v-model="form.base_url"
          placeholder="https://api.example.com/v1"
          :disabled="!!busy"
          @input="emit('clear-error', 'base_url')"
        />
      </el-form-item>

      <el-form-item :label="t('settings.providers.key')">
        <el-input
          v-model="form.api_key"
          type="password"
          show-password
          :placeholder="form.id ? t('settings.providers.keySaved') : t('settings.providers.keyPlaceholder')"
          :disabled="!!busy"
        />
      </el-form-item>

      <el-form-item :label="t('settings.providers.models')" required :error="errors.models">
        <ProviderModelsField
          :models="form.models"
          :fetched="fetched"
          :error="errors.models"
          :busy="busy"
          :default-context-window="defaultContextWindow"
          :default-max-tokens="defaultMaxTokens"
          @fetch="emit('fetch')"
          @changed="emit('clear-error', 'models')"
        />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button :disabled="!!busy" @click="emit('update:modelValue', false)">
        {{ t('common.cancel') }}
      </el-button>
      <el-button type="primary" :loading="busy === 'save'" :disabled="!!busy" @click="emit('save')">
        {{ t('common.save') }}
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.form-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--wb-sp-3);
}
.hint {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
}
@media (max-width: 720px) {
  .form-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
