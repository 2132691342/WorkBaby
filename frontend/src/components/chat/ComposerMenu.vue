<script setup lang="ts">
// 模型选择面板：左栏服务商 + 右栏模型清单，顶部搜索跨服务商过滤。
// 从 SessionChips 拆出：这里要做键盘导航与搜索，留在 chip 组件里会把它撑到 500 行外。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { ProviderVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'

/** 面板里的一条模型（已带上它所属的服务商）。 */
interface FlatModel {
  /** 服务商 id */
  provider_id: string
  /** 服务商显示名 */
  provider_name: string
  /** 模型 id */
  model_id: string
  /** 模型显示名 */
  model_name: string
  /** 上下文窗口（0 = 未配置） */
  model_context_window: number
  /** 服务商或模型带思考档 */
  model_reasoning: boolean
  /** 支持图片输入 */
  model_vision: boolean
}

const props = defineProps<{
  /** 全部服务商（含各自的模型） */
  providers: ProviderVO[]
  /** 当前生效的服务商 id */
  currentProvider: string
  /** 当前生效的模型 id */
  currentModel: string
  /** 正在切换的目标（`provider/model`），切完前锁住整个面板 */
  applying: string
}>()

const emit = defineEmits<{
  /** 选定一个模型 */
  pick: [providerId: string, modelId: string]
  /** 去服务商设置页 */
  manage: []
}>()

const { t } = useI18n()
const router = useRouter()

/** 左栏高亮的服务商；搜索时失效（结果已跨服务商） */
const vendor = ref(props.currentProvider || props.providers[0]?.provider_id || '')
const query = ref('')
const index = ref(0)
const listEl = ref<HTMLElement | null>(null)

const searching = computed(() => query.value.trim().length > 0)

const activeModels = computed<FlatModel[]>(() => {
  const p = props.providers.find((x) => x.provider_id === vendor.value)
  if (!p) return []
  return p.provider_models.map((m) => ({
    provider_id: p.provider_id,
    provider_name: p.provider_name || p.provider_id,
    model_id: m.model_id,
    model_name: m.model_name || m.model_id,
    model_context_window: m.model_context_window,
    model_reasoning: m.model_reasoning,
    model_vision: m.model_vision,
  }))
})

/** 搜索结果：跨服务商，按「服务商名 · 模型名」匹配。 */
const searchResults = computed<FlatModel[]>(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return []
  const out: FlatModel[] = []
  for (const p of props.providers) {
    for (const m of p.provider_models) {
      const hay = `${p.provider_name || p.provider_id} ${m.model_name} ${m.model_id}`.toLowerCase()
      if (!hay.includes(q)) continue
      out.push({
        provider_id: p.provider_id,
        provider_name: p.provider_name || p.provider_id,
        model_id: m.model_id,
        model_name: m.model_name || m.model_id,
        model_context_window: m.model_context_window,
        model_reasoning: m.model_reasoning,
        model_vision: m.model_vision,
      })
    }
  }
  return out
})

/** 面板里真正显示的那一列。 */
const shown = computed(() => (searching.value ? searchResults.value : activeModels.value))

watch([query, vendor], () => (index.value = 0))

/** 高亮行必须可见：否则键盘上下键在长清单里等于瞎走。 */
function ensureVisible(i: number) {
  const box = listEl.value
  if (!box) return
  const el = box.querySelector<HTMLElement>(`[data-idx="${i}"]`)
  if (!el) return
  const top = el.offsetTop - box.offsetTop
  const bottom = top + el.offsetHeight
  if (top < box.scrollTop) box.scrollTop = top
  else if (bottom > box.scrollTop + box.clientHeight) box.scrollTop = bottom - box.clientHeight
}

function pickAt(i: number) {
  const m = shown.value[i]
  if (!m || props.applying) return
  emit('pick', m.provider_id, m.model_id)
}

function onKeydown(e: KeyboardEvent) {
  const n = shown.value.length
  if (e.key === 'ArrowDown' && n) {
    e.preventDefault()
    index.value = (index.value + 1) % n
    void nextTick(() => ensureVisible(index.value))
    return
  }
  if (e.key === 'ArrowUp' && n) {
    e.preventDefault()
    index.value = (index.value - 1 + n) % n
    void nextTick(() => ensureVisible(index.value))
    return
  }
  if (e.key === 'Enter' && n) {
    e.preventDefault()
    pickAt(Math.min(index.value, n - 1))
    return
  }
  if (e.key === 'Escape') {
    query.value = ''
    return
  }
  // 搜索框有内容时，左右键在服务商之间切换不打断输入
  if (e.key === 'Tab' && !searching.value) {
    e.preventDefault()
    const ids = props.providers.map((p) => p.provider_id)
    const at = ids.indexOf(vendor.value)
    vendor.value = ids[(at + 1) % ids.length] ?? ''
  }
}

const searchEl = ref<HTMLInputElement | null>(null)

onMounted(() => {
  void nextTick(() => searchEl.value?.focus())
})

// 面板随 chip 一起卸载；这里只清掉自动聚焦留下的光标位置，不需额外监听
onBeforeUnmount(() => {
  query.value = ''
})

/** 当前选中的那一条（用于打勾）。 */
function isCurrent(m: FlatModel): boolean {
  return m.provider_id === props.currentProvider && m.model_id === props.currentModel
}

function contextOf(m: FlatModel): string {
  return m.model_context_window ? `${Math.round(m.model_context_window / 1000)}k` : ''
}

function goManage() {
  emit('manage')
  void router.push('/models')
}
</script>

<template>
  <div class="menu" @keydown="onKeydown">
    <div class="menu-head">
      <span class="wb-label">{{ t('chat.modelPickerTitle') }}</span>
      <button class="menu-link" type="button" @click="goManage">{{ t('chat.goProviders') }}</button>
    </div>

    <div class="menu-search">
      <AppIcon name="search" size="ic-2xs" class="ms-icon" />
      <input
        ref="searchEl"
        v-model="query"
        class="ms-input"
        :placeholder="t('chat.modelSearchPlaceholder')"
      />
      <button v-if="searching" class="ms-clear" type="button" @click="query = ''">
        <AppIcon name="close" size="ic-2xs" />
      </button>
    </div>

    <div v-if="!providers.length" class="menu-empty">{{ t('chat.noProviders') }}</div>

    <div v-else class="menu-body">
      <div v-if="!searching" class="menu-vendors">
        <button
          v-for="p in providers"
          :key="p.provider_id"
          class="mv"
          type="button"
          :class="{ 'is-on': p.provider_id === vendor }"
          @click="vendor = p.provider_id"
        >
          <span class="mv-avatar">{{ (p.provider_name || p.provider_id).slice(0, 1).toUpperCase() }}</span>
          <span class="mv-name">{{ p.provider_name || p.provider_id }}</span>
          <span class="mv-count">{{ p.provider_models.length }}</span>
        </button>
      </div>

      <div ref="listEl" class="menu-models">
        <div v-if="searching" class="menu-hint">
          {{ t('chat.modelSearchCount', { n: shown.length }) }}
        </div>
        <button
          v-for="(m, i) in shown"
          :key="`${m.provider_id}/${m.model_id}`"
          :data-idx="i"
          class="mm"
          type="button"
          :class="{ 'is-on': isCurrent(m), 'is-hi': i === index, 'is-busy': applying === `${m.provider_id}/${m.model_id}` }"
          :disabled="!!applying"
          @mouseenter="index = i"
          @click="pickAt(i)"
        >
          <span class="mm-body">
            <span class="mm-name">{{ m.model_name }}</span>
            <span v-if="searching" class="mm-prov">{{ m.provider_name }}</span>
          </span>
          <span class="mm-meta">
            <span v-if="contextOf(m)">{{ contextOf(m) }}</span>
            <span v-if="m.model_vision">img</span>
            <span v-if="m.model_reasoning">think</span>
          </span>
          <AppIcon v-if="isCurrent(m)" name="check" size="ic-2xs" class="mm-check" />
        </button>
        <div v-if="!shown.length" class="menu-empty">
          {{ searching ? t('chat.modelSearchEmpty') : t('common.empty') }}
        </div>
      </div>
    </div>

    <div class="menu-foot">{{ t('chat.modelMenuHint') }}</div>
  </div>
</template>

<style scoped>
.menu {
  width: calc(500px * var(--wb-vw-scale));
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.menu-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.menu-link {
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-primary);
}
.menu-link:hover {
  text-decoration: underline;
}
.menu-search {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  height: var(--wb-ctl-h-sm);
  padding: 0 var(--wb-sp-2);
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-elev-raised);
  transition: border-color var(--wb-dur) var(--wb-ease);
}
.menu-search:focus-within {
  border-color: var(--wb-primary);
}
.ms-icon {
  color: var(--wb-muted);
  flex: none;
}
.ms-input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  color: var(--wb-ink);
  font-size: var(--wb-fs-sm);
}
.ms-clear {
  display: grid;
  place-items: center;
  width: calc(18px * var(--wb-vw-scale));
  height: calc(18px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  color: var(--wb-muted);
  flex: none;
  transition: background var(--wb-dur-fast) var(--wb-ease);
}
.ms-clear:hover {
  background: var(--wb-elev-hover);
  color: var(--wb-ink);
}
.menu-body {
  display: grid;
  grid-template-columns: calc(176px * var(--wb-vw-scale)) 1fr;
  max-height: calc(340px * var(--wb-vw-scale));
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius);
  overflow: hidden;
}
.menu-body:has(.menu-models:only-child) {
  grid-template-columns: 1fr;
}
.menu-vendors {
  background: var(--wb-elev-hover);
  border-right: 1px solid var(--wb-border);
  overflow-y: auto;
  padding: var(--wb-sp-1);
}
.mv {
  width: 100%;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-sm);
  text-align: left;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.mv:hover {
  background: var(--wb-tint);
  transform: scale(1.02);
}
.mv:active {
  transform: scale(var(--wb-row-scale-press));
}
.mv.is-on {
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
}
.mv-avatar {
  display: grid;
  place-items: center;
  width: calc(22px * var(--wb-vw-scale));
  height: calc(22px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  background: var(--wb-elev-raised);
  border: 1px solid var(--wb-border);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  flex: none;
}
.mv-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mv-count {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  flex: none;
}
.menu-models {
  overflow-y: auto;
  padding: var(--wb-sp-1);
  min-height: 0;
}
.menu-hint {
  padding: 0 var(--wb-sp-2) var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.mm {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  text-align: left;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.mm:hover:not(:disabled),
.mm.is-hi:not(:disabled) {
  background: var(--wb-accent-tint);
  transform: scale(1.01);
}
.mm:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.mm.is-busy {
  opacity: 1;
  background: var(--wb-accent-tint);
}
.mm-body {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}
.mm-name {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mm.is-on .mm-name {
  font-weight: 700;
  color: var(--wb-primary);
}
.mm-prov {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mm-meta {
  display: flex;
  gap: var(--wb-sp-1);
  flex: none;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.mm-check {
  flex: none;
  color: var(--wb-primary);
}
.menu-empty {
  padding: var(--wb-sp-6) var(--wb-sp-4);
  text-align: center;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.menu-foot {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  text-align: center;
}
@media (prefers-reduced-motion: reduce) {
  .mv:hover,
  .mv:active,
  .mm:hover:not(:disabled),
  .mm.is-hi:not(:disabled) {
    transform: none;
  }
}
</style>
