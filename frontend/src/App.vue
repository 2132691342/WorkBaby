<script setup lang="ts">
// 应用壳：无边框窗口的自绘标题栏 + 全局导航轨道 + 路由出口。
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { syncFromSettings } from './composables/useAppearance'
import { useSessionPanel } from './composables/useSessionPanel'
import { useViewportScale } from './composables/useViewportScale'
import { sseConnected, sseSubscribed } from './composables/useSse'
import { intentFile, intentNewSession, pushShellIntent } from './composables/shellIntent'
import { useChatStore } from './stores/chat'
import { useSessionStore } from './stores/session'
import { useSettingsStore } from './stores/settings'
import { useToastStore } from './stores/toast'
import AppIcon from './components/common/AppIcon.vue'
import NavRail from './components/common/NavRail.vue'
import ToastHost from './components/common/ToastHost.vue'
import { status, stage, message, retryHandshake } from './bootstrap'
import { inject, type Ref } from 'vue'
import { ForceQuit } from '../wailsjs/go/main/App'
import { EventsOn, Quit, WindowMinimise, WindowToggleMaximise } from '../wailsjs/runtime/runtime'

const booted = ref(false)
const settings = useSettingsStore()
const session = useSessionStore()
const chat = useChatStore()
const toast = useToastStore()
const router = useRouter()
const route = useRoute()
const { collapsed: panelCollapsed, toggle: togglePanel } = useSessionPanel()
// 视口比例缩放：分辨率不同的机器上全套刻度等比跟随，不改代码
useViewportScale()
// EP 内置文案的当前语言（main.ts 提供）
const epLocale = inject<Ref>('epLocale')

const failed = computed(() => status.value === 'failed')
const ready = computed(() => booted.value && !failed.value)

// 标题栏中区：对话页显示当前会话标题（会话名就是导航），其余页显示页面名。
const isChat = computed(() => !/^\/(dashboard|help|settings|workspace)/.test(route.path))
const pageTitle = computed(() => {
  const p = route.path
  if (p.startsWith('/dashboard')) return '仪表盘'
  if (p.startsWith('/workspace')) return '工作区'
  if (p.startsWith('/help')) return '帮助'
  if (p.startsWith('/settings')) return '设置'
  return session.current?.title || '新对话'
})

// 关闭按钮的去向由「关闭到托盘」设置说了算，悬停提示如实反映，别让用户猜。
const closeHint = computed(() =>
  settings.values['minimize_to_tray'] !== 'false' ? '关闭（收进托盘）' : '关闭',
)

function winctl(action: string) {
  if (action === 'min') WindowMinimise()
  else if (action === 'max') WindowToggleMaximise()
  // 关闭必须走 Quit：它触发 Go 侧 OnBeforeClose 判定——收进托盘还是真退出
  // 由设置与退出流程决定。直接 WindowHide 会绕过判定，把「关闭」一律变成收托盘。
  else if (action === 'close') Quit()
}

async function loadBoot() {
  try {
    stage.value = 'env'
    await settings.loadBoot()
    stage.value = 'done'
    // 预热在后台跑：不等它，对话页立即可用，设置页进面板时已有缓存
    void settings.warmup()
  } catch {
    // 拿不到引导数据不该让整个界面卡住：聊天页会各自处理空态
  }
  // 已保存的外观与显示偏好要落到 DOM 上，否则刷新后主题之外的设置全部失效。
  syncFromSettings()
  chat.setShowThinking(settings.values['show_thinking'] !== 'false')
  booted.value = true
  // 环境未就绪就进向导：没有 PI 就没有对话，留一个空壳只会让用户以为程序坏了。
  const envReady = settings.boot?.pi_env?.ready !== false
  if (!envReady && router.currentRoute.value.name !== 'setup') {
    void router.replace('/setup')
  }
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

// 加载页阶段读数：三行依次点亮，把「正在启动」拆成可感知的进度
const bootSteps = computed(() => [
  { key: 'handshake', label: '连接本地服务', done: stage.value !== 'handshake' },
  { key: 'bootstrap', label: '读取引导数据', done: stage.value === 'env' || stage.value === 'done' },
  { key: 'env', label: '检查助手环境', done: stage.value === 'done' },
])

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
  <el-config-provider :locale="epLocale">
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
      <span v-if="ready && sseSubscribed && !sseConnected" class="tb-chip">连接断开，正在重连</span>
      <span
        v-if="ready && sseSubscribed"
        class="led"
        :class="sseConnected ? 'g' : 'w'"
        :title="sseConnected ? '已连接' : '连接断开，正在重连'"
      />
      <div class="winctl">
        <button class="win-btn" type="button" title="最小化" aria-label="最小化" @click="winctl('min')">
          <AppIcon name="window-min" size="ic-sm" />
        </button>
        <button class="win-btn" type="button" title="最大化" aria-label="最大化" @click="winctl('max')">
          <AppIcon name="window-max" size="ic-sm" />
        </button>
        <button class="win-btn close" type="button" :title="closeHint" :aria-label="closeHint" @click="winctl('close')">
          <AppIcon name="close" size="ic-sm" />
        </button>
      </div>
    </header>
    <main class="win-body">
      <NavRail v-if="ready" />
      <router-view v-if="ready" v-slot="{ Component }">
        <Transition name="view" mode="out-in">
          <component :is="Component" />
        </Transition>
      </router-view>
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
        <div class="boot-aura" aria-hidden="true">
          <span class="aura a1" /><span class="aura a2" />
        </div>
        <div class="boot-mark"><span>WB</span></div>
        <div class="boot-steps">
          <div
            v-for="s in bootSteps"
            :key="s.key"
            class="boot-step"
            :class="{ 'is-done': s.done, 'is-live': !s.done }"
          >
            <span class="bs-dot" aria-hidden="true">
              <AppIcon v-if="s.done" name="check" size="ic-2xs" />
            </span>
            {{ s.label }}
          </div>
        </div>
        <p class="boot-tip">首次启动需要准备运行环境，稍等片刻</p>
      </div>
    </main>
    <ToastHost />
  </div>
  </el-config-provider>
</template>

<style scoped>
/* 视图切换过渡（.view-*）已收进 wb-ui.css：ConsoleView 与 SettingsView 也用它，
   放在这里时 scoped 特性只对本组件生效，另两处等于没写过渡。 */
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
  max-width: var(--wb-w-crumbs);
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
  gap: var(--wb-sp-4);
  color: var(--wb-muted);
  position: relative;
  overflow: hidden;
  width: 100%;
}
/* 双光晕：与欢迎页同一手法（纯装饰、低透明度、不参与交互） */
.boot-aura {
  position: absolute;
  inset: 0;
  pointer-events: none;
}
.aura {
  position: absolute;
  border-radius: 50%;
  filter: blur(var(--wb-aura-blur));
  opacity: 0.55;
  animation: aura-float 5s ease-in-out infinite alternate;
}
.aura.a1 {
  width: calc(420px * var(--wb-vw-scale));
  height: calc(420px * var(--wb-vw-scale));
  left: 12%;
  top: 18%;
  background: var(--wb-glow-1);
}
.aura.a2 {
  width: calc(340px * var(--wb-vw-scale));
  height: calc(340px * var(--wb-vw-scale));
  right: 12%;
  bottom: 16%;
  background: var(--wb-glow-2);
  animation-delay: -2.5s;
}
@keyframes aura-float {
  from {
    transform: translateY(-12px) scale(0.97);
  }
  to {
    transform: translateY(12px) scale(1.03);
  }
}
.boot-mark {
  width: var(--wb-tile-lg);
  height: var(--wb-tile-lg);
  border-radius: var(--wb-radius-lg);
  display: grid;
  place-items: center;
  background: var(--wb-primary);
  color: var(--wb-primary-ink);
  font-family: var(--font-display);
  font-weight: 700;
  font-size: var(--wb-fs-xl);
  position: relative;
  z-index: 1;
  animation: boot-breathe 2.2s var(--wb-ease) infinite;
  box-shadow: var(--wb-shadow-2);
}
@keyframes boot-breathe {
  0%,
  100% {
    transform: scale(1);
    box-shadow: 0 0 0 0 var(--wb-live-soft);
  }
  50% {
    transform: scale(1.05);
    box-shadow: 0 0 0 calc(10px * var(--wb-vw-scale)) var(--wb-live-soft);
  }
}
.boot-steps {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  position: relative;
  z-index: 1;
  margin-top: var(--wb-sp-2);
}
.boot-step {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  transition: color var(--wb-dur) var(--wb-ease);
}
.boot-step.is-done {
  color: var(--wb-ink-2);
}
.boot-step.is-live .bs-dot::after {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: 50%;
  border: 1px solid var(--wb-live);
  animation: bs-ping 1.4s var(--wb-ease) infinite;
}
@keyframes bs-ping {
  from {
    transform: scale(1);
    opacity: 0.8;
  }
  to {
    transform: scale(1.9);
    opacity: 0;
  }
}
.bs-dot {
  position: relative;
  width: calc(16px * var(--wb-vw-scale));
  height: calc(16px * var(--wb-vw-scale));
  border-radius: 50%;
  display: grid;
  place-items: center;
  background: var(--wb-tint);
  color: var(--wb-success);
  flex: none;
  transition: background var(--wb-dur) var(--wb-ease);
}
.boot-step.is-done .bs-dot {
  background: var(--wb-success-soft);
}
.boot-tip {
  font-size: var(--wb-fs-hint);
  color: var(--wb-muted);
  position: relative;
  z-index: 1;
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
