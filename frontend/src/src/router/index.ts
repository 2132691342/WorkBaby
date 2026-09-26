import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'

/**
 * 路由表：Wails 单 HTML 资源下 history 模式无法 fallback，统一用 hash 路由。
 * 鉴权由 Wails 绑定承载，路由层不做守卫。
 *
 * <p>信息架构：外壳左栏只保留任务主链路（聊天 / 设置），
 * 功能页全部收进设置中心 —— 旧路由（/memory /skills …）重定向到 /settings?tab=x，
 * 命令面板与历史书签里的深链仍然可达。
 */

const routes: RouteRecordRaw[] = [
  // 启动即进工作区：AI 助手的主场景是聊天，概览经命令面板进入
  { path: '/', redirect: '/chat' },

  // 顶层（主链路）
  { path: '/chat', name: 'chat', component: () => import('@/components/chat/ChatView.vue') },
  { path: '/chat/:id', name: 'chat-session', component: () => import('@/components/chat/ChatView.vue'), props: true },
  { path: '/settings', name: 'settings', component: () => import('@/components/settings/SettingsView.vue') },

  // 兼容重定向：旧侧栏入口全部归入设置中心对应 section
  { path: '/home', redirect: '/chat' },
  { path: '/memory', redirect: '/settings?tab=memory' },
  { path: '/kdocs', redirect: '/settings?tab=kdocs' },
  { path: '/skills', redirect: '/settings?tab=skills' },
  { path: '/mcp', redirect: '/settings?tab=mcp' },
  { path: '/tools', redirect: '/settings?tab=tools' },
  { path: '/docs', redirect: '/settings?tab=docs' },
  { path: '/admin', redirect: '/settings?tab=about' },

  // 404 → 回聊天工作区
  { path: '/:pathMatch(.*)*', redirect: '/chat' }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

export default router
