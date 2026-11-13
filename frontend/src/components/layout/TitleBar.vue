<script setup lang="ts">
// 自绘标题栏：无边框窗口的唯一拖动与窗控区域。
// 中区放当前位置，右区放连接灯与窗控——只回答「我在哪、连上没有」。
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { windowCtl } from '@/api/wails'

const app = useAppStore()
const route = useRoute()
const { t } = useI18n()

const title = computed(() => (route.meta.titleKey ? t(String(route.meta.titleKey)) : t('nav.chat')))
const connection = computed(() => {
  if (!app.ready) return { cls: 'warn', text: t('titlebar.starting') }
  if (Boolean(app.runtime?.detecting)) return { cls: 'warn', text: t('titlebar.detecting') }
  if (!app.hasEngine) return { cls: 'bad', text: t('titlebar.noEngine') }
  return { cls: 'ok', text: t('titlebar.connected') }
})
</script>

<template>
  <header class="titlebar" style="--wails-draggable: drag">
    <div class="brand">
      <span class="brand-mark" aria-hidden="true">
        <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round">
          <path d="M6.8 8 9.4 16.4 12 12l2.6 4.4L17.2 8" />
          <path d="M19.6 4.1l.55 1.55L21.7 6.2l-1.55.55-.55 1.55-.55-1.55L17.5 6.2l1.55-.55z" fill="currentColor" stroke="none" />
        </svg>
      </span>
      <span class="brand-name">workbaby</span>
    </div>
    <div class="crumb">
      <span class="crumb-sep">/</span>
      <span class="crumb-cur">{{ title }}</span>
    </div>
    <div class="right">
      <span class="conn" :title="connection.text">
        <span class="dot" :class="connection.cls" />
        <span class="conn-text">{{ connection.text }}</span>
      </span>
      <div class="win-ctl" style="--wails-draggable: no-drag">
        <button class="win-btn" :title="t('titlebar.minimize')" @click="windowCtl.minimize()">
          <svg viewBox="0 0 12 12" width="11" height="11" stroke="currentColor" stroke-width="1.2"><path d="M2 6h8" /></svg>
        </button>
        <button class="win-btn" :title="t('titlebar.maximize')" @click="windowCtl.toggleMaximize()">
          <svg viewBox="0 0 12 12" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.2"><rect x="2.5" y="2.5" width="7" height="7" rx="1" /></svg>
        </button>
        <!-- 唯一的关闭钮：按设置走（收托盘 / 退出），后端在 WindowClosing 里分流 -->
        <button class="win-btn close" :title="t('common.close')" @click="windowCtl.close()">
          <svg viewBox="0 0 12 12" width="11" height="11" stroke="currentColor" stroke-width="1.2"><path d="M3 3l6 6M9 3l-6 6" /></svg>
        </button>
      </div>
    </div>
  </header>
</template>

<style scoped>
.titlebar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  height: var(--wb-topbar);
  flex: none;
  padding-left: var(--wb-sp-3);
  background: var(--wb-window);
  border-bottom: 1px solid var(--wb-line);
  user-select: none;
}

.brand {
  display: flex;
  align-items: center;
  gap: 7px;
  width: calc(var(--wb-rail) + var(--wb-sp-4));
  flex: none;
}

.brand-mark {
  display: grid;
  place-items: center;
  width: 22px;
  height: 22px;
  border-radius: var(--wb-r-sm);
  background: linear-gradient(135deg, var(--wb-brand), var(--wb-brand-2));
  color: var(--wb-brand-ink);
  flex: none;
}

.brand-name {
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-sm);
  font-weight: 650;
  letter-spacing: 0.01em;
  color: var(--wb-ink);
}

.crumb {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
}

.crumb-sep {
  color: var(--wb-faint);
  font-size: var(--wb-fs-sm);
}

.crumb-cur {
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
  letter-spacing: 0.01em;
}

.right {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
}

.conn {
  display: flex;
  align-items: center;
  gap: 6px;
}

.conn-text {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
  letter-spacing: 0.02em;
}

.win-ctl {
  display: flex;
  height: 100%;
}

.win-btn {
  display: grid;
  place-items: center;
  width: 42px;
  height: 100%;
  border: none;
  background: transparent;
  color: var(--wb-mute);
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}

.win-btn:hover {
  background: var(--wb-hover);
  color: var(--wb-ink);
}

.win-btn:active {
  background: var(--wb-selected);
}

.win-btn.close:hover {
  background: var(--wb-bad);
  color: var(--wb-brand-ink);
}
</style>
