import { ref, watch } from 'vue'

/**
 * 焦点模式：单例 ref，跨组件共享。
 *
 * <p>开启后 MessageList 隐藏思考面板与工具时间线，仅保留用户/助手正文与最终交付。
 *
 * <p>这是「用户偏好」而不是「临时视图」：偏好写 localStorage，重启与切会话都保持。
 * 之前只活在内存里，刷新后静默重置，用户会以为按钮失灵。
 */
const PREF_KEY = 'wb.focusMode'

function readPref(): boolean {
  try {
    return localStorage.getItem(PREF_KEY) === 'true'
  } catch {
    return false
  }
}

const enabled = ref(readPref())

watch(enabled, (v) => {
  try {
    localStorage.setItem(PREF_KEY, String(v))
  } catch {
    // 隐私模式下不可写，仅本次会话生效
  }
})

export function useFocusMode() {
  return {
    enabled,
    toggle(): void { enabled.value = !enabled.value },
    set(v: boolean): void { enabled.value = v }
  }
}
