<script setup lang="ts">
// 全局导航轨道：对话 / 仪表盘 / 帮助 / 设置，底部主题切换。全站唯一主导航。
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useTheme } from '../../composables/useTheme'
import AppIcon from './AppIcon.vue'

const route = useRoute()
const router = useRouter()
const { theme, setTheme } = useTheme()

const items = [
  { key: 'chat', to: '/', icon: 'chat', label: '对话' },
  { key: 'dashboard', to: '/dashboard', icon: 'chart', label: '仪表盘' },
  { key: 'help', to: '/help', icon: 'book', label: '帮助' },
  { key: 'settings', to: '/settings', icon: 'settings', label: '设置' },
]

const active = computed(() => {
  const p = route.path
  if (p.startsWith('/dashboard')) return 'dashboard'
  if (p.startsWith('/help')) return 'help'
  if (p.startsWith('/settings')) return 'settings'
  return 'chat'
})

function go(to: string) {
  if (route.path !== to) void router.push(to)
}
</script>

<template>
  <nav class="rail" aria-label="主导航">
    <button
      v-for="it in items"
      :key="it.key"
      class="rail-item"
      :class="{ 'is-on': active === it.key }"
      type="button"
      :title="it.label"
      :aria-current="active === it.key ? 'page' : undefined"
      @click="go(it.to)"
    >
      <AppIcon :name="it.icon" />
      <span class="rail-label">{{ it.label }}</span>
    </button>
    <span class="rail-sp" />
    <button
      class="rail-item"
      type="button"
      :title="theme === 'light' ? '切换到暗色' : '切换到浅色'"
      @click="setTheme(theme === 'light' ? 'dark' : 'light')"
    >
      <AppIcon :name="theme === 'light' ? 'moon' : 'sun'" />
      <span class="rail-label">{{ theme === 'light' ? '暗色' : '浅色' }}</span>
    </button>
  </nav>
</template>
