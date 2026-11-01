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

const THINKING = ['off', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'] as const

const { t } = useI18n()
const router = useRouter()
const session = useSessionStore()
const settings = useSettingsStore()
const toast = useToastStore()

type Panel = 'model' | 'thinking' | null
const panel = ref<Panel>(null)
const vendor = ref('')
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

const activeVendor = computed(
  () => providers.value.find((p) => p.provider_id === vendor.value) || providers.value[0],
)
const activeModels = computed(() => activeVendor.value?.provider_models ?? [])

function open(next: Exclude<Panel, null>) {
  panel.value = panel.value === next ? null : next
  if (panel.value === 'model') {
    vendor.value = currentProvider.value || providers.value[0]?.provider_id || ''
    if (!providers.value.length) void settings.loadProviders()
  }
}

/** 供 /model 命令直接打开面板 */
function show(which: 'model' | 'thinking') {
  panel.value = which
  if (which === 'model') {
    vendor.value = currentProvider.value || providers.value[0]?.provider_id || ''
    void settings.loadProviders()
  }
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
      <div v-if="panel === 'model'" class="panel mp">
        <div class="mp-head">
          <span class="wb-label">{{ t('chat.modelPickerTitle') }}</span>
          <button class="mp-link" type="button" @click="goProviders">{{ t('chat.goProviders') }}</button>
        </div>
        <div v-if="!providers.length" class="mp-empty">{{ t('chat.noProviders') }}</div>
        <div v-else class="mp-body">
          <div class="mp-vendors">
            <button
              v-for="p in providers"
              :key="p.provider_id"
              class="mp-vendor"
              type="button"
              :class="{ 'is-on': p.provider_id === activeVendor?.provider_id }"
              @click="vendor = p.provider_id"
            >
              <span class="mp-avatar">{{ (p.provider_name || p.provider_id).slice(0, 1).toUpperCase() }}</span>
              <span class="mp-vname">{{ p.provider_name || p.provider_id }}</span>
              <span class="mp-count">{{ p.provider_models.length }}</span>
            </button>
          </div>
          <div class="mp-models">
            <button
              v-for="m in activeModels"
              :key="m.model_id"
              class="mp-model"
              type="button"
              :class="{
                'is-on': m.model_id === currentModel && activeVendor?.provider_id === currentProvider,
                'is-busy': applying === `${activeVendor?.provider_id}/${m.model_id}`,
              }"
              :disabled="!!applying"
              @click="chooseModel(activeVendor!.provider_id, m.model_id)"
            >
              <span class="mp-mname">{{ m.model_name || m.model_id }}</span>
              <span class="mp-meta">
                <span v-if="m.model_context_window">{{ Math.round(m.model_context_window / 1000) }}k</span>
                <span v-if="m.model_vision">img</span>
                <span v-if="m.model_reasoning">think</span>
              </span>
              <AppIcon
                v-if="m.model_id === currentModel && activeVendor?.provider_id === currentProvider"
                name="check"
                size="ic-2xs"
                class="mp-check"
              />
            </button>
            <div v-if="!activeModels.length" class="mp-empty">{{ t('common.empty') }}</div>
          </div>
        </div>
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
.mp {
  width: calc(468px * var(--wb-vw-scale));
}
.mp-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--wb-sp-1) var(--wb-sp-2) var(--wb-sp-2);
}
.mp-link {
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-primary);
}
.mp-link:hover {
  text-decoration: underline;
}
.mp-body {
  display: grid;
  grid-template-columns: calc(168px * var(--wb-vw-scale)) 1fr;
  max-height: calc(320px * var(--wb-vw-scale));
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius);
  overflow: hidden;
}
.mp-vendors {
  background: var(--wb-elev-hover);
  border-right: 1px solid var(--wb-border);
  overflow-y: auto;
  padding: var(--wb-sp-1);
}
.mp-vendor {
  width: 100%;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-sm);
  text-align: left;
  transition: background var(--wb-dur-fast) var(--wb-ease);
}
.mp-vendor:hover {
  background: var(--wb-tint);
}
.mp-vendor.is-on {
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
  font-weight: 700;
}
.mp-avatar {
  width: calc(20px * var(--wb-vw-scale));
  height: calc(20px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  display: grid;
  place-items: center;
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  flex: none;
}
.mp-vname {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mp-count {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  flex: none;
}
.mp-models {
  overflow-y: auto;
  padding: var(--wb-sp-1);
}
.mp-model {
  width: 100%;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  color: var(--wb-ink);
  font-size: var(--wb-fs-sm);
  text-align: left;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.mp-model:hover:not(:disabled) {
  background: var(--wb-elev-hover);
}
.mp-model.is-on {
  color: var(--wb-primary);
  font-weight: 700;
  background: var(--wb-accent-tint);
}
.mp-model.is-busy {
  opacity: 1;
  cursor: progress;
}
.mp-model:disabled:not(.is-busy) {
  opacity: 0.55;
  cursor: progress;
}
.mp-mname {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mp-meta {
  display: flex;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  flex: none;
}
.mp-check {
  color: var(--wb-primary);
  flex: none;
}
.mp-empty {
  padding: var(--wb-sp-4) var(--wb-sp-3);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
}
.tp {
  width: calc(180px * var(--wb-vw-scale));
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
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-sm);
  transition: background var(--wb-dur-fast) var(--wb-ease);
}
.tp-item:hover:not(:disabled) {
  background: var(--wb-elev-hover);
}
.tp-item.is-on {
  color: var(--wb-primary);
  font-weight: 700;
  background: var(--wb-accent-tint);
}
.tp-item:disabled {
  opacity: 0.55;
  cursor: progress;
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
