// 路由表：hash 模式，视图按段懒加载。
// 概览打头（落地页），对话紧随，模型 / 技能 / 插件 / MCP / 用量各自一等，低频的压在底部。
import { createRouter, createWebHashHistory } from 'vue-router'

// 桌面单窗口应用：hash 模式最稳，不依赖任何服务端配置。
// 设置页用 /settings/:tab 而不是子路由，这样刷新与前进后退都能停在原处。
const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    // 落地页是概览：开窗口先看状态，而不是掉进一个还没模型的输入框
    { path: '/', redirect: '/overview' },
    { path: '/overview', name: 'overview', component: () => import('../views/OverviewView.vue') },
    { path: '/chat', name: 'chat', component: () => import('../views/ChatView.vue') },
    { path: '/workspace', name: 'workspace', component: () => import('../views/WorkspaceView.vue') },
    { path: '/sessions', name: 'sessions', component: () => import('../views/SessionsView.vue') },
    { path: '/agents', name: 'agents', component: () => import('../views/AgentsView.vue') },
    { path: '/apps', name: 'apps', component: () => import('../views/AppsView.vue') },
    { path: '/models', name: 'models', component: () => import('../views/ModelsView.vue') },
    { path: '/skills', name: 'skills', component: () => import('../views/SkillsView.vue') },
    { path: '/plugins', name: 'plugins', component: () => import('../views/FeaturesView.vue') },
    { path: '/mcp', name: 'mcp', component: () => import('../views/McpView.vue') },
    { path: '/usage', name: 'usage', component: () => import('../views/DashboardView.vue') },
    { path: '/help/:name?', name: 'help', component: () => import('../views/HelpView.vue') },
    // 设置是单页滚动，旧的板块路径一律落在同一处
    { path: '/settings/:tab?', name: 'settings', component: () => import('../views/SettingsView.vue') },
    // 兼容旧入口：文件已并入工作区，书签与托盘菜单里可能还留着 /files
    { path: '/features', redirect: '/plugins' },
    { path: '/dashboard', redirect: '/usage' },
    { path: '/files', redirect: '/workspace' },
    { path: '/:pathMatch(.*)*', redirect: '/overview' },
  ],
})

export default router
