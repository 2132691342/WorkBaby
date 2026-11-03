// 输入区的待发送附件：两条来源（粘贴图片 / 选择文件），共用一张列表——
// 用户心里只有「这次要发什么」，没有「附件分两种」。
// 下发时才分开（kind=image 走 images 字段，kind=file 只拼路径），那是协议的事。
import { computed, ref } from 'vue'
import type { ChatAttachment } from '@/types/api'

/** 一个待发送附件。界面上要显示的是 name，其余字段原样交给后端。 */
export interface PendingAttachment {
  /** 稳定标识：用于 v-for 的 key 与删除 */
  id: string
  /** 展示名（文件名，或粘贴图片的默认名） */
  name: string
  /** 图片的预览地址（data URL）；文件类没有 */
  preview?: string
  /** 下发时用的形态 */
  payload: ChatAttachment
}

/** 单张图片的字节上限，与后端 maxImageBytes 对齐。 */
const MAX_IMAGE_BYTES = 4 << 20
/** 一次最多几张图 / 几个文件，与后端上限对齐。 */
const MAX_IMAGES = 8
const MAX_FILES = 40

/** 按字节头认 MIME：浏览器给的 file.type 在粘贴场景下常常是空的。 */
function sniffMIME(bytes: Uint8Array): string {
  if (bytes.length >= 8 && bytes[0] === 0x89 && bytes[1] === 0x50) return 'image/png'
  if (bytes.length >= 3 && bytes[0] === 0xff && bytes[1] === 0xd8) return 'image/jpeg'
  if (bytes.length >= 4 && bytes[0] === 0x47 && bytes[1] === 0x49) return 'image/gif'
  if (
    bytes.length >= 12 &&
    bytes[0] === 0x52 && bytes[1] === 0x49 && bytes[2] === 0x46 && bytes[3] === 0x46 &&
    bytes[8] === 0x57 && bytes[9] === 0x45 && bytes[10] === 0x42 && bytes[11] === 0x50
  ) {
    return 'image/webp'
  }
  return ''
}

/** Uint8Array → base64。不用 btoa(String.fromCharCode(...bytes))：
 *  展开一大片字节会把调用栈打爆，分块处理。 */
function toBase64(bytes: Uint8Array): string {
  const CHUNK = 0x8000
  let binary = ''
  for (let i = 0; i < bytes.length; i += CHUNK) {
    binary += String.fromCharCode(...bytes.subarray(i, i + CHUNK))
  }
  return btoa(binary)
}

let seq = 0
function nextId(): string {
  seq += 1
  return `att-${Date.now().toString(36)}-${seq}`
}

/**
 * 附件列表。返回的操作都带「会失败」的返回值，由调用方提示用户，
 * 而不是在这里弹 toast —— 这个 composable 不该知道界面的语言。
 */
export function useAttachments() {
  const items = ref<PendingAttachment[]>([])

  const images = computed(() => items.value.filter((a) => a.payload.kind === 'image'))
  const files = computed(() => items.value.filter((a) => a.payload.kind === 'file'))
  const isEmpty = computed(() => items.value.length === 0)

  /** 下发用的纯净载荷（去掉界面的 id / 预览）。 */
  function payloads(): ChatAttachment[] {
    return items.value.map((a) => a.payload)
  }

  function remove(id: string): void {
    items.value = items.value.filter((a) => a.id !== id)
  }

  function clear(): void {
    items.value = []
  }

  /**
   * 加一张图片。返回错误话术（成功返回空串）。
   *
   * 为什么返回字符串而不是抛异常：批量粘贴时一张失败不该中断整批，
   * 调用方要能把「哪张、为什么」逐条汇总给用户。
   */
  async function addImage(file: File): Promise<string> {
    if (images.value.length >= MAX_IMAGES) {
      return `一次最多 ${MAX_IMAGES} 张图片`
    }
    const buffer = new Uint8Array(await file.arrayBuffer())
    if (buffer.length > MAX_IMAGE_BYTES) {
      return `${file.name || '图片'} 超过 4MB`
    }
    const mime = file.type.startsWith('image/') ? file.type : sniffMIME(buffer)
    if (!mime) {
      return `${file.name || '这个文件'} 不是图片`
    }
    const base64 = toBase64(buffer)
    items.value.push({
      id: nextId(),
      name: file.name || `粘贴的图片-${images.value.length + 1}`,
      preview: `data:${mime};base64,${base64}`,
      payload: { kind: 'image', image_base64: base64, image_mime: mime, file_name: file.name },
    })
    return ''
  }

  /** 加一个工作区内的文件引用（相对路径）。 */
  function addWorkspaceFile(relPath: string, name?: string): string {
    if (files.value.length >= MAX_FILES) {
      return `一次最多 ${MAX_FILES} 个文件`
    }
    // 同一个文件重复引用只是噪音：引擎按路径去重，界面上却会多一条。
    if (files.value.some((a) => a.payload.file_path === relPath)) {
      return ''
    }
    items.value.push({
      id: nextId(),
      name: name || relPath.split(/[\\/]/).pop() || relPath,
      payload: { kind: 'file', file_path: relPath, file_name: name },
    })
    return ''
  }

  /** 从粘贴事件里捞图片。返回逐条错误（空数组即全部成功）。 */
  async function addFromClipboard(data: DataTransfer | null): Promise<string[]> {
    if (!data) return []
    const all = Array.from(data.files ?? [])
    if (all.length === 0) return []
    const imageFiles = all.filter((f) => f.type.startsWith('image/'))
    if (imageFiles.length === 0) {
      return ['剪贴板里没有图片；其他文件请用回形针选择']
    }
    const errors: string[] = []
    for (const file of imageFiles) {
      const err = await addImage(file)
      if (err) errors.push(err)
    }
    return errors
  }

  return { items, images, files, isEmpty, payloads, remove, clear, addImage, addWorkspaceFile, addFromClipboard }
}

/** 取路径最后一段作为展示名。 */
export function baseName(path: string): string {
  const parts = path.split(/[\\/]/)
  return parts[parts.length - 1] || path
}

/**
 * 绝对路径 → 工作区相对路径。不在工作区内返回 null。
 *
 * 逐段比大小写：Windows 上盘符可能是 D:\ 也可能是 d:\，直接用 startsWith
 * 会判错，而判错的后果是附件路径变成一整条绝对路径——引擎仍能读，
 * 但界面上显示的是一串很长的本机路径。
 */
export function toWorkspaceRelative(fullPath: string, workspace: string): string | null {
  const root = workspace.replace(/[\\/]+$/, '')
  if (!root) return null
  const norm = (p: string) => p.replace(/\//g, '\\').toLowerCase()
  if (!norm(fullPath).startsWith(norm(root) + '\\')) return null
  return fullPath.slice(root.length + 1).replace(/\\/g, '/')
}
