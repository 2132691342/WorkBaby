<script setup lang="ts">
// 模型页：服务商（可写）/ 引擎眼下的模型（只读）/ 登录（OAuth）。
// 三块的分界线是「能不能改」，可写的与只读的不混一张表；
// 编辑草稿与上游探测在 useProviderForm，这一页只管读哪些数据、各块摆哪。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import AppIcon from '@/components/common/AppIcon.vue'
import ProviderDialog from '@/components/models/ProviderDialog.vue'
import ModelTable from '@/components/models/ModelTable.vue'
import LoginPanel from '@/components/models/LoginPanel.vue'
import ProviderList from '@/components/models/ProviderList.vue'
import EngineScopeBar from '@/components/common/EngineScopeBar.vue'
import { useEngineScope } from '@/composables/useEngineScope'
import { useProviderForm } from '@/composables/useProviderForm'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import { useAppStore } from '@/stores/app'
import type { EngineKind, LoginProvider, ModelRow, ProviderView } from '@/types/api'

const { t } = useI18n()
const toasts = useToasts()
const app = useAppStore()

// 本页的作用域（omp / pi / 全部），不跟对话引擎联动。
const { scope, set: setScope } = useEngineScope('models')
const installedKinds = computed(() => app.installed.map((e) => e.kind))

/** 写操作必须落在一个具体引擎上：作用域是「全部」时靠行上的 engine_kind 定向。 */
const writeKind = computed<EngineKind>(() => (scope.value === 'all' ? 'omp' : scope.value))
const caps = computed(() => app.capsOf(writeKind.value))

const providers = ref<ProviderView[]>([])
const configPath = ref('')
const models = ref<ModelRow[]>([])
const logins = ref<LoginProvider[]>([])
const loadingProviders = ref(false)
const loadingModels = ref(false)
const loadingLogins = ref(false)
const engineMissing = ref(false)

const loggingIn = ref('')
// 正在跑的上游探测：'test:<服务商>'。拉模型在编辑弹窗里做，不占这个标志。
const probing = ref('')

const apiOptions = [
  'openai-completions',
  'openai-responses',
  'anthropic-messages',
  'google-generative-ai',
  'openai-codex-responses',
]

// api 的 engine 参数：'all' 时省略，后端会两个都返回。
const engineParam = computed<EngineKind | undefined>(() => (scope.value === 'all' ? undefined : scope.value))

async function loadProviders() {
  loadingProviders.value = true
  try {
    const view = await api.providers(engineParam.value)
    providers.value = view.providers ?? []
    configPath.value = view.config_path
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loadingProviders.value = false
  }
}

/** 引擎没起来时读不到模型列表，这是预期状态，不当失败报警。 */
async function loadModels() {
  loadingModels.value = true
  try {
    const data = await api.models(engineParam.value)
    models.value = data.models ?? []
    engineMissing.value = false
  } catch {
    models.value = []
    engineMissing.value = true
  } finally {
    loadingModels.value = false
  }
}

async function loadLogins() {
  // 登录只有 omp 有 RPC 面；作用域是 pi / 全部时不发这个请求。
  if (scope.value !== 'omp' && scope.value !== 'all') {
    logins.value = []
    return
  }
  loadingLogins.value = true
  try {
    const data = await api.loginProviders('omp')
    logins.value = data.providers ?? []
  } catch {
    logins.value = []
  } finally {
    loadingLogins.value = false
  }
}

/** 重拉三块：保存服务商之后配置变了，三块都要跟上。 */
async function reloadAll(): Promise<void> {
  await Promise.all([loadProviders(), loadModels(), loadLogins()])
}

// 编辑草稿：表单内容与「打开 / 保存 / 拉模型」。
const form = useProviderForm(reloadAll, () => writeKind.value)

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
    const res = await api.testProvider({
      engine_kind: view.engine_kind,
      provider_name: view.provider_name,
    })
    if (res.ok) {
      toasts.push('ok', t('models.testOk', { n: res.model_count }))
    } else {
      toasts.push('error', res.message || t('models.testFail'))
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
    // 删除必须落在行自己的引擎上："全部"下两个引擎可能同名。
    const target = (view.engine_kind || writeKind.value) as EngineKind
    await api.deleteProvider(view.provider_name, target)
    toasts.push('ok', t('common.saved'))
    await reloadAll()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

async function login(provider: LoginProvider): Promise<void> {
  loggingIn.value = provider.id
  try {
    await api.startLogin(provider.id, 'omp')
    toasts.push('info', t('models.loginHint'))
    await loadLogins()
    await loadModels()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loggingIn.value = ''
  }
}

const authenticated = computed(() => logins.value.filter((p) => p.authenticated).length)

watch(scope, () => {
  providers.value = []
  models.value = []
  logins.value = []
  void reloadAll()
})

onMounted(() => {
  void reloadAll()
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('models.title') }}</h1>
        <p class="page-sub">{{ t('models.scopedSubtitle') }}</p>
      </div>
      <div class="head-actions">
        <EngineScopeBar :scope="scope" :installed="installedKinds" @pick="setScope" />
        <button
          class="wb-btn icon ghost"
          :class="{ 'is-loading': loadingProviders }"
          :title="t('common.refresh')"
          @click="loadProviders"
        >
          <AppIcon name="refresh" :size="15" />
        </button>
        <button class="wb-btn primary" @click="form.openCreate()">
          <AppIcon name="plus" :size="14" />
          {{ t('models.addProvider') }}
        </button>
      </div>
    </header>

    <div class="page-body">
      <!-- 服务商 -->
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

      <!-- 登录：只有支持界面登录的引擎才渲染（pi 的登录在终端里做） -->
      <LoginPanel
        v-if="caps?.login"
        :logins="logins"
        :logging-in="loggingIn"
        :authenticated="authenticated"
        @login="login"
      />

      <!-- 可用模型（只读） -->
      <ModelTable :models="models" :engine-missing="engineMissing" />
    </div>

    <!-- 服务商编辑 -->
    <ProviderDialog
      v-model="form.dialogOpen.value"
      :form="form.form.value"
      :editing="form.editing.value"
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

/* 服务商卡 / 模型胶囊 / 登录 / 编辑弹窗的样式都随各自的子组件搬走了，
   这一页只管页头与各节之间的间距。 */
</style>
