<script setup lang="ts">
// 应用壳：无边框窗口的顶栏 + 路由出口 + 启动 / 失败态。
import { computed, inject, onMounted, ref, watch, type Ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { syncFromSettings } from './composables/useAppearance'
import { installChatBridge } from './composables/chatBridge'
import { useViewportScale } from './composables/useViewportScale'
import { intentFile, intentNewSession, pushShellIntent } from './composables/shellIntent'
import { useChatStore } from './stores/chat'
import { useSettingsStore } from './stores/settings'
import { useToastStore } from './stores/toast'
import AppIcon from './components/common/AppIcon.vue'
import NavRail from './components/layout/NavRail.vue'
import ToastHost from './components/common/ToastHost.vue'
import TitleBar from './components/layout/TitleBar.vue'
import { status, stage, message, retryHandshake } from './bootstrap'
import { onShellEvent, quitApp } from './api/shell'

const { t } = useI18n()
const booted = ref(false)
const settings = useSettingsStore()
const chat = useChatStore()
const toast = useToastStore()
const router = useRouter()
// 视口比例缩放：分辨率不同的机器上整套刻度等比跟随，不改代码
useViewportScale()
// EP 内置文案的当前语言（main.ts 提供）
const epLocale = inject<Ref>('epLocale')

// 事件桥挂在应用壳上：对话流不随视图卸载而丢失，切换到设置再回来还能接上。
installChatBridge()

const failed = computed(() => status.value === 'failed')
const ready = computed(() => booted.value && !failed.value)

async function loadBoot() {
  try {
    await settings.loadBoot()
    stage.value = 'done'
    // 预热在后台跑：不等它，对话页立即可用
    void settings.warmup()
  } catch {
    // 拿不到引导数据不该让整个界面卡住：各视图自己处理空态
  }
  // 已保存的外观与显示偏好要落到 DOM 上，否则刷新后主题之外的设置全部失效
  syncFromSettings()
  chat.setShowThinking(settings.values['show_thinking'] !== 'false')
  booted.value = true
  // 没配模型就直接送到「模型」页：那是现在唯一能配模型的地方。
  // 送到设置页等于把用户丢在一个再也配不上模型的地方——配不了就没有对话。
  if (settings.boot?.boot_ready === false && router.currentRoute.value.path !== '/models') {
    void router.replace('/models')
  }
}

onMounted(() => {
  if (status.value === 'ready') void loadBoot()
  watch(status, (s) => {
    if (s === 'ready' && !booted.value) void loadBoot()
  })
  // 收进托盘前外壳会提示一句再隐藏，否则窗口「凭空消失」最让人困惑
  void onShellEvent('app:to-tray', () => toast.info(t('boot.toTray')))
  // 托盘「新建对话」与外部打开文件：切回对话页，落到对话视图里执行。
  // 先存意图再广播——视图可能还没挂载，事件不能作为唯一载体。
  void onShellEvent('app:new-session', () => {
    intentNewSession.value = true
    void router.push('/chat')
    pushShellIntent()
  })
  void onShellEvent('app:open-file', (data) => {
    if (!data?.path) return
    intentFile.value = String(data.path)
    void router.push('/chat')
    pushShellIntent()
  })
})

/** 启动阶段两段读数：把「正在启动」拆成可感知的进度 */
const bootSteps = computed(() => [
  { key: 'handshake', label: t('boot.handshake'), done: stage.value !== 'handshake' },
  { key: 'bootstrap', label: t('boot.bootstrap'), done: stage.value === 'done' },
])

</script>

<template>
  <el-config-provider :locale="epLocale">
    <div class="app-shell">
      <TitleBar :ready="ready" />

      <div class="app-frame">
        <NavRail v-if="ready" />

        <main class="app-body">
        <router-view v-if="ready" v-slot="{ Component }">
          <Transition name="wb-view" mode="out-in">
            <component :is="Component" />
          </Transition>
        </router-view>

        <div v-else-if="failed" class="boot">
          <div class="boot-badge is-bad">
            <AppIcon name="alert" size="ic-xl" />
          </div>
          <h2 class="boot-title">{{ t('boot.failed') }}</h2>
          <p class="boot-why">{{ message }}</p>
          <div class="boot-acts">
            <el-button :loading="false" @click="retryHandshake">{{ t('boot.retryHint') }}</el-button>
            <el-button type="primary" @click="quitApp">{{ t('boot.quit') }}</el-button>
          </div>
          <p class="boot-tip">
            <span class="wb-mono">%APPDATA%\WorkBaby\logs\app.log</span>
          </p>
        </div>

        <div v-else class="boot">
          <div aria-hidden="true" class="boot-aura">
            <span class="wb-aura wb-aura--left" />
            <span class="wb-aura wb-aura--right" />
          </div>
          <div class="boot-badge"><span>WB</span></div>
          <div class="boot-steps">
            <div
              v-for="s in bootSteps"
              :key="s.key"
              class="boot-step"
              :class="{ 'is-done': s.done, 'is-live': !s.done }"
            >
              <span class="bs-dot">
                <AppIcon v-if="s.done" name="check" size="ic-2xs" />
              </span>
              {{ s.label }}
            </div>
          </div>
          <p class="boot-tip">{{ t('boot.tip') }}</p>
        </div>
        </main>
      </div>

      <ToastHost />
    </div>
  </el-config-provider>
</template>

<style scoped>
.app-shell {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--wb-bg);
  color: var(--wb-ink);
}
/* 顶栏之下横向切一刀：左 rail 固定，右侧才是内容区 */
.app-frame {
  flex: 1;
  min-height: 0;
  display: flex;
  overflow: hidden;
}
.app-body {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  overflow: hidden;
}

.boot {
  flex: 1;
  min-width: 0;
  display: grid;
  place-content: center;
  justify-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-6);
  position: relative;
  overflow: hidden;
}
.boot-aura {
  position: absolute;
  inset: 0;
  pointer-events: none;
}
.boot-badge {
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
  box-shadow: var(--wb-card-shadow);
  animation: boot-breathe 2.4s var(--wb-ease) infinite;
}
.boot-badge.is-bad {
  background: var(--wb-danger-soft);
  color: var(--wb-danger);
  animation: none;
}
@keyframes boot-breathe {
  0%,
  100% {
    transform: scale(1);
  }
  50% {
    transform: scale(1.05);
  }
}
.boot-title {
  font-family: var(--font-display);
  font-size: var(--wb-fs-lg);
  font-weight: 700;
  color: var(--wb-ink);
  position: relative;
  z-index: 1;
}
.boot-why {
  max-width: calc(460px * var(--wb-vw-scale));
  text-align: center;
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-sm);
  line-height: var(--wb-lh-base);
  position: relative;
  z-index: 1;
}
.boot-steps {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  position: relative;
  z-index: 1;
  margin-top: var(--wb-sp-1);
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
.bs-dot {
  position: relative;
  width: calc(16px * var(--wb-vw-scale));
  height: calc(16px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  display: grid;
  place-items: center;
  background: var(--wb-elev-hover);
  color: var(--wb-success);
  flex: none;
}
.boot-step.is-done .bs-dot {
  background: var(--wb-success-soft);
}
.boot-step.is-live .bs-dot::after {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: var(--wb-radius-full);
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
.boot-acts {
  display: flex;
  gap: var(--wb-sp-2);
  position: relative;
  z-index: 1;
}
.boot-tip {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  position: relative;
  z-index: 1;
}
</style>
