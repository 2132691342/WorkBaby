// 路由表：hash 模式，视图按段懒加载。
import { createRouter, createWebHashHistory } from 'vue-router'

// 桌面单窗口应用：hash 模式最稳，不依赖任何服务端配置。
// 设置页用 /settings/:tab 而不是子路由，这样刷新与前进后退都能停在原处。
const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', name: 'chat', component: () => import('../views/ChatView.vue') },
    // 环境向导：pi_env.ready=false 时强制进这里，没有 PI 就没有对话。
    { path: '/setup', name: 'setup', component: () => import('../views/SetupView.vue') },
    { path: '/workspace', name: 'workspace', component: () => import('../views/WorkspaceView.vue') },
    { path: '/dashboard', name: 'dashboard', component: () => import('../views/DashboardView.vue') },
    { path: '/help/:name?', name: 'help', component: () => import('../views/HelpView.vue') },
    { path: '/settings/:tab?', name: 'settings', component: () => import('../views/SettingsView.vue') },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

export default router
