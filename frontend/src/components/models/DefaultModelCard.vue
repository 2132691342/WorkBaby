<script setup lang="ts">
// 默认模型：新会话从哪个模型起步。
// 与「当前会话的模型」是两件事——那个由引擎按会话记住，这个写在 settings.json 里，
// 管的是下一次开新会话时选谁。两者混在一个控件里，用户会以为改了就换掉进行中的对话。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import type { ModelDefaults, ProviderView } from '@/types/api'

const props = defineProps<{
  /** 已配置的服务商（含它们的模型）：默认模型只能从这里面选 */
  providers: ProviderView[]
}>()

const { t } = useI18n()
const toasts = useToasts()

/** pi 认的思考档；与引擎 get_available_thinking_levels 的口径一致。 */
const LEVELS = ['off', 'minimal', 'low', 'medium', 'high']

const saved = ref<ModelDefaults | null>(null)
const loading = ref(false)
const saving = ref(false)
// 草稿：保存前不动盘，取消即恢复已保存的那份。
const provider = ref('')
const modelId = ref('')
const level = ref('')

/** 服务商下拉项：按名字排序，两次打开的顺序才不会跳。 */
const providerOptions = computed(() =>
  [...props.providers].sort((a, b) => a.provider_name.localeCompare(b.provider_name)),
)

/** 当前选中的服务商下有哪些模型。 */
const modelOptions = computed(() => {
  const hit = props.providers.find((v) => v.provider_name === provider.value)
  return hit?.models ?? []
})

function applyDefaults(view: ModelDefaults): void {
  saved.value = view
  provider.value = view.default_provider ?? ''
  modelId.value = view.default_model ?? ''
  level.value = view.default_thinking_level ?? ''
}

async function load(): Promise<void> {
  loading.value = true
  try {
    applyDefaults(await api.modelDefaults())
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

/** 换服务商时清掉模型：另一个服务商下多半没有同名模型，留着就是提交一个不存在的组合。 */
function onProviderChange(next: string): void {
  provider.value = next
  modelId.value = ''
}

async function save(): Promise<void> {
  if (saving.value) return
  saving.value = true
  try {
    applyDefaults(
      await api.saveModelDefaults({
        default_provider: provider.value,
        default_model: modelId.value,
        default_thinking_level: level.value,
      }),
    )
    toasts.push('ok', t('common.saved'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    saving.value = false
  }
}

/** 清空：三个键一起撤掉，回到引擎自己的默认。 */
async function clear(): Promise<void> {
  provider.value = ''
  modelId.value = ''
  level.value = ''
  await save()
}

/** 草稿与已保存的是否一致：不一致才让保存按钮亮起来。 */
const dirty = computed(() => {
  const base = saved.value
  if (!base) return Boolean(provider.value || modelId.value || level.value)
  return (
    (base.default_provider ?? '') !== provider.value ||
    (base.default_model ?? '') !== modelId.value ||
    (base.default_thinking_level ?? '') !== level.value
  )
})

onMounted(() => {
  void load()
})
</script>

<template>
  <section>
    <div class="wb-sec-head">
      <span class="wb-sec-title">{{ t('models.defaultsTitle') }}</span>
      <span v-if="saved?.settings_file" class="wb-mono settings-path">{{ saved.settings_file }}</span>
    </div>
    <p class="hint">{{ t('models.defaultsHint') }}</p>

    <div class="defaults">
      <label class="field">
        <span class="field-label">{{ t('models.defaultProvider') }}</span>
        <el-select
          :model-value="provider"
          :loading="loading"
          :placeholder="t('models.defaultInherit')"
          clearable
          filterable
          @change="(v: string) => onProviderChange(v ?? '')"
        >
          <el-option
            v-for="item in providerOptions"
            :key="item.provider_name"
            :label="item.provider_name"
            :value="item.provider_name"
          />
        </el-select>
      </label>

      <label class="field">
        <span class="field-label">{{ t('models.defaultModel') }}</span>
        <el-select
          :model-value="modelId"
          :disabled="!provider"
          :placeholder="provider ? t('models.defaultPickModel') : t('models.defaultPickProviderFirst')"
          clearable
          filterable
          @change="(v: string) => (modelId = v ?? '')"
        >
          <el-option
            v-for="item in modelOptions"
            :key="item.model_id"
            :label="item.model_name || item.model_id"
            :value="item.model_id"
          />
        </el-select>
      </label>

      <label class="field">
        <span class="field-label">{{ t('models.defaultThinking') }}</span>
        <el-select
          :model-value="level"
          :placeholder="t('models.defaultInherit')"
          clearable
          @change="(v: string) => (level = v ?? '')"
        >
          <el-option v-for="item in LEVELS" :key="item" :label="item" :value="item" />
        </el-select>
      </label>

      <div class="field actions">
        <button class="wb-btn primary" :class="{ 'is-loading': saving }" :disabled="saving || !dirty" @click="save">
          <AppIcon name="check" :size="13" />
          {{ t('common.save') }}
        </button>
        <button class="wb-btn ghost" :disabled="saving || !dirty" @click="clear">
          {{ t('models.defaultClear') }}
        </button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.hint {
  margin: var(--wb-sp-1) 0 var(--wb-sp-3);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.settings-path {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

/* 三个选择器并排：它们是一条组合（服务商 → 模型 → 思考档），
   拆成两行就断了「先选谁再选谁」的读法。 */
.defaults {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: var(--wb-sp-3);
}

.field {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  min-width: 180px;
}

.field-label {
  font-size: var(--wb-fs-3xs);
  font-weight: 600;
  color: var(--wb-mute);
}

.actions {
  flex-direction: row;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
}
</style>
