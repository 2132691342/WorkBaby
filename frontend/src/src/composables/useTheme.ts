import { ref, watch } from 'vue'

// 与 themes.css 的 [data-theme] 令牌块一一对应；改这里必须同步 CSS。
// swatch 走 tokens 里的主题预览色：切到深色时浅色卡也要显示自己的紫，不能跟着变。
export const THEMES = [
  { key: 'light', name: '晨紫', swatch: 'var(--wb-swatch-light)' },
  { key: 'dark', name: '夜紫', swatch: 'var(--wb-swatch-dark)' },
] as const

// 主题模块级单例：localStorage 只是「设置表还没读到时的兜底」，权威在 settings。
export const theme = ref<string>(localStorage.getItem('wb-theme') || 'light')

export function applyTheme(next: string) {
  document.documentElement.setAttribute('data-theme', next)
  localStorage.setItem('wb-theme', next)
}

// watch 放模块级只注册一次：写在 useTheme() 里会让每个调用方各注册一份，
// 主题变化被重复应用，且没有任何组件调用它就完全失效。
watch(theme, applyTheme, { immediate: true })

// 切换时同步写设置表：只存 localStorage 的话，换机器 / 清缓存后主题就丢了，
// 「启动时以库为准」这条路径也就不存在。落库失败不打断本地切换，下次会再试。
async function persist(next: string) {
  try {
    const { useSettingsStore } = await import('../stores/settings')
    await useSettingsStore().setValue('theme', next)
  } catch {
    /* 本地已生效；库里没写进去不阻塞用户 */
  }
}

export function useTheme() {
  const setTheme = (next: string) => {
    theme.value = next
    void persist(next)
  }
  return { theme, setTheme, THEMES }
}
