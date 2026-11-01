<script setup lang="ts">
// 输入区的两个会话开关：模型 chip 与思考档 chip。
// 面板自绘（绝对定位 + 外部点击关闭）：它是带键盘导航与两列联动的菜单，不是表单基元。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSessionStore } from '../../stores/session'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import AppIcon from '../common/AppIcon.vue'
import ComposerMenu from './ComposerMenu.vue'

const THINKING = ['off', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'] as const

const { t } = useI18n()
const router = useRouter()
const session = useSessionStore()
const settings = useSettingsStore()
const toast = useToastStore()

type Panel = 'model' | 'thinking' | null
const panel = ref<Panel>(null)
const applying = ref('')
const root = ref<HTMLElement | null>(null)

const providers = computed(() => settings.providers ?? [])
const currentProvider = computed(
  () => session.current?.session_provider || settings.boot?.boot_default_provider || '',
)
const currentModel = computed(
  () => session.current?.session_model || settings.boot?.boot_default_model || '',
)

const modelLabel = computed(() => {
  const p = providers.value.find((x) => x.provider_id === currentProvider.value)
  return (
    p?.provider_models.find((m) => m.model_id === currentModel.value)?.model_name ||
    currentModel.value ||
    t('chat.modelPickerTitle')
  )
})
const thinkingKey = computed(() => session.current?.session_thinking_level || 'medium')
const thinkingName = computed(() => t(`thinking.${thinkingKey.value}`))

function open(next: Exclude<Panel, null>) {
  panel.value = panel.value === next ? null : next
  if (panel.value === 'model' && !providers.value.length) void settings.loadProviders()
}

/** 供 /model 命令直接打开面板 */
function show(which: 'model' | 'thinking') {
  panel.value = which
  if (which === 'model') void settings.loadProviders()
}
function close() {
  panel.value = null
}

async function chooseModel(providerId: string, modelId: string) {
  applying.value = `${providerId}/${modelId}`
  try {
    // 新对话还没会话文件：改的是全局默认模型，下一条会话直接用它
    if (!session.currentId) {
      await api.providers.setDefault(providerId, modelId)
      await settings.refreshProviderData()
      toast.ok(t('chat.modelDefaultSet', { model: modelLabelOf(providerId, modelId) }))
      close()
      return
    }
    await session.setModel(providerId, modelId)
    await settings.refreshBoot()
    close()
  } catch (e) {
    toast.bad(t('chat.modelSwitchFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    applying.value = ''
  }
}

/** 面板里显示的名字：有显示名就用显示名，没有就退回模型 id */
function modelLabelOf(providerId: string, modelId: string) {
  const p = providers.value.find((x) => x.provider_id === providerId)
  return p?.provider_models.find((m) => m.model_id === modelId)?.model_name || modelId
}

async function chooseThinking(level: string) {
  // 思考档只按会话存：没有会话就没有可写的默认档
  if (!session.currentId) {
    toast.info(t('chat.slashNeedSession'))
    return
  }
  applying.value = `t:${level}`
  try {
    await session.setThinking(level)
    close()
  } catch (e) {
    toast.bad(t('chat.thinkingSwitchFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    applying.value = ''
  }
}

function goProviders() {
  close()
  void router.push('/models')
}

function onDocPointerDown(e: PointerEvent) {
  if (root.value?.contains(e.target as Node)) return
  close()
}
function onEsc(e: KeyboardEvent) {
  if (e.key === 'Escape') close()
}
onMounted(() => {
  document.addEventListener('pointerdown', onDocPointerDown)
  document.addEventListener('keydown', onEsc)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocPointerDown)
  document.removeEventListener('keydown', onEsc)
})

defineExpose({ show, close })
</script>

<template>
  <div ref="root" class="chips">
    <button class="chip" type="button" :class="{ 'is-on': panel === 'model' }" @click="open('model')">
      <AppIcon name="sparkles" size="ic-2xs" />
      <span class="chip-t">{{ modelLabel }}</span>
      <AppIcon :name="panel === 'model' ? 'chevron-down' : 'chevron-up'" size="ic-2xs" class="chip-caret" />
    </button>
    <button class="chip" type="button" :class="{ 'is-on': panel === 'thinking' }" @click="open('thinking')">
      <AppIcon name="brain" size="ic-2xs" />
      <span class="chip-t">{{ thinkingName }}</span>
    </button>

    <Transition name="pop">
      <div v-if="panel === 'model'" class="panel">
        <ComposerMenu
          :providers="providers"
          :current-provider="currentProvider"
          :current-model="currentModel"
          :applying="applying"
          @pick="chooseModel"
          @manage="goProviders"
        />
      </div>
    </Transition>

    <Transition name="pop">
      <div v-if="panel === 'thinking'" class="panel tp">
        <div class="wb-label tp-head">{{ t('chat.thinkingPickerTitle') }}</div>
        <button
          v-for="k in THINKING"
          :key="k"
          class="tp-item"
          type="button"
          :class="{ 'is-on': k === thinkingKey }"
          :disabled="!!applying"
          @click="chooseThinking(k)"
        >
          <span>{{ t(`thinking.${k}`) }}</span>
          <AppIcon v-if="k === thinkingKey" name="check" size="ic-2xs" />
        </button>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.chips {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  min-width: 0;
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  height: var(--wb-ctl-h-sm);
  padding: 0 var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  background: transparent;
  color: var(--wb-muted);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  max-width: calc(200px * var(--wb-vw-scale));
  transition:
    background var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.chip:hover {
  background: var(--wb-elev-hover);
  border-color: var(--wb-border-strong);
  color: var(--wb-ink-2);
  transform: scale(1.06);
}
.chip:active {
  transform: scale(0.92);
}
.chip.is-on {
  background: var(--wb-accent-tint);
  border-color: var(--wb-primary-line);
  color: var(--wb-primary);
}
.chip-t {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.chip-caret {
  opacity: 0.6;
}
.panel {
  position: absolute;
  bottom: calc(100% + var(--wb-sp-2));
  left: 0;
  z-index: var(--wb-z-pop);
  background: var(--wb-elev-raised);
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-lg);
  box-shadow: var(--wb-shadow-3);
  padding: var(--wb-sp-2);
}
/* 思考档面板：窄列表，一项一行 */
.tp {
  min-width: calc(140px * var(--wb-vw-scale));
}
.tp-head {
  padding: var(--wb-sp-1) var(--wb-sp-2) var(--wb-sp-2);
}
.tp-item {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-sm);
  text-align: left;
  transition: background var(--wb-dur-fast) var(--wb-ease);
}
.tp-item:hover:not(:disabled) {
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
}
.tp-item.is-on {
  color: var(--wb-primary);
  font-weight: 700;
}
.tp-item:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
</style>
