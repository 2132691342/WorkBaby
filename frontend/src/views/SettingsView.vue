<script setup lang="ts">
// 设置页：外观 / 环境 / 工作区 / 关于。
// 「环境」这一节只做两件事：说清引擎在哪、没找到时给可复制的安装命令。
// 本应用不下载、不安装、不改 PATH——装上装不上是用户自己的事。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/http'
import EnginePanel from '@/components/settings/EnginePanel.vue'
import AppearancePanel from '@/components/settings/AppearancePanel.vue'
import BehaviorPanel from '@/components/settings/BehaviorPanel.vue'
import ConfigPanel from '@/components/settings/ConfigPanel.vue'
import WorkspacePanel from '@/components/settings/WorkspacePanel.vue'
import DiagPanel from '@/components/settings/DiagPanel.vue'
import AboutPanel from '@/components/settings/AboutPanel.vue'
import { useAppStore } from '@/stores/app'
import { usePrefsStore } from '@/stores/prefs'
import { useToasts } from '@/composables/useToasts'

const { t } = useI18n()
const app = useAppStore()
const prefs = usePrefsStore()
const toasts = useToasts()

const saving = ref(false)
const logs = ref<string[]>([])
const detecting = ref(false)
const restarting = ref('')
// 引擎路径与 profile 的草稿都搬进 EnginePanel 了：它们只在那张卡里用，
// 留在这里会让设置页为一个子组件的内部输入持有状态。

// 启动行为：开关状态以注册表为准，所以每次进来都真读一次。
const autoStartBusy = ref(false)
const restoreBusy = ref(false)
// 配置原文只能看不能改：写引擎配置有节点级接口，原文编辑会打乱它的假设。
const configKind = ref<'omp' | 'pi'>('omp')

async function toggleAutoStart(enabled: boolean) {
  autoStartBusy.value = true
  try {
    await prefs.setAutoStart(enabled)
    toasts.push('ok', t('common.saved'))
  } catch (err) {
    // 失败时开关会自己拨回去：prefs 里的值是后端的权威值。
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    autoStartBusy.value = false
  }
}

// 关窗行为一旦选定就记住：下次按关闭不会再问。
async function toggleCloseAction(action: string) {
  try {
    await prefs.setCloseAction(action === 'quit' ? 'quit' : 'tray')
    toasts.push('ok', t('common.saved'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

async function toggleRestore(enabled: boolean) {
  restoreBusy.value = true
  try {
    await prefs.setRestoreSession(enabled)
    toasts.push('ok', t('common.saved'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    restoreBusy.value = false
  }
}

async function openDataDir() {
  try {
    await api.openDataDir()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

async function loadConfig(kind: 'omp' | 'pi') {
  configKind.value = kind
  try {
    await prefs.openConfig(kind)
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

// 最近工作目录：后端合并了「记住的」与「会话里推出来的」，这里只负责点选与移除。
const recentRows = computed(() => app.workspaces.filter((r) => r.workspace_exists))
const forgetting = ref('')

// 每个引擎一张卡：探测结果、版本、显式路径、落盘位置、重启。
// 「装」不在这里做——它统一在「应用」页，两处各放一个安装按钮必然会出现
// 一个说装了、一个说没装。这里只给一句入口与命令原文。

// 重新探测：两个引擎各起一次子进程（pi 要起 Node），所以要给 loading。
async function detectEngines() {
  detecting.value = true
  try {
    await app.detectEngines()
    toasts.push('ok', t('settings.detected'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    detecting.value = false
  }
}

async function restartEngines() {
  restarting.value = 'pi'
  try {
    await api.restartEngine()
    await app.refreshEngines()
    toasts.push('ok', t('settings.restartEngine'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    restarting.value = ''
  }
}

// save 只负责保存与报错；成功与否用返回值告知调用处——
// 失败必须可见，静默成功会让用户以为改上了。
async function save(kv: Record<string, string>): Promise<boolean> {
  saving.value = true
  try {
    await app.saveSettings(kv)
    return true
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
    return false
  } finally {
    saving.value = false
  }
}


async function loadLogs() {
  try {
    const data = await api.logs()
    logs.value = data.lines.slice(-200)
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

async function revealPath(path: string) {
  try {
    await api.reveal(path)
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

async function useWorkspace(path: string) {
  if (path === app.workspace) return
  await app.setWorkspace(path)
  toasts.push('ok', t('settings.workspaceSwitched'))
}

// 移除只影响「最近」这份记忆，不动目录本身。
async function forgetWorkspace(path: string) {
  forgetting.value = path
  try {
    const res = await api.forgetWorkspace(path)
    app.workspaces = res.workspaces ?? []
    toasts.push('ok', t('settings.workspaceForgotten'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    forgetting.value = ''
  }
}

onMounted(() => {
  void loadLogs()
  void app.refreshEngines()
  void prefs.load()
  // 只给已经装了的引擎看配置：都没装时这一整节不该出现。
  const first = app.engines?.engines?.[0]?.engine_kind
  if (first) void loadConfig(first)
})

</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('settings.title') }}</h1>
        <p class="page-sub">{{ t('settings.subtitle') }}</p>
      </div>
    </header>

    <div class="page-body">
      <!-- 外观 -->
      <AppearancePanel @save="save" />

      <!-- 启动行为 -->
      <BehaviorPanel
        :auto-start-busy="autoStartBusy"
        :restore-busy="restoreBusy"
        @toggle-auto-start="toggleAutoStart"
        @set-close-action="toggleCloseAction"
        @toggle-restore="toggleRestore"
        @open-data-dir="openDataDir"
      />

      <!-- 环境：每个引擎一张卡。不安装，只探测 + 给命令原文 -->
      <EnginePanel
        :apps="app.apps"
        :detecting="detecting"
        :restarting="Boolean(restarting)"
        :runtime="app.runtime"
        @redetect="detectEngines"
        @restart="restartEngines"
        @reveal="revealPath"
      />

      <!-- 工作区 -->
      <WorkspacePanel
        :recent="recentRows"
        :forgetting="forgetting"
        @use="useWorkspace"
        @forget="forgetWorkspace"
      />

      <!-- 引擎配置原文：只看不改 -->
      <ConfigPanel :kind="configKind" @pick="loadConfig" />

      <!-- 诊断：失败记录 + 后端日志。两者回答同一个问题：刚才那一下哪一层出的错 -->
      <DiagPanel :logs="logs" @refresh-logs="loadLogs" />

      <!-- 关于 -->
      <AboutPanel :apps="app.apps" />
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.page-head {
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-4);
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
  margin: 4px 0 0;
  max-width: 720px;
  font-size: var(--wb-fs-sm);
  color: var(--wb-mute);
  line-height: 1.65;
}

.page-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-10);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}

/* 六个小节各自带了 .set-row / .seg / .mono 这些行内样式，
   这里只留页面骨架：页头、滚动区、各卡之间的间距。 */
</style>
