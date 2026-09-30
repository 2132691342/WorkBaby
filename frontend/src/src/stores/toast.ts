// 全局操作反馈：每个写操作成功与否都必须让用户看见。
// 「点了没反应」是最伤信任的交互缺陷——成功一句、失败一句、三秒半消失。
import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface Toast {
  id: number
  text: string
  kind: 'ok' | 'bad' | 'info'
}

export const useToastStore = defineStore('toast', () => {
  const items = ref<Toast[]>([])
  let seq = 0

  function push(text: string, kind: Toast['kind'] = 'info') {
    const t: Toast = { id: ++seq, text, kind }
    items.value.push(t)
    // 最多同屏 4 条，旧的先走
    if (items.value.length > 4) items.value.shift()
    setTimeout(() => dismiss(t.id), 3500)
  }

  function dismiss(id: number) {
    items.value = items.value.filter((t) => t.id !== id)
  }

  const ok = (text: string) => push(text, 'ok')
  const bad = (text: string) => push(text, 'bad')
  const info = (text: string) => push(text, 'info')

  return { items, push, dismiss, ok, bad, info }
})
