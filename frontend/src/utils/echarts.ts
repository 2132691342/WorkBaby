// ECharts 唯一装配点：按需注册图表与组件，色值一律运行时从 --wb-* 读取。
// ECharts 不认 CSS 变量，只能在渲染前把计算值取出来；主题切换后重读重绘。
import * as echarts from 'echarts/core'
import { LineChart, PieChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([LineChart, PieChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

export { echarts }

/** 读一个 CSS 变量（去空白），读不到时给兜底色 */
export function cssVar(name: string, fallback = '#6366f1'): string {
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v || fallback
}

/** 图表序列配色：四档靛蓝 + 一档兜底 */
export function chartPalette(): string[] {
  return [
    cssVar('--wb-ch-1'),
    cssVar('--wb-ch-2'),
    cssVar('--wb-ch-3'),
    cssVar('--wb-ch-4'),
    cssVar('--wb-ch-5'),
  ]
}

/** 所有图表共用的坐标轴 / 提示框样式 */
export function chartBase() {
  return {
    ink: cssVar('--wb-ink'),
    muted: cssVar('--wb-muted'),
    line: cssVar('--wb-border'),
    surface: cssVar('--wb-surface'),
    strong: cssVar('--wb-border-strong'),
  }
}

export function tooltipStyle() {
  const base = chartBase()
  return {
    backgroundColor: base.surface,
    borderColor: base.line,
    borderWidth: 1,
    padding: [8, 12] as [number, number],
    textStyle: { color: base.ink, fontSize: 12 },
    extraCssText: 'border-radius:10px;box-shadow:0 10px 30px -8px rgba(15,23,42,.18);',
  }
}
