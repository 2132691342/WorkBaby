// 会话面板折叠状态：标题栏的折叠按钮与对话页共用，选择记进 localStorage。
import { ref, watch } from 'vue'

const KEY = 'wb.session-panel.collapsed'
const collapsed = ref(read())

function read(): boolean {
  try {
    return localStorage.getItem(KEY) === '1'
  } catch {
    return false
  }
}

watch(collapsed, (v) => {
  try {
    localStorage.setItem(KEY, v ? '1' : '0')
  } catch {
    // 隐私模式下写不进去：折叠行为本次会话内仍然生效
  }
})

export function useSessionPanel() {
  return {
    collapsed,
    toggle: () => (collapsed.value = !collapsed.value),
  }
}
