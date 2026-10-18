<script setup lang="ts">
// 输入区的两个会话开关：模型 chip 与思考档 chip。
// 模型清单来自 PI 的 models.json——界面选的与 pi 命令用的是同一份配置。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useSessionStore } from '../../stores/session'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import AppIcon from '../common/AppIcon.vue'

const THINKING = [
  { key: 'off', name: '不思考' },
  { key: 'minimal', name: '很少' },
  { key: 'low', name: '少' },
  { key: 'medium', name: '适中' },
  { key: 'high', name: '多' },
  { key: 'xhigh', name: '很多' },
  { key: 'max', name: '最多' },
] as const

const session = useSessionStore()
const settings = useSettingsStore()
const toast = useToastStore()

type Panel = 'model' | 'thinking' | null
const panel = ref<Panel>(null)
const vendor = ref('')
const applying = ref('')
const root = ref<HTMLElement | null>(null)

const providers = computed(() => settings.providers?.providers ?? [])
const currentProvider = computed(() => session.current?.provider || settings.boot?.default_provider || '')
const currentModel = computed(() => session.current?.model || settings.boot?.default_model || '')

const providerName = computed(
  () => providers.value.find((p) => p.id === currentProvider.value)?.name || currentProvider.value || '未选模型',
)
const modelLabel = computed(() => {
  const p = providers.value.find((x) => x.id === currentProvider.value)
  return p?.models.find((m) => m.id === currentModel.value)?.name || currentModel.value || '选模型'
})
const thinkingName = computed(
  () => THINKING.find((t) => t.key === session.current?.thinking_level)?.name || '默认',
)

const activeVendor = computed(() => providers.value.find((p) => p.id === vendor.value) || providers.value[0])
const activeModels = computed(() => activeVendor.value?.models ?? [])

function toggle(next: Panel) {
  panel.value = panel.value === next ? null : next
  if (panel.value === 'model') {
    vendor.value = currentProvider.value || providers.value[0]?.id || ''
    void settings.loadProviders()
  }
}

/** 供 /model 命令直接打开面板 */
function show(which: 'model' | 'thinking') {
  panel.value = which
  if (which === 'model') {
    vendor.value = currentProvider.value || providers.value[0]?.id || ''
    void settings.loadProviders()
  }
}

function close() {
  panel.value = null
}

async function chooseModel(providerId: string, modelId: string) {
  if (!session.currentId) {
    toast.info('先发一句话再换模型：模型属于某一条对话')
    close()
    return
  }
  applying.value = `${providerId}/${modelId}`
  try {
    await session.setModel(providerId, modelId)
    // 没有服务商条目时把默认模型也写一次：下一次新对话才跟着变。
    if (!session.current?.provider) await settings.refreshBoot()
    close()
  } catch (e) {
    toast.bad(`换模型失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    applying.value = ''
  }
}

async function chooseThinking(level: string) {
  if (!session.currentId) {
    toast.info('先发一句话再调思考档：它属于某一条对话')
    close()
    return
  }
  applying.value = `t:${level}`
  try {
    await session.setThinking(level)
    close()
  } catch (e) {
    toast.bad(`调整思考档失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    applying.value = ''
  }
}

function onDocClick(e: MouseEvent) {
  if (!root.value) return
  if (!root.value.contains(e.target as Node)) close()
}
function onEsc(e: KeyboardEvent) {
  if (e.key === 'Escape') close()
}
onMounted(() => {
  document.addEventListener('mousedown', onDocClick)
  document.addEventListener('keydown', onEsc)
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocClick)
  document.removeEventListener('keydown', onEsc)
})

defineExpose({ show, close })
</script>

<template>
  <div ref="root" class="chips">
    <button
      class="chip"
      type="button"
      :class="{ 'is-hi': panel === 'model' }"
      :title="`模型：${providerName} / ${modelLabel}`"
      @click="toggle('model')"
    >
      <AppIcon name="sparkles" size="ic-2xs" />
      <span class="chip-t">{{ modelLabel }}</span>
    </button>
    <button
      class="chip"
      type="button"
      :class="{ 'is-hi': panel === 'thinking' }"
      :title="`思考档：${thinkingName}`"
      @click="toggle('thinking')"
    >
      <AppIcon name="brain" size="ic-2xs" />
      <span class="chip-t">思考 {{ thinkingName }}</span>
    </button>

    <Transition name="pop">
      <div v-if="panel === 'model'" class="pop pop-model no-pop-anim">
        <div class="pm-head">
          <span>{{ providers.length ? '选择模型' : '还没有服务商' }}</span>
          <RouterLink class="pm-link" to="/settings/providers">去配置</RouterLink>
        </div>
        <div v-if="!providers.length" class="pm-empty">
          先在「设置 · 模型服务」里加一个服务商，这里就会列出它的模型。
        </div>
        <div v-else class="pm-body">
          <div class="pm-vendors">
            <button
              v-for="p in providers"
              :key="p.id"
              class="pm-vendor"
              type="button"
              :class="{ 'is-on': p.id === activeVendor?.id }"
              @click="vendor = p.id"
              @mouseenter="vendor = p.id"
            >
              <span class="pm-avatar">{{ (p.name || p.id).slice(0, 1).toUpperCase() }}</span>
              <span class="pm-vname">{{ p.name || p.id }}</span>
              <span class="pm-count">{{ p.models.length }}</span>
            </button>
          </div>
          <div class="pm-models">
            <button
              v-for="m in activeModels"
              :key="m.id"
              class="pm-model"
              type="button"
              :class="{
                'is-on': m.id === currentModel && activeVendor?.id === currentProvider,
                'is-loading': applying === `${activeVendor?.id}/${m.id}`,
              }"
              @click="chooseModel(activeVendor!.id, m.id)"
            >
              <span class="pm-mname">{{ m.name || m.id }}</span>
              <span class="pm-meta">
                <span v-if="m.context_window">{{ Math.round(m.context_window / 1000) }}k</span>
                <span v-if="m.vision">识图</span>
                <span v-if="!m.reasoning">无思考</span>
              </span>
              <AppIcon
                v-if="m.id === currentModel && activeVendor?.id === currentProvider"
                name="check"
                size="ic-2xs"
                class="pm-check"
              />
            </button>
            <div v-if="!activeModels.length" class="pm-empty">这个服务下还没有模型</div>
          </div>
        </div>
      </div>
    </Transition>

    <Transition name="pop">
      <div v-if="panel === 'thinking'" class="pop pop-think no-pop-anim">
        <button
          v-for="t in THINKING"
          :key="t.key"
          class="pr"
          type="button"
          :class="{ 'is-hi': t.key === session.current?.thinking_level, 'is-loading': applying === `t:${t.key}` }"
          @click="chooseThinking(t.key)"
        >
          <span>{{ t.name }}</span>
          <AppIcon v-if="t.key === session.current?.thinking_level" name="check" size="ic-2xs" />
        </button>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.chips {
  display: flex;
  align-items: center;
  gap: 4px;
  position: relative;
  min-width: 0;
}
.chip-t {
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pop {
  position: absolute;
  bottom: calc(100% + 8px);
  left: 0;
  z-index: 40;
  background: var(--wb-surface);
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-lg);
  box-shadow: var(--wb-shadow-pop);
  overflow: hidden;
}
.pop-model {
  width: 460px;
}
.pm-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-bottom: 1px solid var(--wb-line);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.pm-link {
  color: var(--wb-primary);
  text-decoration: none;
  font-weight: 600;
}
.pm-body {
  display: grid;
  grid-template-columns: 176px 1fr;
  max-height: 340px;
}
.pm-vendors {
  background: var(--wb-surface-2);
  border-right: 1px solid var(--wb-line);
  overflow-y: auto;
  padding: 6px;
}
.pm-vendor {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border: none;
  background: none;
  border-radius: var(--wb-radius-sm);
  cursor: pointer;
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-sm);
  text-align: left;
}
.pm-vendor.is-on {
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
  font-weight: 600;
}
.pm-avatar {
  width: 20px;
  height: 20px;
  border-radius: 6px;
  display: grid;
  place-items: center;
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  flex: none;
}
.pm-vname {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pm-count {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.pm-models {
  overflow-y: auto;
  padding: 6px;
}
.pm-model {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 9px;
  border: none;
  background: none;
  border-radius: var(--wb-radius-sm);
  cursor: pointer;
  color: var(--wb-ink);
  font-size: var(--wb-fs-sm);
  text-align: left;
}
.pm-model:hover:not(:disabled) {
  background: var(--wb-surface-2);
}
.pm-model.is-on {
  color: var(--wb-primary);
  font-weight: 600;
}
.pm-mname {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pm-meta {
  display: flex;
  gap: 6px;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.pm-check {
  color: var(--wb-ok);
  flex: none;
}
.pm-empty {
  padding: 14px 12px;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  line-height: 1.6;
}
.pop-think {
  min-width: 148px;
  padding: 6px;
}
.pr {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 7px 9px;
  border: none;
  background: none;
  border-radius: var(--wb-radius-sm);
  cursor: pointer;
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-sm);
}
.pr:hover:not(:disabled) {
  background: var(--wb-surface-2);
}
.pr.is-hi {
  color: var(--wb-primary);
  font-weight: 600;
  background: var(--wb-primary-soft);
}
</style>
