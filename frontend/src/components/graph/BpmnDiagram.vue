<script setup lang="ts">
// 工作流图：BPMN-JS 渲染 BPMN 2.0 XML。只读画布，缩放与平移可用。
// 没有 XML 时给一张占位图——空画布比报错更诚实。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import BpmnModeler from 'bpmn-js/lib/Modeler'
import type Viewer from 'bpmn-js/lib/Viewer'

const props = withDefaults(
  defineProps<{
    /** BPMN 2.0 XML 文本 */
    xml?: string
    height?: number
  }>(),
  { height: 360 },
)

const host = ref<HTMLDivElement | null>(null)
let viewer: Viewer | null = null
const failed = ref(false)

async function render() {
  if (!viewer) return
  failed.value = false
  if (!props.xml) {
    await viewer.clear()
    return
  }
  try {
    await viewer.importXML(props.xml)
    const canvas = viewer.get('canvas') as { zoom: (mode: string, padding?: unknown) => void }
    canvas.zoom('fit-viewport', 'auto')
  } catch {
    // XML 不合法时不让整页崩：标出画布不可用
    failed.value = true
  }
}

onMounted(async () => {
  if (!host.value) return
  const modeler = new BpmnModeler({ container: host.value })
  viewer = modeler as unknown as Viewer
  await render()
})

watch(() => props.xml, render)

onBeforeUnmount(() => {
  ;(viewer as unknown as { destroy?: () => void } | null)?.destroy?.()
  viewer = null
})
</script>

<template>
  <div class="bpmn-wrap">
    <div ref="host" class="bpmn" :style="{ height: `${height}px` }" />
    <div v-if="failed" class="bpmn-mask">
      <span>{{ $t('common.bpmnInvalid') }}</span>
    </div>
    <div v-else-if="!xml" class="bpmn-mask">
      <span>{{ $t('common.bpmnEmpty') }}</span>
    </div>
  </div>
</template>

<style scoped>
.bpmn-wrap {
  position: relative;
  border-radius: var(--wb-radius-lg);
  background: var(--wb-elev-raised);
  border: 1px solid var(--wb-border);
  overflow: hidden;
}
.bpmn {
  width: 100%;
}
.bpmn-mask {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--wb-elev-raised);
  color: var(--wb-muted);
  font-size: var(--wb-fs-sm);
  pointer-events: none;
}
</style>
