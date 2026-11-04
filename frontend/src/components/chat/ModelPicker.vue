<script setup lang="ts">
// 模型芯片：输入框底栏上的「现在用哪个模型」，点开是厂商 + 模型两级面板。
// 在输入框里因为模型是这一条消息的参数，放页头像在改全局设置。
// 显示模型名（有展示名用展示名），窄屏截断。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import ModelPanel from '@/components/chat/ModelPanel.vue'
import { useChatStore } from '@/stores/chat'

const { t } = useI18n()
const chat = useChatStore()

const open = ref(false)
const vendor = ref('')
const root = ref<HTMLElement | null>(null)
/** 面板相对芯片的水平偏移（px）。打开时算一次，见 offsetX。 */
const offsetX = ref(0)

/** 面板宽度，与样式里的 width 同一个值（需保持同步）。 */
const PANEL_WIDTH = 440

/**
 * 把面板夹回视口内。
 *
 * 芯片在底栏中部，而面板有 440px 宽且默认从芯片左缘往右长。
 * 800px 窗口下它会直接溢出屏幕右边——模型列表最重要的一列就被切掉了。
 * 这里没有用到浮层库：只有“夹到视口内”这一条规则，算一次即可。
 */
function clampToViewport(): void {
  const el = root.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  const margin = 8
  const maxRight = window.innerWidth - margin
  offsetX.value = Math.min(0, maxRight - (rect.left + PANEL_WIDTH))
}

/** 芯片上显示的名字：优先当前模型的展示名，其次模型 id。 */
const label = computed(() => {
  const current = chat.model
  if (!current) return t('chat.pickModel')
  const row = chat.models.find((m) => m.provider_name === current.provider_name && m.model_id === current.model_id)
  return row?.model_name || current.model_name || current.model_id
})

watch(open, (on) => {
  if (!on) return
  // 面板打开就落在当前模型所属的厂商上：用户要找的多半是现在这个或它的邻居。
  vendor.value = chat.model?.provider_name ?? ''
  // 清单是空的：先补拉一次。引擎冷启动可能要几秒，
  // 面板上给骨架而不是一个空列表。
  if (!chat.models.length && !chat.modelsLoading) void chat.loadModels(true)
  void nextTick(clampToViewport)
})

/**
 * 换完模型就把面板收起来。
 *
 * 不收的话用户得自己点外面——而刚点完一条模型，下一个动作几乎总是回去说话。
 */
watch(
  () => `${chat.model?.provider_name ?? ''}/${chat.model?.model_id ?? ''}`,
  (now, before) => {
    if (!open.value || !before || now === before) return
    open.value = false
  },
)

// `/model` 这类内置命令要求打开面板：面板自己 watch 这个意图位。
watch(
  () => chat.pickerRequest,
  (want) => {
    if (!want) return
    chat.pickerRequest = ''
    open.value = true
  },
)

function onDocPointerDown(event: PointerEvent): void {
  if (root.value?.contains(event.target as Node)) return
  open.value = false
}

function onEsc(event: KeyboardEvent): void {
  if (event.key === 'Escape') open.value = false
}

onMounted(() => {
  document.addEventListener('pointerdown', onDocPointerDown)
  document.addEventListener('keydown', onEsc)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocPointerDown)
  document.removeEventListener('keydown', onEsc)
})
</script>

<template>
  <div ref="root" class="mp">
    <button
      class="mp-chip"
      :class="{ 'is-on': open }"
      type="button"
      :title="t('chat.pickModel')"
      @click="open = !open"
    >
      <AppIcon name="cpu" :size="13" />
      <span class="mp-t mono">{{ label }}</span>
      <AppIcon name="chevron" :size="12" class="caret" :class="{ up: open }" />
    </button>

    <Transition name="pop">
      <div v-if="open" class="mp-panel" :style="{ transform: `translateX(${offsetX}px)` }">
        <ModelPanel :vendor="vendor" @pick-vendor="vendor = $event" />
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.mp {
  position: relative;
  min-width: 0;
}

.mp-chip {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  height: var(--wb-ctl-sm);
  max-width: 220px;
  padding: 0 var(--wb-sp-2);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-sm);
  background: transparent;
  color: var(--wb-mute);
  font-family: inherit;
  font-size: var(--wb-fs-3xs);
  font-weight: 600;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.mp-chip:hover:not(:disabled) {
  background: var(--wb-hover);
  border-color: var(--wb-line-strong);
  color: var(--wb-ink-2);
  transform: scale(var(--wb-press-hover));
}

.mp-chip:active:not(:disabled) {
  transform: scale(var(--wb-press-active));
  transition-duration: 60ms;
}

.mp-chip.is-on {
  background: var(--wb-brand-soft);
  border-color: var(--wb-brand-3);
  color: var(--wb-brand);
}

.mp-t {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.caret {
  flex: none;
  opacity: 0.55;
  transform: rotate(90deg);
  transition: transform var(--wb-dur-fast) var(--wb-ease);
}

.caret.up {
  transform: rotate(-90deg);
}

/* 面板贴着芯片往上弹：换模型是「接着输入」的动作，
   弹到屏幕外会让人找不到。
   水平位移由 clampToViewport 写在 style 上，这里只管尺寸与外观。 */
.mp-panel {
  position: absolute;
  bottom: calc(100% + var(--wb-sp-2));
  left: 0;
  z-index: var(--wb-z-pop);
  width: 440px;
  max-width: calc(100vw - 16px);
  padding: var(--wb-sp-2);
  border: 1px solid var(--wb-line-strong);
  border-radius: var(--wb-r-lg);
  background: var(--wb-raised);
  box-shadow: var(--wb-shadow-lift);
}

.pop-enter-active,
.pop-leave-active {
  transition:
    opacity var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.pop-enter-from,
.pop-leave-to {
  opacity: 0;
  transform: translateY(4px);
}
</style>
