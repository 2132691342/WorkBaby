// 外观设置的取值表与落地逻辑：主题、字体、字号三件事。
// 单独成文件是因为「保存设置」和「把设置应用到 DOM」发生在两个地方
// （设置页点一下要立刻生效，App 启动时也要把已保存的值重新落一次），
// 放在组件里没法被第二个地方复用。
import { ref, watch } from 'vue'
import { useSettingsStore } from '../stores/settings'

// 字体族：只换正文族，标题与等宽保持各自语义。
export const FONTS = [
  { key: 'system', name: '系统默认', stack: 'var(--font-sans)', desc: '微软雅黑 / Segoe UI，最稳' },
  { key: 'song', name: '宋体', stack: "'SimSun', 'Songti SC', serif", desc: '正式文档观感' },
  { key: 'kai', name: '楷体', stack: "'KaiTi', 'STKaiti', serif", desc: '偏手写，公文常用' },
  { key: 'hei', name: '黑体', stack: "'SimHei', 'Microsoft YaHei', sans-serif", desc: '对比更强' },
  { key: 'fang', name: '仿宋', stack: "'FangSong', 'STFangsong', serif", desc: '公文标准字体' },
] as const

// 字号档位是刻度而不是任意值：改一处全站等比跟随，布局不会错位。
export const FONT_SIZES = [
  { key: 'sm', name: '小', scale: 0.9 },
  { key: 'md', name: '标准', scale: 1 },
  { key: 'lg', name: '大', scale: 1.12 },
  { key: 'xl', name: '特大', scale: 1.25 },
] as const

export const currentFont = ref('system')
export const currentSize = ref('md')

// applyAppearance 把当前取值写回 CSS 变量。theme 由 useTheme 负责，这里只管字体与字号。
export function applyAppearance() {
  const font = FONTS.find((f) => f.key === currentFont.value)
  document.documentElement.style.setProperty('--wb-font-ui', font?.stack || 'var(--font-sans)')
  const size = FONT_SIZES.find((s) => s.key === currentSize.value)
  document.documentElement.style.setProperty('--wb-fs-scale', String(size?.scale ?? 1))
}

// syncFromSettings 从设置表同步到 DOM：App 启动与设置页保存后都调它。
export function syncFromSettings() {
  const s = useSettingsStore()
  currentFont.value = s.values['font_family'] || 'system'
  currentSize.value = s.values['font_size'] || 'md'
  applyAppearance()
}

// 任何一处改了取值都自动落到 DOM，省掉每个调用点各写一遍 apply。
watch([currentFont, currentSize], applyAppearance)
