// 输入区草稿：按会话 id 存 localStorage，无会话（新建态）用 __new__ 作键。
// 抽出来是为了让 ChatInput 只管界面；草稿的读写与容量兜底与渲染无关。
import { ref, watch } from 'vue'

const DRAFT_KEY = 'wb-composer-drafts'
/** 草稿上限：超过就丢最旧的一条，避免 localStorage 被无限撑大。 */
const MAX_DRAFTS = 20

function load(): Record<string, string> {
  try {
    return JSON.parse(localStorage.getItem(DRAFT_KEY) || '{}') as Record<string, string>
  } catch {
    // 存储被清空或写坏：当作没有草稿，不挡输入
    return {}
  }
}

export function useComposerDraft(sessionId: () => string | null) {
  const drafts = load()
  const text = ref('')
  /** 当前 text 归属哪个会话：切回来时据此判断要不要重新载入 */
  let loadedFor = ''

  function key(): string {
    return sessionId() || '__new__'
  }

  function save() {
    const k = key()
    if (text.value.trim()) drafts[k] = text.value
    else delete drafts[k]
    loadedFor = k
    try {
      const keys = Object.keys(drafts)
      if (keys.length > MAX_DRAFTS) delete drafts[keys[0]]
      localStorage.setItem(DRAFT_KEY, JSON.stringify(drafts))
    } catch {
      // 配额用尽 / 隐私模式：草稿是便利功能，丢了不该报错打断输入
    }
  }

  function loadCurrent() {
    const k = key()
    if (loadedFor === k) return
    loadedFor = k
    text.value = drafts[k] || ''
  }

  watch(sessionId, loadCurrent)

  return { text, save, loadCurrent }
}
