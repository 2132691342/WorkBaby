<script setup lang="ts">
// 全局侧栏：品牌 · 新对话 · 会话列表 · 仪表盘/设置。对话页与仪表盘页共用一份。
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSettingsStore } from '../../stores/settings'
import SessionList from '../chat/SessionList.vue'
import AppIcon from './AppIcon.vue'

const emit = defineEmits<{ newSession: [] }>()

const route = useRoute()
const router = useRouter()
const settings = useSettingsStore()

const onDashboard = computed(() => route.path === '/dashboard')

// 在仪表盘上点会话时先回对话页：点了却停在原地会让人以为没点上
function onListClick(e: MouseEvent) {
  if (route.path === '/') return
  const el = e.target as HTMLElement
  if (el.closest('.si-act') || el.closest('.rename')) return
  router.push('/')
}
</script>

<template>
  <aside class="side">
    <div class="side-brand">
      <div class="logo">WB</div>
      <div>
        <b>WorkBaby</b>
        <small>{{ settings.boot?.version || '' }}</small>
      </div>
    </div>
    <button class="btn btn-lav side-btn" type="button" @click="emit('newSession')">
      <AppIcon name="plus" /> 新对话
    </button>
    <div class="sess" @click.capture="onListClick">
      <SessionList />
    </div>
    <div class="side-foot">
      <button
        class="foot-btn"
        type="button"
        :class="{ 'is-on': onDashboard }"
        title="看看这段时间用了多少"
        @click="router.push('/dashboard')"
      >
        <AppIcon name="chart" /> 仪表盘
      </button>
      <button class="foot-btn" type="button" @click="router.push('/settings')">
        <AppIcon name="settings" /> 设置
      </button>
    </div>
  </aside>
</template>

<style scoped>
.side-btn {
  margin: var(--wb-sp-2) var(--wb-sp-3) var(--wb-sp-3);
  width: calc(100% - var(--wb-sp-6));
  justify-content: flex-start;
  gap: var(--wb-sp-2);
}
.sess {
  flex: 1 1 40%;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
</style>
