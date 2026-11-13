// 命令面板的开关状态。模块级单例：全应用只有一块面板，
// 由外壳统一挂载，各处（标题栏按钮、快捷键）都开同一块。
import { ref } from 'vue'

const open = ref(false)

/** 命令面板：全局一处开关，Ctrl+K 与标题栏按钮都走它。 */
export function useCommandPalette() {
  /** 打开面板并聚焦搜索框。 */
  function show(): void {
    open.value = true
  }

  /** 关闭面板。 */
  function hide(): void {
    open.value = false
  }

  /** 开关切换（快捷键用）。 */
  function toggle(): void {
    open.value = !open.value
  }

  return { open, show, hide, toggle }
}
