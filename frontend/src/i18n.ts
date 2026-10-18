// i18n 装配：中文默认 + 英文；语言偏好走设置表的 language 键（后端 KV）。
import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN'
import en from './locales/en'

export type Locale = 'zh-CN' | 'en'

export const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  fallbackLocale: 'zh-CN',
  messages: { 'zh-CN': zhCN, en },
})

/** 语言清单：设置页语言切换的取值表。 */
export const LOCALES: Array<{ key: Locale; name: string }> = [
  { key: 'zh-CN', name: '中文' },
  { key: 'en', name: 'English' },
]

export function setLocale(locale: Locale) {
  i18n.global.locale.value = locale
  document.documentElement.setAttribute('lang', locale)
}
