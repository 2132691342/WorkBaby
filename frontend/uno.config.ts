// UnoCSS 配置：原子类用于布局与间距（flex/grid/gap/w-），色值与尺寸仍走 themes.css 的 --wb-* 令牌。
// presetUno 提供常用原子类；shortcuts 把高频组合收成语义名，避免模板里堆一长串类名。
import { defineConfig, presetUno, presetIcons, transformerDirectives } from 'unocss'

export default defineConfig({
  presets: [presetUno(), presetIcons()],
  transformers: [transformerDirectives()],
  shortcuts: {
    // 布局高频组合：与案例（Tailwind 风格）同名的写法直接可用
    'flex-center': 'flex items-center justify-center',
    'flex-between': 'flex items-center justify-between',
    'flex-col': 'flex flex-col',
    // 语义面：面板卡 / 读数卡（视觉走 --wb-* 令牌，不写死色值）
    'wb-card':
      'bg-[var(--wb-surface)] border border-[var(--wb-border)] rounded-[var(--wb-radius)]',
  },
  // 原子类里用到的 --wb-* 变量不需要主题化：色值永远回到令牌层
  theme: {},
})
