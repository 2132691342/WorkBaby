import { computed, ref } from 'vue'

/**
 * 前端 i18n：内置 zh-CN / en-US 全量字典（权威来源，离线可用），
 * `t()` 支持 `{0}` 占位插值；切语言只改本地 locale，全站响应式刷新。
 */
import { zhCN } from './dict-zh'
import { enUS } from './dict-en'
import type { Dict } from './types'

/** 语言选择的本地持久化键。 */
const STORAGE_KEY = 'wb.locale'

/** 内置字典表（按 locale tag 索引）。 */
const messages: Record<string, Dict> = {
  'zh-CN': zhCN,
  'en-US': enUS
}

/** 恢复上次选择的语言；无记录或不可用时回落 zh-CN。 */
function restoreLocale(): string {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved && messages[saved]) return saved
  } catch {
    // 无 localStorage（隐私模式 / 非安全上下文）时用默认语言
  }
  return 'zh-CN'
}

/** 当前语言 tag（zh-CN / en-US）。 */
const locale = ref<string>(restoreLocale())

/** 当前 locale 的权威字典（内置；无后端覆盖）。 */
const dict = computed<Dict>(() => messages[locale.value] ?? zhCN)

/** 保留 init 为幂等空操作：历史调用方（App.vue onMounted）无需改动。 */
export async function init(): Promise<void> {
  // 已无后端 i18n 端点，本地字典即权威来源。
}

/** 切换语言：本地即时生效（内置字典）并持久化，重启后保持。 */
export function setLocale(lang: string): void {
  if (!messages[lang]) return
  locale.value = lang
  try {
    localStorage.setItem(STORAGE_KEY, lang)
  } catch {
    // 持久化失败不影响本次会话生效
  }
}

/** 响应式当前 locale tag。 */
export const currentLocale = computed(() => locale.value)

/**
 * 取翻译；找不到 key 时回退 {@code key} 本身（开发期便于排查）。
 * 占位符：{@code {0}} / {@code {1}} …
 */
export function t(key: string, ...args: unknown[]): string {
  const v = dict.value[key] ?? zhCN[key] ?? key
  if (args.length === 0) return v
  return v.replace(/\{(\d+)\}/g, (_m, idx) => {
    const i = Number(idx)
    const a = args[i]
    return a === undefined || a === null ? '' : String(a)
  })
}
