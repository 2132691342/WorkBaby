<script setup lang="ts">
// 一级导航 rail：图标 + 短标签竖排，全站唯一的一级入口。
// 顺序：概览 → 对话 → 工作区 → 会话 → 模型 → 技能 → 插件 → MCP → 用量；
// 低频且「改了影响全局」的（文件 / 帮助 / 设置）压在底部，不占黄金位置。
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppIcon from '../common/AppIcon.vue'

const { t } = useI18n()
const route = useRoute()

/** 当前页高亮判定：按路由 name 比，路径改名不会连带失效。 */
function isActive(name: string): boolean {
  return route.name === name
}

/** 一级入口：name 对齐路由命名，图标取 AppIcon 现有表。 */
interface NavItem {
  /** 路由 name */
  name: string
  /** AppIcon 图标名 */
  icon: string
  /** 完整文案键（tooltip 用） */
  labelKey: string
  /** 短文案键（rail 宽度有限，只显示这个） */
  shortKey: string
}

const ITEMS: NavItem[] = [
  { name: 'overview', icon: 'monitor', labelKey: 'nav.overview', shortKey: 'navShort.overview' },
  { name: 'chat', icon: 'chat', labelKey: 'nav.chat', shortKey: 'navShort.chat' },
  { name: 'workspace', icon: 'braces', labelKey: 'nav.workspace', shortKey: 'navShort.workspace' },
  { name: 'sessions', icon: 'clock', labelKey: 'nav.sessions', shortKey: 'navShort.sessions' },
  { name: 'agents', icon: 'cpu', labelKey: 'nav.agents', shortKey: 'navShort.agents' },
  { name: 'models', icon: 'sliders', labelKey: 'nav.models', shortKey: 'navShort.models' },
  { name: 'skills', icon: 'sparkles', labelKey: 'nav.skills', shortKey: 'navShort.skills' },
  { name: 'plugins', icon: 'zap', labelKey: 'nav.plugins', shortKey: 'navShort.plugins' },
  { name: 'mcp', icon: 'database', labelKey: 'nav.mcp', shortKey: 'navShort.mcp' },
  { name: 'apps', icon: 'zap', labelKey: 'nav.apps', shortKey: 'navShort.apps' },
  { name: 'usage', icon: 'chart', labelKey: 'nav.usage', shortKey: 'navShort.usage' },
]

// 底部两项都是低频旁路：帮助与设置改了影响全局。文件已并入工作区，不再单独占位。
const FOOTER: NavItem[] = [
  { name: 'help', icon: 'help', labelKey: 'nav.help', shortKey: 'navShort.help' },
  { name: 'settings', icon: 'settings', labelKey: 'nav.settings', shortKey: 'navShort.settings' },
]
</script>

<template>
  <aside class="rail" aria-label="Primary">
    <nav class="rail-nav">
      <RouterLink
        v-for="it in ITEMS"
        :key="it.name"
        :to="{ name: it.name }"
        class="rail-item"
        :class="{ 'is-on': isActive(it.name) }"
        :title="t(it.labelKey)"
        :aria-current="isActive(it.name) ? 'page' : undefined"
      >
        <span v-if="isActive(it.name)" class="rail-bar" aria-hidden="true" />
        <AppIcon :name="it.icon" size="ic-sm" />
        <span class="rail-tx">{{ t(it.shortKey) }}</span>
      </RouterLink>
    </nav>

    <div class="rail-foot">
      <RouterLink
        v-for="it in FOOTER"
        :key="it.name"
        :to="{ name: it.name }"
        class="rail-item"
        :class="{ 'is-on': isActive(it.name) }"
        :title="t(it.labelKey)"
        :aria-current="isActive(it.name) ? 'page' : undefined"
      >
        <span v-if="isActive(it.name)" class="rail-bar" aria-hidden="true" />
        <AppIcon :name="it.icon" size="ic-sm" />
        <span class="rail-tx">{{ t(it.shortKey) }}</span>
      </RouterLink>
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
/* 五态：默认 / 悬停放大 / 按下缩小 / 选中（淡底 + 左缘竖条 + 主色）；rail 窄，禁用放大以免溢出 */
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
  text-decoration: none;
  cursor: pointer;
  transition:
    background var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.rail-item:hover {
  background: var(--wb-elev-hover);
  color: var(--wb-ink);
}
.rail-item:active {
  transform: scale(var(--wb-row-scale-press));
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
