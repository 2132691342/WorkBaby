<script setup lang="ts">
// 应用壳：无边框窗口的自绘标题栏 + 路由出口。
import { computed, onMounted, ref, watch } from 'vue'
import { syncFromSettings } from './composables/useAppearance'
import { useTheme } from './composables/useTheme'
import { sseConnected } from './composables/useSse'
import { useChatStore } from './stores/chat'
import { useSettingsStore } from './stores/settings'
import AppIcon from './components/common/AppIcon.vue'
import ToastHost from './components/common/ToastHost.vue'
import { status, message, retryHandshake } from './bootstrap'
import { ForceQuit } from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'

const booted = ref(false)
const settings = useSettingsStore()
const chat = useChatStore()
useTheme()

const failed = computed(() => status.value === 'failed')
const ready = computed(() => booted.value && !failed.value)

function winctl(action: string) {
  void import('../wailsjs/runtime/runtime').then((rt) => {
    if (action === 'min') rt.WindowMinimise()
    else if (action === 'max') rt.WindowToggleMaximise()
    else if (action === 'close') rt.WindowHide()
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
  EventsOn('app:new-session', () => window.dispatchEvent(new CustomEvent('wb:new-session')))
  EventsOn('app:open-file', (data: { path?: string }) => {
    window.dispatchEvent(new CustomEvent('wb:open-file', { detail: data?.path }))
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
      <span class="tb-chip" :class="{ 'is-on': ready }">{{ ready ? '已就绪' : '启动中…' }}</span>
      <span class="tb-sp" />
      <span class="led" :class="sseConnected ? 'g' : 'w'" :title="sseConnected ? '已连接' : '连接断开，正在重连'" />
      <div class="winctl">
        <button class="win-btn" type="button" title="最小化" @click="winctl('min')">
          <AppIcon name="window-min" size="ic-sm" />
        </button>
        <button class="win-btn" type="button" title="最大化" @click="winctl('max')">
          <AppIcon name="window-max" size="ic-sm" />
        </button>
        <button class="win-btn close" type="button" title="关闭（收进托盘）" @click="winctl('close')">
          <AppIcon name="close" size="ic-sm" />
        </button>
      </div>
    </header>
    <main class="win-body">
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
