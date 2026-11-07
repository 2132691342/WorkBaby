import { createRouter, createWebHashHistory } from 'vue-router'

// 桌面应用用 hash 路由：WebView 里的 file 协议不接受 history 模式的路径。
// 标题只存 i18n key，展示时由标题栏翻译——存死中文的话切了语言也不会跟着变。
const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/overview' },
    { path: '/overview', name: 'overview', component: () => import('@/views/OverviewView.vue'), meta: { titleKey: 'nav.overview' } },
    { path: '/chat', name: 'chat', component: () => import('@/views/ChatView.vue'), meta: { titleKey: 'nav.chat' } },
    { path: '/workspace', name: 'workspace', component: () => import('@/views/WorkspaceView.vue'), meta: { titleKey: 'nav.workspace' } },
    { path: '/sessions', name: 'sessions', component: () => import('@/views/SessionsView.vue'), meta: { titleKey: 'nav.sessions' } },
    { path: '/apps', name: 'apps', component: () => import('@/views/AppsView.vue'), meta: { titleKey: 'nav.apps' } },
    { path: '/models', name: 'models', component: () => import('@/views/ModelsView.vue'), meta: { titleKey: 'nav.models' } },
    { path: '/skills', name: 'skills', component: () => import('@/views/SkillsView.vue'), meta: { titleKey: 'nav.skills' } },
    { path: '/plugins', name: 'plugins', component: () => import('@/views/PluginsView.vue'), meta: { titleKey: 'nav.plugins' } },
    { path: '/mcp', name: 'mcp', component: () => import('@/views/McpView.vue'), meta: { titleKey: 'nav.mcp' } },
    { path: '/usage', name: 'usage', component: () => import('@/views/UsageView.vue'), meta: { titleKey: 'nav.usage' } },
    { path: '/settings', name: 'settings', component: () => import('@/views/SettingsView.vue'), meta: { titleKey: 'nav.settings' } },
  ],
})

export default router
