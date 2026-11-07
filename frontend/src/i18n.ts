import { createI18n } from 'vue-i18n'
import type { Composer } from 'vue-i18n'
import zhCN from './locales/zh-CN'
import en from './locales/en'

// 文案一律走 i18n：key 按域分层（nav / chat / models / skills / plugins / usage / settings / common）。
// 注意 locale 消息里出现 @: 或 @. 或 {@ 时会被当成 linked 消息语法，正文里要写成 {'@'}。
const instance = createI18n({
  legacy: false,
  locale: 'zh-CN',
  fallbackLocale: 'zh-CN',
  messages: { 'zh-CN': zhCN, en },
})

export default instance

// 语言切换的唯一入口：Composer 模式下 locale 是个 ref。
// 设置页改 ui.locale 后经 store 调到这里，全场文案立刻换。
export function setLocale(locale: 'zh-CN' | 'en') {
  const composer = instance.global as unknown as Composer
  composer.locale.value = locale
}
