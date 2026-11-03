import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN'
import en from './locales/en'

// 文案一律走 i18n：key 按域分层（nav / chat / models / skills / plugins / usage / settings / common）。
// 模板里出现 @ 时必须写成 {'@'}，否则会被当成 linked 消息语法。
export default createI18n({
  legacy: false,
  locale: 'zh-CN',
  fallbackLocale: 'zh-CN',
  messages: { 'zh-CN': zhCN, en },
})
