// 输入区的待发送内容：技能芯片 + 图片列表，与正文分离。
// 抽出来是因为三处都要用它（输入区、渲染、发送），而它自己有一套解码与去重规则。

import { computed, ref } from 'vue'
import type { PendingImage } from '../utils/composerMenu'

/** 单条消息的图片上限：再多上游也吃不下，且会顶爆上下文。 */
const MAX_IMAGES = 8

/** 单图字节上限（base64 前）：超过就不收，直说原因比发出去报错好。 */
const MAX_BYTES = 5 * 1024 * 1024

/** 一条消息最多挂一个技能：技能正文会整体进上下文，多个会互相打架。 */
export function useComposerPayload() {
  const skill = ref('')
  const images = ref<PendingImage[]>([])
  let seq = 0

  const empty = computed(() => !skill.value && images.value.length === 0)

  function setSkill(name: string) {
    skill.value = name
  }

  function clearSkill() {
    skill.value = ''
  }

  /** 添加一张图片；返回错误文案，成功返回空串。 */
  function addImage(name: string, mime: string, base64: string): string {
    if (images.value.length >= MAX_IMAGES) return `最多 ${MAX_IMAGES} 张图`
    const bytes = Math.ceil((base64.length * 3) / 4)
    if (bytes > MAX_BYTES) return `${name} 超过 5MB`
    if (images.value.some((i) => i.base64 === base64)) return ''
    const item: PendingImage = {
      id: ++seq,
      name,
      mime,
      base64,
      width: 0,
      height: 0,
    }
    // 先把尺寸读出来再入列：宽高决定缩略图占位，读不到就让 CSS 兜住
    const img = new Image()
    img.onload = () => {
      item.width = img.naturalWidth
      item.height = img.naturalHeight
    }
    img.src = `data:${mime};base64,${base64}`
    images.value = [...images.value, item]
    return ''
  }

  function removeImage(id: number) {
    images.value = images.value.filter((i) => i.id !== id)
  }

  /** 发送用的载荷形态（与 /chat/send 的 chat_images 对齐）。 */
  function imagePayload(): { mime: string; base64: string }[] {
    return images.value.map((i) => ({ mime: i.mime, base64: i.base64 }))
  }

  function clear() {
    skill.value = ''
    images.value = []
  }

  return { skill, images, empty, setSkill, clearSkill, addImage, removeImage, imagePayload, clear }
}

/** FileReader 的 promise 包装：粘贴与选择两条路径共用同一段解码。 */
export function readFileBase64(file: File): Promise<{ name: string; mime: string; base64: string }> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(new Error(`读取 ${file.name} 失败`))
    reader.onload = () => {
      const dataUrl = String(reader.result || '')
      const comma = dataUrl.indexOf(',')
      if (comma < 0) {
        reject(new Error(`读取 ${file.name} 失败`))
        return
      }
      resolve({
        name: file.name || `paste-${Date.now()}.png`,
        mime: file.type || 'image/png',
        base64: dataUrl.slice(comma + 1),
      })
    }
    reader.readAsDataURL(file)
  })
}
