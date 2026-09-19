<script setup lang="ts">
import { t } from '@/i18n'

/**
 * 全局统一表单弹窗：新增 / 编辑 / 配置类 CRUD 一律走它，避免页内折叠卡片与弹窗两套交互并存。
 *
 * <p>尺寸与排版对齐「编辑模型」弹窗（720px + 28px 控件 + 12px 字号）：
 * 遮罩点击、ESC 关闭、提交中锁定由本组件统一保证。
 */
interface Props {
  /** 是否显示（v-model）。 */
  modelValue: boolean
  title: string
  description?: string
  width?: string
  /** 提交中：锁定关闭与重复提交。 */
  submitting?: boolean
  confirmText?: string
  cancelText?: string
  confirmDisabled?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  description: '',
  width: '720px',
  submitting: false,
  confirmText: '',
  cancelText: '',
  confirmDisabled: false
})

const emit = defineEmits<{
  'update:modelValue': [boolean]
  confirm: []
  close: []
}>()

function close(): void {
  if (props.submitting) return
  emit('update:modelValue', false)
}
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    :title="title"
    :width="width"
    :close-on-click-modal="!submitting"
    :close-on-press-escape="!submitting"
    :show-close="!submitting"
    align-center
    append-to-body
    class="wb-form-dialog"
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
    @closed="emit('close')"
  >
    <p v-if="description" class="wb-form-dialog__desc">{{ description }}</p>
    <div class="wb-form-dialog__body">
      <slot />
    </div>

    <template #footer>
      <div class="wb-form-actions">
        <slot name="footer-left" />
        <button class="btn" :disabled="submitting" @click="close">{{ cancelText || t('ui.btn.cancel') }}</button>
        <button
          class="btn btn-primary"
          :disabled="submitting || confirmDisabled"
          @click="emit('confirm')"
        >
          {{ confirmText || t('ui.btn.save') }}
        </button>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.wb-form-dialog__desc {
  margin: 0 0 10px;
  font-size: var(--wb-fs-hint, 11px);
  line-height: 1.55;
  color: var(--wb-muted);
}
.wb-form-dialog__body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
</style>
