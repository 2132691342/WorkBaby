<script setup lang="ts">
// 首屏的「下一步」条：概览页最上面那一行，回答「我现在该干什么」。
// 三种状态各自只有一个主行动，不并列三个按钮——并列等于没有主次：
//   没选工作区 → 选工作区
//   没配服务商 → 去配服务商
//   都齐了     → 开始对话
// 全齐之后它自动收起成一行「一切就绪」，不占一屏。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'

const props = defineProps<{
  /** 当前工作区路径；空串表示还没选 */
  workspace: string
  /** 已配置服务商数（0 表示一个都没有） */
  providerCount: number
  /** 是否已经配了至少一个模型 */
  hasModel: boolean
  /** 引擎是否就绪：没就绪时「开始对话」不该是可点的主按钮 */
  engineReady: boolean
  /** 开着没动：正在做选择 / 切换 */
  choosing?: boolean
}>()

const emit = defineEmits<{ pickWorkspace: []; openModels: []; startChat: [] }>()

const { t } = useI18n()

/** 第一步：有没有工作区。没有的话后面两步都无从谈起。 */
const step = computed<'workspace' | 'provider' | 'model' | 'ready'>(() => {
  if (!props.workspace) return 'workspace'
  if (props.providerCount === 0) return 'provider'
  if (!props.hasModel) return 'model'
  return 'ready'
})

const title = computed(() => {
  switch (step.value) {
    case 'workspace':
      return t('overview.nextPickWorkspace')
    case 'provider':
      return t('overview.nextAddProvider')
    case 'model':
      return t('overview.nextAddModel')
    default:
      return t('overview.nextReady')
  }
})

const hint = computed(() => {
  switch (step.value) {
    case 'workspace':
      return t('overview.nextPickWorkspaceHint')
    case 'provider':
      return t('overview.nextAddProviderHint')
    case 'model':
      return t('overview.nextAddModelHint')
    default:
      return t('overview.nextReadyHint')
  }
})

/** 唯一的主行动：状态决定它是哪一个，不并列。 */
const primary = computed<{ label: string; run: () => void; disabled: boolean } | null>(() => {
  switch (step.value) {
    case 'workspace':
      return { label: t('overview.nextPickWorkspaceAct'), run: () => emit('pickWorkspace'), disabled: Boolean(props.choosing) }
    case 'provider':
      return { label: t('overview.nextAddProviderAct'), run: () => emit('openModels'), disabled: false }
    case 'model':
      return { label: t('overview.nextAddModelAct'), run: () => emit('openModels'), disabled: false }
    default:
      return props.engineReady
        ? { label: t('overview.nextReadyAct'), run: () => emit('startChat'), disabled: false }
        : null
  }
})

/** 就绪态只占一行小字：不需要再教人怎么开始。 */
const compact = computed(() => step.value === 'ready')
</script>

<template>
  <section class="next" :class="{ 'is-compact': compact, 'is-blocking': step === 'workspace' }">
    <span class="next-mark" aria-hidden="true">
      <AppIcon :name="step === 'ready' ? 'check' : 'spark'" :size="compact ? 13 : 16" />
    </span>

    <div class="next-text">
      <p class="next-title">{{ title }}</p>
      <p class="next-hint">{{ hint }}</p>
      <!-- 没有工作区时把当前值也说出来：空白路径是最容易被忽略的一步 -->
      <p v-if="step === 'workspace'" class="next-path mono">{{ workspace || t('overview.nextNoWorkspace') }}</p>
    </div>

    <button
      v-if="primary"
      class="wb-btn primary"
      :class="{ 'is-loading': primary.disabled }"
      :disabled="primary.disabled"
      type="button"
      @click="primary.run()"
    >
      {{ primary.label }}
    </button>
  </section>
</template>

<style scoped>
.next {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-3) var(--wb-sp-4);
  border: 1px solid var(--wb-line-strong);
  border-radius: var(--wb-r-lg);
  background: var(--wb-raised);
  box-shadow: var(--wb-card-shadow);
}

/* 缺工作区：这一步不完成后面全是空读数，所以给它主色描边 */
.next.is-blocking {
  border-color: var(--wb-brand-line);
  background: var(--wb-brand-soft);
}

/* 一切就绪：退回一行淡底提示，不占视觉重量 */
.next.is-compact {
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-color: var(--wb-line);
  background: var(--wb-surface);
  box-shadow: none;
}

.next-mark {
  display: grid;
  place-items: center;
  flex: none;
  width: 28px;
  height: 28px;
  border-radius: var(--wb-r-sm);
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
}

.next.is-compact .next-mark {
  width: 20px;
  height: 20px;
  background: var(--wb-ok-soft);
  color: var(--wb-ok);
}

.next-text {
  flex: 1;
  min-width: 0;
}

.next-title {
  margin: 0;
  font-size: var(--wb-fs-sm);
  font-weight: 650;
  color: var(--wb-ink);
}

.next.is-compact .next-title {
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
  color: var(--wb-ink-2);
}

.next-hint {
  margin: 2px 0 0;
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-mute);
  max-width: var(--wb-w-desc);
}

.next.is-compact .next-hint {
  display: none;
}

.next-path {
  margin: var(--wb-sp-1) 0 0;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
