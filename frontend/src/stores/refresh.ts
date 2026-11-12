// 后台动作驱动的清单刷新信号。
// 页面在自己的动作之后会重拉清单；但后台跑完的事情（技能落位之类）页面可能开着没动，
// 事件路由 bump 这里的计数，各页 watch 到变化就重拉一份。
import { defineStore } from 'pinia'

export const useRefreshStore = defineStore('refresh', {
  state: () => ({ skills: 0, workspace: 0 }),
  actions: {
    bumpSkills() {
      this.skills++
    },
    bumpWorkspace() {
      this.workspace++
    },
  },
})
