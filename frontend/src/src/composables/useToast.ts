import { ElMessage, ElNotification } from 'element-plus'

/**
 * 全局轻提示：success / warning / info 走 `ElMessage`，error 走 `ElNotification`（更显眼、可手动关闭）。
 */
export type ToastVariant = 'success' | 'error' | 'warning' | 'info'

export interface ToastApi {
  success: (title: string, description?: string, duration?: number) => void
  error: (title: string, description?: string, duration?: number) => void
  warning: (title: string, description?: string, duration?: number) => void
  info: (title: string, description?: string, duration?: number) => void
}

const DEFAULT_DURATION = 3000

function text(title: string, description?: string): string {
  return description ? `${title}：${description}` : title
}

export const toast: ToastApi = {
  success: (title, description, duration = DEFAULT_DURATION) =>
    ElMessage({ type: 'success', message: text(title, description), duration, grouping: true }),

  warning: (title, description, duration = DEFAULT_DURATION) =>
    ElMessage({
      type: 'warning',
      message: text(title, description),
      duration,
      grouping: true,
      showClose: true
    }),

  info: (title, description, duration = DEFAULT_DURATION) =>
    ElMessage({ type: 'info', message: text(title, description), duration, grouping: true }),

  error: (title, description, duration = 4500) =>
    ElNotification({
      type: 'error',
      title,
      message: description ?? '',
      duration,
      showClose: true,
      position: 'top-right'
    })
}

export function useToast(): ToastApi {
  return toast
}
