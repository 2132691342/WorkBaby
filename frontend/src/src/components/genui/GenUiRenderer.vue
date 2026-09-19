<script setup lang="ts">
/**
 * GenUI 树渲染器：递归渲染 LLM 产出的 UiNode 子树，按 `node.kind` 派发到 70 个节点分支。
 *
 * <p>交互与表单节点走 ElementPlus；`model3d` / `live-camera` / `three-js-frame`
 * 等重依赖节点先以占位渲染。样式色板复用 Tailwind 语义色，由外层 `.wb-genui`
 * 作用域重映射到 wb-* 主题变量（见 style.css）。
 */
import { computed } from 'vue'
import ChartNode from './ChartNode.vue'
import { openExternal } from '@/api/shellBridge'
import { t } from '@/i18n'

export interface UiNode {
  node_id: string
  kind: string
  props?: Record<string, unknown>
  children?: UiNode[]
}

const props = defineProps<{ node: UiNode }>()

function num(v: unknown, d = 0): number {
  return typeof v === 'number' ? v : d
}
function str(v: unknown, d = ''): string {
  return typeof v === 'string' ? v : d
}
function bool(v: unknown): boolean {
  return v === true || v === 'true'
}
function arr(v: unknown): unknown[] {
  return Array.isArray(v) ? v : []
}

const nodeKind = computed(() => props.node.kind.toLowerCase())
</script>

<template>
  <!-- ===== Layout (11) ===== -->
  <div v-if="nodeKind === 'stack'" class="flex flex-col" :style="{ gap: num(node.props?.gap, 12) + 'px', padding: num(node.props?.padding, 0) + 'px' }">
    <GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" />
  </div>

  <div v-else-if="nodeKind === 'grid'" class="grid" :style="{ gridTemplateColumns: `repeat(${num(node.props?.cols, 3)}, minmax(0, 1fr))`, gap: num(node.props?.gap, 12) + 'px' }">
    <GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" />
  </div>

  <div v-else-if="nodeKind === 'row'" class="flex flex-row items-center" :style="{ gap: num(node.props?.gap, 8) + 'px' }">
    <GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" />
  </div>

  <div v-else-if="nodeKind === 'spacer'" :style="{ height: num(node.props?.size, 16) + 'px', width: num(node.props?.size, 16) + 'px' }" />

  <div v-else-if="nodeKind === 'scroll-area'" class="overflow-auto" :style="{ maxHeight: str(node.props?.maxHeight, '400px') }">
    <GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" />
  </div>

  <div v-else-if="nodeKind === 'tabs'" class="rounded-lg border border-gray-300 bg-gray-50/60">
    <div class="flex gap-1 border-b border-gray-300 px-2 py-1">
      <button v-for="(t, i) in arr(node.props?.labels)" :key="i" type="button" class="rounded px-2 py-0.5 text-xs text-gray-700" :class="i === num(node.props?.active, 0) ? 'bg-gray-200' : ''">{{ String(t) }}</button>
    </div>
    <div class="p-2"><GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" /></div>
  </div>

  <div v-else-if="nodeKind === 'tab-item'" class="px-1 py-0.5 text-sm text-gray-800">{{ str(node.props?.label, '') }}</div>

  <div v-else-if="nodeKind === 'accordion'" class="rounded-lg border border-gray-300 bg-gray-50/60 divide-y divide-gray-200">
    <GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" />
  </div>

  <details v-else-if="nodeKind === 'accordion-item'" class="group p-2" :open="bool(node.props?.open)">
    <summary class="cursor-pointer text-sm font-semibold text-gray-800">{{ str(node.props?.label, '') }}</summary>
    <div class="pt-1"><GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" /></div>
  </details>

  <div v-else-if="nodeKind === 'aspect-box'" class="relative w-full" :style="{ aspectRatio: str(node.props?.ratio, '16/9') }">
    <div class="absolute inset-0"><GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" /></div>
  </div>

  <div v-else-if="nodeKind === 'design-surface'" class="rounded-lg border border-dashed border-gray-300 bg-gray-50/40 p-4" :style="{ minHeight: num(node.props?.minHeight, 120) + 'px' }">
    <GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" />
  </div>

  <!-- ===== Typography (4) ===== -->
  <component :is="`h${Math.max(1, Math.min(6, num(node.props?.level, 2)))}`" v-else-if="nodeKind === 'heading'" class="font-semibold text-gray-900">
    {{ str(node.props?.content, '') }}
  </component>

  <p v-else-if="nodeKind === 'text'" class="text-gray-800" :class="bool(node.props?.muted) ? 'text-gray-500' : ''">{{ str(node.props?.content, '') }}</p>

  <hr v-else-if="nodeKind === 'divider'" class="border-gray-300" :class="bool(node.props?.vertical) ? 'border-l h-full' : 'border-t w-full'" />

  <div v-else-if="nodeKind === 'skeleton'" class="animate-pulse rounded bg-gray-200" :style="{ width: str(node.props?.width, '100%'), height: str(node.props?.height, '16px') }" />

  <!-- ===== Data Display (18) ===== -->
  <span v-else-if="nodeKind === 'badge'" class="inline-block rounded-full bg-emerald-100/40 px-2 py-0.5 text-xs text-emerald-700" :class="str(node.props?.color, '') === 'red' ? 'bg-red-100/40 text-red-700' : ''">{{ str(node.props?.label, '') }}</span>

  <span v-else-if="nodeKind === 'tag'" class="inline-block rounded bg-gray-200 px-2 py-0.5 text-xs text-gray-800">{{ str(node.props?.label, '') }}</span>

  <div v-else-if="nodeKind === 'stat'" class="rounded-lg border border-gray-300 bg-gray-50/60 p-3">
    <div class="text-xs text-gray-500">{{ str(node.props?.label, '') }}</div>
    <div class="mt-1 text-2xl font-bold text-gray-900">{{ num(node.props?.value, 0) }}<span v-if="node.props?.unit" class="ml-1 text-sm text-gray-500">{{ str(node.props?.unit, '') }}</span></div>
    <div v-if="node.props?.delta !== undefined" class="mt-1 text-xs" :class="Number(node.props.delta) >= 0 ? 'text-emerald-400' : 'text-red-400'">{{ Number(node.props.delta) >= 0 ? '↑' : '↓' }} {{ Math.abs(Number(node.props.delta)) }}%</div>
  </div>

  <div v-else-if="nodeKind === 'progress'" class="h-2 w-full overflow-hidden rounded bg-gray-100"><div class="h-full bg-emerald-500" :style="{ width: Math.min(100, Math.max(0, num(node.props?.value, 0))) + '%' }" /></div>

  <div v-else-if="nodeKind === 'avatar'" class="flex h-10 w-10 items-center justify-center overflow-hidden rounded-full bg-gray-200 text-sm text-gray-800">
    <img v-if="node.props?.src" :src="str(node.props?.src, '')" class="h-full w-full object-cover" />
    <span v-else>{{ str(node.props?.label, '?').slice(0, 2) }}</span>
  </div>

  <img v-else-if="nodeKind === 'image'" :src="str(node.props?.src, '')" :alt="str(node.props?.alt, '')" class="max-w-full rounded" :style="{ width: str(node.props?.width, ''), height: str(node.props?.height, '') }" />

  <video v-else-if="nodeKind === 'video'" :src="str(node.props?.src, '')" controls class="max-w-full rounded" :style="{ width: str(node.props?.width, ''), height: str(node.props?.height, '') }" />

  <div v-else-if="nodeKind === 'model3d'" class="flex h-40 items-center justify-center rounded-lg border border-gray-300 bg-gray-50/60 text-xs text-gray-500">{{ t('genui.model3dPlaceholder', str(node.props?.src, '')) }}</div>

  <div v-else-if="nodeKind === 'live-camera'" class="flex h-32 items-center justify-center rounded-lg border border-gray-300 bg-gray-50/60 text-xs text-gray-500">{{ t('genui.cameraPlaceholder') }}</div>

  <span v-else-if="nodeKind === 'icon'" class="text-gray-700">{{ str(node.props?.glyph, '●') }}</span>

  <table v-else-if="nodeKind === 'table'" class="w-full text-sm"><GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" /></table>

  <tr v-else-if="nodeKind === 'table-row'" class="border-b border-gray-200"><GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" /></tr>

  <td v-else-if="nodeKind === 'table-cell'" class="px-2 py-1 text-gray-800" :class="bool(node.props?.header) ? 'font-semibold text-gray-900' : ''">{{ str(node.props?.content, '') }}</td>

  <ul v-else-if="nodeKind === 'list'" class="list-disc pl-5 text-gray-800"><GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" /></ul>

  <li v-else-if="nodeKind === 'list-item'" class="text-gray-800">{{ str(node.props?.content, '') }}<GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" /></li>

  <pre v-else-if="nodeKind === 'code-block'" class="overflow-auto rounded bg-wb-surface p-3 text-xs leading-relaxed text-gray-800"><code>{{ str(node.props?.content, '') }}</code></pre>

  <div v-else-if="nodeKind === 'markdown'" class="text-sm"><pre class="whitespace-pre-wrap font-sans text-gray-800">{{ str(node.props?.content, '') }}</pre></div>

  <ChartNode v-else-if="nodeKind === 'chart'" :type="str(node.props?.type, 'line')" :data="node.props?.data" :labels="node.props?.labels" />

  <!-- ===== Cards (17) ===== -->
  <div v-else-if="nodeKind === 'card'" class="rounded-lg border border-gray-300 bg-gray-50/60 p-3">
    <div v-if="node.props?.title" class="mb-2 text-sm font-semibold text-gray-800">{{ str(node.props?.title, '') }}</div>
    <GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" />
  </div>

  <div v-else-if="nodeKind === 'weather-card'" class="rounded-lg border border-gray-300 bg-gray-50/60 p-3">
    <div class="text-xs text-gray-500">{{ str(node.props?.location, '') }}</div>
    <div class="text-2xl font-bold text-gray-900">{{ num(node.props?.temp, 0) }}°</div>
    <div class="text-xs text-gray-700">{{ str(node.props?.condition, '') }}</div>
  </div>

  <div v-else-if="nodeKind === 'data-card'" class="rounded-lg border border-gray-300 bg-gray-50/60 p-3">
    <div class="text-xs text-gray-500">{{ str(node.props?.label, '') }}</div>
    <div class="text-xl font-semibold text-gray-900">{{ str(node.props?.value, '') }}</div>
  </div>

  <div v-else-if="nodeKind === 'metric-card'" class="rounded-lg border border-gray-300 bg-gray-50/60 p-3">
    <div class="text-xs text-gray-500">{{ str(node.props?.label, '') }}</div>
    <div class="text-2xl font-bold text-gray-900">{{ str(node.props?.value, '') }}</div>
    <div class="text-xs" :class="Number(node.props?.delta ?? 0) >= 0 ? 'text-emerald-400' : 'text-red-400'">{{ Number(node.props?.delta ?? 0) >= 0 ? '↑' : '↓' }} {{ Math.abs(Number(node.props?.delta ?? 0)) }}%</div>
  </div>

  <div v-else-if="nodeKind === 'profile-card'" class="flex items-center gap-3 rounded-lg border border-gray-300 bg-gray-50/60 p-3">
    <div class="flex h-10 w-10 items-center justify-center rounded-full bg-gray-200 text-gray-800">{{ str(node.props?.name, '?').slice(0, 1) }}</div>
    <div><div class="text-sm font-semibold text-gray-900">{{ str(node.props?.name, '') }}</div><div class="text-xs text-gray-500">{{ str(node.props?.subtitle, '') }}</div></div>
  </div>

  <div v-else-if="nodeKind === 'media-card'" class="rounded-lg border border-gray-300 bg-gray-50/60 p-3">
    <img v-if="node.props?.src" :src="str(node.props?.src, '')" class="mb-2 max-w-full rounded" />
    <div class="text-sm font-semibold text-gray-900">{{ str(node.props?.title, '') }}</div>
    <div class="text-xs text-gray-500">{{ str(node.props?.subtitle, '') }}</div>
  </div>

  <div v-else-if="nodeKind === 'alert-card'" class="rounded-lg border border-amber-300 bg-amber-50/40 p-3">
    <div class="text-sm font-semibold text-amber-700">{{ str(node.props?.title, '') }}</div>
    <div class="text-xs text-amber-700">{{ str(node.props?.content, '') }}</div>
  </div>

  <div v-else-if="nodeKind === 'timeline-card'" class="rounded-lg border border-gray-300 bg-gray-50/60 p-3">
    <div v-for="(e, i) in arr(node.props?.events)" :key="i" class="flex gap-2 border-l border-gray-300 pl-3" :class="i > 0 ? 'mt-2' : ''">
      <span class="text-xs text-gray-500">{{ str((e as any)?.time, '') }}</span>
      <span class="text-sm text-gray-800">{{ str((e as any)?.title, '') }}</span>
    </div>
  </div>

  <div v-else-if="nodeKind === 'slide-deck'" class="rounded-lg border border-gray-300 bg-gray-50/60 p-3"><GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" /></div>

  <div v-else-if="nodeKind === 'slide'" class="rounded border border-gray-300 p-3"><GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" /></div>

  <div v-else-if="nodeKind === 'kpi-board'" class="grid grid-cols-2 gap-2">
    <div v-for="(c, i) in arr(node.props?.cards)" :key="i" class="rounded-lg border border-gray-300 bg-gray-50/60 p-3">
      <div class="text-xs text-gray-500">{{ str((c as any)?.label, '') }}</div>
      <div class="text-xl font-bold text-gray-900">{{ num((c as any)?.value, 0) }}<span v-if="(c as any)?.unit" class="ml-1 text-sm text-gray-500">{{ str((c as any)?.unit, '') }}</span></div>
    </div>
  </div>

  <div v-else-if="nodeKind === 'feature-grid'" class="grid grid-cols-2 gap-2">
    <div v-for="(f, i) in arr(node.props?.features)" :key="i" class="rounded-lg border border-gray-300 bg-gray-50/60 p-2"><div class="text-sm text-gray-900">{{ str((f as any)?.title, '') }}</div><div class="text-xs text-gray-500">{{ str((f as any)?.desc, '') }}</div></div>
  </div>

  <div v-else-if="nodeKind === 'stepper'" class="flex items-center gap-1">
    <template v-for="(s, i) in arr(node.props?.steps)" :key="i">
      <div class="flex items-center gap-1"><span class="flex h-5 w-5 items-center justify-center rounded-full text-xs" :class="i < num(node.props?.current, 0) ? 'bg-emerald-600 text-white' : 'bg-gray-200 text-gray-700'">{{ i + 1 }}</span><span class="text-xs text-gray-800">{{ String(s) }}</span></div>
      <span v-if="i < arr(node.props?.steps).length - 1" class="h-px w-4 bg-gray-200" />
    </template>
  </div>

  <blockquote v-else-if="nodeKind === 'quote-card'" class="rounded-lg border-l-4 border-emerald-600 bg-gray-50/60 p-3 italic text-gray-800">{{ str(node.props?.text, '') }}<footer class="mt-1 text-xs not-italic text-gray-500">— {{ str(node.props?.author, '') }}</footer></blockquote>

  <div v-else-if="nodeKind === 'image-gallery'" class="grid grid-cols-2 gap-2"><img v-for="(s, i) in arr(node.props?.images)" :key="i" :src="str(s, '')" class="max-w-full rounded" /></div>

  <div v-else-if="nodeKind === 'key-value-list'" class="divide-y divide-gray-200 text-sm">
    <div v-for="(it, i) in arr(node.props?.items)" :key="i" class="flex justify-between py-1"><span class="text-gray-500">{{ str((it as any)?.key, '') }}</span><span class="text-gray-900">{{ str((it as any)?.value, '') }}</span></div>
  </div>

  <div v-else-if="nodeKind === 'section-header'" class="flex items-center gap-2"><h3 class="text-lg font-semibold text-gray-900">{{ str(node.props?.title, '') }}</h3><span v-if="node.props?.subtitle" class="text-xs text-gray-500">{{ str(node.props?.subtitle, '') }}</span></div>

  <!-- ===== Interactive (8) ===== -->
  <el-button
    v-else-if="nodeKind === 'button'"
    type="primary"
    size="small"
    :disabled="bool(node.props?.disabled)"
  >
    {{ str(node.props?.label, 'Button') }}
  </el-button>

  <el-button v-else-if="nodeKind === 'interactive-button'" type="primary" size="small">
    {{ str(node.props?.label, 'Action') }}
  </el-button>

  <el-button
    v-else-if="nodeKind === 'toggle-button'"
    size="small"
    :type="bool(node.props?.active) ? 'primary' : 'default'"
  >
    {{ str(node.props?.label, '') }}
  </el-button>

  <el-button
    v-else-if="nodeKind === 'link-button'"
    link
    type="primary"
    size="small"
    tag="a"
    :href="str(node.props?.href, '#')"
    @click.prevent="openExternal(str(node.props?.href, '#'))"
  >
    {{ str(node.props?.label, '') }}
  </el-button>

  <el-input
    v-else-if="nodeKind === 'input'"
    size="small"
    :model-value="str(node.props?.value, '')"
    :placeholder="str(node.props?.placeholder, '')"
    :type="str(node.props?.type, 'text')"
    readonly
  />

  <el-select v-else-if="nodeKind === 'select'" size="small" :model-value="str(arr(node.props?.options)[0], '')">
    <el-option v-for="(o, i) in arr(node.props?.options)" :key="i" :label="str(o, '')" :value="str(o, '')" />
  </el-select>

  <el-tag v-else-if="nodeKind === 'chip'" size="small" effect="plain" round>
    {{ str(node.props?.label, '') }}
  </el-tag>

  <div v-else-if="nodeKind === 'chip-group'" class="flex flex-wrap gap-1"><GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" /></div>

  <!-- ===== Forms (6) ===== -->
  <el-form v-else-if="nodeKind === 'form'" class="flex flex-col gap-2" @submit.prevent>
    <GenUiRenderer v-for="c in node.children" :key="c.node_id" :node="c" />
  </el-form>

  <el-input-number
    v-else-if="nodeKind === 'number-input'"
    size="small"
    :model-value="num(node.props?.value, 0)"
    :placeholder="str(node.props?.placeholder, '')"
  />

  <el-switch
    v-else-if="nodeKind === 'switch'"
    size="small"
    :model-value="bool(node.props?.checked)"
    :active-text="str(node.props?.label, '')"
  />

  <el-slider
    v-else-if="nodeKind === 'slider'"
    size="small"
    :min="num(node.props?.min, 0)"
    :max="num(node.props?.max, 100)"
    :model-value="num(node.props?.value, 50)"
  />

  <input v-else-if="nodeKind === 'file-input'" type="file" class="text-sm text-gray-700" />

  <el-input
    v-else-if="nodeKind === 'textarea'"
    type="textarea"
    :rows="3"
    :model-value="str(node.props?.value, '')"
    :placeholder="str(node.props?.placeholder, '')"
    readonly
  />

  <!-- ===== Feedback (2) ===== -->
  <el-alert
    v-else-if="nodeKind === 'alert'"
    :type="str(node.props?.level, 'info') === 'error' ? 'error' : str(node.props?.level, 'info') === 'warning' ? 'warning' : 'info'"
    :title="str(node.props?.content, '')"
    :closable="false"
    show-icon
  />

  <div v-else-if="nodeKind === 'callout'" class="rounded-lg border-l-4 border-emerald-600 bg-gray-50/60 px-3 py-2 text-sm text-gray-800">{{ str(node.props?.content, '') }}</div>

  <!-- ===== Embed (4) ===== -->
  <iframe v-else-if="nodeKind === 'hosted-canvas-frame'" :src="str(node.props?.src, '')" class="w-full rounded border border-gray-300" :style="{ height: str(node.props?.height, '320px') }" />

  <iframe v-else-if="nodeKind === 'html-frame'" :srcdoc="str(node.props?.html, '')" class="w-full rounded border border-gray-300" :style="{ height: str(node.props?.height, '240px') }" sandbox="" />

  <div v-else-if="nodeKind === 'three-js-frame'" class="flex h-48 items-center justify-center rounded-lg border border-gray-300 bg-gray-50/60 text-xs text-gray-500">{{ t('genui.threejsPlaceholder', str(node.props?.src, '')) }}</div>

  <pre v-else-if="nodeKind === 'json-debug'" class="overflow-auto rounded bg-wb-surface p-3 text-xs text-gray-700">{{ JSON.stringify(node.props?.data, null, 2) }}</pre>

  <!-- 未知节点兜底 -->
  <div v-else class="rounded border border-amber-300 bg-amber-50/40 px-2 py-1 text-xs text-amber-700">{{ t('genui.unknownNode', node.kind, node.node_id) }}</div>
</template>
