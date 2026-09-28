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
app.use(createPinia())
app.use(router)
app.mount('#app')
