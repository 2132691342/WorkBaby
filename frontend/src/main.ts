import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'virtual:uno.css'
import './themes.css'
import './base.css'
// EP 的令牌桥必须排在 element-plus 自带样式之后：
// 同样的选择器，后加载的赢。反过来则我们的映射全部被默认值盖掉。
import './ep-theme.css'
import './content.css'
import App from './App.vue'
import router from './router'
import i18n from './i18n'

createApp(App).use(createPinia()).use(router).use(i18n).use(ElementPlus).mount('#app')
