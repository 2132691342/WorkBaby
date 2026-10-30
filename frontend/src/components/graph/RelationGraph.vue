<script setup lang="ts">
// 关系图谱：AntV X6 画节点与连线。用于把技能/工具/会话的关系画成图。
// 只做「看」不做「改」：节点由外部数据喂进来，拖动与连线不落库。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Graph } from '@antv/x6'

const props = withDefaults(
  defineProps<{
    /** 节点：id / 文本 / 分组（决定配色） */
    nodes: Array<{ id: string; label: string; group?: string }>
    /** 连线：源 / 目标 */
    edges: Array<{ source: string; target: string }>
    height?: number
  }>(),
  { height: 320 },
)

const host = ref<HTMLDivElement | null>(null)
let graph: Graph | null = null

// 分组配色：只取品牌五档里的三档，不另造色相
const GROUP_COLORS: Record<string, string> = {
  feature: '#6366f1',
  skill: '#818cf8',
  tool: '#1e293b',
}

function render() {
  if (!graph) return
  graph.getCells().forEach((cell) => cell.remove())
  for (const n of props.nodes) {
    graph.addNode({
      id: n.id,
      x: 40 + (Math.abs(hash(n.id)) % 5) * 140,
      y: 40 + (Math.abs(hash(n.id + 'y')) % 3) * 90,
      width: 130,
      height: 40,
      label: n.label,
      style: {
        fill: GROUP_COLORS[n.group ?? 'tool'] ?? GROUP_COLORS.tool,
        stroke: '#e0e7ff',
        lineWidth: 1,
        radius: 6,
        fontSize: 12,
        color: '#ffffff',
        textWrap: { width: 118, height: 36, ellipsis: true },
      },
    })
  }
  for (const e of props.edges) {
    if (!props.nodes.some((n) => n.id === e.source)) continue
    if (!props.nodes.some((n) => n.id === e.target)) continue
    graph.addEdge({
      source: { cell: e.source },
      target: { cell: e.target },
      connector: { name: 'smooth', args: { strength: 0.4 } },
      style: { stroke: '#818cf8', lineWidth: 1.5, targetMarker: { name: 'block', width: 8, height: 6 } },
      router: { name: 'manhattan' },
    })
  }
  graph.zoomToFit({ padding: 24, maxScale: 1.2 })
}

/** 简单字符串散列：只用来打散布局，不追求均匀分布。
 */
function hash(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0
  return h
}

onMounted(() => {
  if (!host.value) return
  graph = new Graph({
    container: host.value,
    autoResize: true,
    panning: true,
    mousewheel: { enabled: true, modifiers: ['ctrl', 'meta'] },
    interacting: { edgeMovable: false, vertexMovable: true },
    background: { color: 'transparent' },
  })
  render()
})

watch(() => [props.nodes, props.edges], render, { deep: true })

onBeforeUnmount(() => {
  graph?.dispose()
  graph = null
})
</script>

<template>
  <div ref="host" class="graph" :style="{ height: `${height}px` }" />
</template>

<style scoped>
.graph {
  width: 100%;
  border-radius: var(--wb-radius-lg);
  background: var(--wb-elev-raised);
  border: 1px solid var(--wb-border);
  overflow: hidden;
}
</style>
