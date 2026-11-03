// 文件预览抽屉的状态。模块级单例而不是 store：只有一个使用场景（工具卡），
// 放进 store 只是给全局状态加一个没人用的字段。
// 用抽屉而不是就地展开：抽屉不撑开消息列排版，也不打断滚动位置。
import { ref } from 'vue'

const path = ref('')
const title = ref('')

/** 打开抽屉。title 是给抽屉头显示的名字（通常是文件名）。 */
export function openFilePeek(target: string, label = ''): void {
  path.value = target
  title.value = label
}

/** 关闭抽屉。 */
export function closeFilePeek(): void {
  path.value = ''
  title.value = ''
}

/** 抽屉状态。path 为空表示没打开。 */
export function useFilePeek() {
  return { path, title, open: openFilePeek, close: closeFilePeek }
}
