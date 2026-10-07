import { ref, watch } from 'vue'

// 与 themes.css 的 [data-theme] 令牌块一一对应；改这里必须同步 CSS。
// swatch 走 tokens 里的主题预览色：切到深色时浅色卡也要显示自己的蓝，不能跟着变。
export const THEMES = [
  { key: 'light', name: '晨霭', swatch: 'var(--wb-swatch-light)' },
  { key: 'dark', name: '靛夜', swatch: 'var(--wb-swatch-dark)' },
] as const

const theme = ref<string>(localStorage.getItem('wb-theme') || 'light')

export function applyTheme(next: string) {
  document.documentElement.setAttribute('data-theme', next)
  localStorage.setItem('wb-theme', next)
}

export function useTheme() {
  watch(theme, applyTheme, { immediate: true })
  const setTheme = (next: string) => (theme.value = next)
  return { theme, setTheme, THEMES }
}
