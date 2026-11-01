// i18n 装配：首次启动按系统语言选中文 / 英文，之后以设置表为准。
import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN'
import en from './locales/en'

export type Locale = 'zh-CN' | 'en'

/** 系统语言 → 支持的语言清单；没匹配上按中文走。 */
export function detectLocale(): Locale {
  const raw = typeof navigator !== 'undefined' ? navigator.language || '' : ''
  return raw.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en'
}

export const i18n = createI18n({
  legacy: false,
  locale: detectLocale(),
  // 回退链最后落在中文：中文包是全的，英文缺 key 时不会出现空白
  fallbackLocale: 'zh-CN',
  messages: { 'zh-CN': zhCN, en },
})

/** 语言清单：设置页语言切换的取值表。名称用各自语言写，不翻译。 */
export const LOCALES: Array<{ key: Locale; name: string }> = [
  { key: 'zh-CN', name: '中文' },
  { key: 'en', name: 'English' },
]

export function setLocale(locale: Locale) {
  i18n.global.locale.value = locale
  document.documentElement.setAttribute('lang', locale)
}
