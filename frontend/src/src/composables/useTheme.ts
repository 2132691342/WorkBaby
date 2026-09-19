import { ref, computed } from 'vue'

/**
 * 主题系统：两套主题 light（浅蓝工作台，默认）/ dark（中性炭灰）。旧主题 id 由 {@link normalizeThemeID} 平滑归一。
 */

export type ThemeID = 'light' | 'dark'

export interface ThemeConfig {
  id: ThemeID
  nameKey: string
  preview: string          // CSS gradient 作预览色块
  primary: string          // 主色 hex
  primaryStrong: string    // 主色 hover 强
  bg: string              // 主背景
  surface: string         // 卡片表面
}

/** 归一旧主题 id：dark 显式保留，其余未知值→light（默认外观即浅蓝工作台）。 */
export function normalizeThemeID(id: string | null | undefined): ThemeID {
  if (id === 'dark') return 'dark'
  return 'light'
}

export interface BackgroundSettings {
  imageBase64: string | null
  opacity: number         // 0-1
  blur: number            // 0-20px
  extractedPrimary: string | null  // 用户上传图提取的主色
}

// v3 键：默认外观切成 light（浅蓝工作台），换键让存量用户的旧偏好一次性让位给新默认
const THEME_KEY = 'workbaby.theme.v3'
const BG_KEY = 'workbaby.background'

/**
 * 主题登记表：light / dark 两套，与 themes.css 的 {@code [data-theme=...]} 块一一对应。
 * 新增主题必须同时补 themes.css 的 CSS 块并在此登记。
 */
export const THEMES: ThemeConfig[] = [
  { id: 'light',  nameKey: 'settings.theme.light',  preview: 'linear-gradient(135deg,#e8f1fe,#2f7bf6)', primary: '#2f7bf6', primaryStrong: '#1c63dc', bg: '#eef4fd', surface: '#ffffff' },
  { id: 'dark',  nameKey: 'settings.theme.dark',  preview: 'linear-gradient(135deg,#191919,#6d6af0)', primary: '#6d6af0', primaryStrong: '#8f8cff', bg: '#202020', surface: '#262626' }
]

// 全局响应式状态
const currentTheme = ref<ThemeID>('light')
const background = ref<BackgroundSettings>({ imageBase64: null, opacity: 0.3, blur: 0, extractedPrimary: null })

/**
 * 主题 composable。
 */
export function useTheme() {
  /** 切换主题：归一旧值再应用 data-theme + EP dark class。token 唯一定义在 themes.css，本函数不写 inline CSS 变量。 */
  function setTheme(id: string): void {
    const themeID = normalizeThemeID(id)
    currentTheme.value = themeID
    if (typeof document === 'undefined') return
    const root = document.documentElement
    root.setAttribute('data-theme', themeID)
    if (themeID === 'dark') {
      root.classList.add('dark')
    } else {
      root.classList.remove('dark')
    }
    try {
      localStorage.setItem(THEME_KEY, themeID)
    } catch {
      // ignore
    }
  }

  /**
   * 初始化主题：localStorage 优先；无记录默认 light。
   * 不跟随系统 prefers-color-scheme，避免 WebView2 继承 Windows 深色模式造成启动时黑白跳变。
   */
  function initTheme(): void {
    try {
      const stored = localStorage.getItem(THEME_KEY)
      if (stored) {
        setTheme(stored)
        return
      }
    } catch {
      // ignore
    }
    setTheme('light')
  }

  /** 设置背景（图片 + 透明度 + 模糊度） */
  function setBackground(settings: BackgroundSettings): void {
    background.value = { ...settings }
    applyBackground()
    saveBackground()
  }

  /**
   * 上传背景图片：base64 直存 localStorage（≤5MB），自动提取主色覆盖 --wb-primary。
   * 提取失败时仍保存图片，仅不写主色。
   */
  function uploadBackground(file: File): Promise<void> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = async () => {
        const base64 = reader.result as string
        try {
          const extracted = await extractPrimaryColor(base64)
          setBackground({ ...background.value, imageBase64: base64, extractedPrimary: extracted })
          if (extracted && typeof document !== 'undefined') {
            document.documentElement.style.setProperty('--wb-primary', extracted)
          }
          resolve()
        } catch (e) {
          setBackground({ ...background.value, imageBase64: base64 })
          resolve()
        }
      }
      reader.onerror = () => reject(reader.error)
      reader.readAsDataURL(file)
    })
  }

  /** 清除自定义背景 */
  function clearBackground(): void {
    background.value = { imageBase64: null, opacity: 0.3, blur: 0, extractedPrimary: null }
    applyBackground()
    saveBackground()
  }

  /** 应用背景到 DOM */
  function applyBackground(): void {
    if (typeof document === 'undefined') return
    const root = document.documentElement
    if (background.value.imageBase64) {
      root.style.setProperty('--wb-user-bg-url', `url(${background.value.imageBase64})`)
      root.style.setProperty('--wb-user-bg-opacity', String(background.value.opacity))
      root.style.setProperty('--wb-user-bg-blur', `${background.value.blur}px`)
      document.body.classList.add('has-custom-bg')
    } else {
      root.style.removeProperty('--wb-user-bg-url')
      root.style.removeProperty('--wb-user-bg-opacity')
      root.style.removeProperty('--wb-user-bg-blur')
      document.body.classList.remove('has-custom-bg')
    }
  }

  function saveBackground(): void {
    try {
      localStorage.setItem(BG_KEY, JSON.stringify(background.value))
    } catch {
      // ignore
    }
  }

  function loadBackground(): void {
    try {
      const stored = localStorage.getItem(BG_KEY)
      if (stored) {
        background.value = JSON.parse(stored) as BackgroundSettings
        applyBackground()
      }
    } catch {
      // ignore
    }
  }

  /** 从图片 base64 提取主色：128×128 缩略 → 过滤近黑/白/灰像素 → 5-bit 量化 → 众数。 */
  async function extractPrimaryColor(base64: string): Promise<string | null> {
    if (typeof document === 'undefined') return null
    return new Promise((resolve, reject) => {
      const img = new Image()
      img.onload = () => {
        try {
          const SIZE = 128
          const canvas = document.createElement('canvas')
          canvas.width = SIZE
          canvas.height = SIZE
          const ctx = canvas.getContext('2d')
          if (!ctx) {
            resolve(null)
            return
          }
          ctx.drawImage(img, 0, 0, SIZE, SIZE)
          const data = ctx.getImageData(0, 0, SIZE, SIZE).data

          const colorCount = new Map<number, number>()
          for (let i = 0; i < data.length; i += 4) {
            const r = data[i]
            const g = data[i + 1]
            const b = data[i + 2]
            const a = data[i + 3]
            if (a < 128) continue
            // 跳过接近黑/白/灰
            const max = Math.max(r, g, b)
            const min = Math.min(r, g, b)
            if (max < 32 || min > 224) continue
            if (max - min < 16) continue
            // 量化到 5-bit per channel（32×32×32 候选池）
            const q = ((r >> 3) << 10) | ((g >> 3) << 5) | (b >> 3)
            colorCount.set(q, (colorCount.get(q) ?? 0) + 1)
          }

          if (colorCount.size === 0) {
            resolve(null)
            return
          }

          // 找众数
          let best = 0
          let bestCount = -1
          for (const [q, c] of colorCount) {
            if (c > bestCount) {
              best = q
              bestCount = c
            }
          }
          const r = ((best >> 10) & 0x1f) << 3
          const g = ((best >> 5) & 0x1f) << 3
          const b = (best & 0x1f) << 3
          resolve(rgbToHex(r, g, b))
        } catch (e) {
          reject(e)
        }
      }
      img.onerror = () => resolve(null)
      img.src = base64
    })
  }

  /** RGB 转 hex（带 # 前缀）。 */
  function rgbToHex(r: number, g: number, b: number): string {
    const toHex = (n: number): string => Math.max(0, Math.min(255, Math.round(n))).toString(16).padStart(2, '0')
    return `#${toHex(r)}${toHex(g)}${toHex(b)}`
  }

  return {
    currentTheme,
    background,
    themes: computed(() => THEMES),
    setTheme,
    uploadBackground,
    clearBackground,
    setBackground,
    initTheme,
    loadBackground,
    extractPrimaryColor
  }
}
