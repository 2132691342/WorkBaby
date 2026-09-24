<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { Plus, Pencil, Trash2 } from '@/components/common/icons'
import { useSettingsStore } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useDialog } from '@/composables/useDialog'
import { t } from '@/i18n'
import PageState from '@/components/common/PageState.vue'
import FormDialog from '@/components/common/FormDialog.vue'
import Field from '@/components/common/Field.vue'
import type { AiProvider } from '@/types/api'

/**
 * 设置 · 模型 tab：新增模型（弹窗与编辑共用）、全局默认参数（温度/思考强度/压缩阈值/输入上限）、
 * 已配置模型表格（状态徽标 + 测试 / 编辑 / 删除）。
 */
const settings = useSettingsStore()
const toast = useToast()
const dialog = useDialog()
const { providers, providerKinds, providerTiers, providerPresets, loading, error, circuitStates, chatDefaults } = storeToRefs(settings)
const { load, updateProvider, removeProvider, testProvider, resetCircuit, loadChatDefaults, saveChatDefaults } = settings

onMounted(() => {
  void loadChatDefaults()
})

// ===== 统一新增 / 编辑弹窗 =====
type ProviderDraft = {
  name: string
  kind: string
  api_key: string
  base_url: string
  model: string
  alias: string
  tier: string
  enabled: boolean
  context_window: number | null
  max_output_tokens: number | null
  compress_ratio: number | null
  temperature: number | null
  top_p: number | null
  thinking_effort: string | null
  thinking_style: string
  supports_tool_call: boolean | null
  supports_vision: boolean | null
  supports_reasoning: boolean | null
}

const emptyDraft = (): ProviderDraft => ({
  name: '', kind: '', api_key: '', base_url: '', model: '', alias: '',
  tier: 'primary', enabled: true,
  context_window: null, max_output_tokens: null,
  compress_ratio: 0.75, temperature: null, top_p: null,
  thinking_effort: null, thinking_style: '',
  supports_tool_call: null, supports_vision: null, supports_reasoning: null
})

const dialogVisible = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const editingProviderID = ref<string | null>(null)
const draft = ref<ProviderDraft>(emptyDraft())
const dialogFeedback = ref<string | null>(null)
const saving = ref(false)

// 首启引导卡：未配置 Provider 时顶部突出展示预设服务，一键预填新建对话框。
// onboardingDismissed 仅会话内有效（不持久化）：用户主动关闭后再回来也只显示空表格。
const onboardingDismissed = ref(false)
const showOnboarding = computed(
  () => !loading.value && providers.value.length === 0 && providerPresets.value.length > 0 && !onboardingDismissed.value
)
// 只呈现 6 个最常用的预设：3 列 × 2 行正好一屏读完，再多就是让人挑花眼的墙
const topPresets = computed(() => providerPresets.value.slice(0, 6))

function pickPreset(name: string): void {
  openCreate()
  applyPreset(name)
}

function openCreate(): void {
  dialogMode.value = 'create'
  editingProviderID.value = null
  draft.value = emptyDraft()
  presetSelection.value = ''
  // 默认取第一个可用 kind（若尚未选择），减少一次必点操作
  if (!draft.value.kind && providerKinds.value.length > 0) {
    draft.value.kind = providerKinds.value[0]!.kind
  }
  dialogFeedback.value = null
  dialogVisible.value = true
}

// ===== 预设快捷填充：选中常见服务后预填 kind / base_url / 首选模型 =====
const presetSelection = ref('')

function applyPreset(name: string): void {
  presetSelection.value = name
  const preset = providerPresets.value.find((p) => p.name === name)
  if (!preset) return
  if (draft.value.name === '') draft.value.name = preset.name
  draft.value.kind = preset.kind
  draft.value.base_url = preset.base_url
  if (preset.models?.length) draft.value.model = preset.models[0]!
}

function startEditProvider(p: AiProvider): void {
  dialogMode.value = 'edit'
  editingProviderID.value = p.id
  draft.value = {
    name: p.name,
    kind: p.kind,
    api_key: '',
    base_url: p.base_url ?? '',
    model: p.model,
    alias: p.alias ?? '',
    // 历史脏值（如 tier1）回填时归一到后端枚举，避免保存后路由优先级失效
    tier: providerTiers.value.includes(p.tier) ? p.tier : providerTiers.value[0] ?? 'primary',
    enabled: p.enabled,
    context_window: p.context_window ?? null,
    max_output_tokens: p.max_output_tokens ?? null,
    compress_ratio: p.compress_ratio ?? 0.75,
    temperature: p.temperature ?? null,
    top_p: p.top_p ?? null,
    thinking_effort: p.thinking_effort ?? null,
    thinking_style: p.thinking_style ?? '',
    supports_tool_call: p.supports_tool_call ?? null,
    supports_vision: p.supports_vision ?? null,
    supports_reasoning: p.supports_reasoning ?? null
  }
  dialogFeedback.value = null
  dialogVisible.value = true
}

function closeDialog(): void {
  if (saving.value) return
  dialogVisible.value = false
  editingProviderID.value = null
  dialogFeedback.value = null
}

const dialogTitle = computed(() =>
  dialogMode.value === 'create' ? t('settings.addProvider') : t('settings.editProvider')
)

async function submitDialog(): Promise<void> {
  if (!draft.value.name.trim() || !draft.value.model.trim()) {
    dialogFeedback.value = t('settings.errNameModel')
    return
  }
  saving.value = true
  dialogFeedback.value = null
  try {
    const payload = {
      name: draft.value.name.trim(),
      kind: draft.value.kind,
      api_key: draft.value.api_key.trim() || undefined,
      base_url: draft.value.base_url.trim() || undefined,
      model: draft.value.model.trim(),
      alias: draft.value.alias.trim() || undefined,
      tier: draft.value.tier,
      enabled: draft.value.enabled,
      context_window: draft.value.context_window,
      max_output_tokens: draft.value.max_output_tokens,
      compress_ratio: draft.value.compress_ratio,
      temperature: draft.value.temperature,
      top_p: draft.value.top_p,
      thinking_effort: draft.value.thinking_effort,
      thinking_style: draft.value.thinking_style || null,
      supports_tool_call: draft.value.supports_tool_call,
      supports_vision: draft.value.supports_vision,
      supports_reasoning: draft.value.supports_reasoning
    }
    let ok: boolean
    if (dialogMode.value === 'create') {
      // 复用 store 校验 + 提示（缺 base_url / api_key 的提醒在 addProvider 内）
      ok = await settings.addProviderWith(payload)
    } else {
      ok = await updateProvider(editingProviderID.value!, payload)
    }
    if (ok) closeDialog()
    else dialogFeedback.value = error.value ?? null
  } finally {
    saving.value = false
  }
}

// ===== 保存默认参数 =====
async function handleSaveChatDefaults(): Promise<void> {
  chatDefaults.value = {
    default_temperature: chatDefaults.value.default_temperature ?? null,
    default_thinking: chatDefaults.value.default_thinking ?? null,
    compression_ratio: chatDefaults.value.compression_ratio ?? null,
    max_input_chars: chatDefaults.value.max_input_chars ?? null,
    max_run_tokens: chatDefaults.value.max_run_tokens ?? null
  }
  await saveChatDefaults()
}

// ===== Provider 行操作 =====
const testingID = ref<string | null>(null)
async function handleTestProvider(p: AiProvider): Promise<void> {
  if (testingID.value) return
  testingID.value = p.id
  const start = performance.now()
  toast.info(t('settings.testingProvider', p.name))
  try {
    const r = await testProvider(p.id)
    const ms = Math.round(performance.now() - start)
    if (r.ok) toast.success(t('settings.providerOk', p.name, ms))
    else toast.error(t('settings.providerFail', p.name, r.error ?? 'unknown', ms))
  } finally {
    testingID.value = null
  }
}
async function handleRemoveProvider(p: AiProvider): Promise<void> {
  const ok = await dialog.confirm({
    title: t('settings.providerDeleteTitle'),
    content: t('settings.providerDeleteConfirm', p.name),
    danger: true
  })
  if (!ok) return
  try {
    await removeProvider(p.id)
    toast.success(t('settings.providerDeleted'))
  } catch (e) {
    toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
  }
}
async function handleResetCircuit(id: string): Promise<void> {
  if (await resetCircuit(id)) toast.success(t('settings.circuitReset'))
  else toast.error(error.value ?? t('common.saveFailed'))
}

function circuitBadge(p: AiProvider): { label: string; cls: string } | null {
  const s = circuitStates.value.get(p.id)
  if (!s) return null
  if (!p.enabled) return { label: t('common.disabled'), cls: 'b-neutral' }
  if (s.state === 'OPEN') return { label: t('provider.unavailable', s.reason || t('provider.not_ready')), cls: 'b-danger' }
  if (s.state === 'HALF_OPEN') return { label: t('provider.half_open'), cls: 'b-warning' }
  return null
}

// ===== Kind 元数据 =====
const kinds = computed(() => providerKinds.value)
const baseUrlPlaceholder = computed(() => {
  const k = kinds.value.find((x) => x.kind === draft.value.kind)
  return k?.base_url_default ?? 'https://api.openai.com/v1'
})
const modelPlaceholder = computed(() => {
  const k = kinds.value.find((x) => x.kind === draft.value.kind)
  return k?.model_placeholder ?? 'gpt-4o'
})

// 切换 kind：自动填 base_url 默认值（模型必须用户自己填）
watch(() => draft.value.kind, (kind) => {
  const k = kinds.value.find((x) => x.kind === kind)
  if (!k) return
  if (!draft.value.base_url && k.base_url_default) draft.value.base_url = k.base_url_default
})

// ===== 三态能力（自动 / 启用 / 禁用）=====
const capabilityOptions = [
  { value: null, label: 'settings.capAuto' },
  { value: true, label: 'common.enabled' },
  { value: false, label: 'common.disabled' }
]
function capLabel(v: boolean | null | undefined): string {
  if (v === true) return t('common.enabled')
  if (v === false) return t('common.disabled')
  return t('settings.capAuto')
}

// ===== 思维强度 / 协议方言 =====
const effortOptions = [
  { value: null, label: 'settings.thinkingEffortDefault' },
  { value: 'off', label: 'chat.effort.off' },
  { value: 'low', label: 'chat.effort.low' },
  { value: 'medium', label: 'chat.effort.medium' },
  { value: 'high', label: 'chat.effort.high' }
]
const styleOptions = [
  { value: '', label: 'settings.thinkingStyleAuto' },
  { value: 'none', label: 'settings.thinkingStyleNone' },
  { value: 'enabled', label: 'settings.thinkingStyleEnabled' },
  { value: 'adaptive', label: 'settings.thinkingStyleAdaptive' },
  { value: 'reasoning_effort', label: 'settings.thinkingStyleEffort' },
  { value: 'enable_thinking', label: 'settings.thinkingStyleEnableBool' }
]

defineExpose({ load })
</script>

<template>
  <!-- 自带滚动与页边距：本页内容长于视口，交给 .set-body/.set-page（设置区既有的滚动与页距语法） -->
  <div class="set-body wb-ui">
    <div class="set-page page-stack">
      <!-- 范围说明 + 新增入口：标题与图标由设置外壳的 hero 头给出，此处不重复页名 -->
      <div class="flex-r page-lead">
        <p class="fs12 muted">{{ t('settings.modelsHint') }}</p>
        <span class="sp" />
        <button class="btn btn-primary" @click="openCreate">
          <Plus class="ic ic-sm" />
          {{ t('settings.addProvider') }}
        </button>
      </div>

    <!-- 全局默认参数 -->
    <section class="card">
      <h2>{{ t('settings.chatDefaults.title') }}</h2>
      <p class="fs11 muted mt4">{{ t('settings.chatDefaults.desc') }}</p>
      <div class="form-grid mt14">
        <div class="field">
          <label>{{ t('settings.chatDefaults.temperature') }}</label>
          <input v-model.number="chatDefaults.default_temperature" class="input mono" type="number" min="0" max="2" step="0.05" :placeholder="t('settings.chatDefaults.builtin')" />
          <p class="hint">{{ t('settings.chatDefaults.temperatureTip') }}</p>
        </div>
        <div class="field">
          <label>{{ t('settings.chatDefaults.thinking') }}</label>
          <select v-model="chatDefaults.default_thinking" class="input">
            <option :value="null">—</option>
            <option value="off">{{ t('chat.effort.off') }}</option>
            <option value="low">{{ t('chat.effort.low') }}</option>
            <option value="medium">{{ t('chat.effort.medium') }}</option>
            <option value="high">{{ t('chat.effort.high') }}</option>
          </select>
          <p class="hint">{{ t('settings.chatDefaults.thinkingTip') }}</p>
        </div>
        <div class="field">
          <label>{{ t('settings.chatDefaults.compression') }}</label>
          <input v-model.number="chatDefaults.compression_ratio" class="input mono" type="number" min="0.1" max="1" step="0.01" :placeholder="t('settings.chatDefaults.builtin')" />
          <p class="hint">{{ t('settings.chatDefaults.compressionTip') }}</p>
        </div>
        <div class="field">
          <label>{{ t('settings.chatDefaults.maxInput') }}</label>
          <input v-model.number="chatDefaults.max_input_chars" class="input mono" type="number" min="100" step="1000" :placeholder="t('settings.chatDefaults.builtin')" />
          <p class="hint">{{ t('settings.chatDefaults.maxInputTip') }}</p>
        </div>
        <div class="field">
          <label>{{ t('settings.chatDefaults.maxRunTokens') }}</label>
          <input v-model.number="chatDefaults.max_run_tokens" class="input mono" type="number" min="0" step="1000" :placeholder="t('settings.chatDefaults.builtin')" />
          <p class="hint">{{ t('settings.chatDefaults.maxRunTokensTip') }}</p>
        </div>
      </div>
      <div class="flex-r mt14">
        <span class="sp" />
        <button class="btn btn-primary" @click="handleSaveChatDefaults">{{ t('ui.btn.save') }}</button>
      </div>
    </section>

    <!-- 首启引导：未配置 Provider 时出现，预设一键预填新建对话框（中性卡：强调色只用于按钮与链接） -->
    <section v-if="showOnboarding" class="card">
      <div class="flex-r mb10">
        <div>
          <h2>{{ t('settings.onboardTitle') }}</h2>
          <p class="fs11 muted mt4">{{ t('settings.onboardHint') }}</p>
        </div>
        <span class="sp" />
        <button class="btn-ghost fs12" @click="onboardingDismissed = true">{{ t('settings.onboardSkip') }}</button>
      </div>
      <div class="preset-grid">
        <button
          v-for="p in topPresets"
          :key="p.name"
          type="button"
          class="preset"
          @click="pickPreset(p.name)"
        >
          <span class="flex-r" style="gap: 8px">
            <b class="fs12 truncate">{{ p.name }}</b>
            <span class="sp" />
            <span class="badge b-neutral">{{ p.kind }}</span>
          </span>
          <span class="fs11 muted mono truncate">{{ p.base_url }}</span>
          <span v-if="p.models?.[0]" class="fs11 muted truncate">{{ p.models?.[0] }}</span>
          <span class="preset-go fs11">{{ t('settings.onboardUse') }} →</span>
        </button>
      </div>
      <div class="flex-r mt14" style="gap: 12px">
        <p class="fs11 muted grow">{{ t('settings.onboardProtocolHint') }}</p>
        <button class="btn btn-sm" @click="openCreate">{{ t('settings.onboardManual') }}</button>
      </div>
    </section>

    <!-- 已配置模型 -->
    <section class="card p-sm">
      <div class="flex-r mb10">
        <h2>{{ t('settings.models') }} <span v-if="providers.length > 0" class="fs12 muted mono">({{ providers.length }})</span></h2>
      </div>

      <PageState
        compact
        :loading="loading"
        :empty="providers.length === 0"
        :error="error"
        :empty-text="t('settings.empty')"
        :empty-hint="t('settings.emptyHint')"
        @retry="load"
      />

      <div v-if="!loading && providers.length > 0" class="tbl-wrap">
        <table class="tbl">
          <thead>
            <tr>
              <th style="min-width: 220px">{{ t('settings.name') }}</th>
              <th>{{ t('settings.model') }}</th>
              <th class="ta-r">{{ t('settings.context_window') }}</th>
              <th>{{ t('settings.capToolCall') }}</th>
              <th>{{ t('settings.capVision') }}</th>
              <th>{{ t('settings.capReasoning') }}</th>
              <th>{{ t('common.status') }}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in providers" :key="p.id">
              <td>
                <div class="flex-r" style="gap: 8px">
                  <span style="font-weight: 500">{{ p.name }}</span>
                  <span class="badge b-neutral">{{ p.kind }}</span>
                </div>
              </td>
              <td><span class="mono">{{ p.model }}</span></td>
              <td class="ta-r mono muted">{{ p.context_window?.toLocaleString() ?? '—' }}</td>
              <td><span class="badge" :class="p.supports_tool_call === true ? 'b-success' : p.supports_tool_call === false ? 'b-danger' : 'b-neutral'">{{ capLabel(p.supports_tool_call) }}</span></td>
              <td><span class="badge" :class="p.supports_vision === true ? 'b-success' : p.supports_vision === false ? 'b-danger' : 'b-neutral'">{{ capLabel(p.supports_vision) }}</span></td>
              <td><span class="badge" :class="p.supports_reasoning === true ? 'b-success' : p.supports_reasoning === false ? 'b-danger' : 'b-neutral'">{{ capLabel(p.supports_reasoning) }}</span></td>
              <td>
                <span
                  v-if="circuitBadge(p)"
                  class="badge clickable"
                  :class="circuitBadge(p)!.cls"
                  :title="t('settings.circuitReset')"
                  @click="handleResetCircuit(p.id)"
                >{{ circuitBadge(p)!.label }}</span>
                <span v-else-if="p.enabled" class="badge b-success">{{ t('common.enabled') }}</span>
                <span v-else class="badge b-neutral">{{ t('common.disabled') }}</span>
              </td>
              <td>
                <div class="tbl-actions">
                  <button class="btn-icon" :title="t('settings.test')" :disabled="testingID === p.id" @click="handleTestProvider(p)">
                    <span v-if="testingID === p.id" class="ic ic-sm spin">⟳</span>
                    <span v-else class="ic ic-sm">▶</span>
                  </button>
                  <button class="btn-icon" :title="t('ui.btn.edit')" @click="startEditProvider(p)"><Pencil class="ic ic-sm" /></button>
                  <button class="btn-icon" style="color: var(--wb-danger)" :title="t('ui.btn.delete')" @click="handleRemoveProvider(p)"><Trash2 class="ic ic-sm" /></button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      </section>

      <!-- 新增 / 编辑统一弹窗（720px + 28px 控件 + 12px 字号，与全局表单一致） -->
      <FormDialog
      v-model="dialogVisible"
      :title="dialogTitle"
      :submitting="saving"
      @confirm="submitDialog"
      @close="closeDialog"
    >
      <!-- 单列栅格：每个输入项/下拉框独占整行，宽度一致、左缘对齐 -->
      <div class="wb-fgrid wb-fgrid--1">
        <p class="wb-fsect__title">{{ t('settings.section.basic') }}</p>
        <Field v-if="dialogMode === 'create' && providerPresets.length > 0" :label="t('settings.providerPreset')">
          <select class="input" :value="presetSelection" @change="applyPreset(($event.target as HTMLSelectElement).value)">
            <option value="">{{ t('settings.providerPresetCustom') }}</option>
            <option v-for="p in providerPresets" :key="p.name" :value="p.name">{{ p.name }}</option>
          </select>
        </Field>
        <Field :label="t('settings.name')" required :error="dialogFeedback ?? undefined">
          <input v-model="draft.name" class="input" :placeholder="t('settings.namePlaceholder')" />
        </Field>
        <Field v-if="dialogMode === 'create'" :label="t('settings.kindTitle')">
          <select v-model="draft.kind" class="input">
            <option v-for="k in kinds" :key="k.kind" :value="k.kind">{{ k.label }}</option>
          </select>
        </Field>
        <Field :label="t('settings.alias')">
          <input v-model="draft.alias" class="input" :placeholder="t('settings.aliasPlaceholder')" />
        </Field>
        <Field :label="t('settings.tier')">
          <select v-model="draft.tier" class="input">
            <option v-for="tr in providerTiers" :key="tr" :value="tr">{{ t(`settings.tier_${tr}`) }}</option>
          </select>
          <p class="mt-1 fs12 text-wb-muted">{{ t('settings.tierHint') }}</p>
        </Field>
        <Field :label="t('common.status')">
          <div class="flex-r" style="cursor: pointer" @click="draft.enabled = !draft.enabled">
            <span class="switch" :class="{ on: draft.enabled }" />
            <span class="fs12">{{ draft.enabled ? t('common.enabled') : t('common.disabled') }}</span>
          </div>
        </Field>

        <p class="wb-fsect__title">{{ t('settings.section.connection') }}</p>
        <Field :label="t('settings.api_key')">
          <input v-model="draft.api_key" class="input" type="password" :placeholder="dialogMode === 'edit' ? t('settings.apiKeyPlaceholderEdit') : t('settings.apiKeyPlaceholder')" />
        </Field>
        <Field :label="t('settings.base_url')" :hint="t('settings.baseUrlHint')">
          <input v-model="draft.base_url" class="input mono" :placeholder="baseUrlPlaceholder" />
        </Field>
        <Field :label="t('settings.model')" required :hint="t('settings.modelHint')">
          <input v-model="draft.model" class="input mono" :placeholder="modelPlaceholder" />
        </Field>

        <p class="wb-fsect__title">{{ t('settings.section.behavior') }}</p>
        <Field :label="t('settings.context_window')" :hint="t('settings.contextWindowHint')">
          <input v-model.number="draft.context_window" class="input mono" type="number" min="1" placeholder="128000" />
        </Field>
        <Field :label="t('settings.max_output_tokens')" :hint="t('settings.maxOutputTokensTip')">
          <input v-model.number="draft.max_output_tokens" class="input mono" type="number" min="1" placeholder="8192" />
        </Field>
        <Field :label="t('settings.compress_ratio')" :hint="t('settings.compressRatioTip')">
          <input v-model.number="draft.compress_ratio" class="input mono" type="number" min="0.1" max="1" step="0.05" />
        </Field>
        <Field :label="t('settings.providerTemperature')">
          <input v-model.number="draft.temperature" class="input mono" type="number" min="0" max="2" step="0.05" :placeholder="t('settings.providerTemperatureDefault')" />
        </Field>
        <Field :label="t('settings.top_p')" :hint="t('settings.topPTip')">
          <input v-model.number="draft.top_p" class="input mono" type="number" min="0" max="1" step="0.05" placeholder="0.75" />
        </Field>
        <Field :label="t('settings.thinking_effort')">
          <select v-model="draft.thinking_effort" class="input">
            <option v-for="o in effortOptions" :key="String(o.value)" :value="o.value">{{ t(o.label) }}</option>
          </select>
        </Field>
        <Field :label="t('settings.thinking_style')" :hint="t('settings.thinkingStyleTip')">
          <select v-model="draft.thinking_style" class="input">
            <option v-for="s in styleOptions" :key="s.value" :value="s.value">{{ t(s.label) }}</option>
          </select>
        </Field>

        <p class="wb-fsect__title">{{ t('settings.section.capabilities') }}</p>
        <Field :label="t('settings.capToolCall')">
          <select v-model="draft.supports_tool_call" class="input">
            <option v-for="o in capabilityOptions" :key="String(o.value)" :value="o.value">{{ t(o.label) }}</option>
          </select>
        </Field>
        <Field :label="t('settings.capVision')">
          <select v-model="draft.supports_vision" class="input">
            <option v-for="o in capabilityOptions" :key="String(o.value)" :value="o.value">{{ t(o.label) }}</option>
          </select>
        </Field>
        <Field :label="t('settings.capReasoning')">
          <select v-model="draft.supports_reasoning" class="input">
            <option v-for="o in capabilityOptions" :key="String(o.value)" :value="o.value">{{ t(o.label) }}</option>
          </select>
        </Field>
        <p class="fs11 muted" style="grid-column: 1 / -1; margin: 0">{{ t('settings.capabilitiesHint') }}</p>
      </div>
      </FormDialog>
    </div>
  </div>
</template>

<style scoped>
@keyframes wb-spin {
  to { transform: rotate(360deg); }
}
.spin {
  display: inline-block;
  animation: wb-spin 0.9s linear infinite;
}
.badge.clickable {
  cursor: pointer;
}

/* 只补栅格节奏：宽度与页边距沿用全局 .set-page，避免同名覆盖造成页距漂移 */
.page-stack {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-5);
}
.page-lead {
  gap: var(--wb-sp-3);
}

/* 预设网格：固定 3 列（一行 3 个正好读完，不再 4 列挤压 URL），窄窗口降 2 列 */
.preset-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--wb-sp-3);
}
@media (max-width: 1180px) {
  .preset-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
.preset {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 4px;
  padding: 10px 12px;
  border: 1px solid var(--wb-line-2);
  border-radius: var(--wb-radius);
  background: var(--wb-surface-2);
  text-align: left;
  cursor: pointer;
  transition: border-color var(--wb-dur-fast) var(--wb-ease), background var(--wb-dur-fast) var(--wb-ease);
}
/* 悬浮只换描边与底色（不做位移），与全站卡片同一套动效语言 */
.preset:hover {
  border-color: var(--wb-primary);
  background: var(--wb-surface);
}
.preset-go {
  color: var(--wb-primary);
  margin-top: 2px;
}
</style>
