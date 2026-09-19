import { ElMessageBox } from 'element-plus'
import { t } from '@/i18n'

/**
 * 全局确认/输入弹窗：Promise 化 API，底层由 ElementPlus `ElMessageBox` 渲染。
 *
 * <p>取消与关闭统一降级为 falsy 返回值（confirm→false / prompt→null），调用方无需 try-catch。
 */
export interface DialogApi {
  confirm: (options: {
    title: string
    content?: string
    confirmText?: string
    cancelText?: string
    danger?: boolean
  }) => Promise<boolean>
  prompt: (options: {
    title: string
    content?: string
    placeholder?: string
    defaultValue?: string
    confirmText?: string
    cancelText?: string
  }) => Promise<string | null>
  alert: (options: { title: string; content?: string; confirmText?: string }) => Promise<boolean>
}

const dialogApi: DialogApi = {
  confirm: async (options) => {
    try {
      await ElMessageBox.confirm(options.content ?? '', options.title, {
        type: options.danger ? 'warning' : 'info',
        confirmButtonText: options.confirmText ?? t('ui.btn.confirm'),
        cancelButtonText: options.cancelText ?? t('ui.btn.cancel'),
        confirmButtonClass: options.danger ? 'el-button--danger' : '',
        showClose: false,
        closeOnClickModal: false
      })
      return true
    } catch {
      return false
    }
  },

  prompt: async (options) => {
    try {
      const { value } = await ElMessageBox.prompt(options.content ?? '', options.title, {
        inputValue: options.defaultValue ?? '',
        inputPlaceholder: options.placeholder,
        inputValidator: (v: string) => v.trim().length > 0 || t('ui.msg.inputRequired'),
        confirmButtonText: options.confirmText ?? t('ui.btn.confirm'),
        cancelButtonText: options.cancelText ?? t('ui.btn.cancel'),
        closeOnClickModal: false
      })
      return value ?? null
    } catch {
      return null
    }
  },

  alert: async (options) => {
    try {
      await ElMessageBox.alert(options.content ?? '', options.title, {
        type: 'info',
        confirmButtonText: options.confirmText ?? t('ui.btn.ok'),
        closeOnClickModal: true
      })
      return true
    } catch {
      return false
    }
  }
}

export function useDialog(): DialogApi {
  return dialogApi
}
