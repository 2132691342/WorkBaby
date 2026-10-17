// 桌面壳意图（托盘「新建对话」、外部打开文件）：先存下再广播，视图挂载时自行消费。
// 只发事件不存状态的话，意图会因为目标视图还没挂载而丢掉。
import { ref } from 'vue'

// 待打开的文件绝对路径；空串 = 没有
export const intentFile = ref('')
// 是否要新建一段对话
export const intentNewSession = ref(false)

// 广播名：已挂载的视图收到后立即消费
export const SHELL_INTENT_EVENT = 'wb:shell-intent'

// 通知已挂载的视图：有意图待消费
export function pushShellIntent() {
  window.dispatchEvent(new CustomEvent(SHELL_INTENT_EVENT))
}
