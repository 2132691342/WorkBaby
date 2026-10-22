<script setup lang="ts">
// 模型服务面板：读写的就是 PI 的 models.json，界面与 pi 命令共用同一份配置。
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { ModelREQ, ProviderVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import SettingsSection from './SettingsSection.vue'

const { t } = useI18n()
const store = useSettingsStore()
const toast = useToastStore()

// 父级（环境向导）据此重检闸门
const emit = defineEmits<{ (e: 'changed'): void }>()

const APIS = [
  { key: 'openai-completions', label: 'OpenAI Completions' },
  { key: 'openai-responses', label: 'OpenAI Responses' },
  { key: 'anthropic-messages', label: 'Anthropic Messages' },
  { key: 'google-generative-ai', label: 'Google Generative AI' },
  { key: 'azure-openai-responses', label: 'Azure OpenAI Responses' },
]

const editing = ref('')
const busy = ref('')
const dialog = ref(false)

const form = reactive({
  id: '',
  name: '',
  api: 'openai-completions',
  base_url: '',
  api_key: '',
  models: [] as ModelREQ[],
})
const fetched = ref<string[]>([])
const manual = ref('')
// 字段级错误：EP 的 el-form-item 接 error 就变红并显示这句，不再用 toast 说一句含糊的「校验失败」
const errors = reactive({ name: '', base_url: '', models: '' })

const providers = computed(() => store.providers?.providers ?? [])
const defaultId = computed(() => store.providers?.default_provider || '')
const path = computed(() => store.providers?.path || '')

function reset() {
  editing.value = ''
  form.id = ''
  form.name = ''
  form.api = 'openai-completions'
  form.base_url = ''
  form.api_key = ''
  form.models = []
  fetched.value = []
  manual.value = ''
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
  } else if (form.models.some((m) => !m.id.trim())) {
    errors.models = t('settings.providers.modelIdRequired')
  }
  return !errors.name && !errors.base_url && !errors.models
}

const blankModel = (id: string) => !id.trim()

function createNew() {
  reset()
  dialog.value = true
}

function edit(p: ProviderVO) {
  editing.value = p.id
  form.id = p.id
  form.name = p.name
  form.api = p.api || 'openai-completions'
  form.base_url = p.base_url
  form.api_key = ''
  form.models = p.models.map((m) => ({
    id: m.id,
    name: m.name,
    reasoning: m.reasoning,
    vision: m.vision,
    context_window: m.context_window,
    max_tokens: m.max_tokens,
  }))
  fetched.value = []
  manual.value = ''
  dialog.value = true
}

function addModel(id: string) {
  const name = id.trim()
  if (!name || form.models.some((m) => m.id === name)) return
  form.models.push({ id: name, name, reasoning: false, vision: true, context_window: 0, max_tokens: 0 })
}

function addManual() {
  for (const line of manual.value.split(/[\s,]+/)) addModel(line)
  manual.value = ''
}

function dropModel(id: string) {
  form.models = form.models.filter((m) => m.id !== id)
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
    fetched.value = resp.models ?? []
    toast.ok(t('settings.providers.fetched', { n: fetched.value.length }))
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
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
      id: form.id || undefined,
      name: form.name.trim(),
      api: form.api,
      base_url: form.base_url.trim(),
      api_key: form.api_key || undefined,
      models: form.models.map((m) => ({ ...m, id: m.id.trim(), name: m.name.trim() || m.id.trim() })),
    })
    await store.refreshProviderData()
    toast.ok(t('common.save'))
    dialog.value = false
    reset()
    emit('changed')
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function remove(p: ProviderVO) {
  if (busy.value) return
  busy.value = `del:${p.id}`
  try {
    await api.providers.remove(p.id)
    await store.refreshProviderData()
    if (editing.value === p.id) {
      dialog.value = false
      reset()
    }
    toast.ok(`${t('common.delete')} ${p.name}`)
    emit('changed')
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function test(p: ProviderVO) {
  if (busy.value) return
  busy.value = `test:${p.id}`
  try {
    const r = await api.providers.test(p.id)
    if (r.ok) toast.ok(t('settings.providers.testOk', { model: r.model }))
    else toast.bad(t('settings.providers.testFail', { msg: r.message }))
  } catch (e) {
    toast.bad(t('settings.providers.testFail', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function reveal(p: ProviderVO) {
  if (busy.value) return
  busy.value = `key:${p.id}`
  try {
    const r = await api.providers.reveal(p.id)
    try {
      await navigator.clipboard.writeText(r.api_key || '')
      toast.ok(t('common.copied'))
    } catch {
      toast.info(r.api_key || '—')
    }
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function setDefault(p: ProviderVO) {
  const model = p.models[0]?.id
  if (!model) {
    toast.bad(t('settings.providers.invalid'))
    return
  }
  if (busy.value) return
  busy.value = `def:${p.id}`
  try {
    await api.providers.setDefault(p.id, model)
    await store.refreshBoot()
    await store.loadProviders()
    toast.ok(`${t('settings.providers.default')}: ${model}`)
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
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

      <p v-if="path" class="wb-mono">{{ path }}</p>

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
        <article v-for="p in providers" :key="p.id" class="prov">
          <header class="prov-head">
            <span class="prov-avatar">{{ (p.name || p.id).slice(0, 1).toUpperCase() }}</span>
            <div class="prov-tx">
              <div class="prov-name">
                {{ p.name || p.id }}
                <el-tag v-if="p.id === defaultId" size="small" type="primary">
                  {{ t('settings.providers.default') }}
                </el-tag>
              </div>
              <div class="prov-sub">{{ p.api }} · {{ baseHost(p.base_url) }}</div>
            </div>
            <el-tag :type="p.has_key ? 'success' : 'info'" size="small">
              {{ p.has_key ? t('settings.providers.key') : t('settings.providers.keyMissing') }}
            </el-tag>
          </header>

          <div class="prov-models">
            <el-tag v-for="m in p.models.slice(0, 4)" :key="m.id" size="small" class="prov-model">
              {{ m.name || m.id }}
            </el-tag>
            <span v-if="p.models.length > 4" class="prov-more">+{{ p.models.length - 4 }}</span>
            <span v-if="!p.models.length" class="prov-more">{{ t('common.empty') }}</span>
          </div>

          <footer class="prov-acts">
            <el-button
              size="small"
              :loading="busy === `test:${p.id}`"
              :disabled="!!busy"
              @click="test(p)"
            >
              {{ t('settings.providers.test') }}
            </el-button>
            <el-button size="small" :disabled="!!busy || !p.has_key" @click="reveal(p)">
              {{ t('settings.providers.reveal') }}
            </el-button>
            <el-button
              v-if="p.id !== defaultId"
              size="small"
              :loading="busy === `def:${p.id}`"
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

    <el-dialog
      v-model="dialog"
      :title="editing ? t('common.edit') : t('settings.providers.add')"
      :width="`calc(760px * var(--wb-vw-scale))`"
      destroy-on-close
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
              @input="errors.name = ''"
            />
            <p v-if="form.id" class="hint">{{ t('settings.providers.idLocked') }}</p>
          </el-form-item>
        </div>

        <el-form-item :label="t('settings.providers.baseUrl')" required :error="errors.base_url">
          <el-input
            v-model="form.base_url"
            placeholder="https://api.example.com/v1"
            :disabled="!!busy"
            @input="errors.base_url = ''"
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
          <div class="models">
            <div class="models-bar">
              <el-input
                v-model="manual"
                size="small"
                :placeholder="t('settings.providers.modelId')"
                @keydown.enter="addManual"
              />
              <el-button size="small" :disabled="!!busy" @click="addManual">{{ t('common.add') }}</el-button>
              <el-button size="small" :loading="busy === 'fetch'" :disabled="!!busy" @click="fetchModels">
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
                  :class="{ 'is-on': form.models.some((x) => x.id === m) }"
                  @click="addModel(m)"
                >
                  {{ m }}
                </button>
              </div>
            </div>

            <ul v-if="form.models.length" class="models-list">
              <li v-for="m in form.models" :key="m.id || form.models.indexOf(m)" class="model-card">
                <div class="model-head">
                  <el-input
                    v-model="m.id"
                    size="small"
                    class="w-id"
                    :class="{ 'is-bad': blankModel(m.id) }"
                    :placeholder="t('settings.providers.modelId')"
                    @input="errors.models = ''"
                  />
                  <el-input v-model="m.name" size="small" class="w-name" :placeholder="t('settings.providers.modelName')" />
                  <button class="model-del" type="button" :title="t('common.delete')" @click="dropModel(m.id)">
                    <AppIcon name="close" size="ic-xs" />
                  </button>
                </div>
                <!-- 第二行只放读数：两行分开之后每个控件都有足够宽度，不再被挤掉 -->
                <div class="model-meta">
                  <label class="model-field">
                    <span class="model-cap">{{ t('settings.providers.contextWindow') }}</span>
                    <el-input-number
                      v-model="m.context_window"
                      size="small"
                      :min="0"
                      :step="1000"
                      :controls="false"
                      class="w-num"
                    />
                  </label>
                  <label class="model-field">
                    <span class="model-cap">{{ t('settings.providers.maxTokens') }}</span>
                    <el-input-number
                      v-model="m.max_tokens"
                      size="small"
                      :min="0"
                      :step="1000"
                      :controls="false"
                      class="w-num"
                    />
                  </label>
                  <el-checkbox v-model="m.reasoning" size="small">
                    {{ t('settings.providers.reasoning') }}
                  </el-checkbox>
                  <el-checkbox v-model="m.vision" size="small">
                    {{ t('settings.providers.vision') }}
                  </el-checkbox>
                </div>
              </li>
            </ul>
            <p v-else class="hint">{{ t('settings.providers.modelsRequired') }}</p>
          </div>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button :disabled="!!busy" @click="dialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="busy === 'save'" :disabled="!!busy" @click="save">
          {{ t('common.save') }}
        </el-button>
      </template>
    </el-dialog>
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
