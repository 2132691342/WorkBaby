<script setup lang="ts">
// 输入区：回车发送 / Shift+回车换行；运行中变「停止」可插话。
// 顶部一行放待发送文件，底部一行放模型、规矩、上下文水位与发送键。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useChatStore } from '../../stores/chat'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import * as api from '../../api'
import type { AttachmentREQ, FileEntryVO } from '../../types/api'
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
  queue: [text: string]
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
const toast = useToastStore()
// 「设置 → 行为」的回车发送开关：界面提示必须跟着它切，否则承诺的行为不成立。
const sendOnEnter = () => settings.values['send_on_enter'] !== 'false'
const sendHint = computed(() => (sendOnEnter() ? 'Enter 发送 · Shift+Enter 换行' : 'Enter 换行 · Ctrl+Enter 发送'))
const root = ref<HTMLElement | null>(null)
const chipsRef = ref<InstanceType<typeof SessionChips> | null>(null)
const ta = ref<HTMLTextAreaElement | null>(null)
const text = ref('')
const files = ref<PendingFile[]>([])
const menu = ref<'slash' | 'mention' | null>(null)
const cmdIndex = ref(0)

const outsideCount = computed(() => files.value.filter((f) => f.outside).length)
const full = computed(() => files.value.length >= MAX_ATTACH)

// ---- 斜杠菜单：内置命令 + 已启用技能，输入 / 名字过滤 ----
interface SlashItem {
  kind: 'cmd' | 'skill'
  key: string
  name: string
  desc: string
}

const MAX_SKILL_MATCH = 8
const slashItems = computed<SlashItem[]>(() => {
  const q = menu.value === 'slash' ? text.value : ''
  if (!q || /\s/.test(q)) return []
  const cmds: SlashItem[] = COMMANDS.filter((c) => `/${c.key}`.startsWith(q)).map((c) => ({
    kind: 'cmd', key: c.key, name: c.name, desc: c.desc,
  }))
  const kw = q.slice(1).toLowerCase()
  const skills: SlashItem[] = settings.skills
    .filter((s) => s.enabled)
    .filter((s) => !kw || s.name.toLowerCase().includes(kw) || s.description.toLowerCase().includes(kw))
    .slice(0, MAX_SKILL_MATCH)
    .map((s) => ({ kind: 'skill', key: s.name, name: s.name, desc: s.description }))
  return [...cmds, ...skills]
})

// 选技能后填入 /skill:名字，需求说明交给用户补；发送时后端把正文注入这条消息
function runSlash(item: SlashItem) {
  if (item.kind === 'cmd') {
    runCommand(item.key as (typeof COMMANDS)[number]['key'])
    return
  }
  clearInput()
  text.value = `/skill:${item.key} `
  nextTick(() => ta.value?.focus())
}

// ---- @ 引用：工作区文件面板，逐级浏览点选，不再弹资源管理器 ----
const browsePath = ref('')
const browseEntries = ref<FileEntryVO[]>([])
const browseLoading = ref(false)
const browseFilter = ref('')
const browseLoaded = ref(false)

const browseCrumbs = computed(() => {
  const parts = browsePath.value ? browsePath.value.split('/').filter(Boolean) : []
  return [
    { name: '工作区', path: '' },
    ...parts.map((p, i) => ({ name: p, path: parts.slice(0, i + 1).join('/') })),
  ]
})
const browseShown = computed(() => {
  const q = browseFilter.value.trim().toLowerCase()
  return q ? browseEntries.value.filter((e) => e.name.toLowerCase().includes(q)) : browseEntries.value
})

async function loadDir(path: string) {
  browseLoading.value = true
  browsePath.value = path
  browseFilter.value = ''
  try {
    browseEntries.value = (await api.files.list(path)).entries
    browseLoaded.value = true
  } catch (e) {
    toast.bad(`读取目录失败：${(e as Error)?.message || '请重试'}`)
    browseEntries.value = []
  } finally {
    browseLoading.value = false
  }
}

// 树里点选的一定在工作区内（后端 SafeJoin 兜底），直接按相对路径挂附件
function pickFromTree(entry: FileEntryVO) {
  if (entry.dir) {
    void loadDir(browsePath.value ? `${browsePath.value}/${entry.name}` : entry.name)
    return
  }
  if (full.value) {
    toast.bad(`一次最多引用 ${MAX_ATTACH} 个文件`)
    return
  }
  const rel = browsePath.value ? `${browsePath.value}/${entry.name}` : entry.name
  if (!files.value.some((f) => f.path === rel)) {
    files.value.push({ path: rel, name: entry.name, outside: false })
  }
  text.value = text.value.replace(/@\S*$/, '').trimEnd()
  menu.value = null
  nextTick(autoGrow)
}

// 上一级目录：根目录的上一级还是根目录
function browseUp() {
  const idx = browsePath.value.lastIndexOf('/')
  void loadDir(idx > 0 ? browsePath.value.slice(0, idx) : '')
}

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
    // 面板只拉一次根目录；进目录后靠条目点击导航
    if (!browseLoaded.value && !browseLoading.value) void loadDir('')
  } else if (v.startsWith('/') && !/\s/.test(v)) {
    menu.value = 'slash'
    cmdIndex.value = 0
    // 技能清单懒加载：第一次打 / 才拉，之后用缓存
    if (!settings.skills.length && !settings.skillsLoading) void settings.loadSkills()
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
  if (menu.value === 'slash' && slashItems.value.length) {
    const n = slashItems.value.length
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
      runSlash(slashItems.value[Math.min(cmdIndex.value, n - 1)])
      return
    }
  }
  if (e.key === 'Escape') {
    closeAll()
    // 运行中按 Esc = 停止。这是面对「停不下来的助手」最直接的本能；
    // 只在输入框为空时响应，免得正在写的内容被一次误按清掉上下文。
    if (props.running && !text.value.trim()) {
      e.preventDefault()
      emit('stop')
    }
    return
  }
  if (e.key === 'Enter' && !e.isComposing) {
    if (e.shiftKey) return // Shift+Enter 一律换行
    // 发送方式跟随「设置 → 行为」的回车发送开关；关掉后回车换行、Ctrl+Enter 发送。
    const wantSend = e.ctrlKey || sendOnEnter()
    if (!wantSend) return
    e.preventDefault()
    submit()
  }
}

// 运行中的发送方式：插话（下一轮立刻生效）或排队（排在队尾慢慢来）。
// 后端两条路共用一个队列，语义差异由这里的按钮表达。
const mode = ref<'steer' | 'queue'>('steer')
watch(
  () => props.running,
  (on) => {
    if (!on) mode.value = 'steer'
  },
)

function submit() {
  const value = text.value.trim()
  if (props.disabled || props.sending) return
  if (props.running) {
    if (!value) return
    if (mode.value === 'queue') emit('queue', value)
    else emit('steer', value)
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
  if (full.value) {
    toast.bad(`一次最多引用 ${MAX_ATTACH} 个文件`)
    return
  }
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
  const all = Array.from(e.clipboardData?.files || [])
  const imgs = all.filter((f) => f.type.startsWith('image/'))
  if (!imgs.length) {
    // 复制的是文件而不是图片：不装作收到了，告诉用户该用 @ 引用
    if (all.length) toast.bad('只支持直接粘贴图片；引用文件请用 @ 或回形针按钮')
    return
  }
  e.preventDefault()
  for (const f of imgs) {
    if (full.value) {
      toast.bad(`一次最多引用 ${MAX_ATTACH} 个文件`)
      break
    }
    const reader = new FileReader()
    reader.onload = () => {
      const dataUrl = String(reader.result || '')
      const base64 = dataUrl.slice(dataUrl.indexOf(',') + 1)
      if (!base64) return
      files.value.push({
        path: '',
        // 本地时间命名：UTC 会差 8 小时，截图名对不上操作时刻
        name: f.name || `截图-${new Date().toLocaleTimeString('zh-CN', { hour12: false }).replace(/:/g, '')}.png`,
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
        <AppIcon name="info" size="ic-xs" />
        {{ mode === 'queue' ? '这条会排进队列，等当前批次处理完再生效' : '运行中：发送会作为插话，按 Esc 停下' }}
      </div>

      <!-- 排队条：与后端队列一一对应的本地视图，注入时刻随 chat:user 消掉 -->
      <div v-if="chat.queued.length" class="queue-dock">
        <div class="qd-head">
          <AppIcon name="clock" size="ic-xs" />
          已排队 {{ chat.queued.length }} 条 · 会在轮间自动生效
        </div>
        <div v-for="(q, i) in chat.queued" :key="`${i}-${q}`" class="qd-item">{{ q }}</div>
      </div>

      <div class="composer-in">
        <textarea
          ref="ta"
          v-model="text"
          class="ta"
          rows="1"
          :disabled="disabled"
          :placeholder="placeholder || '有什么要帮忙的？直接说就行，打 @ 选文件，打 / 用技能，可粘贴截图'"
          @keydown="onKeydown"
          @input="onInput"
          @paste="onPaste"
        />
      </div>

      <div class="composer-bar">
        <WorkspaceChip />
        <span v-if="!running" class="hint">{{ sendHint }}</span>
        <SessionChips ref="chipsRef" />
        <ContextMeter
          :used="chat.contextUsed"
          :win="chat.contextWindow"
          :ratio="chat.contextRatio"
          :known="chat.contextKnown"
          :reserve="chat.contextReserve"
        />
        <span v-if="outsideCount" class="warn">{{ outsideCount }} 个文件助手读不到</span>
        <span class="sp" />
        <div v-if="running" class="seg" role="group" aria-label="发送方式">
          <button type="button" :class="{ 'is-on': mode === 'steer' }" @click="mode = 'steer'">插话</button>
          <button type="button" :class="{ 'is-on': mode === 'queue' }" @click="mode = 'queue'">排队</button>
        </div>
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

      <!-- 斜杠菜单：命令 + 技能，↑↓ 选择、回车执行、Esc 关闭 -->
      <div v-if="menu === 'slash'" class="pop menu-pop">
        <div v-if="!slashItems.length" class="pop-row muted">没有匹配的命令或技能</div>
        <template v-else>
          <button
            v-for="(it, i) in slashItems"
            :key="`${it.kind}-${it.key}`"
            class="pop-row"
            :class="{ 'is-hi': i === cmdIndex }"
            type="button"
            @mouseenter="cmdIndex = i"
            @click="runSlash(it)"
          >
            <span v-if="it.kind === 'cmd'" class="cm">/{{ it.key }}</span>
            <AppIcon v-else name="sparkles" size="ic-xs" />
            <span class="grow">
              <b class="pr-t">{{ it.kind === 'cmd' ? it.name : it.key }}</b>
              <span class="pr-d">{{ it.desc }}</span>
            </span>
            <span v-if="it.kind === 'skill'" class="tag">技能</span>
          </button>
        </template>
      </div>

      <!-- @ 引用：工作区文件逐级浏览点选 -->
      <div v-else-if="menu === 'mention'" class="pop menu-pop">
        <div class="mb-head">
          <button
            class="icon-btn is-sm"
            type="button"
            title="上一级"
            :disabled="!browsePath || browseLoading"
            @click="browseUp"
          >
            <AppIcon name="chevron-left" size="ic-xs" />
          </button>
          <div class="crumbs">
            <template v-for="(cr, i) in browseCrumbs" :key="cr.path || '__root__'">
              <span v-if="i" class="crumb-sep">/</span>
              <button
                class="crumb"
                :class="{ 'is-cur': i === browseCrumbs.length - 1 }"
                type="button"
                @click="loadDir(cr.path)"
              >
                {{ cr.name }}
              </button>
            </template>
          </div>
          <input v-model="browseFilter" class="input input-sm mb-filter" placeholder="过滤" />
        </div>
        <div class="mb-list">
          <div v-if="browseLoading" class="pop-row muted">读取中…</div>
          <div v-else-if="!browseShown.length" class="pop-row muted">
            {{ browseFilter ? '没有匹配的文件' : '这个目录是空的' }}
          </div>
          <button
            v-for="e in browseShown"
            :key="e.name"
            class="pop-row"
            type="button"
            @click="pickFromTree(e)"
          >
            <AppIcon :name="e.dir ? 'folder' : 'doc'" size="ic-xs" />
            <span class="grow"><b class="pr-t">{{ e.name }}</b></span>
            <AppIcon v-if="e.dir" name="chevron-right" size="ic-xs" />
          </button>
        </div>
        <button class="pop-row mb-out" type="button" :disabled="full" @click="pickFile">
          <AppIcon name="search" size="ic-xs" />
          <span class="grow">
            <b class="pr-t">浏览整个电脑…</b>
            <span class="pr-d">工作区之外的文件助手读不到</span>
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
  width: var(--wb-ctl-icon);
  height: var(--wb-ctl-icon);
  border-radius: var(--wb-radius-full);
  color: var(--wb-muted);
  transition: color var(--wb-dur-fast) var(--wb-ease), transform var(--wb-dur-fast) var(--wb-ease);
}
.attach .x:hover:not(:disabled) {
  color: var(--wb-danger);
  transform: scale(1.08);
}
.attach .x:disabled {
  opacity: 0.45;
  cursor: not-allowed;
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
/* @ 引用面板：路径行与底部的「整个电脑」固定，中间列表滚动 */
.menu-pop:has(.mb-list) {
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
}
.mb-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-2) var(--wb-sp-1);
  border-bottom: 1px solid var(--wb-line);
  flex: none;
}
.crumbs {
  display: flex;
  align-items: center;
  gap: 2px;
  min-width: 0;
  overflow: hidden;
  flex: 1;
}
.crumb {
  border: 0;
  background: transparent;
  padding: 2px 4px;
  border-radius: var(--wb-radius-sm);
  font-size: var(--wb-fs-xs);
  font-weight: 600;
  color: var(--wb-muted);
  white-space: nowrap;
  cursor: pointer;
  transition: background var(--wb-dur-fast) var(--wb-ease), color var(--wb-dur-fast) var(--wb-ease);
}
.crumb:hover:not(:disabled) {
  background: var(--wb-tint);
  color: var(--wb-ink);
}
.crumb.is-cur {
  color: var(--wb-ink);
}
.crumb-sep {
  color: var(--wb-muted);
  font-size: var(--wb-fs-xs);
}
.mb-filter {
  width: 88px;
  height: var(--wb-ctl-h-sm);
  font-size: var(--wb-fs-xs);
  flex: none;
}
.mb-list {
  overflow-y: auto;
  min-height: 0;
  flex: 1;
  padding: var(--wb-sp-1) var(--wb-sp-1) var(--wb-sp-2);
}
.mb-out {
  border-top: 1px solid var(--wb-line);
  flex: none;
  border-radius: 0;
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
