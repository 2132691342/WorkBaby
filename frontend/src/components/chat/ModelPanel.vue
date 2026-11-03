<script setup lang="ts">
// 模型面板：厂商 + 模型两级选择，外加一个过滤框。
// 两级是因为清单动辄几十个，先厂商后模型变成两次短跳转；
// 过滤框是因为用户多半记得名字里一段（`glm`、`4.1`），两个字符就缩到几条。
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { useChatStore } from '@/stores/chat'
import { useToasts } from '@/composables/useToasts'
import type { ModelRow } from '@/types/api'

const props = defineProps<{
  /** 当前选中的厂商；空表示还没选 */
  vendor: string
}>()

const emit = defineEmits<{
  /** 换厂商 */
  pickVendor: [name: string]
}>()

const { t } = useI18n()
const chat = useChatStore()
const toasts = useToasts()

/** 正在应用哪一条（`厂商/模型`）；空表示空闲。 */
const applying = ref('')
const filter = ref('')
const filterEl = ref<HTMLInputElement | null>(null)

/** 厂商清单：按模型清单里的 provider 归并，顺序按首次出现。 */
const vendors = computed(() => {
  const seen = new Map<string, number>()
  for (const m of chat.models) {
    if (!seen.has(m.provider)) seen.set(m.provider, seen.size)
  }
  return [...seen.keys()].sort((a, b) => (seen.get(a) ?? 0) - (seen.get(b) ?? 0))
})

/** 厂商没选定时退回到第一个：面板永远要有一列可看。 */
const activeVendor = computed(
  () => vendors.value.find((v) => v === props.vendor) ?? vendors.value[0] ?? '',
)

/**
 * 当前厂商下的模型（可能带过滤）。
 *
 * 过滤命中时跨厂商显示：用户输入 `glm` 想找的就是所有带 glm 的模型，
 * 而不是「在 openrouter 里带 glm 的」。这时厂商列照旧高亮但仍能点。
 */
const activeModels = computed(() => {
  const q = filter.value.trim().toLowerCase()
  const pool = q ? chat.models : chat.models.filter((m) => m.provider === activeVendor.value)
  if (!q) return pool
  return pool.filter(
    (m) => m.id.toLowerCase().includes(q) || (m.name ?? '').toLowerCase().includes(q),
  )
})

/** 某个厂商底下有几个模型：厂商一列上给个数量，省得一个个点。 */
function countOf(name: string): number {
  return chat.models.filter((m) => m.provider === name).length
}

/** 全部厂商的模型总数（面板标题上的读数）。 */
const total = computed(() => chat.models.length)

function isCurrent(m: ModelRow): boolean {
  return m.id === chat.model?.id && m.provider === chat.model?.provider
}

async function choose(m: ModelRow): Promise<void> {
  if (applying.value) return
  applying.value = `${m.provider}/${m.id}`
  try {
    await chat.setModel(m.provider, m.id)
    toasts.push('ok', t('chat.modelSwitched', { model: m.name || m.id }))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    applying.value = ''
  }
}

/** 面板打开时把焦点放进过滤框：想找模型的人第一件事是打字。 */
watch(
  () => chat.modelsLoading,
  (loading) => {
    if (loading) return
    void nextTick(() => filterEl.value?.focus())
  },
  { immediate: true },
)

// 厂商切换后清掉「正在应用」的标记与过滤词：它们属于上一个厂商。
watch(
  () => props.vendor,
  () => {
    applying.value = ''
    filter.value = ''
  },
)
</script>

<template>
  <div class="mp-panel-body">
    <div class="mp-head">
      <span class="mp-label">{{ t('chat.modelPickerTitle') }}</span>
      <span v-if="chat.modelsLoading" class="mp-loading">
        <span class="mp-spin" />
        {{ t('chat.modelsLoading') }}
      </span>
      <span v-else class="mp-count">{{ total }}</span>
    </div>

    <div class="mp-filter-row">
      <AppIcon name="search" :size="12" class="mp-filter-ic" />
      <input
        ref="filterEl"
        v-model="filter"
        class="mp-filter"
        :placeholder="t('chat.modelFilter')"
        spellcheck="false"
      />
      <button
        v-if="filter"
        class="mp-filter-clear"
        type="button"
        :title="t('common.clear')"
        @click="filter = ''"
      >
        <AppIcon name="close" :size="11" />
      </button>
    </div>

    <div v-if="!chat.models.length && chat.modelsLoading" class="mp-skeleton">
      <span v-for="i in 6" :key="i" class="mp-skel-row" />
    </div>

    <div v-else-if="!chat.models.length" class="mp-empty">
      <p class="mp-empty-t">{{ t('chat.noModels') }}</p>
      <p class="mp-empty-h">{{ t('chat.noModelsHint') }}</p>
    </div>

    <div v-else class="mp-body">
      <div class="mp-vendors">
        <button
          v-for="name in vendors"
          :key="name"
          class="mp-vendor"
          :class="{ 'is-on': name === activeVendor && !filter }"
          type="button"
          @click="emit('pickVendor', name)"
        >
          <span class="mp-avatar">{{ name.slice(0, 1).toUpperCase() }}</span>
          <span class="mp-vname">{{ name }}</span>
          <span class="mp-vcount">{{ countOf(name) }}</span>
        </button>
      </div>

      <div class="mp-models">
        <button
          v-for="m in activeModels"
          :key="`${m.provider}/${m.id}`"
          class="mp-model"
          :class="{ 'is-on': isCurrent(m), 'is-busy': applying === `${m.provider}/${m.id}` }"
          type="button"
          :disabled="Boolean(applying)"
          :title="`${m.provider}/${m.id}`"
          @click="choose(m)"
        >
          <span class="mp-mname">{{ m.name || m.id }}</span>
          <!-- 过滤命中跨厂商时，同时显示厂商名，否则不知道这条属于谁 -->
          <span v-if="filter" class="mp-mprov">{{ m.provider }}</span>
          <span class="mp-meta">
            <span v-if="m.context_window" class="mp-tag">{{ Math.round(m.context_window / 1000) }}k</span>
            <span v-if="m.reasoning" class="mp-tag is-r" :title="t('models.reasoning')">R</span>
            <span v-if="m.input?.includes('image')" class="mp-tag" :title="t('models.inputImage')">IMG</span>
          </span>
          <AppIcon v-if="isCurrent(m)" name="check" :size="12" class="mp-check" />
        </button>
        <p v-if="!activeModels.length" class="mp-empty-h">{{ t('chat.modelNoMatch') }}</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.mp-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: 0 var(--wb-sp-2) var(--wb-sp-2);
}

.mp-label {
  font-size: var(--wb-fs-3xs);
  font-weight: 700;
  letter-spacing: 0.07em;
  text-transform: uppercase;
  color: var(--wb-faint);
}

.mp-count {
  margin-left: auto;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.mp-loading {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-left: auto;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
}

.mp-spin {
  width: 11px;
  height: 11px;
  border: 1.6px solid var(--wb-line-strong);
  border-top-color: var(--wb-brand);
  border-radius: 50%;
  animation: wb-spin 700ms linear infinite;
}

/* 过滤框：与下面两列同一圈描边，读起来像面板的一部分 */
.mp-filter-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  margin: 0 var(--wb-sp-2) var(--wb-sp-2);
  padding: 0 var(--wb-sp-2);
  height: var(--wb-ctl-sm);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-sm);
  background: var(--wb-surface);
  transition: border-color var(--wb-dur-fast) var(--wb-ease);
}

.mp-filter-row:focus-within {
  border-color: var(--wb-brand-2);
}

.mp-filter-ic {
  flex: none;
  color: var(--wb-faint);
}

.mp-filter {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: none;
  background: transparent;
  color: var(--wb-ink);
  font-family: inherit;
  font-size: var(--wb-fs-2xs);
}

.mp-filter::placeholder {
  color: var(--wb-faint);
}

.mp-filter-clear {
  display: grid;
  place-items: center;
  width: 16px;
  height: 16px;
  flex: none;
  border: 0;
  border-radius: var(--wb-r-xs);
  background: transparent;
  color: var(--wb-faint);
}

.mp-filter-clear:hover {
  background: var(--wb-hover);
  color: var(--wb-ink);
}

.mp-body {
  display: grid;
  grid-template-columns: 150px 1fr;
  max-height: 320px;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r);
  overflow: hidden;
}

.mp-vendors {
  padding: var(--wb-sp-1);
  background: var(--wb-hover);
  border-right: 1px solid var(--wb-line);
  overflow-y: auto;
}

.mp-vendor {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: var(--wb-sp-2);
  border: 0;
  border-radius: var(--wb-r-sm);
  background: transparent;
  color: var(--wb-ink-2);
  font-family: inherit;
  font-size: var(--wb-fs-2xs);
  text-align: left;
  cursor: pointer;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}

.mp-vendor:hover {
  background: var(--wb-surface);
}

.mp-vendor.is-on {
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
  font-weight: 700;
}

.mp-avatar {
  width: 20px;
  height: 20px;
  flex: none;
  display: grid;
  place-items: center;
  border-radius: var(--wb-r-xs);
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
  font-size: var(--wb-fs-3xs);
  font-weight: 700;
}

.mp-vname {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mp-vcount {
  flex: none;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

/* 过滤命中时模型行变矮：厂商列显示不出匹配数量，而真正能点的只有右列 */
.mp-models {
  padding: var(--wb-sp-1);
  overflow-y: auto;
}

.mp-model {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border: 0;
  border-radius: var(--wb-r-sm);
  background: transparent;
  color: var(--wb-ink);
  font-family: inherit;
  font-size: var(--wb-fs-2xs);
  text-align: left;
  cursor: pointer;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}

.mp-model:hover:not(:disabled) {
  background: var(--wb-hover);
}

.mp-model.is-on {
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
  font-weight: 700;
}

.mp-model.is-busy {
  opacity: 0.6;
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

.mp-mprov {
  flex: none;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.mp-meta {
  display: flex;
  flex: none;
  gap: 4px;
}

/* 能力标记：两个字母的窄条，比图标省位置且不需要额外解释 */
.mp-tag {
  padding: 0 4px;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-xs);
  color: var(--wb-mute);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  line-height: 1.5;
}

.mp-tag.is-r {
  background: var(--wb-brand-soft);
  border-color: transparent;
  color: var(--wb-brand);
}

.mp-check {
  flex: none;
  color: var(--wb-brand);
}

.mp-empty {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--wb-sp-4) var(--wb-sp-3);
  text-align: center;
}

.mp-empty-t {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}

.mp-empty-h {
  margin: 0;
  padding: var(--wb-sp-3);
  font-size: var(--wb-fs-3xs);
  line-height: 1.6;
  color: var(--wb-faint);
}

/* 骨架：清单还没到时给灰条，不给空白——空白会被当成「没有模型」 */
.mp-skeleton {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: var(--wb-sp-2);
}

.mp-skel-row {
  height: 22px;
  border-radius: var(--wb-r-sm);
  background: linear-gradient(90deg, var(--wb-hover) 25%, var(--wb-sunken) 50%, var(--wb-hover) 75%);
  background-size: 200% 100%;
  animation: mp-shimmer 1.2s linear infinite;
}

@keyframes mp-shimmer {
  to {
    background-position: -200% 0;
  }
}
</style>
