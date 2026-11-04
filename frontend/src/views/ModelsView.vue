<script setup lang="ts">
// 模型页：服务商（可写）/ 引擎眼下的模型（只读）。
// 两块的分界线是「能不能改」：可写的与只读的不混一张表；
// 编辑草稿与上游探测在 useProviderForm，这一页只管读哪些数据、各块摆哪。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import AppIcon from '@/components/common/AppIcon.vue'
import ProviderDialog from '@/components/models/ProviderDialog.vue'
import ModelTable from '@/components/models/ModelTable.vue'
import ProviderList from '@/components/models/ProviderList.vue'
import { useProviderForm } from '@/composables/useProviderForm'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import { useAppStore } from '@/stores/app'
import type { ProviderView } from '@/types/api'

const { t } = useI18n()
const toasts = useToasts()
const app = useAppStore()

const providers = ref<ProviderView[]>([])
const configPath = ref('')
const readError = ref('')
const loadingProviders = ref(false)
// 正在跑的上游探测：'test:<服务商>'。拉模型在编辑弹窗里做，不占这个标志。
const probing = ref('')

/** 可选的服务商协议。 */
const apiOptions = [
  'openai',
  'anthropic',
  'google',
]

/** pi 就绪才谈得上「配模型」。 */
const piInstalled = computed(() => Boolean(app.runtime?.pi_ready))

async function loadProviders() {
  loadingProviders.value = true
  try {
    const view = await api.providers()
    providers.value = view.providers ?? []
    configPath.value = view.config_path
    readError.value = view.read_error ?? ''
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loadingProviders.value = false
  }
}

/** 重拉：保存服务商之后配置变了，两块都要跟上。 */
async function reloadAll(): Promise<void> {
  await loadProviders()
}

const form = useProviderForm(reloadAll)

/**
 * 测一条服务商配置通不通。
 *
 * 判据是「上游的模型清单能不能问出来」——它一次验证了地址、协议与密钥，
 * 而这三样是配错时最容易混在一起的。
 */
async function testProvider(view: ProviderView): Promise<void> {
  if (probing.value) return
  probing.value = `test:${view.provider_name}`
  try {
    const res = await api.probeProvider({ provider_name: view.provider_name })
    if (res.probe_ok) {
      toasts.push('ok', t('models.testOk', { n: res.model_count }))
    } else {
      toasts.push('error', res.probe_message || t('models.testFail'))
    }
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    probing.value = ''
  }
}

async function remove(view: ProviderView): Promise<void> {
  try {
    await ElMessageBox.confirm(t('models.deleteConfirm', { name: view.provider_name }), t('models.deleteTitle'), {
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    await api.deleteProvider(view.provider_name)
    toasts.push('ok', t('common.saved'))
    await reloadAll()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

/** 只读的「可用模型」：直接从服务商清单里摊平，不再多一次往返。 */
const models = computed(() =>
  providers.value.flatMap((view) =>
    view.models.map((entry) => ({
      provider_name: view.provider_name,
      model_id: entry.model_id,
      model_name: entry.model_name || entry.model_id,
      api_protocol: entry.api_protocol || view.api_protocol,
      context_window: entry.context_window ?? 0,
      max_tokens: entry.max_tokens ?? 0,
      supports_reasoning: Boolean(entry.supports_reasoning),
      input_kinds: entry.input_kinds,
    })),
  ),
)

onMounted(() => {
  void reloadAll()
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('models.title') }}</h1>
        <p class="page-sub">{{ t('models.subtitle') }}</p>
      </div>
      <div class="head-actions">
        <button
          class="wb-btn icon ghost"
          :class="{ 'is-loading': loadingProviders }"
          :title="t('common.refresh')"
          @click="loadProviders"
        >
          <AppIcon name="refresh" :size="15" />
        </button>
        <button class="wb-btn primary" :disabled="!piInstalled" @click="form.openCreate()">
          <AppIcon name="plus" :size="14" />
          {{ t('models.addProvider') }}
        </button>
      </div>
    </header>

    <div class="page-body">
      <p v-if="readError" class="notice">
        <AppIcon name="alert" :size="14" />
        {{ readError }}
      </p>

      <ProviderList
        :providers="providers"
        :loading="loadingProviders"
        :config-path="configPath"
        :pending="probing"
        @create="form.openCreate()"
        @edit="form.openEdit"
        @remove="remove"
        @test="testProvider"
      />

      <ModelTable :models="models" :engine-missing="!piInstalled" />
    </div>

    <ProviderDialog
      v-model="form.dialogOpen.value"
      :form="form.form.value"
      :editing="form.editing.value"
      :name-locked="form.nameLocked.value"
      :saving="form.saving.value"
      :fetching="form.fetching.value"
      :fetched="form.fetched.value"
      :api-options="apiOptions"
      @save="form.save"
      @fetch-models="form.fetchModels"
    />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

/* 页头：标题与说明在左，动作在右。说明文字限宽——
   一整行 1400px 的灰色说明句没人读。 */
.page-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-4);
  padding: var(--wb-sp-4) var(--wb-sp-6);
  border-bottom: 1px solid var(--wb-line);
  flex: none;
}

.page-title {
  margin: 0;
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 650;
  color: var(--wb-ink);
}

.page-sub {
  margin: 2px 0 0;
  max-width: var(--wb-w-desc);
  font-size: var(--wb-fs-2xs);
  line-height: 1.55;
  color: var(--wb-mute);
}

.head-actions {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: none;
}

.page-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-10);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-6);
}

.notice {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin: 0;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-r);
  background: var(--wb-warn-soft);
  color: var(--wb-warn);
  font-size: var(--wb-fs-xs);
}
</style>
