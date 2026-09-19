<script setup lang="ts">
/**
 * 图片编辑器（Canvas，零依赖）：裁剪 / 旋转 / 翻转 / 缩放 / 自动抠图。
 * 输入输出均为 data URL（输出 PNG 保留 alpha）；自动抠图以四角像素均值为背景色按容差归零 alpha。
 */
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { t } from '@/i18n'

const props = withDefaults(defineProps<{
  modelValue: boolean
  /** 待编辑的源图片（data URL）；置非空时载入画布。 */
  source: string
  /** 输出宽高比（宽/高）；1 = 正方形。 */
  aspect?: number
  /** 输出宽度（像素）。 */
  outputWidth?: number
}>(), {
  aspect: 1,
  outputWidth: 512
})

const emit = defineEmits<{
  'update:modelValue': [boolean]
  confirm: [dataURL: string]
}>()

const canvasRef = ref<HTMLCanvasElement | null>(null)
const img = ref<HTMLImageElement | null>(null)
const rotation = ref(0)
const scale = ref(1)
const flipH = ref(false)
const flipV = ref(false)
const offX = ref(0)
const offY = ref(0)
const dragging = ref(false)
/** 抠图容差（0=关，越大抠得越狠）。 */
const tolerance = ref(0)
const cutting = ref(false)
let lastX = 0
let lastY = 0

const outH = computed(() => Math.round(props.outputWidth / (props.aspect || 1)))

function reset(): void {
  rotation.value = 0
  scale.value = 1
  flipH.value = false
  flipV.value = false
  offX.value = 0
  offY.value = 0
  tolerance.value = 0
}

function load(src: string): void {
  if (!src) return
  const el = new Image()
  el.onload = () => {
    img.value = el
    reset()
    draw()
  }
  el.src = src
}

/** 短边铺满画布的基础缩放；旋转 90/270 度时宽高互换。 */
function fitScale(): number {
  const el = img.value
  if (!el) return 1
  const swap = rotation.value % 180 !== 0
  const iw = swap ? el.naturalHeight : el.naturalWidth
  const ih = swap ? el.naturalWidth : el.naturalHeight
  if (iw <= 0 || ih <= 0) return 1
  return Math.max(props.outputWidth / iw, outH.value / ih)
}

/** 透明格背景：抠图后能直观看出哪些区域真的透明了。 */
function paintBackdrop(ctx: CanvasRenderingContext2D, w: number, h: number): void {
  const cell = 8
  for (let y = 0; y < h; y += cell) {
    for (let x = 0; x < w; x += cell) {
      ctx.fillStyle = ((x / cell + y / cell) | 0) % 2 === 0 ? '#2a2a31' : '#202027'
      ctx.fillRect(x, y, cell, cell)
    }
  }
}

/** 把当前变换画到任意 2D 上下文；backdrop 仅预览用（透明格不进成品）。 */
function render(ctx: CanvasRenderingContext2D | null, w: number, h: number, backdrop: boolean): void {
  const el = img.value
  if (!ctx || !el) return
  ctx.clearRect(0, 0, w, h)
  if (backdrop) paintBackdrop(ctx, w, h)
  ctx.save()
  ctx.translate(w / 2 + offX.value, h / 2 + offY.value)
  ctx.rotate((rotation.value * Math.PI) / 180)
  ctx.scale(flipH.value ? -1 : 1, flipV.value ? -1 : 1)
  const s = fitScale() * scale.value
  ctx.scale(s, s)
  ctx.drawImage(el, -el.naturalWidth / 2, -el.naturalHeight / 2)
  ctx.restore()
}

function draw(): void {
  const c = canvasRef.value
  if (!c) return
  c.width = props.outputWidth
  c.height = outH.value
  render(c.getContext('2d'), props.outputWidth, outH.value, true)
}

function rotateStep(): void {
  rotation.value = (rotation.value + 90) % 360
  draw()
}

/**
 * 自动抠背景：四角均值当背景色，容差内 alpha 归零，容差带内线性过渡。
 * 结果写回 img，后续旋转 / 缩放都基于抠好的图。
 */
function applyCutout(): void {
  const el = img.value
  if (!el) return
  cutting.value = true
  const w = el.naturalWidth
  const h = el.naturalHeight
  const off = document.createElement('canvas')
  off.width = w
  off.height = h
  const ctx = off.getContext('2d')
  if (!ctx) {
    cutting.value = false
    return
  }
  ctx.drawImage(el, 0, 0)
  const data = ctx.getImageData(0, 0, w, h)
  const px = data.data
  const corners = [0, (w - 1) * 4, (h - 1) * w * 4, (h * w - 1) * 4]
  let br = 0
  let bg = 0
  let bb = 0
  for (const i of corners) {
    br += px[i]
    bg += px[i + 1]
    bb += px[i + 2]
  }
  br /= corners.length
  bg /= corners.length
  bb /= corners.length

  const tol = tolerance.value
  for (let i = 0; i < px.length; i += 4) {
    if (px[i + 3] === 0) continue
    const d = (Math.abs(px[i] - br) + Math.abs(px[i + 1] - bg) + Math.abs(px[i + 2] - bb)) / 3
    if (d <= tol) {
      px[i + 3] = 0
    } else if (d < tol * 2) {
      px[i + 3] = Math.round((px[i + 3] * (d - tol)) / tol)
    }
  }
  ctx.putImageData(data, 0, 0)
  const next = new Image()
  next.onload = () => {
    img.value = next
    draw()
    cutting.value = false
  }
  next.src = off.toDataURL('image/png')
}

function onPointerDown(e: PointerEvent): void {
  dragging.value = true
  lastX = e.clientX
  lastY = e.clientY
}

function onPointerMove(e: PointerEvent): void {
  if (!dragging.value) return
  const c = canvasRef.value
  if (!c || c.clientWidth <= 0) return
  // 画布 CSS 宽度与输出宽度不同，位移需按比例换算
  const ratio = props.outputWidth / c.clientWidth
  offX.value += (e.clientX - lastX) * ratio
  offY.value += (e.clientY - lastY) * ratio
  lastX = e.clientX
  lastY = e.clientY
  draw()
}

function onPointerUp(): void {
  dragging.value = false
}

function confirm(): void {
  if (!img.value) return
  // 离屏重绘一次不带透明格的版本：格子只是预览辅助，不能进成品
  const out = document.createElement('canvas')
  out.width = props.outputWidth
  out.height = outH.value
  render(out.getContext('2d'), props.outputWidth, outH.value, false)
  emit('confirm', out.toDataURL('image/png'))
}

function close(): void {
  emit('update:modelValue', false)
}

watch(() => props.source, (s) => {
  if (s && props.modelValue) load(s)
})

/**
 * 打开 dialog 时强制重新载入：
 * <p>el-dialog 默认 lazy render（关闭时不渲染 canvas DOM），`source` 在 dialog 关闭状态下被
 * 设置时 canvas 节点尚未挂载，`load` 内部的 `draw()` 会因 `canvasRef.value === null` 早退，
 * 表现就是"选了图却一片空白"。watch `modelValue` + nextTick 等 canvas 渲染后再触发一次。
 */
watch(() => props.modelValue, (open) => {
  if (open && props.source) {
    nextTick(() => load(props.source))
  }
})

onBeforeUnmount(() => {
  img.value = null
})
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    :title="t('cropper.title')"
    width="480px"
    append-to-body
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
  >
    <div class="flex flex-col items-center gap-3">
      <div
        class="overflow-hidden rounded-xl border border-wb-border"
        :style="{ width: '320px' }"
      >
        <canvas
          ref="canvasRef"
          class="block w-full cursor-grab"
          :style="{ aspectRatio: String(aspect || 1) }"
          @pointerdown="onPointerDown"
          @pointermove="onPointerMove"
          @pointerup="onPointerUp"
          @pointerleave="onPointerUp"
        />
      </div>
      <p class="text-xs text-wb-muted">{{ t('cropper.hint') }}</p>

      <div class="flex w-full items-center gap-3">
        <span class="shrink-0 text-xs text-wb-muted">{{ t('cropper.angle') }}</span>
        <el-slider v-model="rotation" :min="0" :max="360" :step="1" class="flex-1" @input="draw" />
        <span class="w-10 shrink-0 text-right text-xs tabular-nums">{{ rotation }}°</span>
      </div>

      <div class="flex w-full items-center gap-3">
        <span class="shrink-0 text-xs text-wb-muted">{{ t('cropper.zoom') }}</span>
        <el-slider v-model="scale" :min="0.2" :max="4" :step="0.05" class="flex-1" @input="draw" />
        <span class="w-10 shrink-0 text-right text-xs tabular-nums">{{ scale.toFixed(2) }}×</span>
      </div>

      <div class="flex w-full items-center gap-3">
        <span class="shrink-0 text-xs text-wb-muted">{{ t('cropper.tolerance') }}</span>
        <el-slider v-model="tolerance" :min="0" :max="120" :step="1" class="flex-1" />
        <el-button size="small" :loading="cutting" :disabled="!img || tolerance <= 0" @click="applyCutout">
          {{ t('cropper.cutout') }}
        </el-button>
      </div>

      <div class="flex w-full items-center gap-2">
        <el-button size="small" @click="rotateStep">{{ t('cropper.rotate') }} 90°</el-button>
        <el-button size="small" :class="flipH ? 'is-active' : ''" @click="flipH = !flipH; draw()">
          {{ t('cropper.flipH') }}
        </el-button>
        <el-button size="small" :class="flipV ? 'is-active' : ''" @click="flipV = !flipV; draw()">
          {{ t('cropper.flipV') }}
        </el-button>
        <el-button size="small" @click="reset(); draw()">{{ t('cropper.reset') }}</el-button>
        <span class="flex-1" />
        <el-button size="small" @click="close">{{ t('ui.btn.cancel') }}</el-button>
        <el-button size="small" type="primary" :disabled="!img" @click="confirm">
          {{ t('ui.btn.save') }}
        </el-button>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped>
.is-active {
  color: var(--wb-primary-strong);
  border-color: color-mix(in srgb, var(--wb-primary) 45%, transparent);
  background: color-mix(in srgb, var(--wb-primary) 10%, transparent);
}
</style>
