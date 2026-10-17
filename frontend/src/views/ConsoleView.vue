<script setup lang="ts">
// 指挥台：运行 / 评估 / 历史 / 架构四个页签（specs/11-13 / 15）。
// 一级导航只有 NavRail 一处；这里的子导航复用 .nav-item 的竖条语言。
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AppIcon from '../components/common/AppIcon.vue'
import ArchMapPanel from '../components/console/ArchMapPanel.vue'
import EvalsPanel from '../components/console/EvalsPanel.vue'
import HistoryPanel from '../components/console/HistoryPanel.vue'
import RunsPanel from '../components/console/RunsPanel.vue'

const TABS = [
  { key: 'runs', label: '运行', icon: 'activity' },
  { key: 'evals', label: '评估', icon: 'chart' },
  { key: 'history', label: '历史', icon: 'clock' },
  { key: 'archmap', label: '架构', icon: 'git-branch' },
] as const

const route = useRoute()
const tab = computed(() => {
  const t = String(route.params.tab || '')
  return TABS.find((x) => x.key === t) ? t : 'runs'
})
</script>

<template>
  <div class="console">
    <nav class="console-nav" aria-label="指挥台">
      <router-link
        v-for="t in TABS"
        :key="t.key"
        class="nav-item"
        :class="{ on: tab === t.key }"
        :to="`/console/${t.key}`"
      >
        <AppIcon :name="t.icon" size="ic-sm" />
        <span>{{ t.label }}</span>
      </router-link>
    </nav>
    <div class="console-main">
      <!-- key 绑 tab：四个面板都是不同组件，不加 key 时 Vue 会尝试复用实例，
           结果是旧面板的状态（滚动位置、展开的抽屉）串到新面板上。 -->
      <Transition name="view" mode="out-in">
        <RunsPanel v-if="tab === 'runs'" :key="tab" />
        <EvalsPanel v-else-if="tab === 'evals'" :key="tab" />
        <HistoryPanel v-else-if="tab === 'history'" :key="tab" />
        <ArchMapPanel v-else :key="tab" />
      </Transition>
    </div>
  </div>
</template>

<style scoped>
.console {
  display: flex;
  height: 100%;
  min-width: 0;
}
.console-nav {
  width: var(--wb-w-settings);
  flex-shrink: 0;
  border-right: 1px solid var(--wb-line);
  background: var(--wb-side);
  padding: var(--wb-sp-4) var(--wb-sp-2);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}
.console-main {
  flex: 1;
  min-width: 0;
  overflow: auto;
  padding: var(--wb-sp-6);
}
/* view 过渡在 App.vue 已定义（旧页淡出、新页浮入），这里沿用同名类 */
</style>
