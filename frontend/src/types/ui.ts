// 界面基元的契约类型：组件 props 的形状单独放一份，SFC 的 export interface
// 不能被 import（vue-tsc 只认默认导出），而调用方要拿它写 computed 的返回类型。
// 与后端契约无关，纯粹是前端自己的形状。

/** 分段控件的一项。 */
export interface SegItem {
  /** 值：选中时经 update:modelValue 交上去 */
  value: string
  /** 显示文字；只放图标时留空，由 icon 承担语义 */
  label: string
  /** 可选图标：与文字并排，或只放图标不放文字 */
  icon?: string
  /** 隐藏的项：不进 DOM，而不是渲染成禁用 */
  hidden?: boolean
}

/** 浮层与锚点的水平对齐方式。 */
export type PopoverAlign = 'start' | 'end'

/** 卡片内边距档。 */
export type CardPad = 'sm' | 'md' | 'none'

/** 图标瓦片尺寸档。 */
export type TileSize = 'sm' | 'md' | 'lg'

/** 图标瓦片色调。 */
export type TileTone = 'neutral' | 'brand' | 'ok' | 'warn' | 'bad'
