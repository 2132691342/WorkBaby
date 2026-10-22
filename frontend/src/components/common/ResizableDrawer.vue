<!-- 可拖宽度的抽屉：左边缘可左右拖动改宽，默认占窗口 1/3，宽度按 key 记住。
     抽屉宽度是阅读宽度的直接决定因素：固定尺寸在不同分辨率上要么太窄要么太宽。 -->
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'

defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    modelValue: boolean
    title?: string
    /** 宽度记忆键：不同抽屉各自记住自己的宽度 */
    storageKey: string
    /** 首次打开的宽度占窗口比例 */
    defaultRatio?: number
    minRatio?: number
    maxRatio?: number
  }>(),
  { title: '', defaultRatio: 1 / 3, minRatio: 0.2, maxRatio: 0.85 },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: boolean): void }>()

const STORE_PREFIX = 'wb.drawer.'
const width = ref(initialWidth())
const dragging = ref(false)

function clamp(px: number): number {
  const base = window.innerWidth || 1440
  const min = Math.max(320, Math.round(base * props.minRatio))
  const max = Math.round(base * props.maxRatio)
  return Math.min(Math.max(px, min), Math.max(min, max))
}

function initialWidth(): number {
  const base = window.innerWidth || 1440
  const saved = Number(localStorage.getItem(STORE_PREFIX + props.storageKey) || 0)
  if (saved > 0) return clamp(saved)
  return clamp(Math.round(base * props.defaultRatio))
}

function persist() {
  localStorage.setItem(STORE_PREFIX + props.storageKey, String(Math.round(width.value)))
}

let moveHandler: ((e: PointerEvent) => void) | null = null
let upHandler: ((e: PointerEvent) => void) | null = null

/** 拖拽期间只改宽度，松手才落盘：每次 pointermove 都写 localStorage 会让手势发涩 */
function startDrag(e: PointerEvent) {
  if (e.button !== 0) return
  e.preventDefault()
  dragging.value = true
  const base = window.innerWidth || 1440
  moveHandler = (ev: PointerEvent) => {
    // 抽屉贴右边缘：宽度就是「窗口宽度 − 指针横坐标」
    width.value = clamp(base - ev.clientX)
  }
  upHandler = () => {
    dragging.value = false
    persist()
    stopDrag()
  }
  window.addEventListener('pointermove', moveHandler)
  window.addEventListener('pointerup', upHandler)
  window.addEventListener('pointercancel', upHandler)
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
}

function stopDrag() {
  if (moveHandler) window.removeEventListener('pointermove', moveHandler)
  if (upHandler) {
    window.removeEventListener('pointerup', upHandler)
    window.removeEventListener('pointercancel', upHandler)
  }
  moveHandler = null
  upHandler = null
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
}

/** 双击把手回到默认的 1/3：拖坏了不用手动对 */
function resetWidth() {
  width.value = clamp(Math.round((window.innerWidth || 1440) * props.defaultRatio))
  persist()
}

const size = computed(() => `${Math.round(width.value)}px`)

function onResize() {
  width.value = clamp(width.value)
}

watch(
  () => props.modelValue,
  (open) => {
    if (!open) return
    width.value = initialWidth()
    window.addEventListener('resize', onResize)
  },
)

onBeforeUnmount(() => {
  stopDrag()
  window.removeEventListener('resize', onResize)
})
</script>

<template>
  <el-drawer
    :model-value="modelValue"
    direction="rtl"
    :size="size"
    :title="title"
    class="wb-drawer"
    v-bind="$attrs"
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
  >
    <slot />
  </el-drawer>

  <!-- 把手挂在 body 上、贴着抽屉左边缘：抽屉本体是滚动容器，内部绝对定位会跟着内容滚 -->
  <Teleport to="body">
    <div
      v-if="modelValue"
      class="wb-drawer-grip"
      :class="{ 'is-dragging': dragging }"
      :style="{ right: size }"
      role="separator"
      aria-orientation="vertical"
      :aria-label="`${title}`"
      @pointerdown="startDrag"
      @dblclick="resetWidth"
    />
  </Teleport>
</template>

<style scoped>
.wb-drawer-grip {
  position: fixed;
  top: 0;
  bottom: 0;
  width: calc(8px * var(--wb-vw-scale));
  z-index: 2100;
  cursor: col-resize;
  transition: background var(--wb-dur-fast) var(--wb-ease);
}
.wb-drawer-grip::after {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  right: calc(3px * var(--wb-vw-scale));
  width: 2px;
  background: transparent;
  transition: background var(--wb-dur-fast) var(--wb-ease);
}
.wb-drawer-grip:hover::after,
.wb-drawer-grip.is-dragging::after {
  background: var(--wb-primary);
}
</style>
