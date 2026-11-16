<script setup lang="ts">
// 模型页：服务商（可写）/ 引擎眼下的模型（只读）。
// 两块的分界线是「能不能改」：可写的与只读的不混一张表；
// 编辑草稿与上游探测在 useProviderForm，这一页只管读哪些数据、各块摆哪。
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { confirmDanger } from '@/utils/confirm'
import AppIcon from '@/components/common/AppIcon.vue'
import ProviderDialog from '@/components/models/ProviderDialog.vue'
import ModelTable from '@/components/models/ModelTable.vue'
import ProviderList from '@/components/models/ProviderList.vue'
import DefaultModelCard from '@/components/models/DefaultModelCard.vue'
import WbSkeleton from '@/components/common/WbSkeleton.vue'
import { useProviderForm } from '@/composables/useProviderForm'
import { api, abortable } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import { useAppStore } from '@/stores/app'
import { useChatStore } from '@/stores/chat'
import type { ProviderView } from '@/types/api'

const { t } = useI18n()
const toasts = useToasts()
const app = useAppStore()
const chat = useChatStore()

const providers = ref<ProviderView[]>([])
const configPath = ref('')
const readError = ref('')
// 首屏取数是否失败：失败时不能显示「还没有配置服务商」——那是假断言。
const loadFailed = ref(false)
const loadingProviders = ref(false)
// 正在跑的上游探测：'test:<服务商>'。拉模型在编辑弹窗里做，不占这个标志。
const probing = ref('')
// 正在行内删除的模型：'服务商:模型ID'
const removingModel = ref('')
// 竞态守卫：重拉或离开页面后，晚到的旧响应直接丢弃。
let loadSeq = 0
let aborter: AbortController | null = null

onUnmounted(() => {
  loadSeq += 1
  aborter?.abort('unmount')
  aborter = null
})

/** 可选的服务商协议。 */
const apiOptions = [
  'openai',
  'anthropic',
  'google',
]

/** 引擎就绪才谈得上「配模型」：内置引擎恒就绪，所以这里恒真；
 *  留这个判断是为了与「引擎没起来」的界面语义对齐。 */
const engineReady = computed(() => Boolean(app.runtime?.engine_ready))
async function loadProviders() {
  const mine = ++loadSeq
  aborter?.abort('superseded')
  aborter = new AbortController()
  loadingProviders.value = true
  try {
    const view = await abortable.providers(aborter.signal)
    if (mine !== loadSeq) return
    providers.value = view.providers ?? []
    configPath.value = view.config_path
    readError.value = view.read_error ?? ''
    loadFailed.value = false
  } catch (err) {
    if (mine !== loadSeq || (err as { name?: string })?.name === 'CanceledError') return
    loadFailed.value = true
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    if (mine === loadSeq) loadingProviders.value = false
  }
}

/** 重拉：保存服务商之后配置变了，两块都要跟上。 */
async function reloadAll(): Promise<void> {
  await loadProviders()
  // 对话页的模型下拉由 chat store 缓存着：模型页改完必须让它重拉，
  // 否则切回对话页，输入区里还是改之前那一份模型。
  await chat.refreshModels()
}

const form = useProviderForm(reloadAll)

// 测一条服务商配置通不通：判据是「上游的模型清单能不能问出来」，
// 一次验证地址、协议与密钥这三样最容易混的配置。
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
  const ok = await confirmDanger(
    t('models.deleteConfirm', { name: view.provider_name }),
    t('models.deleteTitle'),
    t('common.delete'),
    t('common.cancel'),
  )
  if (!ok) return
  try {
    await api.deleteProvider(view.provider_name)
    toasts.push('ok', t('common.saved'))
    await reloadAll()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

/** 行内删模型：摘一条就是一条，不用打开编辑弹窗整表回传。 */
async function removeModel(view: ProviderView, modelId: string): Promise<void> {
  if (removingModel.value) return
  const ok = await confirmDanger(
    t('models.removeModelConfirm', { name: view.provider_name, id: modelId }),
    t('models.removeModelTitle'),
    t('common.delete'),
    t('common.cancel'),
  )
  if (!ok) return
  removingModel.value = `${view.provider_name}:${modelId}`
  try {
    await api.deleteProviderModel(view.provider_name, modelId)
    toasts.push('ok', t('common.saved'))
    await reloadAll()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    removingModel.value = ''
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
        <button class="wb-btn primary" :disabled="!engineReady" @click="form.openCreate()">
          <AppIcon name="plus" :size="14" />
          {{ t('models.addProvider') }}
        </button>
      </div>
    </header>

    <div class="page-body">
      <WbSkeleton v-if="loadingProviders && !providers.length" block />
      <p v-if="readError" class="notice">
        <AppIcon name="alert" :size="14" />
        {{ readError }}
      </p>

      <ProviderList
        :providers="providers"
        :loading="loadingProviders"
        :load-failed="loadFailed"
        @retry="loadProviders"
        :config-path="configPath"
        :pending="probing"
        :removing="removingModel"
        @create="form.openCreate()"
        @edit="form.openEdit"
        @remove="remove"
        @test="testProvider"
        @remove-model="removeModel"
      />

      <DefaultModelCard :providers="providers" />

      <ModelTable :models="models" :engine-missing="!engineReady" />
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
