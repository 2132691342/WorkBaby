<script setup lang="ts">
import { errorText } from '../../utils/errors'
// 模型服务面板：读写的 models.json；服务商卡片、接口类型、密钥与模型清单都在这里。
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { ModelREQ, ProviderVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import ProviderDialog from './ProviderDialog.vue'
import SettingsSection from './SettingsSection.vue'

const { t } = useI18n()
const store = useSettingsStore()
const toast = useToastStore()

const editing = ref('')
const busy = ref('')
const dialog = ref(false)

// 模型能力的兵底：留空不能等于 0。0 会让水位环永远显示 0%，
// 后端同样拿不到可用预算。与 domain.DefaultContextWindow / DefaultMaxTokens 一致。
const DEFAULT_CONTEXT_WINDOW = 300000
const DEFAULT_MAX_TOKENS = 64000

const form = reactive({
  id: '',
  name: '',
  api: 'openai-completions',
  base_url: '',
  api_key: '',
  models: [] as ModelREQ[],
})
const fetched = ref<string[]>([])
// 字段级错误：EP 的 el-form-item 接 error 就变红并显示这句，不再用 toast 说一句含糊的「校验失败」
const errors = reactive({ name: '', base_url: '', models: '' })

const providers = computed(() => store.providers ?? [])
// 默认模型只活在引导快照里：setDefault 之后由 refreshBoot 重拉
const defaultId = computed(() => store.boot?.boot_default_provider || '')

function reset() {
  editing.value = ''
  form.id = ''
  form.name = ''
  form.api = 'openai-completions'
  form.base_url = ''
  form.api_key = ''
  form.models = []
  fetched.value = []
  clearErrors()
}

function clearErrors() {
  errors.name = ''
  errors.base_url = ''
  errors.models = ''
}

/**
 * 校验顺序 = 界面上字段的顺序：一次只报第一个不对的地方，
 * 否则用户得来回试三次才知道到底差什么（名字与模型 ID 都空时报模型 ID 是最招人烦的）。
 */
function validate(): boolean {
  clearErrors()
  if (!form.name.trim()) errors.name = t('settings.providers.nameRequired')
  if (!form.base_url.trim()) errors.base_url = t('settings.providers.baseUrlRequired')
  if (!form.models.length) {
    errors.models = t('settings.providers.modelsRequired')
  } else if (form.models.some((m) => !m.model_id.trim())) {
    errors.models = t('settings.providers.modelIdRequired')
  }
  return !errors.name && !errors.base_url && !errors.models
}

function createNew() {
  reset()
  dialog.value = true
}

function edit(p: ProviderVO) {
  editing.value = p.provider_id
  form.id = p.provider_id
  form.name = p.provider_name
  form.api = p.provider_api || 'openai-completions'
  form.base_url = p.provider_base_url
  form.api_key = ''
  form.models = p.provider_models.map((m) => ({
    model_id: m.model_id,
    model_name: m.model_name,
    model_reasoning: m.model_reasoning,
    model_vision: m.model_vision,
    model_context_window: m.model_context_window,
    model_max_tokens: m.model_max_tokens,
  }))
  fetched.value = []
  dialog.value = true
}

async function fetchModels() {
  if (busy.value) return
  busy.value = 'fetch'
  try {
    const resp = await api.providers.fetchModels({
      api: form.api,
      base_url: form.base_url,
      api_key: form.api_key || undefined,
      id: form.id || undefined,
    })
    fetched.value = resp.resp_models ?? []
    toast.ok(t('settings.providers.fetched', { n: fetched.value.length }))
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

async function save() {
  if (busy.value) return
  if (!validate()) {
    toast.bad(t('settings.providers.fixHint'))
    return
  }
  busy.value = 'save'
  try {
    await api.providers.upsert({
      req_provider_id: form.id || undefined,
      req_provider_name: form.name.trim(),
      req_provider_api: form.api,
      req_provider_base_url: form.base_url.trim(),
      req_provider_api_key: form.api_key || undefined,
      // 清空输入框时 el-input-number 会给 null / 0：落盘前补回默认值，
      // 否则后端只能收到 0，水位与预算全跟着失效。
      req_models: form.models.map((m) => ({
        ...m,
        model_id: m.model_id.trim(),
        model_name: m.model_name.trim() || m.model_id.trim(),
        model_context_window: m.model_context_window > 0 ? m.model_context_window : DEFAULT_CONTEXT_WINDOW,
        model_max_tokens: m.model_max_tokens > 0 ? m.model_max_tokens : DEFAULT_MAX_TOKENS,
      })),
    })
    await store.refreshProviderData()
    toast.ok(t('common.save'))
    dialog.value = false
    reset()
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

async function remove(p: ProviderVO) {
  if (busy.value) return
  busy.value = `del:${p.provider_id}`
  try {
    await api.providers.remove(p.provider_id)
    await store.refreshProviderData()
    if (editing.value === p.provider_id) {
      dialog.value = false
      reset()
    }
    toast.ok(`${t('common.delete')} ${p.provider_name}`)
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

async function test(p: ProviderVO) {
  if (busy.value) return
  busy.value = `test:${p.provider_id}`
  try {
    const r = await api.providers.test(p.provider_id)
    if (r.resp_ok) toast.ok(t('settings.providers.testOk', { model: r.resp_model }))
    else toast.bad(t('settings.providers.testFail', { msg: r.resp_message }))
  } catch (e) {
    toast.bad(t('settings.providers.testFail', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

async function reveal(p: ProviderVO) {
  if (busy.value) return
  busy.value = `key:${p.provider_id}`
  try {
    const r = await api.providers.reveal(p.provider_id)
    try {
      await navigator.clipboard.writeText(r.resp_api_key || '')
      toast.ok(t('common.copied'))
    } catch {
      toast.info(r.resp_api_key || '—')
    }
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

async function setDefault(p: ProviderVO) {
  const model = p.provider_models[0]?.model_id
  if (!model) {
    toast.bad(t('settings.providers.invalid'))
    return
  }
  if (busy.value) return
  busy.value = `def:${p.provider_id}`
  try {
    await api.providers.setDefault(p.provider_id, model)
    await store.refreshBoot()
    await store.loadProviders()
    toast.ok(`${t('settings.providers.default')}: ${model}`)
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

function baseHost(url: string) {
  try {
    return new URL(url).host
  } catch {
    return url || '—'
  }
}

onMounted(() => {
  if (!store.providers) void store.loadProviders()
})
</script>

<template>
  <div class="panel-wrap">
    <SettingsSection
      icon="sparkles"
      :title="t('settings.providers.title')"
      :desc="t('settings.providers.desc')"
    >
      <template #actions>
        <el-button size="small" type="primary" @click="createNew">
          <AppIcon name="plus" size="ic-xs" />
          <span class="ml-1">{{ t('settings.providers.add') }}</span>
        </el-button>
      </template>

      <div v-if="store.providersLoading && !providers.length" class="grid gap-3">
        <el-skeleton animated :rows="3" class="card-box" />
      </div>

      <div v-else-if="!providers.length" class="wb-card empty-box">
        <span class="wb-tile wb-tile--lg"><AppIcon name="cpu" size="ic-xl" /></span>
        <b>{{ t('settings.providers.empty') }}</b>
        <p>{{ t('settings.providers.emptyHint') }}</p>
        <el-button size="small" type="primary" @click="createNew">{{ t('settings.providers.add') }}</el-button>
      </div>

      <div v-else class="prov-grid">
        <article v-for="p in providers" :key="p.provider_id" class="prov">
          <header class="prov-head">
            <span class="prov-avatar">{{ (p.provider_name || p.provider_id).slice(0, 1).toUpperCase() }}</span>
            <div class="prov-tx">
              <div class="prov-name">
                {{ p.provider_name || p.provider_id }}
                <el-tag v-if="p.provider_id === defaultId" size="small" type="primary">
                  {{ t('settings.providers.default') }}
                </el-tag>
              </div>
              <div class="prov-sub">{{ p.provider_api }} · {{ baseHost(p.provider_base_url) }}</div>
            </div>
            <el-tag :type="p.provider_has_key ? 'success' : 'info'" size="small">
              {{ p.provider_has_key ? t('settings.providers.key') : t('settings.providers.keyMissing') }}
            </el-tag>
          </header>

          <div class="prov-models">
            <el-tag v-for="m in p.provider_models.slice(0, 4)" :key="m.model_id" size="small" class="prov-model">
              {{ m.model_name || m.model_id }}
            </el-tag>
            <span v-if="p.provider_models.length > 4" class="prov-more">+{{ p.provider_models.length - 4 }}</span>
            <span v-if="!p.provider_models.length" class="prov-more">{{ t('common.empty') }}</span>
          </div>

          <footer class="prov-acts">
            <el-button
              size="small"
              :loading="busy === `test:${p.provider_id}`"
              :disabled="!!busy"
              @click="test(p)"
            >
              {{ t('settings.providers.test') }}
            </el-button>
            <el-button size="small" :disabled="!!busy || !p.provider_has_key" @click="reveal(p)">
              {{ t('settings.providers.reveal') }}
            </el-button>
            <el-button
              v-if="p.provider_id !== defaultId"
              size="small"
              :loading="busy === `def:${p.provider_id}`"
              :disabled="!!busy"
              @click="setDefault(p)"
            >
              {{ t('settings.providers.setDefault') }}
            </el-button>
            <span class="sp" />
            <el-tooltip :content="t('common.edit')" placement="top" :show-after="400">
              <el-button size="small" circle :disabled="!!busy" @click="edit(p)">
                <AppIcon name="pencil" size="ic-xs" />
              </el-button>
            </el-tooltip>
            <el-popconfirm
              :title="t('settings.providers.deleteTitle')"
              width="260"
              @confirm="remove(p)"
            >
              <template #reference>
                <el-button size="small" circle :disabled="!!busy">
                  <AppIcon name="trash" size="ic-xs" />
                </el-button>
              </template>
            </el-popconfirm>
          </footer>
        </article>
      </div>
    </SettingsSection>

    <ProviderDialog
      v-model="dialog"
      :editing="editing"
      :form="form"
      :errors="errors"
      :fetched="fetched"
      :busy="busy"
      :default-context-window="DEFAULT_CONTEXT_WINDOW"
      :default-max-tokens="DEFAULT_MAX_TOKENS"
      @fetch="fetchModels"
      @clear-error="(f: 'name' | 'base_url' | 'models') => (errors[f] = '')"
      @save="save"
    />
  </div>
</template>

<style scoped>
.prov-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(calc(320px * var(--wb-vw-scale)), 1fr));
  gap: var(--wb-sp-3);
}
.prov {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-4);
  border-radius: var(--wb-radius-xl);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  box-shadow: var(--wb-card-shadow);
  transition:
    box-shadow var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease);
}
.prov:hover {
  box-shadow: var(--wb-card-shadow-hover);
  border-color: var(--wb-border-strong);
}
.prov-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
}
.prov-avatar {
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border-radius: var(--wb-radius-sm);
  display: grid;
  place-items: center;
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
  font-weight: 700;
  flex: none;
}
.prov-tx {
  flex: 1;
  min-width: 0;
}
.prov-name {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.prov-sub {
  margin-top: 1px;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.prov-models {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--wb-sp-1);
}
.prov-model {
  font-family: var(--font-mono);
}
.prov-more {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.prov-acts {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  flex-wrap: wrap;
}
.empty-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-10) var(--wb-sp-6);
  text-align: center;
}
.empty-box b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.empty-box p {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  max-width: calc(420px * var(--wb-vw-scale));
  line-height: var(--wb-lh-base);
}
.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--wb-sp-3);
}
.hint {
  margin-top: var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.sp {
  flex: 1;
}
.ml-1 {
  margin-left: var(--wb-sp-1);
}
@media (max-width: 900px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
