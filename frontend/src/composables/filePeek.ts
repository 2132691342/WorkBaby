// 就地看文件改动：工具卡点「看改动」就在当前对话上打开一个面板，
// 不必切到工作区再把消息列表的滚动位置找回来。
import { ref } from 'vue'

const path = ref('')

/** 打开某个文件的改动面板（路径相对工作区） */
export function openFilePeek(target: string) {
  path.value = target
}

/** 关掉面板 */
export function closeFilePeek() {
  path.value = ''
}

export function useFilePeek() {
  return { path, open: openFilePeek, close: closeFilePeek }
}
