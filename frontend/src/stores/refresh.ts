// 后台装配驱动的清单刷新信号。
// 页面在自己的动作之后会重拉清单；但首启装配在后台跑完时，页面可能开着没动——
// 事件路由 bump 这里的计数，各页 watch 到变化就重拉一份。
import { defineStore } from 'pinia'

export const useRefreshStore = defineStore('refresh', {
  state: () => ({ plugins: 0, skills: 0, workspace: 0 }),
  actions: {
    bumpPlugins() {
      this.plugins++
    },
    bumpSkills() {
      this.skills++
    },
    bumpWorkspace() {
      this.workspace++
    },
  },
})
