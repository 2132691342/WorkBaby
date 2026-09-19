<script setup lang="ts">
/**
 * 聊天输入框（composer）：顶部工作区/权限 chip、中间输入区（附件、slash 与 mention 浮层）、
 * 底部状态行。保留原生 textarea——IME composition、自增高与光标解析依赖原生事件。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import {
  Check,
  Paperclip,
  Square,
  Upload,
  Send,
  Plus,
  Shield,
  Zap,
  Sparkles,
  ChevronDown
} from '@/components/common/icons'
import { apiGet, apiPost } from '@/api/client'
import { UploadFile, OpenFileDialog } from '@/wailsjs/go/main/App'
import { useToast } from '@/composables/useToast'
import { useDialog } from '@/composables/useDialog'
import { t } from '@/i18n'
import { useChatStore } from '@/stores/chat'
import { useTasksStore } from '@/stores/tasks'
import { ElMessageBox } from 'element-plus'
import type { AvailableModel, CircuitState, EffectiveParams, FileInfo, Message, QueuedMessage, Skill, WorkspaceFile } from '@/types/api'
import ModelSelector from '@/components/chat/ModelSelector.vue'
import AttachmentStrip from '@/components/chat/composer/AttachmentStrip.vue'
import SlashCommandPalette, { type SlashCommand } from '@/components/chat/composer/SlashCommandPalette.vue'
import MentionPicker, { type MentionItem } from '@/components/chat/composer/MentionPicker.vue'
import SkillPicker from '@/components/chat/composer/SkillPicker.vue'
import MidTurnQueue from '@/components/chat/composer/MidTurnQueue.vue'
import ContextUsagePopover from '@/components/chat/composer/ContextUsagePopover.vue'
import ParamsPopover, { type ComposerParams } from '@/components/chat/composer/ParamsPopover.vue'
import { useSkillsStore } from '@/stores/skills'
import { useFoldersStore } from '@/stores/folders'
import { useFilesStore } from '@/stores/files'
import { splitTokens } from '@/chat/models/tokens'
import type { DraftToken } from '@/chat/models/tokens'
import { expandSlashDraft } from '@/chat/commandArgs'

type PermissionLevel = 'restricted' | 'confirm' | 'auto' | 'full'

const props = defineProps<{
  streaming: boolean
  disabled: boolean
  models?: AvailableModel[]
  selectedModelID?: string | null
  /** 上下文窗口使用估值（0~max），占位版用 0/128k。 */
  ctxUsed?: number
  ctxMax?: number
  /** 熔断状态 Map（id → CircuitState）。 */
  circuitStates?: Map<string, CircuitState>
  /** 当前会话绑定的工作区绝对路径（null = 默认会话工作区）。 */
  workspacePath?: string | null
  /** 当前会话生效参数快照（后端三层合并；null = 未加载）。 */
  effectiveParams?: EffectiveParams | null
  /** 当前权限级别（默认 auto）。 */
  permission?: PermissionLevel
}>()

const emit = defineEmits<{
  send: [message: string, fileIds: string[], params: ComposerParams]
  'select-model': [id: string | null]
  stop: []
  enqueue: [message: QueuedMessage]
  'remove-queue': [id: string]
  'send-queued': [message: string]
  regenerate: []
  /** 用户点击上下文占用 popover 内的「压缩历史」按钮。 */
  compact: []
  /** /side：请求打开右侧辅助对话面板（命令不作为消息发送）。 */
  side: []
  /** 工作区切换（弹 WorkspacePickerDialog）。 */
  'pick-workspace': []
  /** 权限级别切换。 */
  'change-permission': [level: PermissionLevel]
}>()

const toast = useToast()
const dialog = useDialog()
const router = useRouter()

/** 输入上限：canSend 在超限时返回 false，真正拦截发送（计数同时标红提示）。
 *  默认 32000 与后端 `domain.DefaultChatMaxInputChars` 对齐；启动时异步拉
 *  `chat.maxInputChars` 覆盖（用户在 Settings → Advanced 改 system_settings 后下次启动生效）。
 *  前端 + 后端都用同源默认值，避免前后端不一致导致「前端能发、后端拒收」。 */
const DEFAULT_MAX_INPUT_CHARS = 32000
const maxInputChars = ref(DEFAULT_MAX_INPUT_CHARS)
const draft = ref('')
const attachments = ref<FileInfo[]>([])
const uploading = ref(false)
const textareaRef = ref<HTMLTextAreaElement | null>(null)

const isComposing = ref(false)

/** 根容器：浮层的「点击外部关闭」判定边界。 */
const composerRef = ref<HTMLElement | null>(null)

const slashOpen = ref(false)
const slashQuery = ref('')
const slashPaletteRef = ref<InstanceType<typeof SlashCommandPalette> | null>(null)
/** 触发字符 `/` 在草稿中的下标（替换时按这个位置截断，而非 lastIndexOf）。 */
const slashStart = ref(-1)
const mentionOpen = ref(false)
const mentionQuery = ref('')
const mentionActiveIndex = ref(-1)
/** 触发字符 `@` 在草稿中的下标（同上）。 */
const mentionStart = ref(-1)
/** 工具行「技能」按钮唤起的技能选择器。 */
const skillPickerOpen = ref(false)

const modelSelectorRef = ref<InstanceType<typeof ModelSelector> | null>(null)
/** 采样参数弹层实例：send 时读取当前 temperature / thinking_effort。 */
const paramsRef = ref<InstanceType<typeof ParamsPopover> | null>(null)

function currentParams(): ComposerParams {
  return paramsRef.value?.current() ?? { temperature: null, thinking_effort: 'medium' }
}

const skillsStore = useSkillsStore()
const foldersStore = useFoldersStore()
const filesStore = useFilesStore()
const chat = useChatStore()
const tasksStore = useTasksStore()
const { contextUsage } = storeToRefs(chat)

/** 上下文分段明细：popover 渲染用，store 未加载时为空数组。 */
const contextSegments = computed(() => contextUsage.value?.segments ?? [])
const contextEstimated = computed(() => contextUsage.value?.estimated ?? false)

/** 艾特候选：按当前工作区真实文件树加载文件夹/文件（满足"按工作区显示"诉求）。 */
const workspaceFiles = ref<WorkspaceFile[]>([])
const workspaceFilesLoadedFor = ref<string | null>(null)
async function loadWorkspaceFiles(sessionID: string): Promise<void> {
  if (workspaceFilesLoadedFor.value === sessionID && workspaceFiles.value.length > 0) return
  workspaceFilesLoadedFor.value = sessionID
  try {
    workspaceFiles.value = await apiGet<WorkspaceFile[]>(`/api/v1/chat/workspace/${sessionID}/files`)
  } catch {
    workspaceFiles.value = []
  }
}

const mentionItems = computed<MentionItem[]>(() => {
  const q = mentionQuery.value.trim().toLowerCase()
  const items: MentionItem[] = []
  for (const s of skillsStore.skills) {
    if (s.enabled === false) continue
    if (q && !s.name.toLowerCase().includes(q)) continue
    items.push({
      type: 'skill',
      id: s.id,
      name: s.name,
      insertText: `@${s.name}`,
      description: s.description ?? undefined
    })
  }
  // 文件夹与文件优先以工作区为根（用户期望「艾特文件列表按工作区显示」）；
  // 工作区未加载或为空时回落到 foldersStore.tree + filesStore.files（旧逻辑兜底）。
  if (workspaceFiles.value.length > 0) {
    for (const f of workspaceFiles.value) {
      const name = f.name
      if (q && !name.toLowerCase().includes(q)) continue
      // WorkspaceFile 没显式 kind='dir'，按 ext + size 推断：目录通常无扩展名且 0 字节
      const isDir = !f.ext && f.size === 0
      if (isDir) {
        items.push({ type: 'folder', id: f.path, name, insertText: `@${name}`, path: f.path })
      } else {
        items.push({
          type: 'file',
          id: f.path,
          name,
          insertText: `@${name}`,
          description: `${(f.size / 1024).toFixed(1)} KB`
        })
      }
    }
  } else {
    const flat = (nodes: { id: string; name: string; path?: string; children?: unknown[] }[]): void => {
      for (const n of nodes) {
        if (q && !n.name.toLowerCase().includes(q)) continue
        items.push({ type: 'folder', id: n.id, name: n.name, insertText: `@${n.name}`, path: n.path })
        if (Array.isArray(n.children) && n.children.length > 0) {
          flat(n.children as { id: string; name: string; path?: string; children?: unknown[] }[])
        }
      }
    }
    flat(foldersStore.tree as unknown as { id: string; name: string; path?: string; children?: unknown[] }[])
    for (const f of filesStore.files) {
      const name = f.original_name || f.name
      if (q && !name.toLowerCase().includes(q)) continue
      items.push({
        type: 'file',
        id: f.id,
        name,
        insertText: `@${name}`,
        description: `${(f.size / 1024).toFixed(1)} KB`
      })
    }
  }
  return items.slice(0, 24)
})

watch(mentionOpen, (open) => {
  if (!open) return
  if (skillsStore.skills.length === 0) void skillsStore.load().catch(() => undefined)
  const sid = chat.currentID
  if (sid) void loadWorkspaceFiles(sid).catch(() => undefined)
  // 未绑定工作区时回落到受管文件（保证基础可用）
  if (workspaceFiles.value.length === 0 && filesStore.files.length === 0) {
    void filesStore.load().catch(() => undefined)
  }
})

const queue = ref<QueuedMessage[]>([])
const queuePaused = ref(false)
const isDragging = ref(false)
const dragCounter = ref(0)

const canSend = computed(() => {
  // 超 maxInputChars 直接禁止发送（计数标红提示），而非仅变色
  if (draft.value.length > maxInputChars.value) return false
  const hasText = draft.value.trim().length > 0
  const hasAttach = attachments.value.length > 0
  return (hasText || hasAttach) && !isComposing.value
})



const permissionLevel = computed<PermissionLevel>(() => props.permission ?? 'auto')

/** 权限档位：值/标签/说明与风险色（下拉菜单消费；full 需要用户看清后果）。 */
const PERMISSION_ITEMS: Array<{ value: PermissionLevel; labelKey: string; descKey: string; danger?: boolean }> = [
  { value: 'restricted', labelKey: 'chat.perm.restricted', descKey: 'chat.perm.desc.restricted' },
  { value: 'confirm', labelKey: 'chat.perm.confirm', descKey: 'chat.perm.desc.confirm' },
  { value: 'auto', labelKey: 'chat.perm.auto', descKey: 'chat.perm.desc.auto' },
  { value: 'full', labelKey: 'chat.perm.full', descKey: 'chat.perm.desc.full', danger: true }
]

/** /trust 别名 → PermissionLevel（接受后端 SessionMode 键、前端显示标签与中文）。 */
const TRUST_ALIASES: Record<string, PermissionLevel> = {
  restricted: 'restricted',
  default: 'confirm',
  auto_edit: 'auto',
  'auto-edit': 'auto',
  yolo: 'full',
  plan: 'restricted',
  confirm: 'confirm',
  auto: 'auto',
  full: 'full',
  计划: 'restricted',
  受限: 'restricted',
  需确认: 'confirm',
  自动: 'auto',
  完全访问: 'full',
}

const permissionLabel = computed(
  () => PERMISSION_ITEMS.find((p) => p.value === permissionLevel.value)?.labelKey ?? 'chat.perm.confirm'
)

const permissionDesc = computed(
  () => PERMISSION_ITEMS.find((p) => p.value === permissionLevel.value)?.descKey ?? 'chat.perm.desc.confirm'
)

/** 完全访问/受限为非常态，chip 高亮提示；完全访问额外用危险色。 */
const permissionDanger = computed(() => permissionLevel.value === 'full')

// ===== @skill 提及可视化：从草稿解析已激活的技能，渲染成可辨识的 chip（问题：看不出是个 skill）=====
const activeSkillMentions = computed(() => {
  const tokens = draft.value.match(/@[\w\u4e00-\u9fa5.-]+/g) ?? []
  const names = new Set(tokens.map((tk) => tk.slice(1)))
  return [...names]
    .map((name) => skillsStore.skills.find((s) => s.name === name))
    .filter((s): s is NonNullable<typeof s> => Boolean(s))
})

// ===== 输入框镜像高亮：textarea 文字透明 + 背后镜像层渲染淡色气泡 =====
//
// 为什么用镜像而不是富文本输入：中文 IME、自增高、`@`/`/` 光标解析都依赖原生 textarea，
// 换成 contenteditable 会一次性丢掉这三样。镜像层只做视觉，不接管输入。
const mirrorRef = ref<HTMLElement | null>(null)

/** `@名称` → 技能 / 文件命中判定（未命中由镜像层渲染成灰气泡提示）。 */
function resolveMention(name: string): 'skill' | 'file' | null {
  if (skillsStore.skills.some((s) => s.name === name && s.enabled !== false)) return 'skill'
  const inFiles = filesStore.files.some((f) => (f.original_name || f.name) === name)
  if (inFiles) return 'file'
  const inFolders = (foldersStore.tree as unknown as { name: string; children?: unknown[] }[]).some(
    (n) => n.name === name
  )
  return inFolders ? 'file' : null
}

const mirrorTokens = computed<DraftToken[]>(() => splitTokens(draft.value, resolveMention))

/** IME 组合期间隐藏镜像并恢复文字颜色：候选窗口前的组合串必须可见。 */
const mirrorVisible = computed(() => !isComposing.value)

/** 滚动同步：textarea 自增高后可滚动，镜像必须同频否则高亮错位。 */
function syncMirrorScroll(): void {
  const ta = textareaRef.value
  if (mirrorRef.value && ta) mirrorRef.value.scrollTop = ta.scrollTop
}

/** 移除一条 @skill 提及（连同尾部空白），草稿其余内容不动。 */
function removeSkillMention(name: string): void {
  const escaped = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  draft.value = draft.value.replace(new RegExp(`\\s*@${escaped}`, 'g'), '').trimStart()
}

// ===== 文件上传（Wails 对话框选路径 → UploadFile 绑定） =====
function pickFile(): void {
  if (props.disabled || props.streaming) return
  void uploadFile()
}

/**
 * 技能 mini：直接弹出技能选择器（不依赖输入框里有没有 `@`）。
 * 选中的技能以 `@名称` 插入光标处，仍走后端 Skill 匹配的同一条路径。
 */
function openSkills(): void {
  if (props.disabled || props.streaming) return
  skillPickerOpen.value = !skillPickerOpen.value
  if (skillPickerOpen.value) {
    slashOpen.value = false
    mentionOpen.value = false
  }
}

/** 在光标处插入文本（替换已有选区），插入后光标停在文本末尾。 */
function insertAtCursor(text: string): void {
  const ta = textareaRef.value
  const start = ta?.selectionStart ?? draft.value.length
  const end = ta?.selectionEnd ?? start
  draft.value = draft.value.slice(0, start) + text + draft.value.slice(end)
  void nextTick(() => {
    ta?.focus()
    const pos = start + text.length
    ta?.setSelectionRange(pos, pos)
    onInput()
    autoResize()
  })
}

/** 技能选择器选中：插入 `@技能名 `（末尾空格方便直接继续写指令）。 */
function pickSkill(skill: Skill): void {
  skillPickerOpen.value = false
  insertAtCursor(`@${skill.name} `)
}

/** 点击浮层外部 / 输入框失焦时收起三个浮层，避免遮住消息流。 */
function onDocumentPointerDown(e: PointerEvent): void {
  if (!skillPickerOpen.value && !slashOpen.value && !mentionOpen.value) return
  const root = composerRef.value
  if (root && e.target instanceof Node && root.contains(e.target)) return
  skillPickerOpen.value = false
  slashOpen.value = false
  mentionOpen.value = false
}

/** 读 File 为裸 base64（去掉 data URI 前缀）。 */
function readAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const fr = new FileReader()
    fr.onload = () => {
      const raw = String(fr.result ?? '')
      const i = raw.indexOf(',')
      resolve(i >= 0 ? raw.slice(i + 1) : raw)
    }
    fr.onerror = () => reject(fr.error ?? new Error('read file failed'))
    fr.readAsDataURL(file)
  })
}

/** 单附件大小上限（base64 会膨胀 1/3，留足余量避免大图拖垮 HTTP 请求）。 */
const MAX_ATTACH_BYTES = 10 * 1024 * 1024

/**
 * 加附件：传入 File 走内容上传（粘贴 / 拖拽，剪贴板图片没有本地路径）；
 * 不传则弹系统文件对话框选本地路径。
 */
async function uploadFile(file?: File): Promise<void> {
  uploading.value = true
  try {
    if (file) {
      if (file.size > MAX_ATTACH_BYTES) {
        toast.error(t('chat.uploadFailed'), t('chat.attachTooLarge'))
        return
      }
      const dataBase64 = await readAsBase64(file)
      const info = await apiPost<FileInfo>('/api/v1/files/upload-data', {
        name: file.name || `clipboard-${Date.now()}.png`,
        data_base64: dataBase64
      })
      attachments.value.push(info)
      return
    }
    const selected = await OpenFileDialog(t('chat.attachment'), '*.*')
    if (!selected) return
    const info = await UploadFile('', selected, '', '')
    attachments.value.push(info)
  } catch (err) {
    toast.error(t('chat.uploadFailed'), err instanceof Error ? err.message : String(err))
  } finally {
    uploading.value = false
  }
}

function removeAttachment(id: string): void {
  attachments.value = attachments.value.filter((a) => a.id !== id)
}

function selectModel(id: string | null): void {
  emit('select-model', id)
}

function submit(): void {
  if (!canSend.value) return
  const text = draft.value.trim()
  if (!text && attachments.value.length === 0) return
  // /goal 目标命令在前端拦截执行（不走消息链路）；其余斜杠消息原样发送
  const goalMatch = /^\/goal(?:\s+([\s\S]+))?$/.exec(text)
  if (goalMatch) {
    draft.value = ''
    resetHeight()
    void handleGoalCommand(goalMatch[1])
    return
  }
  // /side 辅助对话：打开右栏面板，命令不作为消息发送
  if (/^\/(?:side|btw)\s*$/.test(text)) {
    draft.value = ''
    resetHeight()
    emit('side')
    return
  }
  // /trust 执行模式：无参循环；带参按别名直切（plan/confirm/auto/full 或内部键）
  const trustMatch = /^\/trust(?:\s+([\s\S]+))?$/.exec(text)
  if (trustMatch) {
    draft.value = ''
    resetHeight()
    handleTrustCommand(trustMatch[1])
    return
  }
  if (slashOpen.value && /^\/\w*\s*$/.test(draft.value)) {
    return
  }
  // 自定义命令：`/命令名 参数…` 按模板占位符展开后再发送（内置命令与普通文本原样通过）
  const payload = expandSlashDraft(text, chat.commands)
  if (props.streaming) {
    // 流式中发送 = 立即插入（steer）：store 的 sendMessage 检测到流式会走 /steer 注入缝；
    // 「入队」走独立的 Plus 按钮与队列 flush。输入保持可写，插话/排队两不误。
    emit('send', payload, [], currentParams())
    draft.value = ''
    slashOpen.value = false
    slashQuery.value = ''
    mentionOpen.value = false
    resetHeight()
    void nextTick(() => textareaRef.value?.focus())
    return
  }
  emit('send', payload, attachments.value.map((a) => a.id), currentParams())
  draft.value = ''
  attachments.value = []
  slashOpen.value = false
  slashQuery.value = ''
  mentionOpen.value = false
  resetHeight()
  void nextTick(() => textareaRef.value?.focus())
}

function onKeydown(e: KeyboardEvent): void {
  // 技能选择器打开时，导航键归它自己处理（内部搜索框已聚焦）
  if (skillPickerOpen.value && ['Enter', 'ArrowDown', 'ArrowUp', 'Escape'].includes(e.key)) return
  // Ctrl/Cmd+Enter = 立即插入：流式中跳过队列直接 steer（submit 内部分流），
  // 与 Plus 的「排队」构成快慢两档，键盘也能直达而不必点按钮。
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !isComposing.value) {
    e.preventDefault()
    submit()
    return
  }
  if (e.key === 'Enter' && !e.shiftKey && !isComposing.value) {
    e.preventDefault()
    if (slashOpen.value) {
      slashPaletteRef.value?.pickAt(0)
      return
    }
    if (mentionOpen.value && mentionItems.value.length > 0 && mentionActiveIndex.value >= 0) {
      pickMention(mentionItems.value[mentionActiveIndex.value])
      return
    }
    submit()
    return
  }
  if (slashOpen.value) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      slashPaletteRef.value?.moveActive(1)
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      slashPaletteRef.value?.moveActive(-1)
      return
    }
    if (e.key === 'Escape') {
      e.preventDefault()
      slashOpen.value = false
      slashQuery.value = ''
      return
    }
  }
  if (mentionOpen.value && mentionItems.value.length > 0) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      mentionActiveIndex.value = (mentionActiveIndex.value + 1) % mentionItems.value.length
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      mentionActiveIndex.value = mentionActiveIndex.value <= 0
        ? mentionItems.value.length - 1
        : mentionActiveIndex.value - 1
      return
    }
  }
  if (mentionOpen.value && e.key === 'Escape') {
    mentionOpen.value = false
  }
}

function pickMention(item: MentionItem): void {
  // 触发点优先用解析时记下的下标；拿不到（如外部调用）才退化到最后一个 @。
  const idx = mentionStart.value >= 0 ? mentionStart.value : draft.value.lastIndexOf('@')
  if (idx < 0) {
    mentionOpen.value = false
    return
  }
  const text = draft.value
  const before = text.slice(0, idx)
  const caret = textareaRef.value?.selectionStart ?? text.length
  const baseAfter = caret > idx ? text.slice(caret) : ''
  if (item.type === 'file') {
    // 来源区分：受管文件（filesStore.files）继续走"加附件 + 插入 mention"双路径；
    // 工作区文件（workspaceFiles）只插入 @文件名，让后端模型以引用理解，不再塞进附件
    // 避免"未受管路径被当成受管附件"导致 file_read 取不到内容。
    const fromWorkspace = workspaceFiles.value.some((f) => f.path === item.id)
    if (!fromWorkspace) {
      if (!attachments.value.some((a) => a.id === item.id)) {
        attachments.value.push({
          id: item.id,
          name: item.name,
          original_name: item.name,
          file_type: '',
          mime_type: '',
          size: 0,
          status: '',
          session_id: null,
          folder_id: null,
          created_at: 0
        } as FileInfo)
      }
      draft.value = before + baseAfter + item.insertText + ' '
      toast.success(t('mention.fileAttached', item.name))
    } else {
      draft.value = before + baseAfter + item.insertText + ' '
      toast.info(t('mention.fileReferenced', item.name))
    }
  } else {
    // 技能/文件夹/知识库：直接落 `@名称` 令牌——输入框干净，后端 Skill 匹配与模型理解都看得到名称
    draft.value = before + baseAfter + item.insertText + ' '
  }
  mentionOpen.value = false
  mentionQuery.value = ''
  mentionActiveIndex.value = -1
  void nextTick(() => textareaRef.value?.focus())
}

/** 触发字符左侧允许的前导符：行首 / 空白 / 常见中文与括号标点。 */
const TRIGGER_PREFIX = /(?:^|[\s\u3000(（【［{「『"'、，。；：！？,.;:!?])/

/**
 * 按光标位置解析触发态，而不是按整段文本的行尾。
 *
 * 行尾判断在「输入 @ 后又移动光标」「中间插入」「中文标点后输入」等场景下都会漏判，
 * 表现就是 `@` / `/` 时灵时不灵；以 caret 之前的片段为判定依据才是稳的。
 */
function onInput(): void {
  const ta = textareaRef.value
  const pos = ta?.selectionStart ?? draft.value.length
  const before = draft.value.slice(0, pos)

  const at = new RegExp(TRIGGER_PREFIX.source + '@([^\\s@]*)$').exec(before)
  if (at) {
    mentionStart.value = pos - at[1].length - 1
    mentionQuery.value = at[1]
    mentionOpen.value = true
    mentionActiveIndex.value = 0
    slashOpen.value = false
    slashStart.value = -1
    return
  }
  mentionOpen.value = false
  mentionStart.value = -1
  mentionActiveIndex.value = -1

  const sl = new RegExp(TRIGGER_PREFIX.source + '/([A-Za-z]*)$').exec(before)
  if (sl) {
    slashStart.value = pos - sl[1].length - 1
    slashQuery.value = sl[1]
    slashOpen.value = true
    return
  }
  slashOpen.value = false
  slashStart.value = -1
}

/**
 * 命令统一反馈：打开型命令（弹面板 / 聚焦 / 跳页）本身没有可感知结果，
 * 不给提示用户无法确认命令是否生效——双重保险（toast 在桌面壳里有时被窗口遮挡）：
 *   1) toast.success 走右下角气泡
 *   2) 输入框内联反馈条 2.5s 后淡出，定位就近、不可能被遮
 */
const cmdFeedback = ref<{ text: string } | null>(null)
let cmdFeedbackTimer: ReturnType<typeof setTimeout> | null = null

function showCmdFeedback(text: string): void {
  cmdFeedback.value = { text }
  if (cmdFeedbackTimer) clearTimeout(cmdFeedbackTimer)
  cmdFeedbackTimer = setTimeout(() => {
    cmdFeedback.value = null
    cmdFeedbackTimer = null
  }, 2500)
}

function executed(cmd: SlashCommand): void {
  const detail = cmd.descKey ? t(cmd.descKey) : (cmd.desc ?? '')
  toast.success(t('slash.picked', `/${cmd.id}`), detail)
  showCmdFeedback(`/${cmd.id} · ${detail}`)
}

/**
 * /goal 目标命令：无参查看当前目标；pause/resume/clear 直操作；
 * 其余文本整体作为目标描述设定。
 */
async function handleGoalCommand(args?: string): Promise<void> {
  const id = chat.currentID
  if (!id) {
    toast.warning(t('chat.noSession'))
    return
  }
  const arg = (args ?? '').trim()
  try {
    if (!arg) {
      // 无参 = 查看当前目标
      if (!chat.goal) {
        toast.info(t('chat.goal.none'))
      } else {
        showCmdFeedback(`🎯 ${chat.goal.text} · ${t('chat.goal.round', chat.goal.round, chat.goal.max_rounds || 20)}`)
      }
      return
    }
    if (arg === 'pause' || arg === 'resume' || arg === 'clear') {
      await chat.updateGoal(id, arg)
      toast.success(t('chat.goal.' + (arg === 'clear' ? 'cleared' : arg === 'pause' ? 'paused' : 'resumed')))
      executed({ id: 'goal' } as SlashCommand)
      return
    }
    await chat.updateGoal(id, 'set', arg)
    toast.success(t('chat.goal.set'))
    executed({ id: 'goal' } as SlashCommand)
  } catch (e) {
    toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
  }
}

/**
 * /trust 执行模式：无参循环；带参按别名直切。
 * 面板选中（pickSlash）与键入（submit）共用，避免逻辑双份。
 */
function handleTrustCommand(args?: string): void {
  const cycle: PermissionLevel[] = ['restricted', 'confirm', 'auto', 'full']
  const arg = (args ?? '').trim()
  if (!arg) {
    const idx = cycle.indexOf(permissionLevel.value)
    const next = cycle[(idx + 1) % cycle.length]
    applyTrust(next)
    return
  }
  const target = TRUST_ALIASES[arg] ?? TRUST_ALIASES[arg.toLowerCase()] ?? null
  if (!target) {
    toast.error(t('slash.trustUnknown', arg))
    return
  }
  applyTrust(target)
}

/** 应用执行模式：emit 给 ChatView 落库 + toast/内联反馈。 */
function applyTrust(level: PermissionLevel): void {
  emit('change-permission', level)
  const labelKey = PERMISSION_ITEMS.find((p) => p.value === level)?.labelKey ?? 'chat.perm.confirm'
  toast.success(t('slash.trustHint', t(labelKey)))
  showCmdFeedback(`/trust · ${t(labelKey)}`)
}

function pickSlash(cmd: SlashCommand): void {
  slashOpen.value = false
  // 全部命令统一双保险反馈：toast（右下角气泡）+ 输入框内联反馈条。
  // 异步执行的命令（/clear / /compact / /export / /agent / /help）在子函数内 toast 成功，
  // 这里再补内联反馈；同步命令直接走 executed() 同时给两个反馈。
  const okSync = (): void => {
    executed(cmd)
  }
  const okAsync = (): void => {
    showCmdFeedback(`/${cmd.id} · ${cmd.descKey ? t(cmd.descKey) : (cmd.desc ?? '')}`)
  }
  switch (cmd.id) {
    case 'clear':
      void clearSession()
      okAsync()
      break
    case 'new':
      emit('send', '__wb_new_session__', [], currentParams())
      okSync()
      break
    case 'focus':
      textareaRef.value?.focus()
      okSync()
      break
    case 'workspace':
      emit('pick-workspace')
      okSync()
      break
    case 'model':
      modelSelectorRef.value?.open()
      okSync()
      break
    case 'theme':
      void router.push('/settings')
      okSync()
      break
    case 'attach':
      pickFile()
      okSync()
      break
    case 'regenerate':
      emit('regenerate')
      okSync()
      break
    case 'compact':
      void runCompact()
      okAsync()
      break
    case 'tasks':
      window.dispatchEvent(new Event('workbaby:open-tasks'))
      okSync()
      break
    case 'agent':
      void submitAgentTask()
      okAsync()
      break
    case 'trust':
      // 面板选中无参 → 循环；带参由 submit 拦截器处理
      handleTrustCommand()
      break
    case 'goal':
      // 目标命令：留 "/goal " 前缀让用户接着输入描述，Enter 时 submit 拦截执行
      draft.value = '/goal '
      void nextTick(() => textareaRef.value?.focus())
      okSync()
      break
    case 'side':
      // 辅助对话：打开右栏面板，命令本身不作为消息发送
      emit('side')
      okSync()
      break
    case 'context':
      window.dispatchEvent(new Event('workbaby:open-context'))
      okSync()
      break
    case 'export':
      void exportSessionMd()
      okAsync()
      break
    case 'help':
      void showHelp()
      okAsync()
      break
    default:
      // 自定义命令：提示词模板灌入输入框（$ARGUMENTS 占位由用户就地编辑）
      if (cmd.prompt) {
        draft.value = cmd.prompt
        void nextTick(() => textareaRef.value?.focus())
        okSync()
        break
      }
      // 后端命令：默认清空 + 提示。后端暂无专用接口的命令不会出现在面板
      toast.info(t('slash.picked', `/${cmd.id}`))
      showCmdFeedback(`/${cmd.id}`)
  }
}

async function runCompact(): Promise<void> {
  const id = chat.currentID
  if (!id) {
    toast.warning(t('chat.noSession'))
    return
  }
  const res = await chat.compactSession(id)
  if (res) {
    toast.success(t('slash.compactDone', res.compacted, res.freed_chars))
  } else {
    toast.error(t('slash.compactFailed'))
  }
}

/** /clear：真正清空当前会话消息（复用会话删除级联，后端持久化）。 */
async function clearSession(): Promise<void> {
  const id = chat.currentID
  if (!id) {
    toast.warning(t('chat.noSession'))
    return
  }
  try {
    await chat.clearMessages()
    toast.success(t('slash.cleared'))
  } catch (e) {
    toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
  }
}

/** /export：把当前会话导出为本地 Markdown 文件（浏览器 Blob 下载）。 */
async function exportSessionMd(): Promise<void> {
  const id = chat.currentID
  if (!id) {
    toast.warning(t('chat.noSession'))
    return
  }
  try {
    const rows = await apiGet<{ items?: Message[] }>(`/api/v1/chat/sessions/${id}/messages?limit=1000`)
    const items = rows?.items ?? []
    if (items.length === 0) {
      toast.info(t('slash.exportEmpty'))
      return
    }
    const lines: string[] = ['# WorkBaby 会话导出', '']
    for (const m of items) {
      const who = m.role === 'user' ? '你' : 'WorkBaby'
      const when = m.created_at ? new Date(m.created_at).toLocaleString() : ''
      lines.push(`## ${who} ${when ? '· ' + when : ''}`, '')
      if (m.thinking) lines.push(`> 思考：\n> ${m.thinking.replace(/\n/g, '\n> ')}`, '')
      lines.push(m.content ?? '', '')
    }
    const blob = new Blob([lines.join('\n')], { type: 'text/markdown;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `workbaby-${new Date().toISOString().slice(0, 10)}.md`
    a.click()
    URL.revokeObjectURL(url)
    toast.success(t('slash.exportDone'))
  } catch (e) {
    toast.error(t('slash.exportFailed'), e instanceof Error ? e.message : String(e))
  }
}

/** /agent：把一段自包含任务交给后台 Agent 跑（任务中心可看进度、可取消）。 */
async function submitAgentTask(): Promise<void> {
  if (!chat.currentID) {
    toast.warning(t('chat.noSession'))
    return
  }
  try {
    const { value } = await ElMessageBox.prompt(
      t('slash.agentPromptTip'),
      t('slash.agentTitle'),
      {
        inputType: 'textarea',
        inputPlaceholder: t('slash.agentPlaceholder'),
        confirmButtonText: t('slash.agentSubmit'),
        cancelButtonText: t('ui.btn.cancel')
      }
    )
    const prompt = String(value ?? '').trim()
    if (!prompt) return
    await tasksStore.submit(chat.currentID, 'default', prompt)
    // 任务中心是聊天页右侧面板，提交后就地展开
    window.dispatchEvent(new Event('workbaby:open-tasks'))
  } catch (e) {
    if (e === 'cancel') return // 用户主动取消，不算失败
    toast.error(t('slash.agentFailed'), e instanceof Error ? e.message : String(e))
  }
}

/** /help：弹出命令清单（所有可执行命令与去向一目了然）。 */
async function showHelp(): Promise<void> {
  await ElMessageBox.alert(
    `<div style="line-height:1.9;font-size:13px">
      <b>/new</b> 新建会话<br/>
      <b>/clear</b> 清空当前会话<br/>
      <b>/compact</b> 压缩历史（后端摘要）<br/>
      <b>/model</b> 切换模型 · <b>/workspace</b> 绑定目录 · <b>/theme</b> 外观<br/>
      <b>/attach</b> 添加附件 · <b>/regenerate</b> 重新生成 · <b>/focus</b> 聚焦输入<br/>
      <b>/export</b> 导出会话 Markdown · <b>/tasks</b> 后台任务 · <b>/agent</b> 子代理<br/>
      <b>/trust</b> 查看权限 · <b>/help</b> 本帮助<br/>
      <span style="color:#888">输入 <code>/</code> 实时列出，输入 <code>@</code> 可引用技能/文件夹/文件</span>
    </div>`,
    t('slash.helpTitle'),
    { dangerouslyUseHTMLString: true, confirmButtonText: t('ui.btn.ok') }
  )
}

/** 输入框最大高度：视口 55%（可超过原 240px 上限，长文编辑不憋屈）。 */
const taMaxHeight = Math.round(window.innerHeight * 0.55)

/** 打字时只增不减：用户手动拉伸（resize: vertical）设置的高度由用户做主，不覆盖。 */
function autoResize(): void {
  const ta = textareaRef.value
  if (!ta) return
  if (ta.scrollHeight > ta.clientHeight) {
    ta.style.height = `${Math.min(ta.scrollHeight, taMaxHeight)}px`
  }
}

/** 清空草稿后高度回弹到单行。 */
function resetHeight(): void {
  const ta = textareaRef.value
  if (ta) ta.style.height = ''
}

function addToQueue(): void {
  const text = draft.value.trim()
  if (!text) return
  queue.value.push({ id: `q-${Date.now()}`, text })
  draft.value = ''
  resetHeight()
}

function removeQueueItem(id: string): void {
  queue.value = queue.value.filter((m) => m.id !== id)
  emit('remove-queue', id)
}

/** 逐条编辑：弹窗改文本（复用全局 prompt 对话框）。 */
async function editQueueItem(id: string, current: string): Promise<void> {
  const text = await dialog.prompt({
    title: t('queue.edit'),
    defaultValue: current
  })
  if (text == null) return
  const trimmed = text.trim()
  const item = queue.value.find((m) => m.id === id)
  if (!item) return
  if (!trimmed) {
    removeQueueItem(id)
    return
  }
  item.text = trimmed
}

/** 上下移动排序。 */
function moveQueueItem(id: string, dir: -1 | 1): void {
  const i = queue.value.findIndex((m) => m.id === id)
  const j = i + dir
  if (i < 0 || j < 0 || j >= queue.value.length) return
  const next = [...queue.value]
  ;[next[i], next[j]] = [next[j], next[i]]
  queue.value = next
}

/** 暂停 / 恢复自动出队。 */
function toggleQueuePause(): void {
  queuePaused.value = !queuePaused.value
}

function flushQueue(): void {
  if (queuePaused.value || queue.value.length === 0) return
  const next = queue.value.shift()
  if (next) {
    emit('remove-queue', next.id)
    emit('send-queued', next.text)
  }
}

// ===== 队列持久化：按会话存 localStorage，导航 / 刷新不丢 =====
const QUEUE_KEY_PREFIX = 'wb.queue.'
function queueKey(): string {
  return QUEUE_KEY_PREFIX + (chat.currentID ?? 'none')
}
function loadQueue(): void {
  try {
    const raw = localStorage.getItem(queueKey())
    queue.value = raw ? (JSON.parse(raw) as QueuedMessage[]) : []
  } catch {
    queue.value = []
  }
}
watch(
  () => chat.currentID,
  () => loadQueue(),
  { immediate: true }
)
watch(
  [queue, () => chat.currentID],
  () => {
    try {
      if (queue.value.length > 0) localStorage.setItem(queueKey(), JSON.stringify(queue.value))
      else localStorage.removeItem(queueKey())
    } catch {
      // 隐私模式不可写：队列退化为会话内有效
    }
  },
  { deep: true }
)

onMounted(() => {
  textareaRef.value?.focus()
  // 全局快捷键聚焦输入框（Ctrl+/ 与 Ctrl+Shift+P，App.vue 注册）
  window.addEventListener('wb:focus-input', focusFromShortcut)
  document.addEventListener('pointerdown', onDocumentPointerDown, true)
  // 拉取输入上限：system_settings.chat.maxInputChars（key 不存在时后端返 404，catcher 兜底默认）
  void loadMaxInputChars()
})

/** 异步拉取用户配置的输入上限；缺失/解析失败 fallback DEFAULT_MAX_INPUT_CHARS。 */
async function loadMaxInputChars(): Promise<void> {
  try {
    const row = await apiGet<{ k: string; v: string }>('/api/v1/kv/chat.maxInputChars')
    const n = parseInt(row.v, 10)
    if (Number.isFinite(n) && n > 0) maxInputChars.value = n
  } catch {
    // 后端无值 / 网络失败 / 用户没改 → 保持 DEFAULT_MAX_INPUT_CHARS
  }
}

onBeforeUnmount(() => {
  window.removeEventListener('wb:focus-input', focusFromShortcut)
  document.removeEventListener('pointerdown', onDocumentPointerDown, true)
  if (cmdFeedbackTimer) clearTimeout(cmdFeedbackTimer)
})

function focusFromShortcut(): void {
  void nextTick(() => textareaRef.value?.focus())
}

watch(() => props.streaming, (isStreaming, prev) => {
  if (!isStreaming && prev === true && queue.value.length > 0) {
    flushQueue()
  }
})

function onDragEnter(e: DragEvent): void {
  if (props.disabled) return
  e.preventDefault()
  dragCounter.value++
  if (e.dataTransfer?.types.includes('Files')) {
    isDragging.value = true
  }
}

function onDragOver(e: DragEvent): void {
  if (props.disabled) return
  e.preventDefault()
}

function onDragLeave(e: DragEvent): void {
  if (props.disabled) return
  e.preventDefault()
  dragCounter.value--
  if (dragCounter.value <= 0) {
    dragCounter.value = 0
    isDragging.value = false
  }
}

async function onDrop(e: DragEvent): Promise<void> {
  if (props.disabled) return
  e.preventDefault()
  isDragging.value = false
  dragCounter.value = 0
  const files = e.dataTransfer?.files
  if (!files || files.length === 0) return
  for (const file of Array.from(files)) {
    await uploadFile(file)
  }
}

/**
 * 粘贴：只拦「文件型」剪贴板（截图 / 复制的图片），文本照常走浏览器默认粘贴。
 * 流式中同样允许——粘图是给下一轮（或插话）准备的，不该被禁用。
 */
async function onPaste(e: ClipboardEvent): Promise<void> {
  if (props.disabled) return
  const items = e.clipboardData?.items
  if (!items) return
  const files: File[] = []
  for (const item of Array.from(items)) {
    if (item.kind === 'file') {
      const file = item.getAsFile()
      if (file) files.push(file)
    }
  }
  if (files.length > 0) {
    e.preventDefault()
    for (const file of files) {
      await uploadFile(file)
    }
  }
}

function onCompositionStart(): void {
  isComposing.value = true
}
function onCompositionEnd(): void {
  isComposing.value = false
}

function stop(): void {
  emit('stop')
}

/** 供父组件（ChatView）调用：把示例 prompt 灌进输入框并聚焦。
 *  暴露 setDraft / focusInput 让 ChatView 接住 MessageList 的 use-quick-prompt 事件。
 */
function setDraft(text: string): void {
  draft.value = text
  void nextTick(() => {
    textareaRef.value?.focus()
    autoResize()
  })
}

defineExpose({
  setDraft,
  /** 追加文本（工作区文件树「添加到聊天」）：保留用户已输入内容，只追加引用后聚焦。 */
  appendText: (text: string) => {
    draft.value = draft.value.trimEnd() + (draft.value.trim() ? ' ' : '') + text + ' '
    void nextTick(() => textareaRef.value?.focus())
  },
  focusInput: () => textareaRef.value?.focus()
})
</script>

<template>
  <div
    ref="composerRef"
    class="composer relative"
    @dragenter="onDragEnter"
    @dragover="onDragOver"
    @dragleave="onDragLeave"
    @drop="onDrop"
  >
    <Transition
      enter-active-class="transition-opacity duration-200"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition-opacity duration-150"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="isDragging"
        class="absolute inset-0 z-50 flex items-center justify-center border-2 border-dashed border-wb-primary bg-wb-primary/10"
      >
        <div class="flex flex-col items-center gap-2 text-wb-primary">
          <Upload class="h-8 w-8" />
          <span class="text-sm font-medium">{{ t('chat.dropToUpload') }}</span>
        </div>
      </div>
    </Transition>

    <!-- 输入容器（原型 .composer-in）：聚焦时主色描边 + 光晕。队列 / 技能提及 / 附件
         条带都收进容器内，视觉上属于「这一条消息」的一部分 -->
    <div class="composer-in">
      <MidTurnQueue
        class="comp-strip"
        :queue="queue"
        :paused="queuePaused"
        @remove="removeQueueItem"
        @edit="editQueueItem"
        @move="moveQueueItem"
        @toggle-pause="toggleQueuePause"
        @flush="flushQueue"
      />

      <div v-if="activeSkillMentions.length > 0" class="skill-mention-strip">
        <span
          v-for="sk in activeSkillMentions"
          :key="sk.name"
          class="skill-mention-chip"
          :title="sk.description ?? ''"
        >
          <el-icon :size="11"><Sparkles /></el-icon>
          <span class="max-w-[200px] truncate">{{ t('chat.skillMentionLabel', sk.name) }}</span>
          <button
            type="button"
            class="ml-0.5 cursor-pointer opacity-70 hover:opacity-100"
            :aria-label="t('common.delete')"
            @click="removeSkillMention(sk.name)"
          >×</button>
        </span>
      </div>

      <AttachmentStrip class="comp-strip" :attachments="attachments" @remove="removeAttachment" />

      <div class="relative">
        <!-- 镜像层：与 textarea 同字体同内边距，只渲染淡色 token 气泡 -->
        <div
          v-show="mirrorVisible"
          ref="mirrorRef"
          class="ta-mirror"
          aria-hidden="true"
        ><template v-for="(seg, si) in mirrorTokens" :key="si"><span v-if="seg.kind" class="tk" :class="`tk-${seg.kind}`">{{ seg.text }}</span><template v-else>{{ seg.text }}</template></template></div>
        <textarea
          ref="textareaRef"
          v-model="draft"
          rows="1"
          class="ta-input"
          :class="{ 'is-mirror': mirrorVisible }"
          :placeholder="streaming ? t('chat.placeholderSteer') : t('chat.placeholder')"
          :disabled="disabled"
          @input="onInput(); autoResize()"
          @keydown="onKeydown"
          @scroll="syncMirrorScroll"
          @paste="onPaste"
          @compositionstart="onCompositionStart"
          @compositionend="onCompositionEnd"
        />

        <SlashCommandPalette
          ref="slashPaletteRef"
          :visible="slashOpen"
          :query="slashQuery"
          :backend-commands="chat.commands"
          @pick="pickSlash"
          @close="slashOpen = false"
        />

        <MentionPicker
          :visible="mentionOpen"
          :query="mentionQuery"
          :items="mentionItems"
          :active-index="mentionActiveIndex"
          @pick="pickMention"
          @update-active="mentionActiveIndex = $event"
          @close="mentionOpen = false"
        />
      </div>

      <!-- 工具行（原型 .composer-bar）：胶囊 mini 控件 + 上下文计数 + 发送键 -->
      <div class="composer-bar">
        <ModelSelector
          ref="modelSelectorRef"
          :models="props.models ?? []"
          :selectedModelID="props.selectedModelID"
          :circuit-states="props.circuitStates"
          @select-model="selectModel"
        />

        <!-- 权限 mini：下拉选择权限档位。trigger 必须是单个原生元素，el-dropdown 直接挂 -->
        <el-dropdown trigger="click" @command="(v: PermissionLevel) => emit('change-permission', v)">
          <button
            type="button"
            class="mini"
            :title="t(permissionDesc)"
            :class="{ 'mini--active': permissionLevel !== 'auto', 'mini--danger': permissionDanger }"
          >
            <Shield class="mic" />
            {{ t(permissionLabel) }}
            <ChevronDown class="mic mic-chev" />
          </button>
          <template #dropdown>
            <el-dropdown-menu class="wb-perm-menu">
              <el-dropdown-item
                v-for="p in PERMISSION_ITEMS"
                :key="p.value"
                :command="p.value"
                :class="{ 'is-danger': p.danger, 'is-selected': permissionLevel === p.value }"
              >
                <div class="flex flex-col gap-0.5 py-0.5">
                  <span class="flex items-center gap-1.5 text-xs font-medium">
                    <span
                      v-if="permissionLevel === p.value"
                      class="inline-block h-1.5 w-1.5 rounded-full bg-wb-primary"
                    />
                    {{ t(p.labelKey) }}
                  </span>
                  <span class="max-w-[260px] text-[11px] leading-snug text-wb-muted">{{ t(p.descKey) }}</span>
                </div>
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>

        <!-- 采样参数 mini：展示生效值 + 可覆盖（与设置页同一数据源） -->
        <ParamsPopover ref="paramsRef" :effective="props.effectiveParams" />

        <!-- 技能 mini：直接弹出技能选择器（选中后以 @技能名 插入光标处） -->
        <div class="relative">
          <button
            type="button"
            class="mini"
            :class="{ 'mini--active': skillPickerOpen }"
            :title="t('nav.skills')"
            :disabled="disabled || streaming"
            @click="openSkills"
          >
            <Zap class="mic" />
            {{ t('nav.skills') }}
          </button>
          <SkillPicker :visible="skillPickerOpen" @pick="pickSkill" @close="skillPickerOpen = false" />
        </div>

        <!-- 附件 mini -->
        <button
          type="button"
          class="mini"
          :title="t('chat.attachment')"
          :disabled="disabled || streaming || uploading"
          @click="pickFile"
        >
          <Paperclip class="mic" />
          {{ t('chat.attachment') }}
        </button>

        <!-- 流式中入队：不打断当前回合，排队等待 flush -->
        <button
          v-if="streaming && draft.trim()"
          type="button"
          class="mini"
          :title="t('queue.enqueue')"
          @click="addToQueue"
        >
          <Plus class="mic" />
          {{ t('queue.enqueue') }}
        </button>

        <span class="sp" />
        <span v-if="draft.length > 0" class="ctx-num" :class="{ over: draft.length > maxInputChars }">
          {{ draft.length }} / {{ maxInputChars }}
        </span>
        <ContextUsagePopover
          :used="ctxUsed ?? 0"
          :max="ctxMax ?? 128000"
          :segments="contextSegments"
          :estimated="contextEstimated"
          :compact-disabled="disabled || streaming"
          @compact="emit('compact')"
        />

        <button
          v-if="!streaming"
          type="button"
          class="send"
          :disabled="!canSend"
          :aria-label="t('ui.btn.send')"
          @click="submit"
        >
          <Send />
        </button>
        <!-- 流式中：⚡立即插入（steer，下一轮前注入不打断执行）+ 停止，并排可用 -->
        <template v-else>
          <button
            type="button"
            class="send send--steer"
            :disabled="!canSend"
            :aria-label="t('chat.steerNow')"
            :title="`${t('chat.steerNow')} (Ctrl+Enter)`"
            @click="submit"
          >
            <Zap />
          </button>
          <button type="button" class="send send--stop" :aria-label="t('chat.stop')" @click="stop">
            <Square class="fill-current" />
          </button>
        </template>
      </div>

      <!-- 斜杠命令执行反馈：贴近输入区，2.5s 后淡出，避免被桌面壳遮挡 -->
      <Transition
        enter-active-class="transition-all duration-200 ease-out"
        enter-from-class="opacity-0 translate-y-1"
        enter-to-class="opacity-100 translate-y-0"
        leave-active-class="transition-opacity duration-200"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
      >
        <div v-if="cmdFeedback" class="cmd-feedback">
          <Check class="h-3 w-3 shrink-0 text-wb-mint" />
          <span>{{ cmdFeedback.text }}</span>
        </div>
      </Transition>
    </div>
  </div>
</template>

<style scoped>
/* ===== 原型 composer：全局 wb-ui.css 已提供 .composer / .composer-in / .mini / .send
   基础样式，这里只做组件级微调。chip 触发器的幽灵化在各子组件自己的 scoped
   样式内完成（ParamsPopover / ModelSelector）——父组件 :deep 跨组件覆盖会因
   样式注入顺序而失效，不要再加回来 ===== */

.mini:disabled,
.composer :deep(.chip-btn:disabled) {
  opacity: 0.5;
  cursor: not-allowed;
}

/* mini 内联图标：统一 12.5px，chevron 更小更淡 */
.mini :deep(svg.mic) {
  width: 12.5px;
  height: 12.5px;
  flex: none;
}
.mini :deep(svg.mic-chev) {
  width: 10px;
  height: 10px;
  opacity: 0.7;
}

/* 斜杠命令执行反馈：紧贴输入区底边内边距对齐（13px），mint 圆点 + 主色文字 */
.cmd-feedback {
  display: inline-flex;
  align-self: flex-start;
  align-items: center;
  gap: 6px;
  margin: 4px 13px 8px;
  padding: 4px 10px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--wb-mint) 12%, transparent);
  color: var(--wb-mint-strong, var(--wb-mint));
  font-size: 11.5px;
  font-weight: 500;
  white-space: nowrap;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 非默认权限态需要被看见：文字色 + 极淡底 + 一道内描边。
 * 「完全访问」原先只有 12% 红底，在白色输入框上饱和度偏高、像报错；
 * 拆成底色压到 8% + 内描边承担边界，警示仍在但不刺眼。 */
.mini--active {
  color: var(--wb-warning);
  background: color-mix(in srgb, var(--wb-warning) 9%, transparent);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--wb-warning) 22%, transparent);
}
.mini--danger {
  color: var(--wb-danger);
  background: color-mix(in srgb, var(--wb-danger) 8%, transparent);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--wb-danger) 22%, transparent);
}

/* 草稿字符计数：mono 数字，超限标红 */
.ctx-num {
  flex: none;
  font-family: var(--font-mono, ui-monospace, monospace);
  font-size: 11px;
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.ctx-num.over {
  color: var(--wb-danger);
}

/* 发送键：禁用弱化；插入键主色底（流式中与停止键并排）；停止键 danger 底 + 呼吸动画 */
.send:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.send--steer {
  background: var(--wb-primary);
}
.send--stop {
  background: var(--wb-danger);
  animation: stop-breathe 1.6s ease-in-out infinite;
}
@keyframes stop-breathe {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.72;
  }
}

/* textarea 占位符：与原型一致的弱化提示 */
.composer-in textarea::placeholder {
  color: color-mix(in srgb, var(--wb-muted) 75%, transparent);
}

/* 队列 / 附件条带：内边距对齐 textarea（13px），与输入文本视觉同列 */
.comp-strip {
  padding: 10px 13px 0;
}

/* 激活的 @skill 提及 chip：与附件条同级，视觉上明确「这条消息带着技能上下文」；
   只用底色 + 文字色，不描边；内边距对齐 textarea（13px） */
.skill-mention-strip {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 10px 13px 0;
}
.skill-mention-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  max-width: 240px;
  height: 24px;
  padding: 0 9px;
  border: 1px solid transparent;
  border-radius: 9999px;
  background: color-mix(in srgb, var(--wb-lavender) 14%, transparent);
  color: var(--wb-lavender);
  font-size: 11px;
  line-height: 1;
  white-space: nowrap;
}

/* 权限下拉的样式在 style.css 的 .wb-perm-menu 段落：菜单 teleport 到 body，
   scoped :deep 命中不到，故不在此重复声明。 */
</style>