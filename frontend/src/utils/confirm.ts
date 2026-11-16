// 确认框的唯一出口：破坏性动作用 danger 确认钮，普通动作用主色确认钮。
// 收成一处是因为 ElMessageBox 的按钮样式要显式传类名——散着写必然有几处漏传，
// 于是「删掉一整个会话」比「保存」看起来还温和。
import { ElMessageBox } from 'element-plus'
import type { ElMessageBoxOptions } from 'element-plus'

/** 破坏性确认钮的类名：样式在 ep-buttons.css 里接 .wb-btn.danger 的语言。 */
export const DANGER_CONFIRM_CLASS = 'is-danger-confirm'

/** 确认一句话；用户取消（或按 Esc / 点叉）返回 false，不抛错。 */
export async function confirmAction(
  body: string,
  title: string,
  okText: string,
  cancelText: string,
  options: { danger?: boolean; type?: 'warning' | 'info' } = {},
): Promise<boolean> {
  try {
    await ElMessageBox.confirm(body, title, {
      confirmButtonText: okText,
      cancelButtonText: cancelText,
      type: options.type ?? 'warning',
      ...(options.danger ? { confirmButtonClass: DANGER_CONFIRM_CLASS } : {}),
    })
    return true
  } catch {
    return false
  }
}

/** 破坏性确认：删 / 丢弃 / 中止这类不可逆动作走它。 */
export function confirmDanger(body: string, title: string, okText: string, cancelText: string): Promise<boolean> {
  return confirmAction(body, title, okText, cancelText, { danger: true })
}

/** 带输入框的确认；取消返回 null。校验失败时留在框里，不返回空串。 */
export async function promptAction(
  body: string,
  title: string,
  options: ElMessageBoxOptions = {},
): Promise<string | null> {
  try {
    const result = await ElMessageBox.prompt(body, title, options)
    return result.value ?? ''
  } catch {
    return null
  }
}
