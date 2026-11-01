<script setup lang="ts">
// 设置视图：左侧板块目录 + 右侧面板。当前板块落在 URL（/settings/:tab）上。
// 只管「这个程序自己」：外观、行为、内省快照。
// 模型 / 工具 / 技能 / 插件 / MCP 已升格为一级导航下的独立页面。
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppIcon from '../components/common/AppIcon.vue'
import PageHeader from '../components/layout/PageHeader.vue'
import SidePanel from '../components/layout/SidePanel.vue'
import AppearancePanel from '../components/settings/AppearancePanel.vue'
import BehaviorPanel from '../components/settings/BehaviorPanel.vue'
import InspectPanel from '../components/settings/InspectPanel.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const GROUPS = [
  {
    key: 'app',
    items: [
      { key: 'appearance', icon: 'contrast' },
      { key: 'behavior', icon: 'sliders' },
    ],
  },
  {
    key: 'advanced',
    items: [{ key: 'inspect', icon: 'search' }],
  },
] as const

const VALID = new Set<string>(GROUPS.flatMap((g) => g.items.map((i) => i.key)))

// 旧二级 tab 的新归属：书签、托盘菜单与历史记录里可能还留着 /settings/providers 这类路径。
const MOVED: Record<string, string> = {
  providers: 'models',
  tools: 'models',
  mcp: 'mcp',
  skills: 'skills',
  features: 'plugins',
}

// 落点是「外观」：加模型这件事已经不在这里发生了。
const tab = ref<string>(
  typeof route.params.tab === 'string' && VALID.has(route.params.tab)
    ? route.params.tab
    : 'appearance',
)

watch(
  () => route.params.tab,
  (next) => {
    if (typeof next !== 'string') return
    // 已经升成一级页的：改跳新页，别停在这里等一个搬走了的面板
    if (MOVED[next]) {
      void router.replace({ name: MOVED[next] })
      return
    }
    if (VALID.has(next)) tab.value = next
  },
  { immediate: true },
)

watch(tab, (next) => {
  if (route.params.tab !== next) void router.replace({ name: 'settings', params: { tab: next } })
})

// 面板按 tab 挂载：只打开「外观」时不去拉内省快照的数据
const PANELS = {
  appearance: AppearancePanel,
  behavior: BehaviorPanel,
  inspect: InspectPanel,
} as const

const activePanel = computed(() => PANELS[tab.value as keyof typeof PANELS])

// 三个板块的文案键 locales 都已收录，不再需要字面量兜底
const titleOf = (key: string) => t(`settings.${key}.title`)
const descOf = (key: string) => t(`settings.${key}.desc`)
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
