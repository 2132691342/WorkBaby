// 浮层基元：贴着锚点弹 + 点外面关 + Esc 关 + 定位夹回视口，收成一处。
// 只做「怎么浮」不规定装什么，内容走默认插槽。
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useSlots, watch } from 'vue'

const props = defineProps<{
  /** 受控的开关；支持 v-model:open */
  open: boolean
  /** 与锚点的水平对齐：start 左缘对齐、end 右缘对齐；锚点窄浮层宽时用 start 再夹视口 */
  align?: 'start' | 'end'
  /** 与锚点的间距（px） */
  gap?: number
  /** 浮层最大高度（CSS 值）。超出时内部滚动，不顶出屏幕 */
  maxHeight?: string
  /** 视口边缘留白（px） */
  margin?: number
}>()

const emit = defineEmits<{ 'update:open': [open: boolean] }>()

const slots = useSlots()

/** 默认值在模板与 computed 里兜底：defineProps 的类型字面量不配 withDefaults（项目惯例）。 */
const align = computed(() => props.align ?? 'start')
const gap = computed(() => props.gap ?? 8)
const maxHeight = computed(() => props.maxHeight ?? '')
const margin = computed(() => props.margin ?? 8)

const root = ref<HTMLElement | null>(null)
const panel = ref<HTMLElement | null>(null)
/** 面板相对锚点的水平位移（px），由 clamp 写进来 */
const shift = ref(0)

/** 有没有锚内容：没有锚点就不该浮（面板会悬空）。 */
const hasAnchor = computed(() => Boolean(slots.default))

/** 关。 */
function close(): void {
  emit('update:open', false)
}

// 把面板夹回视口内：锚点在底栏中部、面板几百 px 宽时默认往右长会溢出屏右，
// 被裁掉的往往正是列表最要紧的一列；打开时量一次即可。
function clamp(): void {
  const anchor = root.value
  const box = panel.value
  if (!anchor || !box) return
  const rect = anchor.getBoundingClientRect()
  const width = box.offsetWidth
  const maxRight = window.innerWidth - margin.value
  const minLeft = margin.value
  if (align.value === 'end') {
    // 右缘对齐：理想位置是锚点右缘往左长
    const ideal = rect.right - width
    shift.value = Math.max(minLeft, Math.min(ideal, maxRight - width))
    return
  }
  const ideal = rect.left
  shift.value = Math.max(minLeft - ideal, Math.min(0, maxRight - (ideal + width)))
}

// 打开时量一次视口：面板刚挂上还没有宽度，必须等下一帧。
watch(
  () => props.open,
  (on) => {
    if (!on) {
      shift.value = 0
      return
    }
    void nextTick(clamp)
  },
)

/** 点在面板与锚点之外就算「点了外面」。 */
function onPointerDown(event: PointerEvent): void {
  if (!props.open) return
  const target = event.target as Node | null
  if (!target) return
  if (root.value?.contains(target)) return
  close()
}

function onKeydown(event: KeyboardEvent): void {
  if (!props.open) return
  if (event.key === 'Escape') {
    event.stopPropagation()
    close()
  }
}

onMounted(() => {
  document.addEventListener('pointerdown', onPointerDown)
  document.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onPointerDown)
  document.removeEventListener('keydown', onKeydown)
})

/** 打开即取焦点：键盘用户按 Enter 选第一项，不必先 Tab 一圈。 */
function focusPanel(): void {
  void nextTick(() => {
    const first = panel.value?.querySelector<HTMLElement>('button, [href], input, select, textarea')
    first?.focus()
  })
}

watch(
  () => props.open,
  (on) => {
    if (on) focusPanel()
  },
)

// 面板的内联样式：位移走 CSS 变量而不是直接写 transform——离场时 shift 已归零，
// 直接写会让面板淡出时跳回未夹取的位置。
const panelStyle = computed(() => ({
  '--wb-pop-shift': `${shift.value}px`,
  maxHeight: maxHeight.value || undefined,
}))

defineExpose({ close })
</script>

<template>
  <div ref="root" class="wb-pop">
    <slot />
    <Transition name="wb-pop">
      <div
        v-if="open && hasAnchor"
        ref="panel"
        class="wb-pop-panel"
        :class="`is-${align}`"
        :style="panelStyle"
        role="dialog"
        aria-modal="false"
      >
        <div class="wb-pop-body">
          <slot name="panel" :close="close" />
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.wb-pop {
  position: relative;
  display: inline-flex;
  min-width: 0;
}

/* 面板贴着锚点往上弹：这些面板都属于「接着输入」的动作，
   弹到屏幕外会让人找不到。水平位移由 clamp 写在 style 上。 */
.wb-pop-panel {
  position: absolute;
  bottom: calc(100% + v-bind(gap + 'px'));
  left: 0;
  z-index: var(--wb-z-pop);
  min-width: 180px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: var(--wb-sp-1);
  border: 1px solid var(--wb-line-strong);
  border-radius: var(--wb-r-lg);
  background: var(--wb-raised);
  box-shadow: var(--wb-shadow-lift);
  transform: translateX(var(--wb-pop-shift, 0));
}

.wb-pop-panel.is-end {
  left: auto;
  right: 0;
}

/* 内容自己滚：面板只负责不超出屏幕，长列表不把窗口顶翻 */
.wb-pop-body {
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
}

/* 入场：淡入 + 4px 位移。不做缩放——面板里的行自己会缩。 */
.wb-pop-enter-active,
.wb-pop-leave-active {
  transition:
    opacity var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.wb-pop-enter-from,
.wb-pop-leave-to {
  opacity: 0;
  transform: translateX(var(--wb-pop-shift, 0)) translateY(4px);
}
</style>
