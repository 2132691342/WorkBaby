// UnoCSS 配置：原子类是「布局 + 间距 + 排版」的唯一写法，刻度全部接到 --wb-* 令牌，
// 所以色值与尺寸仍然只在 themes.css 定义一处（缩放、主题切换自动跟随）。
import { defineConfig, presetUno, presetIcons, transformerDirectives } from 'unocss'

/** 间距刻度：与 themes.css 的 --wb-sp-* 一一对应（1 单位 = 4px × 视口比例） */
const sp = (n: number) => `var(--wb-sp-${n})`

export default defineConfig({
  presets: [presetUno(), presetIcons()],
  transformers: [transformerDirectives()],
  theme: {
    spacing: {
      0: '0',
      px: '1px',
      1: sp(1),
      2: sp(2),
      3: sp(3),
      4: sp(4),
      5: sp(5),
      6: sp(6),
      8: sp(8),
      10: sp(10),
      12: sp(12),
    },
    borderRadius: {
      none: '0',
      xs: 'var(--wb-radius-xs)',
      sm: 'var(--wb-radius-sm)',
      DEFAULT: 'var(--wb-radius)',
      lg: 'var(--wb-radius-lg)',
      xl: 'var(--wb-radius-xl)',
      '2xl': 'var(--wb-radius-2xl)',
      full: 'var(--wb-radius-full)',
    },
    fontSize: {
      '2xs': ['var(--wb-fs-2xs)', '1.45'],
      xs: ['var(--wb-fs-xs)', '1.5'],
      sm: ['var(--wb-fs-sm)', '1.55'],
      md: ['var(--wb-fs-md)', '1.6'],
      lg: ['var(--wb-fs-lg)', '1.4'],
      xl: ['var(--wb-fs-xl)', '1.3'],
      '2xl': ['var(--wb-fs-2xl)', '1.2'],
    },
    colors: {
      bg: 'var(--wb-bg)',
      surface: 'var(--wb-surface)',
      surface2: 'var(--wb-surface-2)',
      hover: 'var(--wb-surface-hover)',
      border: 'var(--wb-border)',
      'border-strong': 'var(--wb-border-strong)',
      ink: 'var(--wb-ink)',
      ink2: 'var(--wb-ink-2)',
      muted: 'var(--wb-muted)',
      primary: 'var(--wb-primary)',
      'primary-strong': 'var(--wb-primary-strong)',
      'primary-soft': 'var(--wb-primary-soft)',
      live: 'var(--wb-live)',
      success: 'var(--wb-success)',
      warning: 'var(--wb-warning)',
      danger: 'var(--wb-danger)',
    },
  },
  shortcuts: {
    // 布局高频组合
    'f-center': 'flex items-center justify-center',
    'f-between': 'flex items-center justify-between',
    'f-start': 'flex items-center',
    'f-col': 'flex flex-col',
    'f-1': 'flex-1 min-w-0',
    // 语义面
    'wb-panel': 'bg-surface border border-border rounded-xl',
    'wb-inset': 'bg-surface2 border border-border rounded-lg',
    // 卡片内边距两档
    'wb-pad': 'p-5',
    'wb-pad-sm': 'p-4',
    // 可点行（侧栏 / 列表）
    'wb-row':
      'flex items-center gap-3 px-3 rounded-lg cursor-pointer transition-all duration-150 hover:bg-[var(--wb-nav-hover-bg)]',
  },
  content: {
    pipeline: {
      include: [/\.(vue|ts|html)($|\?)/],
    },
  },
})
