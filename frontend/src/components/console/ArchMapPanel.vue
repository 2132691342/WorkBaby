<script setup lang="ts">
// 架构页签：本地静态分析的分层依赖图（specs/13）。
// 布局按依赖深度分层：入口在上、被依赖的在下；边用贝塞尔曲线，不引图形库。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import type { ArchGraph, ArchNode } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import PageState from '../common/PageState.vue'

const graph = ref<ArchGraph | null>(null)
// 初始就是 true：首屏正在做全仓静态分析（几十秒），先给骨架而不是「还没有图」，
// 后者会让用户以为功能坏了而先点一次「重新分析」。
const loading = ref(true)
const error = ref('')
const picked = ref<ArchNode | null>(null)
// 防重入用独立的标志：拿 loading 当守卫会把 onMounted 的首次分析一起挡掉，
// 页面永远停在骨架上——「架构页打不开」就是这么来的。
const analyzing = ref(false)

// 分层布局常量：列宽固定，同层横向均布。确定性布局 = 刷新不跳。
const COL_W = 168
const ROW_H = 56
const NODE_W = 132
const NODE_H = 34

interface Placed extends ArchNode { x: number; y: number; col: number }

const placed = computed<Placed[]>(() => {
  if (!graph.value) return []
  const byDepth = new Map<number, ArchNode[]>()
  for (const n of graph.value.nodes ?? []) {
    const arr = byDepth.get(n.depth) || []
    arr.push(n)
    byDepth.set(n.depth, arr)
  }
  const out: Placed[] = []
  for (const [depth, list] of byDepth) {
    list.forEach((n, i) => {
      out.push({
        ...n,
        depth,
        col: i,
        x: depth * COL_W,
        y: i * ROW_H,
      })
    })
  }
  return out
})

const posOf = computed(() => {
  const m = new Map<string, Placed>()
  for (const p of placed.value) m.set(p.id, p)
  return m
})

const width = computed(() => {
  const maxX = placed.value.reduce((m, p) => Math.max(m, p.x), 0)
  return maxX + NODE_W + 48
})
const height = computed(() => {
  const maxY = placed.value.reduce((m, p) => Math.max(m, p.y), 0)
  return maxY + NODE_H + 48
})

// 边：从父节点右侧中点画到子节点左侧中点。
const paths = computed(() => {
  if (!graph.value) return []
  return (graph.value.edges ?? []).map((e) => {
    const a = posOf.value.get(e.from)
    const b = posOf.value.get(e.to)
    if (!a || !b) return ''
    const x1 = a.x + NODE_W
    const y1 = a.y + NODE_H / 2
    const x2 = b.x
    const y2 = b.y + NODE_H / 2
    const mx = (x1 + x2) / 2
    return `M ${x1} ${y1} C ${mx} ${y1}, ${mx} ${y2}, ${x2} ${y2}`
  }).filter(Boolean)
})

async function analyze() {
  if (analyzing.value) return
  analyzing.value = true
  loading.value = true
  error.value = ''
  try {
    graph.value = await api.archmap.analyze('')
  } catch (e) {
    error.value = (e as Error)?.message || '分析失败'
  } finally {
    analyzing.value = false
    loading.value = false
  }
}
onMounted(analyze)

// 被依赖次数决定节点视觉重量：地基一眼可见。
const weight = (n: ArchNode) => (n.indegree >= 5 ? 'heavy' : n.indegree >= 2 ? 'mid' : '')
function pick(n: ArchNode) {
  picked.value = picked.value?.id === n.id ? null : n
}
</script>

<template>
  <PageState :loading="loading" :error="error">
    <header class="pane-head">
      <div>
        <h2>架构</h2>
        <p>本地解析代码的模块与依赖，不联网、不调模型。点一个模块看它被谁依赖。</p>
      </div>
      <button
        class="btn btn-sm btn-ghost"
        type="button"
        :class="{ 'is-loading': loading }"
        :disabled="loading"
        @click="analyze"
      >
        <AppIcon v-if="!loading" name="refresh" size="ic-xs" /> 重新分析
      </button>
    </header>

    <p v-if="graph" class="cov">
      {{ graph.coverage.parsed }} 个文件的引用可解析 ·
      {{ graph.coverage.dynamic }} 个动态引用未解析 ·
      {{ graph.coverage.skipped }} 个其他语言文件（只建了索引，没有依赖边）
    </p>

    <div v-if="graph" class="arch-wrap">
      <div class="arch-scroll">
        <svg
          class="arch-svg"
          :width="width"
          :height="height"
          :viewBox="`0 0 ${width} ${height}`"
          role="img"
          aria-label="模块依赖图"
          @click.self="picked = null"
        >
        <g>
          <path
            v-for="(d, i) in paths"
            :key="i"
            class="edge"
            :d="d"
            fill="none"
          />
        </g>
        <!-- SVG 的 <g> 默认不可聚焦。给 tabindex + role="button" + 回车/空格，
             整张依赖图才对键盘用户可达——否则图只是一个「只能看」的图片。 -->
        <g
          v-for="p in placed"
          :key="p.id"
          class="node"
          :class="[weight(p), { on: picked?.id === p.id }]"
          :transform="`translate(${p.x} ${p.y})`"
          role="button"
          tabindex="0"
          :aria-label="`${p.label}：${p.files} 个文件，被依赖 ${p.indegree} 次`"
          :aria-pressed="picked?.id === p.id"
          @click="pick(p)"
          @keydown.enter.prevent="pick(p)"
          @keydown.space.prevent="pick(p)"
        >
          <rect :width="NODE_W" :height="NODE_H" rx="8" />
          <text x="10" y="15">{{ p.label }}</text>
          <text x="10" y="28" class="sub">{{ p.files }} 个文件 · 被依赖 {{ p.indegree }}</text>
        </g>
        </svg>
      </div>

      <aside v-if="picked" class="node-info">
        <h3>{{ picked.label || '（根目录）' }}</h3>
        <dl>
          <dt>文件</dt><dd>{{ picked.files }}</dd>
          <dt>行数</dt><dd>{{ picked.lines }}</dd>
          <dt>被依赖</dt><dd>{{ picked.indegree }} 次</dd>
          <dt>依赖深度</dt><dd>{{ picked.depth }}</dd>
        </dl>
      </aside>
    </div>

    <div v-if="!graph && !loading" class="card blank">
      <span class="bt"><AppIcon name="git-branch" size="ic-lg" /></span>
      <b>还没有图</b>
      <p>点「重新分析」试一次。</p>
    </div>
  </PageState>
</template>

<style scoped>
.pane-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--wb-sp-4);
  margin-bottom: var(--wb-sp-3);
}
.pane-head h2 { font-size: var(--wb-fs-xl); color: var(--wb-ink); }
.pane-head p { font-size: var(--wb-fs-sm); color: var(--wb-muted); margin-top: var(--wb-sp-1); max-width: var(--wb-w-desc); }
.cov { font-size: var(--wb-fs-hint); color: var(--wb-muted); margin-bottom: var(--wb-sp-4); }
.arch-wrap { display: flex; gap: var(--wb-sp-4); align-items: flex-start; }
/* 画布按节点数算出来的尺寸渲染，超出就横竖滚动：
   SVG 不设 flex:1 —— 拉伸会连坐标一起缩放，图会被压扁。 */
.arch-scroll {
  flex: 1;
  min-width: 0;
  max-height: 62vh;
  overflow: auto;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius);
  background: var(--wb-surface);
}
.arch-svg {
  display: block;
}
.edge { stroke: var(--wb-line-2); stroke-width: 1.2; }
.node { cursor: pointer; }
.node rect {
  fill: var(--wb-surface-2);
  stroke: var(--wb-line-2);
  transition: stroke var(--wb-dur-fast) var(--wb-ease), fill var(--wb-dur-fast) var(--wb-ease);
}
.node:hover rect { stroke: var(--wb-primary); }
.node.on rect { stroke: var(--wb-primary); fill: var(--wb-primary-soft); }
/* SVG 里的焦点环要自己画：全局 ::focus-visible 的 outline 在 <g> 上不生效，
   键盘用户 Tab 到某个模块时看不到自己停在哪 */
.node:focus-visible {
  outline: none;
}
.node:focus-visible rect {
  stroke: var(--wb-primary);
  stroke-width: 2;
}
.node text { font-size: var(--wb-fs-sm); fill: var(--wb-ink); font-family: var(--font-sans); }
.node text.sub { font-size: var(--wb-fs-2xs); fill: var(--wb-muted); }
.node.mid rect { stroke: var(--wb-primary-line); }
.node.heavy rect { stroke: var(--wb-primary); stroke-width: 1.6; }
.node-info {
  width: 220px;
  flex-shrink: 0;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius);
  background: var(--wb-surface);
  padding: var(--wb-sp-4);
  animation: slide-in var(--wb-dur) var(--wb-ease);
}
@keyframes slide-in {
  from { transform: translateX(8px); opacity: 0; }
  to { transform: none; opacity: 1; }
}
.node-info h3 { font-size: var(--wb-fs-md); color: var(--wb-ink); margin-bottom: var(--wb-sp-3); }
.node-info dl { display: grid; grid-template-columns: auto 1fr; gap: var(--wb-sp-1) var(--wb-sp-3); font-size: var(--wb-fs-sm); }
.node-info dt { color: var(--wb-muted); }
.node-info dd { color: var(--wb-ink); text-align: right; }
.blank {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-10) var(--wb-sp-6);
  text-align: center;
  color: var(--wb-muted);
}
.blank b { color: var(--wb-ink); }
</style>
