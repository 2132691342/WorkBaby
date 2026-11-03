// UnoCSS 配置：刻度全部接到 CSS 令牌上，原子类不绕过令牌。
// 这样「改一个令牌 = 全站跟着变」这条纪律才成立。
import { defineConfig, presetUno, presetAttributify, transformerDirectives } from 'unocss'

export default defineConfig({
  presets: [presetUno(), presetAttributify()],
  transformers: [transformerDirectives()],
  theme: {
    colors: {
      ink: 'var(--wb-ink)',
      mute: 'var(--wb-mute)',
      faint: 'var(--wb-faint)',
      line: 'var(--wb-line)',
      line2: 'var(--wb-line-strong)',
      surface: 'var(--wb-surface)',
      raised: 'var(--wb-raised)',
      hover: 'var(--wb-hover)',
      selected: 'var(--wb-selected)',
      work: 'var(--wb-work)',
      brand: 'var(--wb-brand)',
      brand2: 'var(--wb-brand-2)',
      brand3: 'var(--wb-brand-3)',
      ink5: 'var(--wb-ink-5)',
      ok: 'var(--wb-ok)',
      warn: 'var(--wb-warn)',
      bad: 'var(--wb-bad)',
    },
    spacing: {
      1: 'var(--wb-sp-1)',
      2: 'var(--wb-sp-2)',
      3: 'var(--wb-sp-3)',
      4: 'var(--wb-sp-4)',
      5: 'var(--wb-sp-5)',
      6: 'var(--wb-sp-6)',
      8: 'var(--wb-sp-8)',
      10: 'var(--wb-sp-10)',
      12: 'var(--wb-sp-12)',
      16: 'var(--wb-sp-16)',
    },
    borderRadius: {
      xs: 'var(--wb-r-xs)',
      sm: 'var(--wb-r-sm)',
      DEFAULT: 'var(--wb-r)',
      lg: 'var(--wb-r-lg)',
      xl: 'var(--wb-r-xl)',
    },
    fontSize: {
      '3xs': ['var(--wb-fs-3xs)', '1.35'],
      '2xs': ['var(--wb-fs-2xs)', '1.4'],
      xs: ['var(--wb-fs-xs)', '1.5'],
      sm: ['var(--wb-fs-sm)', '1.55'],
      base: ['var(--wb-fs-base)', '1.6'],
      lg: ['var(--wb-fs-lg)', '1.45'],
      xl: ['var(--wb-fs-xl)', '1.3'],
      '2xl': ['var(--wb-fs-2xl)', '1.25'],
    },
  },
  shortcuts: {
    // 卡片：分层只靠圆角 + 描边 + 一层极轻阴影，不做渐变
    card: 'bg-raised border border-line rounded-xl',
    'card-pad': 'card p-4',
    // 标签：全大写微字，靠字距拉开
    plate: 'text-2xs font-600 uppercase tracking-[0.08em] text-faint',
    row: 'flex items-center gap-3',
    'row-between': 'flex items-center justify-between gap-3',
    // 只读读数：不套描边胶囊、不做悬停缩放（有盒子没动作等于骗用户去点）
    readout: 'font-mono text-xs text-mute tabular-nums',
  },
})
