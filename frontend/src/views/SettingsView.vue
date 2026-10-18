<script setup lang="ts">
// 设置页：六个板块共用一套「左侧导航 + 右侧内容」的骨架。
// 每个板块只管自己那块，切换时才懒加载对应数据。
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppIcon from '../components/common/AppIcon.vue'
import AppearancePanel from '../components/settings/AppearancePanel.vue'
import BehaviorPanel from '../components/settings/BehaviorPanel.vue'
import MCPPanel from '../components/settings/MCPPanel.vue'
import ExtensionPanel from '../components/settings/ExtensionPanel.vue'
import ProviderPanel from '../components/settings/ProviderPanel.vue'
import SkillPanel from '../components/settings/SkillPanel.vue'
import ToolPanel from '../components/settings/ToolPanel.vue'
import { useSettingsStore } from '../stores/settings'

const router = useRouter()
const route = useRoute()
const settings = useSettingsStore()

type TabKey = 'providers' | 'skills' | 'extensions' | 'tools' | 'mcp' | 'appearance' | 'behavior'

// tab 直接反映 URL：设置页支持前进后退，刷新后停在原处。
const tab = ref<TabKey>('providers')

const tabs: { key: TabKey; name: string; icon: string; title: string; sub: string }[] = [
  { key: 'providers', name: '模型服务', icon: 'cpu', title: '模型服务', sub: '告诉助手用哪家服务、哪个模型' },
  { key: 'skills', name: '技能', icon: 'sparkles', title: '技能', sub: '把「怎么做一件事」写成秘籍，助手会照着做' },
  { key: 'extensions', name: '插件', icon: 'zap', title: '插件', sub: 'PI 的扩展与包：新工具、新命令、新技能' },
  { key: 'tools', name: '工具', icon: 'wrench', title: '工具', sub: 'PI 当前声明给模型的工具（含 MCP 与插件）' },
  { key: 'mcp', name: 'MCP', icon: 'braces', title: 'MCP 外部工具', sub: '把外部服务的工具接进来' },
  { key: 'appearance', name: '外观', icon: 'contrast', title: '外观', sub: '主题、字体与字号' },
  { key: 'behavior', name: '行为', icon: 'sliders', title: '行为', sub: '执行方式、后台驻留与工作目录' },
]

const current = computed(() => tabs.find((t) => t.key === tab.value) || tabs[0])

function readTabFromPath(raw: unknown): TabKey {
  const key = typeof raw === 'string' ? raw : ''
  return tabs.some((t) => t.key === key) ? (key as TabKey) : 'providers'
}

// URL 是唯一真相：进设置页、刷新、前进后退都靠它对齐。
watch(
  () => route.params.tab,
  (raw) => {
    tab.value = readTabFromPath(raw)
  },
  { immediate: true },
)

function go(key: TabKey) {
  tab.value = key
  router.push(`/settings/${key}`)
}

onMounted(async () => {
  // 引导数据拿不到时不阻塞页面：各面板自己会显示错误态
  try {
    await settings.loadBoot()
  } catch {
    /* ignore */
  }
  // 进设置页时按服务端权威值重读一次：本地 values 可能在别处被乐观改写过
  // （改设置是先落本地再发请求），进来时对齐一次才不会显示一个服务端没有的值。
  if (settings.ready) await settings.syncSettings()
})
</script>

<template>
  <div class="settings-page wb-ui">
    <aside class="side">
      <div class="side-brand">
        <div class="logo">WB</div>
        <div>
          <b>设置</b>
          <small>WorkBaby</small>
        </div>
      </div>
      <div class="nav">
        <!-- EP 菜单：选中态与导航语言一致（淡底 + 主色） -->
        <el-menu :default-active="tab" class="nav-menu" @select="(k: string) => go(k as TabKey)">
          <el-menu-item v-for="t in tabs" :key="t.key" :index="t.key">
            <AppIcon :name="t.icon" />
            <span>{{ t.name }}</span>
          </el-menu-item>
        </el-menu>
      </div>
      <div class="side-foot">
        <span class="ver" :title="settings.boot?.version ? `WorkBaby ${settings.boot.version}` : ''">
          WorkBaby{{ settings.boot?.version ? ` v${settings.boot.version}` : '' }}
        </span>
      </div>
    </aside>

    <div class="settings-main scroll">
      <div class="wrap-md">
        <header class="page-head">
          <h1>{{ current.title }}</h1>
          <p>{{ current.sub }}</p>
        </header>
        <!-- v-if 懒挂载：只打开「外观」时不去拉模型服务与技能列表。
             key 绑 tab，否则同一份组件实例会被复用、过渡不触发。 -->
        <Transition name="view" mode="out-in">
          <ProviderPanel v-if="tab === 'providers'" :key="tab" />
          <SkillPanel v-else-if="tab === 'skills'" :key="tab" />
          <ExtensionPanel v-else-if="tab === 'extensions'" :key="tab" />
          <ToolPanel v-else-if="tab === 'tools'" :key="tab" />
          <MCPPanel v-else-if="tab === 'mcp'" :key="tab" />
          <AppearancePanel v-else-if="tab === 'appearance'" :key="tab" />
          <BehaviorPanel v-else :key="tab" />
        </Transition>
      </div>
    </div>
  </div>
</template>

<style scoped>
.settings-page {
  display: flex;
  height: 100%;
  min-height: 0;
  /* 同 ChatView：flex 子项不写 flex:1 会塌成内容宽度 */
  flex: 1;
  min-width: 0;
}
.side {
  /* flex 子项默认 min-width:auto，窄屏下会把主区挤到 0 宽 */
  flex: none;
  min-width: 0;
}
.nav {
  display: grid;
  gap: 2px;
  padding: 0 var(--wb-sp-3);
}
/* EP 菜单去自带样式：底透明、无边框、高度跟控件刻度 */
.nav-menu {
  border-right: none;
  background: transparent;
  --el-menu-bg-color: transparent;
  --el-menu-hover-bg-color: var(--wb-tint);
  --el-menu-active-color: var(--wb-primary);
  --el-menu-text-color: var(--wb-ink-2);
  --el-menu-item-height: var(--wb-ctl-h-lg);
}
.nav-menu .el-menu-item {
  border-radius: var(--wb-radius);
  margin-bottom: 2px;
  gap: var(--wb-sp-2);
}
.nav-menu .el-menu-item.is-active {
  background: var(--wb-primary-soft);
  font-weight: 600;
}
.settings-main {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  overflow-x: hidden;
}
.wrap-md {
  /* 宽屏不再把内容挤在半边：上限跟随视口比例令牌，窄窗自身可收缩 */
  width: 100%;
  max-width: calc(1280px * var(--wb-vw-scale));
  margin: 0 auto;
  padding: var(--wb-sp-8) var(--wb-sp-6) var(--wb-sp-10);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
  min-width: 0;
}
.page-head {
  display: grid;
  gap: var(--wb-sp-1);
  padding-bottom: var(--wb-sp-1);
}
.page-head h1 {
  font-family: var(--font-display);
  font-size: var(--wb-fs-lg);
  font-weight: 600;
  color: var(--wb-ink);
  letter-spacing: -0.01em;
}
.page-head p {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
/* 面板本身也要能收缩，否则里面的长路径照样撑破 */
.wrap-md > * {
  min-width: 0;
}
/* 侧栏底部：版本号居中弱化，不抢板块导航 */
.side-foot {
  justify-content: center;
}
.ver {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  opacity: 0.75;
}
</style>
