import type { ComputedRef } from 'vue'
import { computed } from 'vue'
import { useTheme } from './useTheme'

/**
 * 图表 token 化通道：ECharts / 其他 canvas 无法直接消费 CSS 变量，
 * 此 composable 把 themes.css 的 --wb-* token 同步成具体颜色字符串，
 * 随明暗主题重算（依赖 currentTheme，切换即失效重读）。
 *
 * <p>使用方不再写 `isDark ? '#xxx' : '#yyy'` 双份硬编码，
 * 图表配色与全站 token 单一真相源。
 */
export interface WbChartTheme {
  primary: string
  primaryStrong: string
  mint: string
  lemon: string
  lavender: string
  sky: string
  ink: string
  muted: string
  border: string
  surface: string
  surface2: string
}

const FALLBACK = '#0000' // 全透明：token 缺失时图表不出现脏色

function readVar(name: string): string {
  try {
    const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
    return v || FALLBACK
  } catch {
    return FALLBACK
  }
}

/**
 * 具体颜色 → 带透明度 rgba 字符串。
 * ECharts canvas 解析不了 color-mix()/var()，必须产出可解析值：
 * 支持 #rrggbb 与 rgba()（按原 alpha 缩放）；其他格式原样返回（无法加透明）。
 */
export function withAlpha(color: string, alpha: number): string {
  const hex = /^#([0-9a-fA-F]{6})$/.exec(color)
  if (hex) {
    const n = parseInt(hex[1], 16)
    return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`
  }
  const rgba = /^rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:\s*[,/]\s*([\d.]+))?\)/.exec(color)
  if (rgba) {
    const base = rgba[4] === undefined ? 1 : parseFloat(rgba[4])
    return `rgba(${rgba[1]}, ${rgba[2]}, ${rgba[3]}, ${(base * alpha).toFixed(3)})`
  }
  return color
}

/** 图表主题：读 CSS 变量实时值（主题切换时 computed 失效重读）。 */
export function useWbChartTheme(): ComputedRef<WbChartTheme> {
  const { currentTheme } = useTheme()
  return computed<WbChartTheme>(() => {
    void currentTheme.value // 主题切换 → 依赖失效 → 重算
    return {
      primary: readVar('--wb-primary'),
      primaryStrong: readVar('--wb-primary-strong'),
      mint: readVar('--wb-mint'),
      lemon: readVar('--wb-lemon'),
      lavender: readVar('--wb-lavender'),
      sky: readVar('--wb-sky'),
      ink: readVar('--wb-ink'),
      muted: readVar('--wb-muted'),
      border: readVar('--wb-border'),
      surface: readVar('--wb-surface'),
      surface2: readVar('--wb-surface-2')
    }
  })
}
