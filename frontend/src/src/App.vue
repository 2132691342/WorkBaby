<script setup lang="ts">
// 应用壳：无边框窗口的自绘标题栏 + 全局导航轨道 + 路由出口。
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { syncFromSettings } from './composables/useAppearance'
import { useSessionPanel } from './composables/useSessionPanel'
import { sseConnected } from './composables/useSse'
import { intentFile, intentNewSession, pushShellIntent } from './composables/shellIntent'
import { useChatStore } from './stores/chat'
import { useSessionStore } from './stores/session'
import { useSettingsStore } from './stores/settings'
import { useToastStore } from './stores/toast'
import AppIcon from './components/common/AppIcon.vue'
import NavRail from './components/common/NavRail.vue'
import ToastHost from './components/common/ToastHost.vue'
import { status, message, retryHandshake } from './bootstrap'
import { ForceQuit } from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'

const booted = ref(false)
const settings = useSettingsStore()
const session = useSessionStore()
const chat = useChatStore()
const toast = useToastStore()
const router = useRouter()
const route = useRoute()
const { collapsed: panelCollapsed, toggle: togglePanel } = useSessionPanel()

const failed = computed(() => status.value === 'failed')
const ready = computed(() => booted.value && !failed.value)

// 标题栏中区：对话页显示当前会话标题（会话名就是导航），其余页显示页面名。
const isChat = computed(() => !/^\/(dashboard|help|settings)/.test(route.path))
const pageTitle = computed(() => {
  const p = route.path
  if (p.startsWith('/dashboard')) return '仪表盘'
  if (p.startsWith('/help')) return '帮助'
  if (p.startsWith('/settings')) return '设置'
  return session.current?.title || '新对话'
})

// 关闭按钮的去向由「关闭到托盘」设置说了算，悬停提示如实反映，别让用户猜。
const closeHint = computed(() =>
  settings.values['minimize_to_tray'] !== 'false' ? '关闭（收进托盘）' : '关闭',
)

function winctl(action: string) {
  void import('../wailsjs/runtime/runtime').then((rt) => {
    if (action === 'min') rt.WindowMinimise()
    else if (action === 'max') rt.WindowToggleMaximise()
    // 关闭必须走 Quit：它触发 Go 侧 OnBeforeClose 判定——收进托盘还是真退出
    // 由设置与退出流程决定。直接 WindowHide 会绕过判定，把「关闭」一律变成收托盘。
    else if (action === 'close') rt.Quit()
  })
}

async function loadBoot() {
  try {
    await settings.loadBoot()
  } catch {
    // 拿不到引导数据不该让整个界面卡住：聊天页会各自处理空态
  }
  // 已保存的外观与显示偏好要落到 DOM 上，否则刷新后主题之外的设置全部失效。
  syncFromSettings()
  chat.setShowThinking(settings.values['show_thinking'] !== 'false')
  booted.value = true
}

onMounted(() => {
  if (status.value === 'ready') void loadBoot()
  watch(status, (s) => {
    if (s === 'ready' && !booted.value) void loadBoot()
  })
  // 收进托盘前外壳会提示一句再隐藏，否则窗口「凭空消失」最让人困惑
  EventsOn('app:to-tray', () => toast.info('已收进托盘，点右下角图标重新打开'))
  // 托盘「新建对话」与外部打开文件：切回对话页，落到对话视图里执行。
  // 先存意图再广播——视图可能还没挂载，事件不能作为唯一载体。
  EventsOn('app:new-session', () => {
    intentNewSession.value = true
    void router.push('/')
    pushShellIntent()
  })
  EventsOn('app:open-file', (data: { path?: string }) => {
    if (!data?.path) return
    intentFile.value = data.path
    void router.push('/')
    pushShellIntent()
  })
})

// 退不掉的进程是最糟糕的失败：给一个一定走真正退出流程的入口。
function quitApp() {
  try {
    ForceQuit()
  } catch {
    window.close()
  }
}
</script>

<template>
  <div class="win wb-ui">
    <header class="titlebar" style="--wails-draggable: drag">
      <div class="tb-mark"><span class="logo-mini">WB</span></div>
      <span class="tb-name">WorkBaby</span>
      <span class="tb-sep" aria-hidden="true" />
      <button
        v-if="isChat && ready"
        class="tb-act"
        type="button"
        :title="panelCollapsed ? '展开会话列表' : '折叠会话列表'"
        @click="togglePanel"
      >
        <AppIcon name="panel-left" size="ic-sm" />
      </button>
      <span class="tb-title" :title="pageTitle">{{ pageTitle }}</span>
      <span class="tb-sp" />
      <span v-if="ready && !sseConnected" class="tb-chip">连接断开，正在重连</span>
      <span class="led" :class="sseConnected ? 'g' : 'w'" :title="sseConnected ? '已连接' : '连接断开，正在重连'" />
      <div class="winctl">
        <button class="win-btn" type="button" title="最小化" @click="winctl('min')">
          <AppIcon name="window-min" size="ic-sm" />
        </button>
        <button class="win-btn" type="button" title="最大化" @click="winctl('max')">
          <AppIcon name="window-max" size="ic-sm" />
        </button>
        <button class="win-btn close" type="button" :title="closeHint" @click="winctl('close')">
          <AppIcon name="close" size="ic-sm" />
        </button>
      </div>
    </header>
    <main class="win-body">
      <NavRail v-if="ready" />
      <router-view v-if="ready" />
      <div v-else-if="failed" class="boot">
        <div class="boot-mark bad"><AppIcon name="alert" size="ic-lg" /></div>
        <h2>没能启动起来</h2>
        <p class="why">{{ message }}</p>
        <div class="boot-acts">
          <button class="btn" type="button" @click="retryHandshake">重试</button>
          <button class="btn btn-primary" type="button" @click="quitApp">退出应用</button>
        </div>
        <p class="tip">详细原因在 <span class="mono">%APPDATA%\WorkBaby\logs\app.log</span></p>
      </div>
      <div v-else class="boot">
        <div class="boot-mark">WB</div>
        <p>正在启动…</p>
      </div>
    </main>
    <ToastHost />
  </div>
</template>

<style scoped>
.logo-mini {
  font-family: var(--font-display);
  font-weight: 700;
  font-size: var(--wb-fs-xs);
  color: var(--wb-primary-ink);
}
/* 品牌与中区之间的细分隔：会话标题是导航信息，跟品牌名分开读 */
.tb-sep {
  width: 1px;
  height: 14px;
  background: var(--wb-line-2);
  flex: none;
}
.tb-title {
  max-width: 460px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.boot {
  height: 100%;
  display: grid;
  place-content: center;
  justify-items: center;
  gap: var(--wb-sp-3);
  color: var(--wb-muted);
}
.boot-mark {
  width: 56px;
  height: 56px;
  border-radius: var(--wb-radius-lg);
  display: grid;
  place-items: center;
  background: var(--wb-primary);
  color: var(--wb-primary-ink);
  font-family: var(--font-display);
  font-weight: 700;
  font-size: var(--wb-fs-xl);
}
.boot-mark.bad {
  background: var(--wb-danger);
  color: var(--wb-surface);
}
.boot h2 {
  font-family: var(--font-display);
  font-size: var(--wb-fs-lg);
  color: var(--wb-ink);
}
.boot .why {
  max-width: 460px;
  text-align: center;
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-sm);
  line-height: 1.6;
}
.boot-acts {
  display: flex;
  gap: var(--wb-sp-2);
  margin-top: var(--wb-sp-2);
}
.boot .tip {
  font-size: var(--wb-fs-hint);
  color: var(--wb-muted);
}
</style>
