<script setup lang="ts">
// 顶栏：品牌 + 当前区标题 + 连接灯 + 窗控。
// 一级导航在左 rail（NavRail），这里只回答「我现在在哪、连上没有」。
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { sseConnected, sseSubscribed } from '../../composables/useSse'
import { useChatStore } from '../../stores/chat'
import { useSettingsStore } from '../../stores/settings'
import { useSessionStore } from '../../stores/session'
import { useSessionPanel } from '../../composables/useSessionPanel'
import { windowCtl } from '../../api/shell'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{ ready: boolean }>()

const { t } = useI18n()
const route = useRoute()
const session = useSessionStore()
const chat = useChatStore()
const settings = useSettingsStore()
const { collapsed, toggle } = useSessionPanel()

const section = computed(() => {
  const p = route.path
  if (p.startsWith('/workspace')) return 'workspace'
  if (p.startsWith('/dashboard')) return 'dashboard'
  if (p.startsWith('/settings')) return 'settings'
  if (p.startsWith('/help')) return 'help'
  return 'chat'
})

const isChat = computed(() => section.value === 'chat')
const pageName = computed(() => {
  switch (section.value) {
    case 'workspace':
      return t('nav.workspace')
    case 'dashboard':
      return t('nav.dashboard')
    case 'settings':
      return t('nav.settings')
    case 'help':
      return t('nav.help')
    default:
      return session.current?.session_title || chat.suggestedTitle || t('chat.newSession')
  }
})

const connection = computed(() => {
  if (!props.ready) return { tone: '', label: t('titlebar.idle'), hint: t('titlebar.idleHint') }
  if (!sseSubscribed.value) return { tone: '', label: t('titlebar.idle'), hint: t('titlebar.idleHint') }
  if (sseConnected.value) return { tone: 'ok', label: t('titlebar.online'), hint: t('titlebar.onlineHint') }
  return { tone: 'warn', label: t('titlebar.offline'), hint: t('titlebar.offlineHint') }
})

const closeHint = computed(() =>
  settings.values['minimize_to_tray'] !== 'false' ? t('titlebar.closeToTray') : t('titlebar.close'),
)

function winctl(action: 'min' | 'max' | 'close') {
  // 关闭走后端：由关窗策略（收进托盘 / 真退出）决定行为
  windowCtl(action)
}
</script>

<template>
  <header class="titlebar" style="--wails-draggable: drag">
    <div class="tb-brand">
      <span class="tb-tile">WB</span>
      <span class="tb-name">{{ t('app.name') }}</span>
    </div>

<!-- 一级导航已迁到左 rail，顶栏只留品牌、上下文标题与系统区 -->

    <button
      v-if="isChat && ready"
      class="tb-fold"
      style="--wails-draggable: no-drag"
      type="button"
      :title="collapsed ? t('nav.expandSide') : t('nav.collapseSide')"
      :aria-label="collapsed ? t('nav.expandSide') : t('nav.collapseSide')"
      @click="toggle"
    >
      <AppIcon name="panel-left" size="ic-sm" />
    </button>
    <span class="tb-ctx" :title="pageName">{{ pageName }}</span>

    <span class="tb-sp" />

    <div class="tb-actions" style="--wails-draggable: no-drag">
      <el-tooltip :content="connection.hint" placement="bottom" :show-after="400">
        <span class="tb-led" :class="connection.tone">
          <span class="wb-led" :class="connection.tone ? `wb-led--${connection.tone}` : ''" />
          <span class="tb-led-tx">{{ connection.label }}</span>
        </span>
      </el-tooltip>
    </div>

    <div class="winctl" style="--wails-draggable: no-drag">
      <button class="win-btn" type="button" :title="t('titlebar.minimize')" @click="winctl('min')">
        <AppIcon name="window-min" size="ic-sm" />
      </button>
      <button class="win-btn" type="button" :title="t('titlebar.maximize')" @click="winctl('max')">
        <AppIcon name="window-max" size="ic-sm" />
      </button>
      <button class="win-btn is-close" type="button" :title="closeHint" @click="winctl('close')">
        <AppIcon name="close" size="ic-sm" />
      </button>
    </div>
  </header>
</template>

<style scoped>
.titlebar {
  height: var(--wb-h-titlebar);
  flex: none;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: 0 0 0 var(--wb-sp-3);
  background: var(--wb-elev-raised);
  border-bottom: 1px solid var(--wb-border);
  position: relative;
  z-index: var(--wb-z-panel);
}

.tb-brand {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: none;
}
.tb-tile {
  width: calc(26px * var(--wb-vw-scale));
  height: calc(26px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-sm);
  display: grid;
  place-items: center;
  background: var(--wb-primary);
  color: var(--wb-primary-ink);
  font-family: var(--font-display);
  font-weight: 700;
  font-size: var(--wb-fs-2xs);
  letter-spacing: 0.02em;
}
.tb-name {
  font-family: var(--font-display);
  font-size: var(--wb-fs-md);
  font-weight: 700;
  color: var(--wb-ink);
  letter-spacing: -0.01em;
}

.tb-seg {
  margin-left: var(--wb-sp-2);
  flex: none;
}

.tb-fold {
  display: grid;
  place-items: center;
  width: var(--wb-ctl-icon);
  height: var(--wb-ctl-icon);
  border-radius: var(--wb-radius-sm);
  color: var(--wb-muted);
  transition:
    background var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.tb-fold:hover {
  background: var(--wb-nav-hover-bg);
  color: var(--wb-ink);
  transform: scale(1.06);
}
.tb-fold:active {
  transform: scale(0.92);
}
.tb-ctx {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  max-width: var(--wb-w-crumbs);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding-left: var(--wb-sp-2);
  border-left: 1px solid var(--wb-border);
  line-height: 1.2;
  height: calc(16px * var(--wb-vw-scale));
  display: flex;
  align-items: center;
}

.tb-sp {
  flex: 1;
  min-width: var(--wb-sp-2);
}

.tb-actions {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
}

.tb-led {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-2);
  height: var(--wb-ctl-icon);
  padding: 0 var(--wb-sp-3);
  border-radius: var(--wb-radius-full);
  border: 1px solid var(--wb-border);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-muted);
  white-space: nowrap;
}
.tb-led.ok {
  color: var(--wb-success);
  border-color: color-mix(in srgb, var(--wb-success) 30%, var(--wb-border));
}
.tb-led.warn {
  color: var(--wb-warning);
  border-color: color-mix(in srgb, var(--wb-warning) 34%, var(--wb-border));
}

/* 窗控：系统级控件，用自绘保持命中区域与关闭键的危险语义 */
.winctl {
  display: flex;
  align-items: stretch;
  height: 100%;
  margin-left: var(--wb-sp-1);
}
.win-btn {
  width: calc(44px * var(--wb-vw-scale));
  display: grid;
  place-items: center;
  color: var(--wb-muted);
  transition:
    background var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease);
}
.win-btn:hover {
  background: var(--wb-elev-hover);
  color: var(--wb-ink);
}
.win-btn:active {
  background: var(--wb-surface-hover);
}
.win-btn.is-close:hover {
  background: var(--wb-danger);
  color: var(--wb-primary-ink);
}
</style>
