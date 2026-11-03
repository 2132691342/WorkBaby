import { createRouter, createWebHashHistory } from 'vue-router'

// 桌面应用用 hash 路由：WebView 里的 file 协议不接受 history 模式的路径
const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/overview' },
    { path: '/overview', name: 'overview', component: () => import('@/views/OverviewView.vue'), meta: { title: '概览' } },
    { path: '/chat', name: 'chat', component: () => import('@/views/ChatView.vue'), meta: { title: '对话' } },
    { path: '/workspace', name: 'workspace', component: () => import('@/views/WorkspaceView.vue'), meta: { title: '工作区' } },
    { path: '/sessions', name: 'sessions', component: () => import('@/views/SessionsView.vue'), meta: { title: '会话' } },
    { path: '/apps', name: 'apps', component: () => import('@/views/AppsView.vue'), meta: { title: '应用' } },
    { path: '/models', name: 'models', component: () => import('@/views/ModelsView.vue'), meta: { title: '模型' } },
    { path: '/skills', name: 'skills', component: () => import('@/views/SkillsView.vue'), meta: { title: '技能' } },
    { path: '/plugins', name: 'plugins', component: () => import('@/views/PluginsView.vue'), meta: { title: '插件' } },
    { path: '/mcp', name: 'mcp', component: () => import('@/views/McpView.vue'), meta: { title: 'MCP' } },
    { path: '/usage', name: 'usage', component: () => import('@/views/UsageView.vue'), meta: { title: '用量' } },
    { path: '/help/:name?', name: 'help', component: () => import('@/views/HelpView.vue'), meta: { title: '帮助' } },
    { path: '/settings', name: 'settings', component: () => import('@/views/SettingsView.vue'), meta: { title: '设置' } },
  ],
})

export default router
