<script setup lang="ts">
// 设置页：六个板块共用一套「左侧导航 + 右侧内容」的骨架。
// 每个板块只管自己那块，切换时才懒加载对应数据。
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppIcon from '../components/common/AppIcon.vue'
import AppearancePanel from '../components/settings/AppearancePanel.vue'
import BehaviorPanel from '../components/settings/BehaviorPanel.vue'
import KnowledgePanel from '../components/settings/KnowledgePanel.vue'
import ProviderPanel from '../components/settings/ProviderPanel.vue'
import SkillPanel from '../components/settings/SkillPanel.vue'
import ToolPanel from '../components/settings/ToolPanel.vue'
import { useSettingsStore } from '../stores/settings'

const router = useRouter()
const route = useRoute()
const settings = useSettingsStore()

type TabKey = 'providers' | 'skills' | 'knowledge' | 'tools' | 'appearance' | 'behavior'

// tab 直接反映 URL：设置页支持前进后退，刷新后停在原处。
const tab = ref<TabKey>('providers')

const tabs: { key: TabKey; name: string; icon: string; title: string; sub: string }[] = [
  { key: 'providers', name: '模型服务', icon: 'cpu', title: '模型服务', sub: '告诉助手用哪家服务、哪个模型' },
  { key: 'skills', name: '技能', icon: 'sparkles', title: '技能', sub: '把「怎么做一件事」写成秘籍，助手会照着做' },
  { key: 'knowledge', name: '知识库', icon: 'book', title: '知识库', sub: '把你的资料放进来，助手回答时会去查' },
  { key: 'tools', name: '工具', icon: 'wrench', title: '工具', sub: '助手现在能干什么、哪些要先问你' },
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
        <button
          v-for="t in tabs"
          :key="t.key"
          class="nav-item"
          :class="{ on: tab === t.key }"
          type="button"
          @click="go(t.key)"
        >
          <AppIcon :name="t.icon" /> {{ t.name }}
        </button>
      </div>
      <div class="side-foot">
        <button class="foot-btn" type="button" @click="router.push('/')">
          <AppIcon name="chevron-left" /> 返回对话
        </button>
      </div>
    </aside>

    <div class="settings-main scroll">
      <div class="wrap-md">
        <header class="page-head">
          <h1>{{ current.title }}</h1>
          <p>{{ current.sub }}</p>
        </header>
        <!-- v-if 懒挂载：只打开「外观」时不去拉模型服务与技能列表 -->
        <ProviderPanel v-if="tab === 'providers'" />
        <SkillPanel v-else-if="tab === 'skills'" />
        <KnowledgePanel v-else-if="tab === 'knowledge'" />
        <ToolPanel v-else-if="tab === 'tools'" />
        <AppearancePanel v-else-if="tab === 'appearance'" />
        <BehaviorPanel v-else />
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
.nav-item {
  width: 100%;
  min-width: 0;
}
.settings-main {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  overflow-x: hidden;
}
.wrap-md {
  width: 100%;
  max-width: 1080px;
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
</style>
