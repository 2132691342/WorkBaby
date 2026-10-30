<script setup lang="ts">
// 设置视图：左侧板块目录（三组七项）+ 右侧面板。当前板块落在 URL（/settings/:tab）上。
// 分组按「谁的事」划：模型 · 能力 · 应用（这个程序自己）。
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppIcon from '../components/common/AppIcon.vue'
import PageHeader from '../components/layout/PageHeader.vue'
import SidePanel from '../components/layout/SidePanel.vue'
import AppearancePanel from '../components/settings/AppearancePanel.vue'
import BehaviorPanel from '../components/settings/BehaviorPanel.vue'
import FeaturePanel from '../components/settings/FeaturePanel.vue'
import InspectPanel from '../components/settings/InspectPanel.vue'
import MCPPanel from '../components/settings/MCPPanel.vue'
import ProviderPanel from '../components/settings/ProviderPanel.vue'
import SkillPanel from '../components/settings/SkillPanel.vue'
import ToolPanel from '../components/settings/ToolPanel.vue'

const { t, te } = useI18n()
const route = useRoute()
const router = useRouter()

const GROUPS = [
  {
    key: 'model',
    items: [
      { key: 'providers', icon: 'sparkles' },
      { key: 'tools', icon: 'wrench' },
      { key: 'mcp', icon: 'globe' },
    ],
  },
  {
    key: 'capability',
    items: [
      { key: 'skills', icon: 'book' },
      { key: 'features', icon: 'zap' },
      { key: 'inspect', icon: 'search' },
    ],
  },
  {
    key: 'app',
    items: [
      { key: 'appearance', icon: 'contrast' },
      { key: 'behavior', icon: 'sliders' },
    ],
  },
] as const

const VALID = new Set<string>(GROUPS.flatMap((g) => g.items.map((i) => i.key)))
// 落点保持「模型服务」：设置页最常被点开的目的是加一个模型，不是看别的。
const tab = ref<string>(
  typeof route.params.tab === 'string' && VALID.has(route.params.tab) ? route.params.tab : 'providers',
)

watch(
  () => route.params.tab,
  (next) => {
    if (typeof next === 'string' && VALID.has(next)) tab.value = next
  },
)
watch(tab, (next) => {
  if (route.params.tab !== next) void router.replace({ name: 'settings', params: { tab: next } })
})

// 八个面板 v-if 懒挂载：只打开「外观」时不去拉模型服务与技能列表
const PANELS = {
  providers: ProviderPanel,
  skills: SkillPanel,
  features: FeaturePanel,
  inspect: InspectPanel,
  tools: ToolPanel,
  mcp: MCPPanel,
  appearance: AppearancePanel,
  behavior: BehaviorPanel,
} as const

const activePanel = computed(() => PANELS[tab.value as keyof typeof PANELS])

// features 的文案键 locales 还没收录：有键用键，没键退回字面量，键补上后自动切换
const titleOf = (key: string) => {
  const k = `settings.${key}.title`
  return te(k) ? t(k) : key === 'features' ? '功能' : key === 'inspect' ? '内省' : k
}
const descOf = (key: string) => {
  const k = `settings.${key}.desc`
  return te(k)
    ? t(k)
    : key === 'features'
      ? '内置能力的开关：关掉后它带的工具不再进入助手的工具表。'
      : key === 'inspect'
        ? '当前装配快照：工具 / 技能 / 命令 / 功能 / 系统提示词。'
        : k
}
</script>

<template>
  <div class="settings">
    <SidePanel width="sub" :title="t('nav.settings')">
      <nav class="set-nav">
        <div v-for="g in GROUPS" :key="g.key" class="set-group">
          <div class="wb-label set-grp">{{ t(`settings.groups.${g.key}`) }}</div>
          <button
            v-for="it in g.items"
            :key="it.key"
            class="set-item"
            :class="{ 'is-on': tab === it.key }"
            type="button"
            @click="tab = it.key"
          >
            <AppIcon :name="it.icon" size="ic-sm" />
            <span>{{ titleOf(it.key) }}</span>
          </button>
        </div>
      </nav>
    </SidePanel>

    <div class="set-main">
      <PageHeader :title="titleOf(tab)" :subtitle="descOf(tab)" />
      <div class="set-body">
        <component :is="activePanel" :key="tab" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.settings {
  display: flex;
  flex: 1;
  min-width: 0;
  min-height: 0;
}
.set-nav {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
  padding-top: var(--wb-sp-1);
}
.set-group {
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.set-grp {
  padding: var(--wb-sp-1) var(--wb-sp-3) var(--wb-sp-2);
}
.set-item {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  height: var(--wb-ctl-h-lg);
  padding: 0 var(--wb-sp-3);
  border-radius: var(--wb-radius);
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  text-align: left;
  transition:
    background var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease),
    box-shadow var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.set-item:hover {
  background: var(--wb-nav-hover-bg);
  color: var(--wb-ink);
}
.set-item:active {
  transform: scale(0.98);
}
.set-item.is-on {
  background: var(--wb-nav-active-bg);
  color: var(--wb-nav-active-ink);
  box-shadow: inset 2px 0 0 var(--wb-primary);
}
.set-main {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--wb-elev-window);
}
.set-body {
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-6);
}
.set-body :deep(.panel-wrap) {
  /* 面板自己可以改 --wb-panel-w：表单类面板要收窄，列表类面板需要宽度 */
  max-width: var(--wb-panel-w, calc(920px * var(--wb-vw-scale)));
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-6);
}
</style>
