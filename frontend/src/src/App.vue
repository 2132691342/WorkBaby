<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter, RouterView } from 'vue-router'
import { EventsOn, WindowMinimise, WindowToggleMaximise, Quit } from '@/wailsjs/runtime/runtime'
import { init as initI18n, setLocale, t, currentLocale } from '@/i18n'
import {
  Bot,
  Plus,
  Search,
  Settings,
  Folder,
  ArrowRight,
  Globe,
  Moon,
  Sunny
} from '@/components/common/icons'
import { storeToRefs } from 'pinia'
import { UploadFile } from '@/wailsjs/go/main/App'
import AppBackground from '@/components/common/AppBackground.vue'
import CommandPalette from '@/components/common/CommandPalette.vue'
import SessionSidebar from '@/components/chat/SessionSidebar.vue'
import { useSettingsStore } from '@/stores/settings'
import { useChatStore } from '@/stores/chat'
import { useToast } from '@/composables/useToast'
import { useDialog } from '@/composables/useDialog'
import { useShortcuts } from '@/composables/useShortcuts'
import { useFocusMode } from '@/composables/useFocusMode'
import { useTheme } from '@/composables/useTheme'
import { openPalette } from '@/composables/useCommandPalette'
import { bootstrapServer } from '@/api/bootstrap'

/**
 * 应用外壳（极简布局）：
 * 自绘标题栏（frameless）+ 极简左栏（新建任务 / 搜索 + 任务列表 + 底部设置）+ RouterView。
 *
 * <p>样式全部来自 wb-ui.css 设计系统层（.win / .titlebar / .side / .rail-* …）。
 * 功能页（记忆 / 知识库 / 技能 / MCP / 工具 / 统计 / 运行 / 文件 …）
 * 全部收进设置中心（/settings 左导航多 tab），旧路由重定向，外壳只保留任务主链路。
 * 启动即进（去账号化）：bootstrapServer 成功才挂载业务视图，失败给可见错误态。
 */
const route = useRoute()
const router = useRouter()
const toast = useToast()
const ready = ref(false)
/** 启动引导状态：后端完成端口注入前显示可见进度，而不是白屏。 */
const booting = ref(true)
/** 左栏整体收起（Ctrl+B；全隐而不是图标条）。 */
const collapsed = ref(false)
/** 本机会话令牌换取失败（后端未就绪 / 端口不对）时的错误提示。 */
const bootError = ref<string | null>(null)

// 全局外观：读取 settings.general.appearance 背景 → 换取签名预览 URL 传给背景层
const settings = useSettingsStore()
const { backgroundUrl } = storeToRefs(settings)

// 任务列表（左栏主体）
const dialog = useDialog()
const chat = useChatStore()
const { sessions, currentID, loadingSessions } = storeToRefs(chat)
/** 任务列表展示模式：分组（按时间）/ 平铺。 */
const listFlat = ref(false)

/** 标题栏主标题：当前任务名；设置页显示「设置」。 */
const tbTitle = computed(() => {
  if (route.path.startsWith('/settings')) return t('settings.title')
  const cur = sessions.value.find((s) => s.id === currentID.value)
  return cur?.name || 'WorkBaby'
})

// 全局快捷键（Ctrl+N 新建任务 / Ctrl+K 命令面板 / Ctrl+/ 聚焦输入框 / Ctrl+Shift+P 命令面板
// / Ctrl+B 收起左栏 / Ctrl+Shift+F 焦点模式）
const { registerShortcut, clearAll: clearAllShortcuts } = useShortcuts()
const focusMode = useFocusMode()

// 顶栏已并入标题栏：主题切换是原先顶栏唯一不重复的动作，搬到窗口控制旁。
const theme = useTheme()
const isDark = computed(() => theme.currentTheme.value === 'dark')
function toggleTheme(): void {
  theme.setTheme(isDark.value ? 'light' : 'dark')
}

function navTo(to: string): void {
  void router.push(to)
}

// ===== 自绘窗口控制（frameless；浏览器预览无 runtime 时静默降级） =====
function minWindow(): void {
  try {
    WindowMinimise()
  } catch {
    /* 非 Wails 环境 */
  }
}
function toggleMaximise(): void {
  try {
    WindowToggleMaximise()
  } catch {
    /* 非 Wails 环境 */
  }
}
function closeWindow(): void {
  try {
    Quit()
  } catch {
    /* 非 Wails 环境 */
  }
}

function onReady(): void {
  ready.value = true
  booting.value = false
  settings.loadBackground()
  settings.loadBehavior()
  chat.loadSessions()
}

// ===== 任务列表操作（左栏常驻） =====
const creating = ref(false)
async function onCreateSession(): Promise<void> {
  // 草稿态：点新建任务不落库——只路由到聊天页 + 聚焦输入框。
  // sendMessage 在无 currentID 时会自动 createSession + 发送（stores/chat.ts:693），
  // 真正发首条消息时才建会话，「点一下就出空会话」的情况彻底消失。
  if (route.name !== 'chat' && route.name !== 'chat-session') {
    void router.push('/chat')
    // 路由切换 + ChatView 挂载需要一帧：给 80ms 兜底。期间不会建任何会话。
    setTimeout(() => window.dispatchEvent(new CustomEvent('wb:focus-composer')), 80)
  } else {
    window.dispatchEvent(new CustomEvent('wb:focus-composer'))
  }
}
async function onSelectSession(id: string): Promise<void> {
  await chat.selectSession(id)
  if (route.name !== 'chat' && route.name !== 'chat-session') {
    void router.push(`/chat/${id}`)
  }
}
async function onRemoveSession(id: string): Promise<void> {
  const ok = await dialog.confirm({
    title: t('chat.deleteTitle'),
    content: t('chat.deleteConfirm'),
    danger: true
  })
  if (!ok) return
  try {
    await chat.deleteSession(id)
    toast.success(t('chat.deletedSuccess'))
  } catch (e) {
    toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
  }
}
async function onDeleteBatch(ids: string[]): Promise<void> {
  if (ids.length === 0) return
  const result = await chat.deleteSessions(ids)
  if (result.failed.length > 0) {
    toast.warning(t('chat.deleteBatchPartial', result.ok, result.failed.length))
  } else {
    toast.success(t('chat.deleteBatchDone', result.ok))
  }
}

/** 左栏底部语言快切：中 / EN 一键互换。 */
async function toggleLang(): Promise<void> {
  await setLocale(currentLocale.value === 'zh-CN' ? 'en-US' : 'zh-CN')
}

/** 桌宠模式已移除。 */

onMounted(async () => {
  await initI18n()
  // 主题已移到 main.ts 在 mount 前同步应用（见该文件说明），此处不再重复初始化
  // 双主机：先等 app:ready 拿到 serverPort 并初始化 HTTP 层，成功才挂载业务视图
  // （后端未就绪给可见错误态，而不是静默白屏）
  try {
    await bootstrapServer()
  } catch (e) {
    bootError.value = e instanceof Error ? e.message : String(e)
    booting.value = false
    return
  }
  registerFileOpenEvents()
  onReady()
  // 斜杠命令元数据：启动期一次缓存（此前从未接线，后端命令从未出现在面板）
  void chat.loadCommands()
  registerShortcut('ctrl+n', () => void onNewSessionShortcut())
  registerShortcut('ctrl+k', () => openPalette())
  registerShortcut('ctrl+/', () => window.dispatchEvent(new CustomEvent('wb:focus-input')))
  registerShortcut('ctrl+shift+p', () => openPalette())
  registerShortcut('ctrl+b', () => (collapsed.value = !collapsed.value))
  registerShortcut('ctrl+shift+f', () => focusMode.toggle())
})

onBeforeUnmount(() => {
  clearAllShortcuts()
  offFileOpen?.()
})

/**
 * 文件关联打开：主实例收到二次启动转交的路径后 emit app:open-file。
 * 处理：导入文件 → 新建会话 → 直接以附件发起一次「阅读总结」，用户双击文件即得到回答。
 */
let offFileOpen: (() => void) | undefined
function registerFileOpenEvents(): void {
  try {
    offFileOpen = EventsOn('app:open-file', (path: string) => {
      void handleOpenFile(path)
    })
  } catch {
    /* 非 Wails 环境（浏览器预览）无事件桥 */
  }
}

async function handleOpenFile(path: string): Promise<void> {
  try {
    const info = await UploadFile('', path, '', '')
    const session = await chat.createSession()
    await router.push(`/chat/${session.id}`)
    await chat.sendMessage(t('chat.fileOpenedPrompt', info.original_name || info.name), [info.id])
    toast.success(t('chat.fileOpened'), info.original_name || info.name)
  } catch (e) {
    toast.error(t('chat.fileOpenFailed'), e instanceof Error ? e.message : String(e))
  }
}

/** 后端就绪事件丢失时允许用户重新建立本地 HTTP 连接。 */
async function recoverBoot(): Promise<void> {
  bootError.value = null
  booting.value = true
  try {
    await bootstrapServer()
  } catch (e) {
    bootError.value = e instanceof Error ? e.message : String(e)
    booting.value = false
  }
}

async function onNewSessionShortcut(): Promise<void> {
  // 与「新建任务」按钮同一语义（草稿态不落库）。
  // 此前此处直接建会话：Ctrl+N 会在会话列表留下空会话，而点按钮不会——两个入口行为必须一致。
  await onCreateSession()
}
</script>

<template>
  <!-- 后端初始化期间显示可见启动态，避免窗口打开后出现长时间白屏 -->
  <div v-if="booting" class="wb-ui relative flex h-screen items-center justify-center" aria-live="polite">
    <AppBackground />
    <div class="card relative z-10 w-full max-w-md space-y-5 text-center">
      <div class="tile tile-xl mx-auto flex items-center justify-center" style="margin: 0 auto 14px">
        <Bot class="ic" />
      </div>
      <h1 class="fs13 font-semibold text-wb-ink">{{ t('boot.startingTitle') }}</h1>
      <p class="fs11 muted">{{ t('boot.startingHint') }}</p>
      <div class="bar mx-auto w-40">
        <div class="h-full w-1/2 animate-pulse rounded-full bg-wb-primary" />
      </div>
      <p class="fs11 muted">{{ t('boot.startingDetail') }}</p>
    </div>
  </div>

  <!-- 后端仍未就绪时给出可见错误态与重试入口 -->
  <div v-else-if="bootError" class="wb-ui relative flex h-screen items-center justify-center">
    <AppBackground />
    <div class="card relative z-10 w-full max-w-md space-y-4 text-center">
      <div class="tile tile-xl mx-auto flex items-center justify-center" style="margin: 0 auto">
        <Bot class="ic" />
      </div>
      <h1 class="fs13 font-semibold text-wb-ink">{{ t('boot.unreachableTitle') }}</h1>
      <p class="fs11 muted">{{ t('boot.unreachableHint') }}</p>
      <p class="mono break-all rounded-lg bg-wb-bg/60 p-2 fs11 muted">{{ bootError }}</p>
      <button class="btn btn-primary" @click="recoverBoot">{{ t('boot.retry') }}</button>
    </div>
  </div>

  <!-- 应用外壳：titlebar + 极简左栏 + main -->
  <div v-else-if="ready" class="wb-ui win">
    <AppBackground :background-url="backgroundUrl" />

    <!-- 自绘标题栏（frameless 拖拽区）：任务名 + 工作区 chip -->
    <div class="titlebar" style="--wails-draggable: drag">
      <div class="tb-mark">
        <Bot class="h-2.5 w-2.5" />
      </div>
      <span class="tb-name truncate" :title="tbTitle">{{ tbTitle }}</span>
      <span class="tb-chip">
        <Folder class="h-3 w-3" />
        WorkBaby
      </span>
      <span class="tb-sp" />
      <button
        class="tb-act"
        style="--wails-draggable: no-drag"
        :title="t('chat.toggleTheme')"
        @click="toggleTheme"
      >
        <Moon v-if="!isDark" class="h-3 w-3" />
        <Sunny v-else class="h-3 w-3" />
      </button>
      <div class="winctl" style="--wails-draggable: no-drag">
        <span :title="t('app.win.minimize')" @click="minWindow">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"><path d="M5 12h14" /></svg>
        </span>
        <span :title="t('app.win.maximize')" @click="toggleMaximise">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4"><rect x="4" y="5" width="16" height="14" rx="1" /></svg>
        </span>
        <span class="close" :title="t('app.win.close')" @click="closeWindow">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"><path d="M18 6 6 18" /><path d="m6 6 12 12" /></svg>
        </span>
      </div>
    </div>

    <div class="win-body">
      <!-- 极简左栏：快捷动作 + 任务列表 + 底部设置 -->
      <aside v-show="!collapsed" class="side">
        <div class="side-brand">
          <div class="logo"><Bot class="h-4 w-4" /></div>
          <div class="grow">
            <b>WorkBaby</b>
            <small>{{ t('nav.localIdentityHint') }}</small>
          </div>
          <button class="rail-collapse" :title="t('nav.collapseRail')" @click="collapsed = true">
            <ArrowRight class="h-3 w-3" />
          </button>
        </div>

        <!-- 快捷动作：新建任务（主行动，镂空主色） / 搜索（领域切换走命令面板 Ctrl+K） -->
        <div class="side-actions">
          <button class="side-btn is-primary" :disabled="creating" @click="onCreateSession">
            <span class="rail-ic"><Plus class="ic-sm" /></span>
            <span class="grow" style="text-align: left">{{ t('nav.newTask') }}</span>
            <kbd class="rail-kbd">Ctrl N</kbd>
          </button>
          <button class="side-btn" @click="openPalette">
            <span class="rail-ic"><Search class="ic-sm" /></span>
            <span class="grow" style="text-align: left">{{ t('nav.search') }}</span>
            <kbd class="rail-kbd">Ctrl K</kbd>
          </button>
        </div>

        <!-- 任务列表（分组 / 平铺由列表头切换） -->
        <SessionSidebar
          class="rail-list"
          :sessions="sessions"
          :currentID="currentID"
          :loading="loadingSessions"
          :creating="creating"
          :flat="listFlat"
          @select="onSelectSession"
          @create="onCreateSession"
          @remove="onRemoveSession"
          @delete-batch="onDeleteBatch"
          @toggle-flat="listFlat = !listFlat"
          @pin="(id, pinned) => void chat.pinSession(id, pinned)"
          @archive="(id, archived) => void chat.archiveSession(id, archived)"
        />

        <div class="side-foot">
          <button
            class="foot-btn"
            :title="currentLocale === 'zh-CN' ? 'English' : '中文'"
            @click="toggleLang"
          >
            <Globe class="ic ic-sm" />
            <span>{{ currentLocale === 'zh-CN' ? '中文' : 'English' }}</span>
          </button>
          <span class="sp" />
          <button
            class="foot-btn"
            :class="{ 'is-on': route.path.startsWith('/settings') }"
            @click="navTo('/settings')"
          >
            <Settings class="ic ic-sm" />
            <span>{{ t('nav.settings') }}</span>
          </button>
        </div>
      </aside>

      <!-- 主内容区 -->
      <main class="main relative z-10">
        <RouterView />
      </main>
    </div>

    <!-- 全局命令面板（Ctrl+K / Ctrl+Shift+P） -->
    <CommandPalette />
  </div>
</template>
