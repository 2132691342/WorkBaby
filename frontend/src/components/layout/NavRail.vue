<script setup lang="ts">
// 一级导航 rail：图标 + 短标签竖排，全部一级页面平级（含设置）。
// 仿管理系统惯例把导航收进左栏，顶栏只留品牌、上下文标题与窗控。
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppIcon from '../common/AppIcon.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const ITEMS = [
  { key: 'chat', path: '/', icon: 'chat', labelKey: 'nav.chat', shortKey: 'navShort.chat' },
  {
    key: 'overview',
    path: '/overview',
    icon: 'cpu',
    labelKey: 'nav.overview',
    shortKey: 'navShort.overview',
  },
  {
    key: 'workspace',
    path: '/workspace',
    icon: 'git-branch',
    labelKey: 'nav.workspace',
    shortKey: 'navShort.workspace',
  },
  {
    key: 'features',
    path: '/features',
    icon: 'zap',
    labelKey: 'nav.features',
    shortKey: 'navShort.features',
  },
  {
    key: 'skills',
    path: '/skills',
    icon: 'sparkles',
    labelKey: 'nav.skills',
    shortKey: 'navShort.skills',
  },
  {
    key: 'files',
    path: '/files',
    icon: 'folder',
    labelKey: 'nav.files',
    shortKey: 'navShort.files',
  },
  {
    key: 'sessions',
    path: '/sessions',
    icon: 'doc',
    labelKey: 'nav.sessions',
    shortKey: 'navShort.sessions',
  },
  {
    key: 'dashboard',
    path: '/dashboard',
    icon: 'chart',
    labelKey: 'nav.dashboard',
    shortKey: 'navShort.dashboard',
  },
] as const

// 设置与帮助压在底部：它们是旁路，但仍是 rail 里的一等入口——
// 用户说「找不到设置入口」的根源是把它们挤进了顶栏角落的纯图标。
const FOOTER = [
  { key: 'help', path: '/help', icon: 'help', labelKey: 'nav.help', shortKey: 'navShort.help' },
  {
    key: 'settings',
    path: '/settings',
    icon: 'settings',
    labelKey: 'nav.settings',
    shortKey: 'navShort.settings',
  },
] as const

function isActive(path: string): boolean {
  if (path === '/') return route.path === '/' || route.path.startsWith('/session')
  return route.path.startsWith(path)
}

function go(path: string) {
  if (route.path !== path) void router.push(path)
}
</script>

<template>
  <aside class="rail" aria-label="Primary">
    <nav class="rail-nav">
      <button
        v-for="it in ITEMS"
        :key="it.key"
        type="button"
        class="rail-item"
        :class="{ 'is-on': isActive(it.path) }"
        :title="t(it.labelKey)"
        :aria-current="isActive(it.path) ? 'page' : undefined"
        @click="go(it.path)"
      >
        <span v-if="isActive(it.path)" class="rail-bar" aria-hidden="true" />
        <AppIcon :name="it.icon" size="ic-sm" />
        <span class="rail-tx">{{ t(it.shortKey) }}</span>
      </button>
    </nav>

    <div class="rail-foot">
      <button
        v-for="it in FOOTER"
        :key="it.key"
        type="button"
        class="rail-item"
        :class="{ 'is-on': isActive(it.path) }"
        :title="t(it.labelKey)"
        :aria-current="isActive(it.path) ? 'page' : undefined"
        @click="go(it.path)"
      >
        <span v-if="isActive(it.path)" class="rail-bar" aria-hidden="true" />
        <AppIcon :name="it.icon" size="ic-sm" />
        <span class="rail-tx">{{ t(it.shortKey) }}</span>
      </button>
    </div>
  </aside>
</template>

<style scoped>
.rail {
  width: var(--wb-w-rail);
  flex: none;
  display: flex;
  flex-direction: column;
  background: var(--wb-elev-workspace);
  border-right: 1px solid var(--wb-border);
  padding: var(--wb-sp-2) 0;
  gap: var(--wb-sp-2);
}
.rail-nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 0 var(--wb-sp-1);
  flex: 1;
  min-height: 0;
}
.rail-foot {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--wb-sp-2) var(--wb-sp-1) 0;
  border-top: 1px solid var(--wb-border);
}
.rail-item {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 3px;
  width: 100%;
  padding: var(--wb-sp-2) 0;
  border-radius: var(--wb-radius-sm);
  color: var(--wb-muted);
  transition:
    background var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.rail-item:hover {
  background: var(--wb-elev-hover);
  color: var(--wb-ink);
  transform: scale(1.06);
}
.rail-item:active {
  transform: scale(0.92);
}
.rail-item.is-on {
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
}
.rail-bar {
  position: absolute;
  left: calc(-1 * var(--wb-sp-1));
  top: 50%;
  transform: translateY(-50%);
  width: 2px;
  height: 14px;
  border-radius: 0 var(--wb-radius-full) var(--wb-radius-full) 0;
  background: var(--wb-primary);
}
.rail-tx {
  font-size: var(--wb-fs-3xs);
  line-height: 1;
  font-weight: 600;
  letter-spacing: -0.01em;
  white-space: nowrap;
}
.rail-item.is-on .rail-tx {
  font-weight: 700;
}
</style>
