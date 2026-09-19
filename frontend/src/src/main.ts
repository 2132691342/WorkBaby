import { computed, createApp, defineComponent, h } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus, { ElConfigProvider } from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import enUs from 'element-plus/es/locale/lang/en'
import App from './App.vue'
import router from './router'
import { useTheme } from '@/composables/useTheme'
import { currentLocale } from '@/i18n'
// Element Plus 全量引入 + dark/css-vars：同 Java 版一致
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import './style.css'
import './themes.css'

// 主题必须在 mount 前同步应用：与 Java 版完全一致（避免首帧闪烁）
useTheme().initTheme()
useTheme().loadBackground()

/**
 * 全局 JS 异常拦截（Phase 1 stub：Wails 桌面端主进程已收日志）。
 *
 * <p>Java 版通过 navigator.sendBeacon → /api/v1/logs/js-error 上报到后端 workbaby.log；
 * Go 版无需此通道：window.go.runtime.ConsoleLog（实际应通过 wails 日志转发器落盘），
 * Phase 1 控制台输出即可，日志桥接放在 Phase 5 的「可观测打磨」中。
 */
function reportError(kind: 'error' | 'unhandledrejection', payload: Record<string, unknown>): void {
  // 控制台 + 静默吞掉，避免 catch 链再抛错
  // eslint-disable-next-line no-console
  console.error(`[js:${kind}]`, payload)
}

window.addEventListener('error', (e) => {
  reportError('error', {
    message: e.message,
    source: e.filename,
    line: e.lineno,
    col: e.colno,
    stack: e.error instanceof Error ? e.error.stack : undefined
  })
})

window.addEventListener('unhandledrejection', (e) => {
  const reason = e.reason
  reportError('unhandledrejection', {
    message: reason instanceof Error ? reason.message : String(reason),
    stack: reason instanceof Error ? reason.stack : undefined
  })
})

/** Element Plus 内置文案（分页 / 日期选择器 / 空状态等）按应用语言映射。 */
const epLocales: Record<string, typeof zhCn> = { 'zh-CN': zhCn, 'en-US': enUs }

/** 根组件：经 ConfigProvider 下发 locale，使 EP 内置文案随应用语言切换。 */
const Root = defineComponent({
  name: 'WorkBabyRoot',
  setup() {
    const locale = computed(() => epLocales[currentLocale.value] ?? zhCn)
    return () => h(ElConfigProvider, { locale: locale.value }, { default: () => h(App) })
  }
})

const app = createApp(Root)
app.use(createPinia())
app.use(router)
app.use(ElementPlus)
app.mount('#app')
