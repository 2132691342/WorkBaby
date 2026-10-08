<script setup lang="ts">
// 输入区：回车发送 / Shift+回车换行；运行中变「停止」可插话。
// 顶部一行放待发送文件，底部一行放模型、规矩、上下文水位与发送键。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useChatStore } from '../../stores/chat'
import { useSettingsStore } from '../../stores/settings'
import type { AttachmentREQ } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import ContextMeter from './ContextMeter.vue'
import SessionChips from './SessionChips.vue'
import WorkspaceChip from './WorkspaceChip.vue'

const props = defineProps<{
  running: boolean
  sending?: boolean
  stopping?: boolean
  disabled?: boolean
  placeholder?: string
}>()
const emit = defineEmits<{
  send: [text: string, attachments: AttachmentREQ[]]
  steer: [text: string]
  stop: []
  newSession: []
  clearSession: []
}>()

// 后端最多认 5 个，前端先卡住，别让用户攒一堆发不上去
const MAX_ATTACH = 5

interface PendingFile {
  path: string
  name: string
  outside: boolean
  image?: string // 粘贴图片的 base64（无路径）
}

const COMMANDS = [
  { key: 'new', name: '新对话', desc: '开一段全新的对话' },
  { key: 'clear', name: '回到开头', desc: '保留这个会话，从第一条消息重新开始' },
  { key: 'model', name: '换个模型', desc: '为这个会话换一个模型' },
  { key: 'permission', name: '动手前规矩', desc: '改文件、跑命令要不要先问你' },
] as const

const chat = useChatStore()
const settings = useSettingsStore()
const root = ref<HTMLElement | null>(null)
const chipsRef = ref<InstanceType<typeof SessionChips> | null>(null)
const ta = ref<HTMLTextAreaElement | null>(null)
const text = ref('')
const files = ref<PendingFile[]>([])
const menu = ref<'slash' | 'mention' | null>(null)
const cmdIndex = ref(0)

const outsideCount = computed(() => files.value.filter((f) => f.outside).length)
const full = computed(() => files.value.length >= MAX_ATTACH)
// 只认行首的整条 /xxx；一旦带空格就不是命令了
const cmds = computed(() => {
  const q = menu.value === 'slash' ? text.value : ''
  if (!q || /\s/.test(q)) return []
  return COMMANDS.filter((c) => `/${c.key}`.startsWith(q))
})

// 输入框高度：0 = 跟随内容自动；拖过顶边把手后由用户说了算。
// 只靠自动撑高的话超过上限就只能在小框里滚，长文本写完自己也看不清。
const TA_MIN = 64
const TA_AUTO_MAX = 180
const TA_MAX = 520
// 消息区至少留这么高。再高输入区就顶到标题栏，聊天记录整块看不见。
const MSGS_MIN = 120
const taH = ref(0)
const resizing = ref(false)

// 拖拽上限跟着窗口走：写死 520px 在矮窗口上会把消息区挤没，
// 输入区整块掉到窗口外面，底下的发送键直接看不见。
function maxTaH(): number {
  const el = ta.value
  const wrap = el?.closest('.composer-wrap') as HTMLElement | null
  if (!el || !wrap) return TA_MAX
  const rest = wrap.offsetHeight - el.offsetHeight // 输入区里除文本框以外的高度
  const top = wrap.getBoundingClientRect().top
  const below = 24 // 输入区下边留白
  const room = window.innerHeight - top - rest - below - MSGS_MIN
  return Math.max(TA_MIN, Math.min(TA_MAX, room))
}

function autoGrow() {
  const el = ta.value
  if (!el) return
  el.style.height = 'auto'
  const auto = el.scrollHeight
  // 手动拉高之后必须以用户给的高度为准：再按内容高度夹一次，
  // 拖上去的框会被立刻压回去，等于拖不动。
  const next = taH.value ? Math.max(auto, taH.value) : Math.min(auto, TA_AUTO_MAX)
  el.style.height = `${Math.min(Math.max(next, TA_MIN), maxTaH())}px`
}

function onGripDown(e: PointerEvent) {
  const el = ta.value
  if (!el) return
  const startY = e.clientY
  const startH = el.offsetHeight
  const cap = maxTaH()
  taH.value = Math.max(Math.min(startH, cap), TA_MIN)
  resizing.value = true
  const move = (ev: PointerEvent) => {
    taH.value = Math.min(Math.max(startH + (startY - ev.clientY), TA_MIN), cap)
    autoGrow()
  }
  const up = () => {
    resizing.value = false
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  e.preventDefault()
}

// 双击把手：交还给自动高度
function onGripDbl() {
  taH.value = 0
  nextTick(autoGrow)
}

function clearInput() {
  text.value = ''
  menu.value = null
  nextTick(autoGrow)
}

function closeAll() {
  menu.value = null
  chipsRef.value?.close()
}

function onInput() {
  const v = text.value
  if (/@\S*$/.test(v)) {
    menu.value = 'mention'
  } else if (v.startsWith('/') && !/\s/.test(v)) {
    menu.value = 'slash'
    cmdIndex.value = 0
  } else {
    menu.value = null
  }
  autoGrow()
}

function runCommand(key: (typeof COMMANDS)[number]['key']) {
  clearInput()
  if (key === 'new') emit('newSession')
  else if (key === 'clear') emit('clearSession')
  else chipsRef.value?.show(key === 'model' ? 'model' : 'perm')
}

function onKeydown(e: KeyboardEvent) {
  if (menu.value === 'slash' && cmds.value.length) {
    const n = cmds.value.length
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      cmdIndex.value = (cmdIndex.value + 1) % n
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      cmdIndex.value = (cmdIndex.value - 1 + n) % n
      return
    }
    if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
      e.preventDefault()
      runCommand(cmds.value[cmdIndex.value].key)
      return
    }
  }
  if (e.key === 'Escape') {
    closeAll()
    return
  }
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    submit()
  }
}

function submit() {
  const value = text.value.trim()
  if (props.disabled || props.sending) return
  if (props.running) {
    if (!value) return
    emit('steer', value)
    clearInput()
    return
  }
  // 允许只发图片不打字
  if (!value && !files.value.length) return
  // 越界的文件助手读不到，宁可不发也不要假装成功
  if (outsideCount.value) return
  emit(
    'send',
    value,
    files.value.map((f) => ({
      path: f.path,
      name: f.name,
      image_base64: f.image,
    })),
  )
  clearInput()
  files.value = []
}

function baseName(p: string): string {
  const parts = p.split(/[\\/]/)
  return parts[parts.length - 1] || p
}

// 逐段比大小写再拼前缀：盘符大小写与分隔符混用时不至于判错
function toRelative(base: string, fullPath: string): string | null {
  const root = base.replace(/[\\/]+$/, '')
  if (!root) return fullPath
  const key = (p: string) => p.replace(/\//g, '\\').toLowerCase()
  if (!key(fullPath).startsWith(key(root) + '\\')) return null
  return fullPath.slice(root.length + 1)
}

// attachPath 把外部给的绝对路径挂成待发送附件：落在工作目录内转相对路径，
// 落在外面标出来（助手读不到，不假装成功）。选文件与外部打开共用这一条。
function attachPath(picked: string) {
  if (full.value) return
  const base = settings.boot?.workspace || ''
  const rel = base ? toRelative(base, picked) : null
  if (rel === null) {
    // 越界：后端按工作目录解析会读不到，UI 上说清楚，不假装成功
    files.value.push({ path: picked, name: baseName(picked), outside: true })
    return
  }
  if (files.value.some((f) => f.path === rel)) return
  files.value.push({ path: rel, name: baseName(rel), outside: false })
}

async function pickFile() {
  if (full.value) return
  menu.value = null
  // 选中后把刚敲的那个 @ 去掉，别留半个词在输入框里
  text.value = text.value.replace(/@\S*$/, '').trimEnd()
  nextTick(autoGrow)
  const { OpenFileDialog } = await import('../../../wailsjs/go/main/App')
  const picked = await OpenFileDialog('', '')
  if (!picked) return
  attachPath(picked)
}

function removeFile(path: string) {
  files.value = files.value.filter((f) => f.path !== path)
}

function onDocPointerDown(e: PointerEvent) {
  if (root.value?.contains(e.target as Node)) return
  closeAll()
}

// 粘贴图片：截图后 Ctrl+V 直接进附件列表，和打 @ 引用文件是同一条发送链路
function onPaste(e: ClipboardEvent) {
  const imgs = Array.from(e.clipboardData?.files || []).filter((f) => f.type.startsWith('image/'))
  if (!imgs.length) return
  e.preventDefault()
  for (const f of imgs) {
    if (full.value) break
    const reader = new FileReader()
    reader.onload = () => {
      const dataUrl = String(reader.result || '')
      const base64 = dataUrl.slice(dataUrl.indexOf(',') + 1)
      if (!base64) return
      files.value.push({
        path: '',
        name: f.name || `截图-${new Date().toISOString().slice(11, 19).replace(/:/g, '')}.png`,
        outside: false,
        image: base64,
      })
    }
    reader.readAsDataURL(f)
  }
}

watch(
  () => props.running,
  () => nextTick(() => ta.value?.focus()),
)

// 窗口变小后，之前拉高的高度可能已经超出可视区，必须跟着收回来。
function onResize() {
  if (resizing.value) return
  if (taH.value > maxTaH()) taH.value = maxTaH()
  autoGrow()
}

onMounted(() => {
  document.addEventListener('pointerdown', onDocPointerDown)
  window.addEventListener('resize', onResize)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocPointerDown)
  window.removeEventListener('resize', onResize)
})

defineExpose({ focus: () => ta.value?.focus(), attachPath })
</script>

<template>
  <div ref="root" class="composer-wrap">
    <div class="composer" :class="{ 'is-resizing': resizing }">
      <div
        class="composer-grip"
        title="拖动调整高度，双击恢复自动"
        @pointerdown="onGripDown"
        @dblclick="onGripDbl"
      />
      <!-- 待发送文件：Wails 一次只给一个路径，所以做成可多次添加的列表 -->
      <div v-if="files.length" class="attach-row">
        <span
          v-for="(f, i) in files"
          :key="`${f.path}-${i}`"
          class="attach"
          :class="{ 'is-bad': f.outside, 'is-img': !!f.image }"
          :title="f.outside ? `${f.path} · 不在工作目录内，助手读不到` : f.image ? `${f.name} · 粘贴的图片` : f.path"
        >
          <AppIcon :name="f.image ? 'image' : 'doc'" size="ic-xs" />
          <span class="fn">{{ f.name }}</span>
          <button class="x" type="button" aria-label="移除" @click="removeFile(f.path)">
            <AppIcon name="close" size="ic-xs" />
          </button>
        </span>
        <span class="attach-tip">{{ full ? `最多 ${MAX_ATTACH} 个` : `${files.length}/${MAX_ATTACH}` }}</span>
      </div>

      <div v-if="running" class="cq">
        <AppIcon name="info" size="ic-xs" /> 运行中：发送会作为插话，助手会尽快响应
      </div>

      <div class="composer-in">
        <textarea
          ref="ta"
          v-model="text"
          class="ta"
          rows="1"
          :disabled="disabled"
          :placeholder="placeholder || '有什么要帮忙的？直接说就行，打 @ 可引用文件，可粘贴截图'"
          @keydown="onKeydown"
          @input="onInput"
          @paste="onPaste"
        />
      </div>

      <div class="composer-bar">
        <WorkspaceChip />
        <span v-if="!running" class="hint">Enter 发送 · Shift+Enter 换行</span>
        <SessionChips ref="chipsRef" />
        <ContextMeter
          :used="chat.contextUsed"
          :win="chat.contextWindow"
          :ratio="chat.contextRatio"
          :known="chat.contextKnown"
        />
        <span v-if="outsideCount" class="warn">{{ outsideCount }} 个文件助手读不到</span>
        <span class="sp" />
        <button
          v-if="running"
          class="btn btn-sm btn-danger-ghost"
          :class="{ 'is-loading': stopping }"
          :disabled="stopping"
          type="button"
          @click="emit('stop')"
        >
          停止
        </button>
        <button
          v-else
          class="btn btn-sm btn-primary"
          :class="{ 'is-loading': sending }"
          type="button"
          :disabled="sending || !text.trim() || disabled || outsideCount > 0"
          @click="submit"
        >
          发送
        </button>
      </div>

      <!-- 斜杠命令：↑↓ 选择、回车执行、Esc 关闭 -->
      <div v-if="menu === 'slash'" class="pop menu-pop">
        <div class="menu-label">命令</div>
        <div v-if="!cmds.length" class="pop-row muted">没有匹配的命令</div>
        <template v-else>
          <button
            v-for="(c, i) in cmds"
            :key="c.key"
            class="pop-row"
            :class="{ 'is-hi': i === cmdIndex }"
            type="button"
            @mouseenter="cmdIndex = i"
            @click="runCommand(c.key)"
          >
            <span class="cm">/{{ c.key }}</span>
            <span class="grow">
              <b class="pr-t">{{ c.name }}</b>
              <span class="pr-d">{{ c.desc }}</span>
            </span>
          </button>
        </template>
      </div>

      <!-- @ 引用：只给一个动作，够用就好 -->
      <div v-else-if="menu === 'mention'" class="pop menu-pop">
        <div class="menu-label">引用文件到这条消息</div>
        <button class="pop-row" type="button" :disabled="full" @click="pickFile">
          <AppIcon name="doc" size="ic-xs" />
          <span class="grow">
            <b class="pr-t">选择文件…</b>
            <span class="pr-d">{{ full ? `一次最多 ${MAX_ATTACH} 个` : '只能选工作目录里的文件，助手才读得到' }}</span>
          </span>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.attach .fn {
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.attach.is-bad {
  border-color: var(--wb-danger);
  color: var(--wb-danger);
}
.attach .x {
  display: grid;
  place-items: center;
  width: 16px;
  height: 16px;
  border-radius: var(--wb-radius-full);
  color: var(--wb-muted);
  transition: color var(--wb-dur-fast) var(--wb-ease), transform var(--wb-dur-fast) var(--wb-ease);
}
.attach .x:hover {
  color: var(--wb-danger);
}
.attach .x:active {
  transform: scale(0.88);
}
.attach-tip {
  align-self: center;
  font-size: var(--wb-fs-hint);
  color: var(--wb-muted);
}
.composer-bar {
  flex-wrap: wrap;
  row-gap: var(--wb-sp-1);
}
.hint {
  font-size: var(--wb-fs-hint);
  color: var(--wb-muted);
  white-space: nowrap;
}
.warn {
  font-size: var(--wb-fs-hint);
  color: var(--wb-danger);
  white-space: nowrap;
}
.menu-pop {
  left: var(--wb-sp-3);
  right: var(--wb-sp-3);
  bottom: calc(100% + 6px);
  max-height: 300px;
  overflow-y: auto;
}
.cm {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-sm);
  color: var(--wb-primary);
  flex: none;
}
.grow {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.pop-row .pr-t {
  font-family: var(--font-sans);
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
}
.pop-row .pr-d {
  display: block;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.pop-row:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
@media (max-width: 1180px) {
  .hint {
    display: none;
  }
}
</style>
