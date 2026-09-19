<script setup lang="ts">
/**
 * 桌宠管理视图，三栏：左配置表单、中实时预览、右 sprite 资产列表；
 * 自定义形象在页内 ImageCropper 完成裁剪 / 旋转 / 抠图。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { PawPrint, Upload } from '@/components/common/icons'
import { usePetStore } from '@/stores/pet'
import { t } from '@/i18n'
import { apiPost } from '@/api/client'
import { useToast } from '@/composables/useToast'
import ImageCropper from '@/components/common/ImageCropper.vue'

const pet = usePetStore()
const { form, error, spriteName, loading, sprites } = storeToRefs(pet)
const { load, save, addSprite, removeSprite, pickImageForEdit, saveEditedSprite } = pet
const toast = useToast()

/** 召唤/收起桌面宠物窗口（Wails 单窗口形态切换：缩小 + 置顶 → 前端切到 /pet/desktop 页）。 */
async function togglePetWindow(show: boolean): Promise<void> {
  try {
    await apiPost(`/api/v1/pet/window/${show ? 'pet' : 'main'}`)
    toast.success(t(show ? 'pet.windowShown' : 'pet.windowHidden'))
  } catch (e) {
    toast.error(t('pet.windowFail'), e instanceof Error ? e.message : String(e))
  }
}

/** 选中本地图 → 打开图片编辑器（裁剪 / 旋转 / 抠图）→ 保存为新形象。 */
const uploading = ref(false)
const cropperOpen = ref(false)
async function pickAndEditSprite(): Promise<void> {
  uploading.value = true
  try {
    if (await pickImageForEdit()) cropperOpen.value = true
  } finally {
    uploading.value = false
  }
}

async function onCropConfirm(dataURL: string): Promise<void> {
  cropperOpen.value = false
  const created = await saveEditedSprite(dataURL)
  if (created) await save()
}

/** 实时预览：跟随下拉所选 sprite（file_path = /files/sprites/{id}，拼时间戳防浏览器缓存）。 */
const previewSprite = computed(() => sprites.value.find((s) => s.id === form.value.sprite_id) ?? null)
const previewUrl = computed(() => {
  const s = previewSprite.value
  if (!s?.file_path) return ''
  return `${s.file_path}?v=${s.created_at ?? 0}`
})
const previewFailed = ref(false)
watch(previewUrl, () => { previewFailed.value = false })
function onPreviewFail(): void { previewFailed.value = true }

const showBubble = ref(true)
const bubbleIdx = ref(0)
const bubbleKeys = ['pet.bubble.0', 'pet.bubble.1', 'pet.bubble.2', 'pet.bubble.3']
const bubbleText = computed(() => t(bubbleKeys[bubbleIdx.value]))

function nextBubble(): void {
  bubbleIdx.value = (bubbleIdx.value + 1) % bubbleKeys.length
  if (form.value.bubble_enabled) {
    showBubble.value = true
    window.setTimeout(() => { showBubble.value = false }, 4000)
  }
}

// ===== 桌宠 mood 真姿态 =====
type MoodKey = 'idle' | 'thinking' | 'happy' | 'sleepy' | 'excited'
const MOODS: { key: MoodKey; emoji: string; labelKey: string }[] = [
  { key: 'idle', emoji: '😺', labelKey: 'pet.mood.idle' },
  { key: 'thinking', emoji: '🤔', labelKey: 'pet.mood.thinking' },
  { key: 'happy', emoji: '😄', labelKey: 'pet.mood.happy' },
  { key: 'sleepy', emoji: '😴', labelKey: 'pet.mood.sleepy' },
  { key: 'excited', emoji: '🤩', labelKey: 'pet.mood.excited' }
]
const currentMood = ref<MoodKey>('idle')
const currentMoodEmoji = computed(() => MOODS.find((m) => m.key === currentMood.value)?.emoji ?? '😺')
function setMood(m: MoodKey): void {
  currentMood.value = m
}

onMounted(load)
</script>

<template>
  <div class="scroll wb-ui">
    <div class="wrap wrap-lg">
      <!-- Hero header（原型 17 屏） -->
      <header class="hero">
        <div class="tile"><PawPrint class="ic" /></div>
        <div>
          <h1>{{ t('pet.title') }}</h1>
          <p>{{ t('pet.subtitle') }}</p>
        </div>
        <span class="sp" />
        <span class="badge b-neutral">{{ t('pet.spritesCount', sprites.length) }}</span>
      </header>

      <div v-if="error" class="alert a-danger">
        {{ error }}
      </div>

      <!-- 响应式断点（移动/小屏 1 列，平板 2 列，桌面 3 列；1024px 以下左侧配置+中预览合并成上下排） -->
      <div class="grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-3">
        <!-- 左：配置表单 -->
        <section class="rounded-2xl border border-wb-border bg-wb-surface/80 p-5 shadow-[var(--wb-shadow)]">
          <h2 class="mb-3 text-sm font-semibold text-wb-primary-strong">{{ t('pet.config') }}</h2>
          <form class="space-y-3" @submit.prevent="save">
            <label class="flex items-center gap-2 text-sm text-wb-ink">
              <input v-model="form.enabled" type="checkbox" class="h-4 w-4 rounded border-wb-border text-wb-primary accent-wb-primary focus:ring-wb-primary/30" />
              {{ t('pet.enabled') }}
            </label>
            <!-- 桌宠为独立桌面窗口，这里只控制召唤 / 收起 -->
            <div class="grid grid-cols-2 gap-2">
              <el-button @click="togglePetWindow(true)">{{ t('pet.windowShow') }}</el-button>
              <el-button @click="togglePetWindow(false)">{{ t('pet.windowHide') }}</el-button>
            </div>

            <el-select v-model="form.sprite_id" class="w-full" clearable :placeholder="t('pet.noSprite')">
              <el-option v-for="s in sprites" :key="s.id" :label="s.name" :value="s.id" />
            </el-select>

            <div class="grid grid-cols-2 gap-3">
              <el-input-number
                v-model="form.position_x"
                size="small"
                class="w-full"
                :placeholder="t('pet.position_x')"
              />
              <el-input-number
                v-model="form.position_y"
                size="small"
                class="w-full"
                :placeholder="t('pet.position_y')"
              />
            </div>

            <div class="flex items-center gap-3 text-sm text-wb-muted">
              <span class="shrink-0">{{ t('pet.scale') }}</span>
              <el-slider v-model="form.scale" :min="0.5" :max="3" :step="0.1" class="flex-1" />
              <span class="w-10 shrink-0 text-right tabular-nums text-wb-ink">{{ form.scale }}×</span>
            </div>

            <div class="flex items-center gap-3">
              <el-switch v-model="form.bubble_enabled" :active-text="t('pet.bubble')" />
              <el-input-number
                v-model="form.bubble_duration_ms"
                size="small"
                class="flex-1"
                :placeholder="t('pet.bubbleDuration')"
              />
            </div>

            <!-- 点击穿透：透明区域不拦截鼠标（Windows 原生窗口区域裁剪） -->
            <div class="flex items-center gap-3">
              <el-switch v-model="form.click_through" :active-text="t('pet.clickThrough')" />
            </div>

            <el-button type="primary" native-type="submit" class="w-full">
              {{ t('pet.save') }}
            </el-button>
          </form>
        </section>

        <!-- 中：实时预览 -->
        <section class="rounded-xl border border-wb-border bg-wb-surface p-5">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="text-sm font-semibold text-wb-ink">{{ t('pet.preview') }}</h2>
            <el-button size="small" text type="primary" @click="nextBubble">
              {{ t('pet.nextBubble') }}
            </el-button>
          </div>

          <!-- 预览舞台 -->
          <div class="relative flex h-64 items-center justify-center overflow-hidden rounded-xl border border-wb-border bg-wb-bg">
            <!-- 气泡 -->
            <transition name="bubble">
              <div
                v-if="form.bubble_enabled && showBubble"
                class="absolute top-6 left-1/2 max-w-[80%] -translate-x-1/2 rounded-2xl rounded-bl-sm border border-wb-border bg-wb-surface px-4 py-2 text-sm text-wb-ink shadow-[var(--wb-shadow-lg)]"
              >
                {{ bubbleText }}
                <div class="absolute -bottom-1.5 left-6 h-3 w-3 rotate-45 border-b border-r border-wb-border bg-wb-surface" />
              </div>
            </transition>

            <!-- 桌宠 sprite + 浮动 + 眨眼动画（mood 真姿态：头顶 mood emoji） -->
            <div class="relative flex flex-col items-center">
              <!-- 头顶 mood emoji（随 mood state 切换） -->
              <div class="pet-mood-emoji absolute -top-8 left-1/2 -translate-x-1/2 text-3xl">
                {{ currentMoodEmoji }}
              </div>
              <img
                v-if="previewUrl && !previewFailed"
                :src="previewUrl"
                :alt="t('pet.alt')"
                class="pet-sprite-anim max-h-48 w-auto"
                :style="{ '--pet-scale': `scale(${form.scale})` }"
                @error="onPreviewFail"
              />
              <!-- 未选 sprite / 内置形象无文件时的占位说明 -->
              <div v-else class="flex flex-col items-center gap-2 px-6 text-center">
                <span class="flex h-14 w-14 items-center justify-center rounded-full bg-wb-primary/10 text-wb-primary">
                  <PawPrint class="h-6 w-6" />
                </span>
                <span class="text-xs text-wb-muted">
                  {{ form.sprite_id ? t('pet.previewNoFile') : t('pet.previewEmptyHint') }}
                </span>
              </div>
              <!-- 阴影椭圆（与浮动同步呼吸） -->
              <div class="pet-shadow mt-1 h-2 w-20 rounded-full bg-black/60 blur-sm" />
            </div>
          </div>

          <div class="mt-3 text-center text-xs text-wb-muted">
            {{ t('pet.scale') }} <span class="text-wb-ink">{{ form.scale }}×</span> · {{ t('pet.position') }} ({{ form.position_x }}, {{ form.position_y }})
          </div>

          <!-- Mood 选择器（决定形象跟随思考/开心/低落/空闲切换） -->
          <div class="mt-3 rounded-xl border border-wb-border bg-wb-primary/[0.03] p-3">
            <p class="mb-2 text-xs font-medium text-wb-primary-strong">{{ t('pet.mood.title') }}</p>
            <div class="flex gap-2">
              <button
                v-for="m in MOODS"
                :key="m.key"
                type="button"
                class="flex flex-1 flex-col items-center gap-1 rounded-lg px-2 py-2 text-xs transition-colors"
                :class="currentMood === m.key ? 'bg-wb-primary/15 text-wb-primary-strong' : 'text-wb-muted hover:bg-wb-primary/5'"
                :title="t(m.labelKey)"
                @click="setMood(m.key)"
              >
                <span class="text-lg">{{ m.emoji }}</span>
                <span>{{ t(m.labelKey) }}</span>
              </button>
            </div>
            <p class="mt-2 text-[10px] text-wb-muted">{{ t('pet.mood.hint') }}</p>
          </div>
        </section>

        <!-- 右：sprite 资产 -->
        <section class="rounded-2xl border border-wb-border bg-wb-surface/80 p-5 shadow-[var(--wb-shadow)]">
          <h2 class="mb-3 text-sm font-semibold text-wb-primary-strong">{{ t('pet.spriteAssets') }}</h2>
          <div class="mb-3 flex gap-2">
            <el-input v-model="spriteName" class="flex-1" :placeholder="t('pet.newSpriteName')" />
            <el-button type="primary" @click="addSprite">{{ t('pet.add') }}</el-button>
          </div>
          <!-- 自定义形象：选图后先进编辑器（裁剪 / 旋转 / 抠图），透明 PNG 直接从背景里「站」出来 -->
          <div class="mb-3">
            <button
              type="button"
              class="flex w-full items-center justify-center gap-1.5 rounded-xl border border-dashed border-wb-primary/40 bg-wb-primary/[0.04] px-3 py-2.5 text-sm text-wb-primary-strong transition-colors hover:bg-wb-primary/10 disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="uploading"
              @click="pickAndEditSprite"
            >
              <Upload class="h-4 w-4" />
              <span>{{ uploading ? t('pet.uploading') : t('pet.uploadSprite') }}</span>
              <span class="ml-1 text-[10px] text-wb-muted">≤5MB · png/webp/gif/jpg</span>
            </button>
            <p class="mt-1 text-[10px] leading-snug text-wb-muted">{{ t('pet.uploadHint') }}</p>
          </div>
          <div v-if="loading" class="text-sm text-wb-muted">{{ t('pet.loading') }}</div>
          <ul v-else class="space-y-1">
            <li v-for="s in sprites" :key="s.id" class="flex items-center justify-between rounded-lg bg-wb-primary/[0.04] px-3 py-2 text-sm">
              <div>
                <span class="text-wb-ink">{{ s.name }}</span>
                <span v-if="s.format" class="ml-2 text-xs text-wb-muted">{{ s.format }}</span>
                <span v-if="s.is_builtin" class="ml-2 rounded bg-wb-sky/15 px-1.5 py-0.5 text-[10px] text-sky-700">{{ t('pet.builtin') }}</span>
              </div>
              <el-button link type="danger" size="small" @click="removeSprite(s.id)">
                {{ t('pet.delete') }}
              </el-button>
            </li>
          </ul>
        </section>
      </div>
    </div>

    <ImageCropper
      v-model="cropperOpen"
      :source="pet.editSource"
      :aspect="1"
      :output-width="512"
      @confirm="onCropConfirm"
    />
  </div>
</template>

<style scoped>
.bubble-enter-active,
.bubble-leave-active {
  transition: opacity 0.25s ease, transform 0.25s ease;
}
.bubble-enter-from,
.bubble-leave-to {
  opacity: 0;
  transform: translate(-50%, -8px);
}
.bubble-enter-to,
.bubble-leave-from {
  opacity: 1;
  transform: translate(-50%, 0);
}

@keyframes pet-float {
  0%, 100% { transform: translateY(0) var(--pet-scale, scale(1)); }
  50%      { transform: translateY(-6px) var(--pet-scale, scale(1)); }
}

@keyframes pet-blink {
  0%, 92%, 100% { filter: brightness(1); }
  94%, 96%      { filter: brightness(0.7) saturate(1.5); }
}

.pet-sprite-anim {
  animation: pet-float 4s ease-in-out infinite, pet-blink 5s ease-in-out infinite;
  transform-origin: center;
  will-change: transform;
}

.pet-shadow {
  animation: shadow-pulse 4s ease-in-out infinite;
}
@keyframes shadow-pulse {
  0%, 100% { opacity: 0.3; transform: translateX(-50%) scaleX(1); }
  50%      { opacity: 0.15; transform: translateX(-50%) scaleX(0.85); }
}

@media (prefers-reduced-motion: reduce) {
  .pet-sprite-anim,
  .pet-shadow {
    animation: none !important;
  }
  .bubble-enter-active,
  .bubble-leave-active {
    transition: none !important;
  }
}
</style>
