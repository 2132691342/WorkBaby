// 开发态假后端必须是第一个 import：App.vue 依赖链上会拉起 bootstrap，
// 而 bootstrap 在模块顶层就注册事件监听，晚一步 window.runtime 还不存在。
import './dev/preview'

import { createPinia } from 'pinia'
import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import './bootstrap'
import './themes.css'
import './wb-ui.css'
import 'highlight.js/styles/github.css'

// 界面基元全部来自 wb-ui.css：不引第三方组件库，弹层/输入/表格都是自研基元。
const app = createApp(App)
// 渲染层异常必须浮出水面：无声白屏是「对话全没了」这类报告的温床。
// toast 在 pinia 装好后才可用，启动早期的错误只落 console。
app.config.errorHandler = (err, _inst, info) => {
  console.error('[WorkBaby]', info, err)
  import('./stores/toast')
    .then(({ useToastStore }) =>
      useToastStore().bad(`界面出现异常：${(err as Error)?.message || '请重启应用'}`),
    )
    .catch(() => {})
}
app.use(createPinia())
app.use(router)
app.mount('#app')
