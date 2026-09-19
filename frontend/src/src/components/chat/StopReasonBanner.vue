<script setup lang="ts">
import { computed } from 'vue'
import { RefreshCw } from '@/components/common/icons'
import { t } from '@/i18n'
import { formatDuration } from '@/utils/time'
import { RESUMABLE_STOP_REASONS, stopReasonSeverity, stopReasonText } from '@/chat/models/blocks'

/**
 * 流终止原因横幅：把 MessageList 里那段 el-alert + 「继续」按钮抽成组件。
 *
 * <p>可恢复终态（RESUMABLE_STOP_REASONS）展示「继续」按钮，
 * 用户一键接着上轮未完成的部分做。
 */
const props = defineProps<{
  stopReason: string | null
  streaming: boolean
  /** 本轮 run 耗时（ms）；有值时把终态说成人话（「你在 12s 后停止」）。 */
  elapsedMs?: number | null
}>()

const emit = defineEmits<{
  continue: []
  dismiss: []
}>()

const text = computed(() => {
  // 人话优先：用户主动停止且知道耗时 → 「你在 12s 后停止」
  //（把系统状态翻译成用户做过的事，而不是丢一个「cancelled」）
  if (props.stopReason === 'cancelled' && (props.elapsedMs ?? 0) > 0) {
    return t('chat.stoppedByUserAfter', formatDuration(props.elapsedMs))
  }
  const key = stopReasonText(props.stopReason ?? '')
  return key ? t(key) : t('chat.stoppedByUser')
})
const type = computed(() => stopReasonSeverity(props.stopReason ?? '') as 'info' | 'warning' | 'error')
const canContinue = computed(
  () => !props.streaming && !!props.stopReason && RESUMABLE_STOP_REASONS.has(props.stopReason)
)
const show = computed(
  () => !props.streaming && !!props.stopReason && props.stopReason !== 'completed'
)

function onContinue(): void {
  if (props.streaming) return
  emit('continue')
}
</script>

<template>
  <el-alert
    v-if="show"
    :title="text"
    :type="type"
    show-icon
    closable
    :close-text="t('chat.dismiss')"
    class="!rounded-xl"
    @close="emit('dismiss')"
  >
    <div v-if="canContinue" class="mt-1">
      <el-button size="small" text type="primary" :disabled="streaming" @click="onContinue">
        <el-icon class="mr-1"><RefreshCw /></el-icon>
        {{ t('chat.continueRun') }}
      </el-button>
    </div>
  </el-alert>
</template>
