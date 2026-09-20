<script setup lang="ts">
/**
 * 列表页三态收敛：loading（骨架屏）/ error（文案 + 重试）/ empty（占位）。
 * 优先级 loading > error > empty，避免状态切换闪烁。
 *
 * <p>骨架与空态复用自研组件（Skeleton / EmptyState）：Element Plus 的 el-empty / el-skeleton
 * 会把整套 EP 依赖拖进懒加载 chunk（实测多出 ~1.1MB），且样式与设计令牌不同源。
 */
import { CircleX } from '@/components/common/icons'
import Skeleton from '@/components/common/Skeleton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { t } from '@/i18n'

defineProps<{
  loading?: boolean
  /** 数据为空（非加载中且无错误时展示）。 */
  empty?: boolean
  /** 错误信息；非空展示错误态。 */
  error?: string | null
  /** 空态文案（缺省走 i18n）。 */
  emptyText?: string
  /** 错误态文案（缺省走 i18n）。 */
  errorText?: string
}>()

defineEmits<{ retry: [] }>()
</script>

<template>
  <!-- 骨架屏（loading 优先） -->
  <div v-if="loading" class="py-8">
    <Skeleton variant="text" :lines="4" />
  </div>

  <!-- 空态 -->
  <EmptyState
    v-else-if="empty && !error"
    variant="empty-search"
    :title="emptyText ?? t('ui.state.empty')"
  />

  <!-- 错误态 -->
  <div v-else-if="error" class="flex flex-col items-center gap-3 py-10 text-center">
    <CircleX class="ic-lg text-wb-danger" style="width: 28px; height: 28px" />
    <p class="max-w-md text-sm text-wb-muted">{{ errorText ?? t('ui.state.error') }}</p>
    <p class="max-w-md text-xs text-wb-muted/70">{{ error }}</p>
    <el-button size="small" @click="$emit('retry')">{{ t('ui.state.retry') }}</el-button>
  </div>
</template>
