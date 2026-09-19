<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ChevronDown, Wrench, Eye, Brain, Sparkles, Clock, AlertTriangle, ZapOff } from '@/components/common/icons'
import type { AvailableModel, CircuitState } from '@/types/api'
import { t } from '@/i18n'

/**
 * 模型选择器：常用记忆（localStorage 最近 5 个）+ 按供应商分组 + 能力徽标 + 熔断状态。
 *
 * <p>下拉内容保留自定义实现——能力徽标（工具调用 / 视觉 / 思考 / 图片输出）、价格等级与
 * 熔断徽标是本项目特有信息，`el-select` 无法承载；仅触发器与搜索框使用 ElementPlus。
 */
const props = defineProps<{
  models: AvailableModel[]
  selectedModelID?: string | null
  /** 熔断状态 Map（id → CircuitState）；传入后显示熔断 / 禁用徽标。 */
  circuitStates?: Map<string, CircuitState>
}>()

const emit = defineEmits<{
  'select-model': [id: string | null]
}>()

/** A3：模型状态徽标类型。
 *  语义区分：disabled（设置里关闭）→ 未就绪（registry 未构建/unready，给出原因）→ 半开探活。
 *  注：circuit-status 的 OPEN 实际是「registry 未就绪」（apiKey 缺失/构建失败/刚新增未加载），
 *  文案不再写成"已熔断 · 0s 后探活"误导用户。 */
function circuitBadge(m: AvailableModel): { kind: 'open' | 'half' | 'disabled' | null; title: string } {
  if (!m.enabled) {
    return { kind: 'disabled', title: '已禁用 — 请到设置启用' }
  }
  const s = props.circuitStates?.get(m.id)
  if (!s) return { kind: null, title: '' }
  if (s.state === 'OPEN') {
    const reason = s.reason || '未就绪（缺少 API Key 或连接配置）'
    return { kind: 'open', title: `不可用：${reason}` }
  }
  if (s.state === 'HALF_OPEN') {
    return { kind: 'half', title: '半开探活中 — 即将恢复' }
  }
  return { kind: null, title: '' }
}

const open = ref(false)
const search = ref('')
const recentIds = ref<string[]>(([]))

// 加载常用模型（localStorage）
function loadRecent(): void {
  try {
    const stored = localStorage.getItem('workbaby.recentModels')
    if (stored) {
      recentIds.value = JSON.parse(stored) as string[]
    }
  } catch {
    recentIds.value = []
  }
}

// 保存常用模型（最多5个）
function saveRecent(modelID: string): void {
  if (!modelID) return
  recentIds.value = [modelID, ...recentIds.value.filter((id) => id !== modelID)].slice(0, 5)
  try {
    localStorage.setItem('workbaby.recentModels', JSON.stringify(recentIds.value))
  } catch {
    // ignore
  }
}

const currentModel = computed(() => {
  if (!props.selectedModelID) return null
  return props.models.find((m) => m.id === props.selectedModelID) ?? null
})

const currentLabel = computed(() => {
  if (!currentModel.value) return t('chat.auto')
  return currentModel.value.alias || currentModel.value.model
})

// 按供应商分组
const providerGroups = computed(() => {
  const filtered = props.models.filter((m) => {
    if (!search.value) return true
    const q = search.value.toLowerCase()
    return (
      m.model.toLowerCase().includes(q) ||
      (m.alias?.toLowerCase().includes(q) ?? false) ||
      (m.name?.toLowerCase().includes(q) ?? false)
    )
  })

  const groups: Record<string, AvailableModel[]> = {}
  for (const m of filtered) {
    const key = m.name ?? t('chat.unknown')
    if (!groups[key]) groups[key] = []
    groups[key].push(m)
  }
  return Object.entries(groups).sort(([a], [b]) => a.localeCompare(b))
})

// 常用模型过滤
const recentModels = computed(() => {
  return recentIds.value
    .map((id) => props.models.find((m) => m.id === id))
    .filter((m): m is AvailableModel => !!m)
})

/** 档位标签：primary/backup 仅为排序档位（取值由后端 domain.ProviderTier 定义）。 */
function tierLabel(model: AvailableModel): string {
  return model.tier === 'backup' ? t('chat.tierBackup') : t('chat.tierPrimary')
}

/** 供应商品牌识别：返回首字母缩写 + 品牌色（未知供应商回退主题色），替代 emoji/通用图标。 */
interface BrandBadge {
  text: string
  bg: string
  fg: string
}
const FALLBACK_BRAND: BrandBadge = { text: 'AI', bg: 'var(--wb-primary)', fg: '#fff' }
function providerBrand(name?: string | null): BrandBadge {
  const n = (name ?? '').trim().toLowerCase()
  const brands: Array<[RegExp, string, string]> = [
    [/openai|gpt-|o[134]-(mini|pro)/, 'OAI', '#10a37f'],
    [/anthropic|claude/, 'CL', '#d97757'],
    [/deepseek/, 'DS', '#4d6bfe'],
    [/qwen|通义/, 'QW', '#615ced'],
    [/zhipu|glm|bigmodel/, 'GLM', '#3859ff'],
    [/google|gemini/, 'GM', '#4285f4'],
    [/ollama/, 'OL', '#7c8b9d'],
    [/moonshot|kimi/, 'MO', '#1f1f1f'],
    [/mistral/, 'M', '#ff7000'],
    [/meta|llama/, 'MA', '#0866ff'],
    [/xai|grok/, 'G', '#14171a']
  ]
  for (const [re, text, color] of brands) {
    if (re.test(n)) return { text, bg: color, fg: '#fff' }
  }
  const first = (name ?? '').replace(/[^a-zA-Z0-9\u4e00-\u9fa5]/g, '').slice(0, 2).toUpperCase()
  return first ? { text: first, bg: 'var(--wb-primary)', fg: '#fff' } : FALLBACK_BRAND
}
/** 模型的品牌标识（按其 provider 名识别）。 */
function modelBrand(m: AvailableModel): BrandBadge {
  return providerBrand(m.name || m.model)
}

function selectModel(id: string | null): void {
  open.value = false
  if (id) saveRecent(id)
  emit('select-model', id)
}

function toggleOpen(): void {
  open.value = !open.value
}

function close(): void {
  open.value = false
  search.value = ''
}

/** 暴露 open() 给父组件（slash 命令 `/model` 触发）。 */
defineExpose({ open: toggleOpen, close, selectModel })

function onDocClick(e: MouseEvent): void {
  const target = e.target as HTMLElement
  if (!target.closest('.wb-model-selector')) {
    close()
  }
}

onMounted(() => {
  loadRecent()
  document.addEventListener('click', onDocClick)
})

onUnmounted(() => {
  document.removeEventListener('click', onDocClick)
})
</script>

<template>
  <div class="wb-model-selector relative">
    <!-- 触发按钮：与 ChatInput 顶部 chip 行同款紧凑样式（.chip-btn），
         视觉权重等同于其它次要控制项，不抢输入框焦点 -->
    <button type="button" class="chip-btn" @click="toggleOpen">
      <!-- 已选模型：供应商品牌圆徽；自动：Sparkles 而非 emoji -->
      <span
        v-if="currentModel"
        class="inline-flex h-4 w-4 shrink-0 items-center justify-center rounded-full text-[8px] font-bold"
        :style="{ background: modelBrand(currentModel).bg, color: modelBrand(currentModel).fg }"
      >
        {{ modelBrand(currentModel).text }}
      </span>
      <Sparkles v-else class="h-3 w-3 shrink-0 text-wb-primary" />
      <span class="max-w-[140px] truncate">{{ currentLabel }}</span>
      <el-icon :size="11" class="transition-transform" :class="open ? 'rotate-180' : ''">
        <ChevronDown />
      </el-icon>
    </button>

    <!-- 下拉面板 -->
    <Transition
      enter-active-class="transition-all duration-200 ease-out"
      enter-from-class="scale-95 opacity-0"
      enter-to-class="scale-100 opacity-100"
      leave-active-class="transition-all duration-150 ease-in"
      leave-from-class="scale-100 opacity-100"
      leave-to-class="scale-95 opacity-0"
    >
      <div
        v-if="open"
        class="absolute bottom-full left-0 z-50 mb-2 w-72 rounded-2xl border border-wb-border bg-wb-surface p-3 shadow-[var(--wb-shadow-lg)]"
      >
        <!-- 搜索框 -->
        <el-input
          v-model="search"
          size="small"
          class="mb-2"
          :placeholder="t('chat.searchModel')"
          clearable
        />

        <!-- Auto 选项 -->
        <button
          type="button"
          class="mb-1 flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-xs transition-colors"
          :class="!selectedModelID ? 'bg-wb-primary/10 text-wb-primary-strong' : 'text-wb-ink hover:bg-wb-primary/5'"
          @click="selectModel(null)"
        >
          <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-wb-primary/15 to-wb-lavender/30 text-wb-primary-strong">
            <Sparkles class="h-3 w-3" />
          </span>
          <span class="min-w-0 flex-1">
            <span class="block font-medium">{{ t('chat.auto') }}</span>
            <span class="block text-[10px] text-wb-muted">{{ t('chat.autoDesc') }}</span>
          </span>
        </button>

        <!-- 常用模型 -->
        <template v-if="recentModels.length > 0 && !search">
          <div class="my-1 flex items-center gap-1 px-2 text-[10px] text-wb-muted">
            <Clock class="h-3 w-3" />
            {{ t('chat.recentModels') }}
          </div>
          <button
            v-for="m in recentModels"
            :key="m.id"
            type="button"
            class="mb-1 flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-xs transition-colors"
            :class="selectedModelID === m.id ? 'bg-wb-primary/10 text-wb-primary-strong' : 'text-wb-ink hover:bg-wb-primary/5'"
            @click="selectModel(m.id)"
          >
            <span
              class="flex h-6 w-6 shrink-0 items-center justify-center rounded-lg text-[9px] font-bold"
              :style="{ background: modelBrand(m).bg, color: modelBrand(m).fg }"
            >
              {{ modelBrand(m).text }}
            </span>
            <span class="min-w-0 flex-1">
              <span class="block truncate font-medium">{{ m.alias || m.model }}</span>
              <span class="block truncate text-[10px] text-wb-muted">{{ m.name }}</span>
            </span>
            <span class="text-[10px] text-wb-muted">{{ tierLabel(m) }}</span>
          </button>
        </template>

        <!-- 按供应商分组 -->
        <div v-if="providerGroups.length > 0" class="max-h-64 overflow-y-auto">
          <div v-for="[provider, models] in providerGroups" :key="provider">
            <div class="my-1 flex items-center gap-1 px-2 text-[10px] text-wb-muted">
              {{ provider }}
            </div>
            <button
              v-for="m in models"
              :key="m.id"
              type="button"
              class="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-xs transition-colors"
              :class="[
                selectedModelID === m.id ? 'bg-wb-primary/10 text-wb-primary-strong' : 'text-wb-ink hover:bg-wb-primary/5',
                circuitBadge(m).kind === 'open' || circuitBadge(m).kind === 'disabled' ? 'opacity-60' : ''
              ]"
              :disabled="circuitBadge(m).kind === 'open' || circuitBadge(m).kind === 'disabled'"
              :title="circuitBadge(m).title"
              @click="selectModel(m.id)"
            >
              <span
                class="flex h-5 w-5 shrink-0 items-center justify-center rounded-md text-[8px] font-bold"
                :style="{ background: providerBrand(provider).bg, color: providerBrand(provider).fg }"
              >
                {{ providerBrand(provider).text }}
              </span>
              <span class="min-w-0 flex-1 truncate font-medium" :title="m.alias || m.model">
                {{ m.alias || m.model }}
              </span>
              <!-- 熔断 / 禁用徽标 -->
              <template v-if="circuitBadge(m).kind === 'open'">
                <ZapOff class="h-3 w-3 text-wb-danger" />
              </template>
              <template v-else-if="circuitBadge(m).kind === 'half'">
                <AlertTriangle class="h-3 w-3 text-wb-warning" />
              </template>
              <template v-else-if="circuitBadge(m).kind === 'disabled'">
                <span class="rounded bg-wb-surface-2 px-1 text-[9px] font-medium text-wb-muted">{{ t('common.disabled') }}</span>
              </template>
              <!-- 能力标签图标 -->
              <span class="flex shrink-0 items-center gap-0.5">
                <Wrench v-if="m.tool_call" class="h-3 w-3 text-wb-muted" :title="t('chat.toolCall')" />
                <Eye v-if="m.vision" class="h-3 w-3 text-wb-muted" :title="t('chat.vision')" />
                <Brain v-if="m.reasoning" class="h-3 w-3 text-wb-muted" :title="t('chat.thinking')" />
              </span>
              <span class="text-[10px] text-wb-muted">{{ tierLabel(m) }}</span>
            </button>
          </div>
        </div>

        <!-- 空状态 -->
        <div v-else class="px-2 py-4 text-center text-xs text-wb-muted">
          {{ t('chat.noModels') }}
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
/* 触发按钮：与 ChatInput 工具行同款幽灵胶囊（无常驻框线，hover 才浮出底色）。
   样式在组件内定义（scoped 不跨组件），勿在父组件用 :deep 覜覆盖——注入顺序会输。 */
.chip-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 24px;
  max-width: 220px;
  padding: 0 8px;
  border: 1px solid transparent;
  border-radius: 9999px;
  background: transparent;
  color: var(--wb-muted);
  font-size: 11px;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
  transition:
    color 0.15s ease,
    background-color 0.15s ease;
}
.chip-btn:hover {
  background: var(--wb-surface-hover);
  color: var(--wb-ink);
}
.chip-btn:focus-visible {
  outline: none;
  background: var(--wb-surface-hover);
  color: var(--wb-ink);
}
</style>
