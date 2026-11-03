// 输入区草稿：按会话分别保存，切走再切回来还在——全局草稿要么冲掉
// 用户刚写的、要么把 A 会话的半句带进 B 会话，两个都不可接受。
// 存 localStorage：草稿是前端临时状态，不该混进设置文件。
import { watch } from 'vue'
import type { Ref } from 'vue'

/** 当前会话的草稿键：没有会话（新建态）时用一个固定键。 */
function keyOf(sessionFile: string): string {
  return sessionFile || '__new__'
}

/** 最多保留多少条草稿。超出时丢最旧的——草稿是便利，不是账本。 */
const MAX_DRAFTS = 20
const STORAGE_KEY = 'omp.composer.drafts'

function readAll(): Record<string, string> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return {}
    // 窄化后取字段：localStorage 里的东西可以被用户手改，不能直接当形状用。
    const out: Record<string, string> = {}
    for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
      if (typeof v === 'string') out[k] = v
    }
    return out
  } catch {
    return {}
  }
}

function writeAll(all: Record<string, string>): void {
  try {
    const keys = Object.keys(all)
    // 超出上限时按插入顺序淘汰：JS 对象保持字符串键的插入顺序，
    // 而现在每次保存都会重新写入当前键，等于把最久没碰的排在最前面。
    while (keys.length > MAX_DRAFTS) {
      const oldest = keys.shift()
      if (oldest === undefined) break
      delete all[oldest]
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(all))
  } catch {
    // 配额满 / 隐私模式：草稿存不住不该影响发送。
  }
}

/**
 * 把一个草稿 ref 绑定到会话。
 *
 * sessionFile 变化时先存旧的、再载入新的。两个动作必须成对：
 * 只存不载会让用户看到上一个会话的内容，只载不存会丢东西。
 */
export function useComposerDraft(
  sessionFile: Ref<string>,
  text: Ref<string>,
): { flush: () => void } {
  let boundTo = keyOf(sessionFile.value)

  function save(key: string): void {
    const all = readAll()
    const value = text.value
    if (value.trim()) {
      // 先删再写：让当前键排到末尾，淘汰时最后才轮到它。
      delete all[key]
      all[key] = value
    } else {
      delete all[key]
    }
    writeAll(all)
  }

  function load(key: string): void {
    text.value = readAll()[key] ?? ''
  }

  watch(sessionFile, (next) => {
    save(boundTo)
    boundTo = keyOf(next)
    load(boundTo)
  })

  // 输入过程中的落盘交给 watch(text)：用户可能直接关窗口，没有「提交」时机。
  // 写 localStorage 是同步的，每个按键一次在 800×600 的输入区里测不出卡顿。
  watch(text, () => save(boundTo))

  // 首次挂载时载入当前会话的草稿。
  load(boundTo)

  return {
    // flush 在发送成功后调用：把「已发出」这件事落盘，避免下次回来还看到它。
    flush: () => save(boundTo),
  }
}
