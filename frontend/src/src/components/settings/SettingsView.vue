<script setup lang="ts">
import { computed, onMounted, ref, watch, defineAsyncComponent, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSettingsStore } from '@/stores/settings'
import { useAdminStore } from '@/stores/admin'
import { t } from '@/i18n'
import { ArrowRight } from '@/components/common/icons'
import SectionFallback from '@/components/settings/SectionFallback.vue'

/**
 * 设置中心：左侧三组导航 + 右侧内容区，路由 query 同步直达。
 *
 * <p>功能页全部收进设置：原侧栏的 记忆 / 知识库 / 技能 / MCP / 工具 / 工作流 /
 * 仪表盘 / 运行 / 任务 / 文件 / 文件夹 / 文档 / 桌宠 都成为这里的一个 section，
 * 旧路由（/memory 等）重定向到 /settings?tab=x，外壳左栏只保留任务主链路。
 * section 组件按需异步加载（首次点击才拉取 chunk）。
 */

interface SectionItem {
  id: SettingsTab
  labelKey: string
}

type SettingsTab =
  | 'models'
  | 'appearance'
  | 'advanced'
  | 'search'
  | 'skills'
  | 'subagents'
  | 'commands'
  | 'hooks'
  | 'mcp'
  | 'tools'
  | 'memory'
  | 'kdocs'
  | 'wiki'
  | 'dashboard'
  | 'runs'
  | 'files'
  | 'pet'
  | 'about'

/**
 * 四个主 tab：按「能 / 存 / 看 / 调」的频率从高到低排，常用前置、低频收进「系统」。
 *  - 模型：首次必填，配置后极少动
 *  - 能力：日常增强（启用 / 调权重）
 *  - 数据：长期沉淀（记忆 / 知识 / 文件 / 统计）
 *  - 系统：外观与一次性开关
 *
 * 删除的旧子项：
 *  - folders：被 files 覆盖（文件 = 附件、文件夹 = 文件分类），路由 /folders → /settings
 *  - docs：开发期 doc 浏览，普通用户无感；doc/ 仍可经 /docs 路由直接访问
 */
const tabs: { id: string; labelKey: string; items: SectionItem[] }[] = [
  {
    id: 'model',
    labelKey: 'settings.tab.model',
    items: [
      { id: 'models', labelKey: 'settings.tab.models' },
      { id: 'search', labelKey: 'settings.tab.search' }
    ]
  },
  {
    id: 'capability',
    labelKey: 'settings.tab.capability',
    items: [
      { id: 'tools', labelKey: 'nav.tools' },
      { id: 'skills', labelKey: 'nav.skills' },
      { id: 'mcp', labelKey: 'nav.mcp' },
      { id: 'subagents', labelKey: 'nav.subagents' },
      { id: 'commands', labelKey: 'nav.commands' },
      { id: 'hooks', labelKey: 'nav.hooks' }
    ]
  },
  {
    id: 'data',
    labelKey: 'settings.tab.data',
    items: [
      { id: 'memory', labelKey: 'settings.tab.memory' },
      { id: 'kdocs', labelKey: 'nav.knowledge' },
      { id: 'wiki', labelKey: 'nav.wiki' },
      { id: 'files', labelKey: 'nav.files' },
      { id: 'dashboard', labelKey: 'nav.dashboard' },
      { id: 'runs', labelKey: 'nav.runs' }
    ]
  },
  {
    id: 'system',
    labelKey: 'settings.tab.system',
    items: [
      { id: 'appearance', labelKey: 'settings.tab.appearance' },
      { id: 'advanced', labelKey: 'settings.tab.advanced' },
      { id: 'pet', labelKey: 'nav.pet' },
      { id: 'about', labelKey: 'settings.tab.about' }
    ]
  }
]

/** 异步 section 包装：chunk 加载失败时给可见错误，而不是静默空白。 */
function lazySection(loader: () => Promise<{ default: Component }>): Component {
  return defineAsyncComponent({
    loader,
    delay: 120,
    timeout: 20000,
    loadingComponent: SectionFallback,
    errorComponent: SectionFallback
  })
}

/** section 组件登记表：id → 异步组件（首次激活才加载 chunk）。 */
const sectionComponents: Record<SettingsTab, Component> = {
  models: lazySection(() => import('@/components/settings/tabs/ProviderSettings.vue')),
  appearance: lazySection(() => import('@/components/settings/tabs/AppearanceSettings.vue')),
  advanced: lazySection(() => import('@/components/settings/tabs/AdvancedSettings.vue')),
  search: lazySection(() => import('@/components/settings/tabs/SearchSettings.vue')),
  about: lazySection(() => import('@/components/settings/tabs/AboutSettings.vue')),
  skills: lazySection(() => import('@/components/skills/SkillsView.vue')),
  subagents: lazySection(() => import('@/components/agents/SubagentsView.vue')),
  commands: lazySection(() => import('@/components/commands/CommandsView.vue')),
  hooks: lazySection(() => import('@/components/hooks/HooksView.vue')),
  wiki: lazySection(() => import('@/components/wiki/WikiView.vue')),
  mcp: lazySection(() => import('@/components/mcp/McpServersView.vue')),
  tools: lazySection(() => import('@/components/tools/ToolsView.vue')),
  memory: lazySection(() => import('@/components/memory/MemoryCenterView.vue')),
  kdocs: lazySection(() => import('@/components/knowledge/KnowledgeDocsView.vue')),
  dashboard: lazySection(() => import('@/components/dashboard/DashboardView.vue')),
  runs: lazySection(() => import('@/components/runs/RunsView.vue')),
  files: lazySection(() => import('@/components/files/FilesView.vue')),
  pet: lazySection(() => import('@/components/pet/PetSpaceView.vue'))
}

const settings = useSettingsStore()
const adminStore = useAdminStore()
const route = useRoute()
const router = useRouter()

/** 当前激活 section：路由 query（settings?tab=memory）为真相源；首次访问默认走「模型」tab。 */
const activeTab = ref<SettingsTab>('models')
const activeComponent = computed(() => sectionComponents[activeTab.value])

/** 当前激活主 tab：含 activeTab 的那一组。 */
const activeGroup = computed(() => tabs.find((g) => g.items.some((i) => i.id === activeTab.value)) ?? tabs[0])

function onTabChange(id: SettingsTab): void {
  activeTab.value = id
  void router.replace({ query: { ...route.query, tab: id } })
}

/** 切主 tab：落到该组第一个子项；深链（?tab=x）仍以具体子项为准。 */
function onGroupChange(g: (typeof tabs)[number]): void {
  if (g.items[0]) onTabChange(g.items[0].id)
}

function backToWorkspace(): void {
  void router.push('/chat')
}

onMounted(async () => {
  const q = route.query.tab
  if (typeof q === 'string' && q in sectionComponents) activeTab.value = q as SettingsTab
  void settings.load()
  void adminStore.load()
})

// 深链 / 命令面板在设置页已挂载时改 query（settings?tab=x）也要跟随切换
watch(
  () => route.query.tab,
  (q) => {
    if (typeof q === 'string' && q in sectionComponents && q !== activeTab.value) {
      activeTab.value = q as SettingsTab
    }
  }
)
</script>

<template>
  <div class="wb-ui flex h-full min-h-0">
    <!-- 左导航：6 个主 tab（先选领域，再在右侧选细节） -->
    <aside class="set-nav">
      <button class="set-back" @click="backToWorkspace">
        <component :is="ArrowRight" class="h-3.5 w-3.5 rotate-180" />
        <span>{{ t('settings.backToWorkspace') }}</span>
      </button>

      <nav class="set-nav-scroll">
        <button
          v-for="g in tabs"
          :key="g.id"
          class="set-item"
          :class="{ 'is-active': g.id === activeGroup.id }"
          @click="onGroupChange(g)"
        >
          {{ t(g.labelKey) }}
        </button>
      </nav>
    </aside>

    <!-- 内容区：组内多子项时顶部给 chip 行 -->
    <div class="flex min-w-0 flex-1 flex-col">
      <div v-if="activeGroup.items.length > 1" class="set-subnav">
        <button
          v-for="item in activeGroup.items"
          :key="item.id"
          class="set-chip"
          :class="{ 'is-active': activeTab === item.id }"
          @click="onTabChange(item.id)"
        >
          {{ t(item.labelKey) }}
        </button>
      </div>
      <div v-if="settings.error" class="alert a-info mx-6 mt-4">
        {{ settings.error }}
      </div>
      <component :is="activeComponent" :key="activeTab" class="min-h-0 flex-1" />
    </div>
  </div>
</template>

<style scoped>
.set-nav {
  width: 216px;
  flex: none;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--wb-border);
  background: var(--wb-side, var(--wb-surface));
}
.set-back {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 12px 12px 4px;
  padding: 0 8px;
  height: 28px;
  border-radius: var(--wb-radius, 8px);
  color: var(--wb-muted);
  font-size: 12px;
}
.set-back:hover {
  background: var(--wb-surface-hover);
  color: var(--wb-ink);
}
.set-nav-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 6px 8px 12px;
}
.set-subnav {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 12px 24px 0;
}
.set-chip {
  height: 26px;
  padding: 0 12px;
  border-radius: 999px;
  border: 1px solid var(--wb-border);
  color: var(--wb-muted);
  font-size: 12px;
}
.set-chip:hover {
  color: var(--wb-ink);
  background: var(--wb-surface-hover);
}
.set-chip.is-active {
  color: var(--wb-primary-ink, #fff);
  background: var(--wb-primary);
  border-color: transparent;
}
.set-item {
  width: 100%;
  display: flex;
  align-items: center;
  padding: 0 8px;
  height: 28px;
  border-radius: var(--wb-radius, 8px);
  color: var(--wb-ink);
  font-size: 12px;
  text-align: left;
  transition: background 0.12s;
}
.set-item:hover {
  background: var(--wb-surface-hover);
}
.set-item.is-active {
  background: var(--wb-primary-soft);
  color: var(--wb-primary-strong);
  font-weight: 600;
}
</style>
