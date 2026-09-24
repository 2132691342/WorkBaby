<script setup lang="ts">
import { computed, onMounted, ref, watch, defineAsyncComponent, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSettingsStore } from '@/stores/settings'
import { useAdminStore } from '@/stores/admin'
import { t } from '@/i18n'
import {
  Activity, ArrowLeft, Bot, BookOpen, Database, FileText, Folder, Globe, LayoutDashboard,
  Link2, Palette, PawPrint, ScrollText, Server, Shield, Sparkles, Terminal, Webhook,
  Wrench, Zap
} from '@/components/common/icons'
import SectionFallback from '@/components/settings/SectionFallback.vue'

/**
 * 设置中心：左导航一级化（19 个子项直接列出，不再「分组按钮 + chip 二次选择」）+ 右侧内容区。
 *
 * <p>功能页全部收进设置：原侧栏的 记忆 / 知识库 / 技能 / MCP / 工具 / 工作流 /
 * 仪表盘 / 运行 / 任务 / 文件 / 文件夹 / 文档 / 桌宠 都成为这里的一个 section，
 * 旧路由（/memory 等）重定向到 /settings?tab=x，外壳左栏只保留任务主链路。
 * section 组件按需异步加载（首次点击才拉取 chunk）。
 */

interface SectionItem {
  id: SettingsTab
  labelKey: string
  icon: Component
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
  | 'docs'
  | 'about'

/**
 * 分组只负责「视觉断行」，不再是可点击的层级：
 * 模型 / 能力 / 数据 / 系统，全部子项一跳直达。
 *
 * 删除的旧子项：
 *  - folders：被 files 覆盖（文件 = 附件、文件夹 = 文件分类），路由 /folders → /settings
 */
const tabs: { id: string; labelKey: string; items: SectionItem[] }[] = [
  {
    id: 'model',
    labelKey: 'settings.tab.model',
    items: [
      { id: 'models', labelKey: 'settings.tab.models', icon: Server },
      { id: 'search', labelKey: 'settings.tab.search', icon: Globe }
    ]
  },
  {
    id: 'capability',
    labelKey: 'settings.tab.capability',
    items: [
      { id: 'tools', labelKey: 'nav.tools', icon: Wrench },
      { id: 'skills', labelKey: 'nav.skills', icon: Zap },
      { id: 'mcp', labelKey: 'nav.mcp', icon: Webhook },
      { id: 'subagents', labelKey: 'nav.subagents', icon: Bot },
      { id: 'commands', labelKey: 'nav.commands', icon: Terminal },
      { id: 'hooks', labelKey: 'nav.hooks', icon: Link2 }
    ]
  },
  {
    id: 'data',
    labelKey: 'settings.tab.data',
    items: [
      { id: 'memory', labelKey: 'settings.tab.memory', icon: Database },
      { id: 'kdocs', labelKey: 'nav.knowledge', icon: BookOpen },
      { id: 'wiki', labelKey: 'nav.wiki', icon: ScrollText },
      { id: 'files', labelKey: 'nav.files', icon: Folder },
      { id: 'dashboard', labelKey: 'nav.dashboard', icon: LayoutDashboard },
      { id: 'runs', labelKey: 'nav.runs', icon: Activity }
    ]
  },
  {
    id: 'system',
    labelKey: 'settings.tab.system',
    items: [
      { id: 'appearance', labelKey: 'settings.tab.appearance', icon: Palette },
      { id: 'advanced', labelKey: 'settings.tab.advanced', icon: Shield },
      { id: 'pet', labelKey: 'nav.pet', icon: PawPrint },
      { id: 'docs', labelKey: 'nav.docs', icon: FileText },
      { id: 'about', labelKey: 'settings.tab.about', icon: Sparkles }
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
  pet: lazySection(() => import('@/components/pet/PetSpaceView.vue')),
  docs: lazySection(() => import('@/components/docs/DocsView.vue'))
}

const settings = useSettingsStore()
const adminStore = useAdminStore()
const route = useRoute()
const router = useRouter()

/** 当前激活 section：路由 query（settings?tab=memory）为真相源；首次访问默认走「模型」tab。 */
const activeTab = ref<SettingsTab>('models')
const activeComponent = computed(() => sectionComponents[activeTab.value])
/** 当前 section 的导航元数据：统一 hero 头直接复用导航的图标与标题，不再各页自写。 */
const activeItem = computed<SectionItem>(
  () => tabs.flatMap((g) => g.items).find((i) => i.id === activeTab.value) ?? tabs[0]!.items[0]!
)

function onTabChange(id: SettingsTab): void {
  activeTab.value = id
  void router.replace({ query: { ...route.query, tab: id } })
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
    <!-- 左导航：一级化——全部子项直接列出，分组标题只做视觉断行 -->
    <aside class="set-nav">
      <button class="set-back" @click="backToWorkspace">
        <ArrowLeft class="ic-sm" />
        <span>{{ t('settings.backToWorkspace') }}</span>
      </button>

      <div class="set-scroll">
        <template v-for="g in tabs" :key="g.id">
          <div class="set-group t-plate">{{ t(g.labelKey) }}</div>
          <button
            v-for="item in g.items"
            :key="item.id"
            type="button"
            class="set-item"
            :class="{ 'is-on': activeTab === item.id }"
            @click="onTabChange(item.id)"
          >
            <component :is="item.icon" class="ic-sm" />
            <span class="grow truncate">{{ t(item.labelKey) }}</span>
          </button>
        </template>
      </div>
    </aside>

    <!-- 内容区：统一 hero 头（图标 + 页名），各 section 自管滚动 -->
    <div class="flex min-w-0 flex-1 flex-col">
      <div class="set-hero flex-none px-6 pt-5 pb-3">
        <div class="hero-glyph">
          <component :is="activeItem.icon" class="ic" />
        </div>
        <div class="hero-txt">
          <h1>{{ t(activeItem.labelKey) }}</h1>
        </div>
      </div>
      <div v-if="settings.error" class="alert a-info mx-6 mt-4">
        {{ settings.error }}
      </div>
      <component :is="activeComponent" :key="activeTab" class="min-h-0 flex-1" />
    </div>
  </div>
</template>

<style scoped>
/* 左导航语法收口到 wb-ui.css 的 .set-nav / .set-back / .set-scroll / .set-group / .set-item，
   这里只补宽度与滚动容器，不再复制一套选中态（两份定义必然漂移）。 */
.set-nav {
  width: var(--wb-w-settings, 208px);
}
.set-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding-bottom: var(--wb-sp-5);
}
/* hero 头标题：设置页一级标题比工作区小一档（15px），与导航字级拉开但不夸张 */
.set-hero h1 {
  font-family: var(--font-display);
  font-size: var(--wb-fs-lg);
  font-weight: 600;
  letter-spacing: -0.01em;
  line-height: var(--wb-lh-tight);
  color: var(--wb-ink);
}
</style>
