<script setup lang="ts">
// 一级导航：图标 + 一行短标签竖排。全站唯一的一级入口，
// 位置固定、宽度恒定，扫一眼就知道「我在哪」。
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppIcon, { type IconName } from '@/components/common/AppIcon.vue'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

interface NavItem {
  name: string
  icon: IconName
  key: string
}

const primary: NavItem[] = [
  { name: 'overview', icon: 'gauge', key: 'nav.overview' },
  { name: 'chat', icon: 'chat', key: 'nav.chat' },
  { name: 'workspace', icon: 'workspace', key: 'nav.workspace' },
  { name: 'sessions', icon: 'clock', key: 'nav.sessions' },
  { name: 'models', icon: 'cpu', key: 'nav.models' },
  { name: 'skills', icon: 'spark', key: 'nav.skills' },
  { name: 'mcp', icon: 'server', key: 'nav.mcp' },
  { name: 'web', icon: 'book', key: 'nav.web' },
  { name: 'index', icon: 'search', key: 'nav.index' },
  { name: 'usage', icon: 'chart', key: 'nav.usage' },
]

// 「应用」放底部：它是装引擎的地方，装完就不怎么回来，
// 与设置同类（低频、改了影响全局），不该占着一级导航的黄金位置。
const bottom: NavItem[] = [
  { name: 'apps', icon: 'box', key: 'nav.apps' },
  { name: 'settings', icon: 'gear', key: 'nav.settings' },
]

function go(name: string) {
  if (route.name !== name) void router.push({ name })
}
</script>

<template>
  <nav class="rail">
    <div class="rail-group">
      <button
        v-for="item in primary"
        :key="item.name"
        class="rail-item wb-pressable-row"
        :class="{ active: route.name === item.name }"
        :title="t(item.key)"
        @click="go(item.name)"
      >
        <AppIcon :name="item.icon" :size="19" />
        <span class="rail-label">{{ t(item.key) }}</span>
      </button>
    </div>
    <div class="rail-group bottom">
      <button
        v-for="item in bottom"
        :key="item.name"
        class="rail-item wb-pressable-row"
        :class="{ active: route.name === item.name }"
        :title="t(item.key)"
        @click="go(item.name)"
      >
        <AppIcon :name="item.icon" :size="19" />
        <span class="rail-label">{{ t(item.key) }}</span>
      </button>
    </div>
  </nav>
</template>

<style scoped>
.rail {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  width: var(--wb-rail);
  flex: none;
  padding: var(--wb-sp-2) 0 var(--wb-sp-3);
  background: var(--wb-window);
  border-right: 1px solid var(--wb-line);
}

.rail-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

/* 五态：悬停放大、按下缩小；选中态 = 淡底 + 左缘竖条 + 主色 */
.rail-item {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  margin: 0 6px;
  padding: 7px 0 6px;
  border: none;
  border-radius: var(--wb-r);
  background: transparent;
  color: var(--wb-mute);
  /* 整行元素：只缩不放（缩放由 .wb-pressable-row 定义）。
     rail 只有 60px 宽，放大 1.04 会溢出被裁。 */
}

.rail-item:hover {
  background: var(--wb-hover);
  color: var(--wb-ink);
}

.rail-item.active {
  background: var(--wb-selected);
  color: var(--wb-brand);
}

.rail-item.active::before {
  content: '';
  position: absolute;
  left: -6px;
  top: 8px;
  bottom: 8px;
  width: 2px;
  border-radius: var(--wb-r-full);
  background: var(--wb-brand);
}

.rail-label {
  font-size: var(--wb-fs-3xs);
  font-weight: 600;
  letter-spacing: 0.01em;
  line-height: 1;
}
</style>
