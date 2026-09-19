<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Cpu } from '@/components/common/icons'
import { t } from '@/i18n'
import type { EffectiveParams } from '@/types/api'

/**
 * 采样参数 chip + 弹层：temperature / 思考强度。
 *
 * <p>与设置页共用同一份「生效值」（props.effective，后端 /params 三层合并结果）：
 * 未覆盖时展示生效值与来源；用户调整后成为请求级覆盖（本次会话 localStorage 记忆），
 * 弹层内可一键恢复跟随配置。父组件经 {@link current} 读取随 send 上抛。
 */
export interface ComposerParams {
  temperature: number | null
  thinking_effort: string
}

const props = defineProps<{
  /** 当前会话生效参数快照（provider/default 合并后；null = 未加载，仅显示覆盖值）。 */
  effective?: EffectiveParams | null
}>()

const PARAMS_KEY = 'workbaby.composerParams'
const paramOpen = ref(false)
/** null = 不覆盖（跟随模型配置或全局默认）。 */
const paramTemperature = ref<number | null>(null)
const paramThinkingEffort = ref('')

/** 弹层展示的当前值：覆盖值优先，否则回显生效值（只读语义，不是默认假值）。 */
const displayTemperature = computed(() =>
  paramTemperature.value ?? props.effective?.temperature ?? null
)
const displayEffort = computed(() => paramThinkingEffort.value || props.effective?.thinking_effort || 'medium')

/** 覆盖态：chip 需要提示「本次会话已覆盖」。 */
const overridden = computed(() => paramTemperature.value !== null || paramThinkingEffort.value !== '')

const fromText = (src?: string) => t(`chat.params.from.${src ?? 'builtin'}`)

/**
 * 工具行紧凑 chip 文案：原型形态「T 0.7 · 思考 中」——
 * 详细字段（来源 / 生效值 / 覆盖态）放进 popover 内，chip 只承担「看一眼」。
 */
const paramSummary = computed(() => {
  const temp = displayTemperature.value == null ? t('chat.paramTempDefault') : String(displayTemperature.value)
  const effortShort = t(`chat.effort.${displayEffort.value}`)
  return `T ${temp} · ${t('chat.params.thinkingShort')} ${effortShort}`
})

/** 恢复跟随配置：清空覆盖，回显生效值。 */
function resetOverride(): void {
  paramTemperature.value = null
  paramThinkingEffort.value = ''
  saveParams()
}

function current(): ComposerParams {
  return { temperature: paramTemperature.value, thinking_effort: paramThinkingEffort.value }
}

/** 恢复上次的覆盖值（仅覆盖态持久化；空值 = 跟随配置）。 */
function loadParams(): void {
  try {
    const raw = localStorage.getItem(PARAMS_KEY)
    if (!raw) return
    const saved = JSON.parse(raw) as Partial<ComposerParams>
    if (typeof saved.temperature === 'number' && saved.temperature >= 0 && saved.temperature <= 2) {
      paramTemperature.value = saved.temperature
    }
    if (saved.thinking_effort && ['off', 'low', 'medium', 'high'].includes(saved.thinking_effort)) {
      paramThinkingEffort.value = saved.thinking_effort
    }
  } catch {
    // ignore：坏数据走默认（后端三层解析链兜底）
  }
}

function saveParams(): void {
  try {
    localStorage.setItem(PARAMS_KEY, JSON.stringify({
      temperature: paramTemperature.value,
      thinking_effort: paramThinkingEffort.value
    }))
  } catch {
    // ignore
  }
}

const effortItems = computed(() => (['off', 'low', 'medium', 'high'] as const).map((v) => ({
  value: v,
  label: t(`chat.effort.${v}`)
})))

const thresholdHint = computed(() => {
  const p = props.effective
  if (!p || p.context_budget <= 0) return ''
  return t('chat.params.thresholdHint', p.context_budget.toLocaleString(), p.context_window.toLocaleString(), p.compression_ratio)
})

onMounted(loadParams)

defineExpose({ current })
</script>

<template>
  <el-popover v-model:visible="paramOpen" placement="bottom-start" :width="300" trigger="click">
    <template #reference>
      <!-- 开关完全交给 trigger="click"；手动再 toggle 一次会造成「开→立即关」闪退 -->
      <button type="button" class="chip-btn" :class="{ 'chip-btn--overridden': overridden }">
        <el-icon :size="12"><Cpu /></el-icon>
        <span class="max-w-[180px] truncate">{{ paramSummary }}</span>
      </button>
    </template>
    <div class="flex flex-col gap-3 p-1">
      <div class="flex items-center justify-between">
        <div class="text-xs font-medium text-wb-ink">{{ t('chat.params.title') }}</div>
        <button
          v-if="overridden"
          type="button"
          class="text-[11px] text-wb-primary hover:underline"
          @click="resetOverride"
        >{{ t('chat.params.resetOverride') }}</button>
      </div>
      <div>
        <div class="mb-1 flex items-center justify-between text-xs">
          <span class="font-medium text-wb-ink">{{ t('chat.params.temperature') }}</span>
          <span class="text-[10px] text-wb-muted">{{ fromText(effective?.temperature_from) }}：{{ effective?.temperature ?? '—' }}</span>
        </div>
        <el-input-number
          v-model="paramTemperature"
          :min="0"
          :max="2"
          :step="0.05"
          :precision="2"
          :placeholder="effective ? String(effective.temperature) : t('chat.paramTempDefault')"
          class="!w-full"
          controls-position="right"
          @change="saveParams"
        />
        <div class="mt-1 text-[10px] text-wb-muted">{{ t('chat.params.effectiveHint') }}</div>
      </div>
      <div>
        <div class="mb-1 flex items-center justify-between text-xs">
          <span class="font-medium text-wb-ink">{{ t('chat.params.thinking') }}</span>
          <span class="text-[10px] text-wb-muted">{{ fromText(effective?.thinking_from) }}：{{ t(`chat.effort.${displayEffort}`) }}</span>
        </div>
        <el-select v-model="paramThinkingEffort" class="!w-full" clearable :placeholder="t('chat.params.resetOverride')" @change="saveParams">
          <el-option v-for="item in effortItems" :key="item.value" :value="item.value" :label="item.label" />
        </el-select>
      </div>
      <template v-if="effective">
        <div class="border-t border-wb-border pt-2 text-[11px] text-wb-muted">
          <div class="flex items-center justify-between py-0.5">
            <span>{{ t('chat.params.contextWindow') }}</span>
            <span class="text-wb-ink">{{ effective.context_window.toLocaleString() }}（{{ fromText(effective.context_window_from) }}）</span>
          </div>
          <div class="flex items-center justify-between py-0.5">
            <span>{{ t('chat.params.compressionRatio') }}</span>
            <span class="text-wb-ink">{{ effective.compression_ratio }}（{{ fromText(effective.compression_from) }}）</span>
          </div>
          <div class="flex items-center justify-between py-0.5">
            <span>{{ t('chat.params.contextBudget') }}</span>
            <span class="text-wb-ink">{{ effective.context_budget.toLocaleString() }} tokens</span>
          </div>
        </div>
        <div v-if="thresholdHint" class="text-[10px] leading-snug text-wb-muted">{{ thresholdHint }}</div>
      </template>
    </div>
  </el-popover>
</template>

<style scoped>
/* 顶部次要控制项 chip：幽灵胶囊（无常驻框线，hover 才浮出底色）。
   样式在组件内定义（scoped 不跨组件），勿在父组件用 :deep 覙覆盖——注入顺序会输。 */
.chip-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 24px;
  max-width: 240px;
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
/* 存在请求级覆盖时提示用户：当前值不再跟随配置（只用文字色 + 底色，不描边） */
.chip-btn--overridden {
  color: var(--wb-primary-strong);
  background: color-mix(in srgb, var(--wb-primary) 10%, transparent);
}
</style>
