// 应用入口：装配状态、路由、样式并挂载根组件。
import { createPinia } from 'pinia'
import { computed, createApp } from 'vue'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import enUs from 'element-plus/es/locale/lang/en'
import App from './App.vue'
import router from './router'
import { i18n } from './i18n'
import './bootstrap'
import { useToastStore } from './stores/toast'
import './themes.css'
import './wb-ui.css'
import './ep-theme.css'
import 'virtual:uno.css'
import 'highlight.js/styles/github.css'

// 组件基元逐步从 wb-ui.css 迁到 Element Plus（阶段迁移期两者共存）；
// EP 的文案跟随 i18n。
const app = createApp(App)
// 渲染层异常必须浮出水面：无声白屏是「对话全没了」这类报告的温床。
// toast 在 pinia 装好后才可用，启动早期的错误只落 console。
app.config.errorHandler = (err, _inst, info) => {
  console.error('[WorkBaby]', info, err)
  try {
    useToastStore().bad(`界面出现异常：${(err as Error)?.message || '请重启应用'}`)
  } catch {
    /* toast 还没装好（pinia 之前）：只落 console */
  }
}
app.use(createPinia())
app.use(router)
app.use(i18n)
app.use(ElementPlus)
// EP 的内置文案跟随当前语言：由 App.vue 根部的 ElConfigProvider 消费这个 computed。
app.provide('epLocale', computed(() => (i18n.global.locale.value === 'en' ? enUs : zhCn)))
app.mount('#app')
