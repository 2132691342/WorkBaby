<script setup lang="ts">
import { computed, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Moon,
  Plus,
  Search,
  Settings,
  Sunny
} from '@/components/common/icons'
import { useTheme } from '@/composables/useTheme'
import { t } from '@/i18n'

/**
 * 顶栏只保留「设置」入口 + 右侧动作（搜索 / 主题 / 新建任务）。
 *
 * <p>六个领域入口（记忆 / 知识库 / 仪表盘 / 工作流 / 工作台 / 自动化）
 * 全部沉到命令面板（Ctrl+K）——顶栏塞 7 项是「领域罗列」，不是导航，
 * 用户切换频率低，留着只会占据视觉焦点。设置是高频入口，保留。
 */
const emit = defineEmits<{ create: []; search: [] }>()

const route = useRoute()
const router = useRouter()
const theme = useTheme()

interface NavItem {
  labelKey: string
  icon: Component
  to: string
  match: () => boolean
}

function inSettings(): boolean {
  return route.path.startsWith('/settings')
}

const items = computed<NavItem[]>(() => [
  { labelKey: 'nav.settings', icon: Settings, to: '/settings', match: inSettings }
])

const isDark = computed(() => theme.currentTheme.value === 'dark')

function go(to: string): void {
  void router.push(to)
}

function toggleTheme(): void {
  theme.setTheme(isDark.value ? 'light' : 'dark')
}
</script>

<template>
  <nav class="appnav" :aria-label="t('nav.navMain')">
    <div class="appnav-menu">
      <button
        v-for="it in items"
        :key="it.labelKey"
        class="appnav-item"
        :class="{ 'is-active': it.match() }"
        @click="go(it.to)"
      >
        <component :is="it.icon" class="ic-sm" />
        <span>{{ t(it.labelKey) }}</span>
      </button>
    </div>

    <span class="appnav-sp" />

    <div class="appnav-actions">
      <button class="btn-icon" :title="t('nav.search')" @click="emit('search')">
        <Search class="ic-sm" />
      </button>
      <button class="btn-icon" :title="t('chat.toggleTheme')" @click="toggleTheme">
        <Moon v-if="!isDark" class="ic-sm" />
        <Sunny v-else class="ic-sm" />
      </button>
      <button class="btn btn-primary btn-sm" @click="emit('create')">
        <Plus class="ic-sm" />
        <span>{{ t('nav.newTask') }}</span>
      </button>
    </div>
  </nav>
</template>
