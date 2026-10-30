<script setup lang="ts">
// 输入区：多行输入 + 待发送附件 + @ 引用 + / 命令菜单 + 模型与思考档 + 发送 / 停止。
// 运行中发送即插话（steer）或排队（follow_up），Esc 在输入框为空时停下当前回合。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import * as api from '../../api'
import { pickFile, pickFileSave } from '../../api/shell'
import type { AttachmentREQ, FileEntryVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import ContextMeter from './ContextMeter.vue'
import SessionChips from './SessionChips.vue'
import WorkspaceChip from './WorkspaceChip.vue'

const props = defineProps<{
  running: boolean
  sending?: boolean
  stopping?: boolean
  /** 运行中正在发的是哪一类：'steer' / 'queue' / '' */
  acting?: 'steer' | 'queue' | ''
  disabled?: boolean
  placeholder?: string
}>()
const emit = defineEmits<{
  send: [text: string, attachments: AttachmentREQ[]]
  steer: [text: string]
  queue: [text: string]
  stop: []
  newSession: []
}>()

const { t } = useI18n()
const router = useRouter()
const chat = useChatStore()
const session = useSessionStore()
const settings = useSettingsStore()
const toast = useToastStore()

interface PendingFile {
  path: string
  name: string
  outside: boolean
  image?: string
}

// 内置命令：都是 WorkBaby 真的能执行的动作，不是空壳条目。
type BuiltinKey =
  | 'new'
  | 'model'
  | 'thinking'
  | 'compact'
  | 'session'
  | 'export'
  | 'copy'
  | 'reload'
  | 'login'
  | 'settings'

const COMMANDS: { key: BuiltinKey; nameKey: string; descKey: string }[] = [
  { key: 'new', nameKey: 'chat.slashNew', descKey: 'chat.slashNewDesc' },
  { key: 'model', nameKey: 'chat.slashModel', descKey: 'chat.slashModelDesc' },
  { key: 'thinking', nameKey: 'chat.slashThinking', descKey: 'chat.slashThinkingDesc' },
  { key: 'compact', nameKey: 'chat.slashCompact', descKey: 'chat.slashCompactDesc' },
  { key: 'session', nameKey: 'chat.slashSession', descKey: 'chat.slashSessionDesc' },
  { key: 'export', nameKey: 'chat.slashExport', descKey: 'chat.slashExportDesc' },
  { key: 'copy', nameKey: 'chat.slashCopy', descKey: 'chat.slashCopyDesc' },
  { key: 'reload', nameKey: 'chat.slashReload', descKey: 'chat.slashReloadDesc' },
  { key: 'login', nameKey: 'chat.slashLogin', descKey: 'chat.slashLoginDesc' },
  { key: 'settings', nameKey: 'chat.slashSettings', descKey: 'chat.slashSettingsDesc' },
]

// ── 草稿：按会话 id 存，无会话用 '__new__'（新建态） ──
const DRAFT_KEY = 'wb-composer-drafts'
const drafts: Record<string, string> = (() => {
  try {
    return JSON.parse(localStorage.getItem(DRAFT_KEY) || '{}') as Record<string, string>
  } catch {
    return {}
  }
})()

const text = ref('')
let draftFor = ''

function saveDraft() {
  const key = session.currentId || '__new__'
  if (text.value.trim()) drafts[key] = text.value
  else delete drafts[key]
  draftFor = key
  try {
    const keys = Object.keys(drafts)
    if (keys.length > 20) delete drafts[keys[0]]
    localStorage.setItem(DRAFT_KEY, JSON.stringify(drafts))
  } catch {
    /* 配额 / 隐私模式 */
  }
}

function loadDraft() {
  const key = session.currentId || '__new__'
  if (draftFor === key) return
  draftFor = key
  text.value = drafts[key] || ''
}

watch(() => session.currentId, loadDraft)

const files = ref<PendingFile[]>([])
const outsideCount = computed(() => files.value.filter((f) => f.outside).length)

// ── 菜单：斜杠命令 / @ 引用 ──
const menu = ref<'slash' | 'mention' | null>(null)
const cmdIndex = ref(0)
const root = ref<HTMLElement | null>(null)
const ta = ref<{ focus: () => void; textarea?: HTMLTextAreaElement; ref?: HTMLTextAreaElement } | null>(null)

interface SlashItem {
  kind: 'cmd' | 'skill'
  key: string
  name: string
  desc: string
}

const MAX_MATCH = 20
const slashItems = computed<SlashItem[]>(() => {
  const q = menu.value === 'slash' ? text.value : ''
  if (!q || /\s/.test(q)) return []
  const kw = q.slice(1).toLowerCase()
  const match = (name: string, desc: string) =>
    !kw || name.toLowerCase().includes(kw) || desc.toLowerCase().includes(kw)
  const cmds: SlashItem[] = COMMANDS.filter((c) => match(c.key, t(c.descKey))).map((c) => ({
    kind: 'cmd',
    key: c.key,
    name: t(c.nameKey),
    desc: t(c.descKey),
  }))
  const skills: SlashItem[] = settings.skills
    .filter((s) => s.skill_enabled)
    .filter((s) => match(s.skill_name, s.skill_description))
    .slice(0, MAX_MATCH)
    .map((s) => ({
      kind: 'skill',
      key: s.skill_name,
      name: s.skill_name,
      desc: s.skill_description,
    }))
  return [...cmds, ...skills]
})

const chipsRef = ref<InstanceType<typeof SessionChips> | null>(null)
const slashList = ref<HTMLElement | null>(null)
/** 正在执行的内置命令：执行中不接受第二条命令 */
const busyCmd = ref('')

function runSlash(item: SlashItem) {
  if (item.kind === 'cmd') {
    void runCommand(item.key as BuiltinKey)
    return
  }
  clearInput()
  text.value = `/skill:${item.key} `
  nextTick(() => ta.value?.focus())
}

async function runCommand(key: BuiltinKey) {
  if (busyCmd.value) return
  clearInput()
  switch (key) {
    case 'new':
      emit('newSession')
      return
    case 'model':
      chipsRef.value?.show('model')
      return
    case 'thinking':
      chipsRef.value?.show('thinking')
      return
    case 'compact':
      await compact()
      return
    case 'session':
      showSessionInfo()
      return
    case 'export':
      await exportHTML()
      return
    case 'copy':
      await copyLastAnswer()
      return
    case 'reload':
      await reloadAll()
      return
    case 'login':
      void router.push({ name: 'settings', params: { tab: 'providers' } })
      return
    case 'settings':
      void router.push({ name: 'settings' })
      return
  }
}

// ── 内置命令的实际动作 ──

async function compact() {
  const sid = session.currentId
  if (!sid) {
    toast.info(t('chat.slashNeedSession'))
    return
  }
  if (chat.running) {
    toast.bad(t('chat.slashBusy'))
    return
  }
  busyCmd.value = 'compact'
  try {
    await api.chat.compact(sid)
    toast.ok(t('chat.slashCompactDone'))
    await session.refresh()
  } catch (e) {
    toast.bad(t('chat.slashFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busyCmd.value = ''
  }
}

function showSessionInfo() {
  const s = session.current
  if (!s) {
    toast.info(t('chat.slashNeedSession'))
    return
  }
  toast.info(
    t('chat.slashSessionInfo', {
      n: s.session_message_count,
      tokens: s.session_total_tokens,
      model: s.session_model || '—',
    }),
  )
}

async function exportHTML() {
  const sid = session.currentId
  if (!sid) {
    toast.info(t('chat.slashNeedSession'))
    return
  }
  busyCmd.value = 'export'
  try {
    const name = `${(session.current?.session_title || 'session').replace(/[\\/:*?"<>|]/g, '_')}.html`
    const target = await pickFileSave(t('chat.slashExportTitle'), name, '*.html')
    if (!target) return
    const saved = await api.sessions.exportHTML(sid, target)
    toast.ok(t('chat.slashExportDone', { path: saved }))
  } catch (e) {
    toast.bad(t('chat.slashFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busyCmd.value = ''
  }
}

async function copyLastAnswer() {
  const last = [...session.messages]
    .reverse()
    .find((m) => m.message_role === 'assistant' && m.message_content?.trim())
  if (!last?.message_content) {
    toast.info(t('chat.slashCopyEmpty'))
    return
  }
  try {
    await navigator.clipboard.writeText(last.message_content)
    toast.ok(t('chat.slashCopyDone'))
  } catch {
    toast.bad(t('common.copyFailed'))
  }
}

async function reloadAll() {
  busyCmd.value = 'reload'
  try {
    await settings.loadSkills()
    toast.ok(t('chat.slashReloadDone'))
  } catch (e) {
    toast.bad(t('chat.slashFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busyCmd.value = ''
  }
}

// ── @ 引用：工作区文件逐级浏览 ──
const browsePath = ref('')
const browseEntries = ref<FileEntryVO[]>([])
const browseLoading = ref(false)
const browseFilter = ref('')
const browseLoaded = ref(false)

const browseCrumbs = computed(() => {
  const parts = browsePath.value ? browsePath.value.split('/').filter(Boolean) : []
  return [
    { name: t('chat.mentionRoot'), path: '' },
    ...parts.map((p, i) => ({ name: p, path: parts.slice(0, i + 1).join('/') })),
  ]
})
const browseShown = computed(() => {
  const q = browseFilter.value.trim().toLowerCase()
  return q
    ? browseEntries.value.filter((e) => e.entry_name.toLowerCase().includes(q))
    : browseEntries.value
})

// 目录请求用序号判新旧：连点两个目录时两次请求并发返回，只看 loading 会错位
let browseSeq = 0
async function loadDir(path: string) {
  const seq = ++browseSeq
  browseLoading.value = true
  browsePath.value = path
  browseFilter.value = ''
  try {
    const entries = (await api.files.list(path)).resp_entries
    if (seq !== browseSeq) return
    browseEntries.value = entries ?? []
    browseLoaded.value = true
  } catch (e) {
    if (seq !== browseSeq) return
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
    browseEntries.value = []
  } finally {
    if (seq === browseSeq) browseLoading.value = false
  }
}

function pickFromTree(entry: FileEntryVO) {
  if (entry.entry_is_dir) {
    void loadDir(browsePath.value ? `${browsePath.value}/${entry.entry_name}` : entry.entry_name)
    return
  }
  const rel = browsePath.value ? `${browsePath.value}/${entry.entry_name}` : entry.entry_name
  if (!files.value.some((f) => f.path === rel)) {
    files.value.push({ path: rel, name: entry.entry_name, outside: false })
  }
  text.value = text.value.replace(/@\S*$/, '').trimEnd()
  menu.value = null
  saveDraft()
  nextTick(() => ta.value?.focus())
}

function browseUp() {
  const idx = browsePath.value.lastIndexOf('/')
  void loadDir(idx > 0 ? browsePath.value.slice(0, idx) : '')
}

// ── 附件 ──
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

function attachPath(picked: string) {
  const base = settings.boot?.boot_workspace || ''
  const rel = base ? toRelative(base, picked) : null
  if (rel === null) {
    files.value.push({ path: picked, name: baseName(picked), outside: true })
    return
  }
  if (files.value.some((f) => f.path === rel)) return
  files.value.push({ path: rel, name: baseName(rel), outside: false })
}

async function pickAttachment() {
  menu.value = null
  text.value = text.value.replace(/@\S*$/, '').trimEnd()
  const picked = await pickFile('', '')
  if (!picked) return
  attachPath(picked)
  nextTick(() => ta.value?.focus())
}

function removeFile(path: string) {
  files.value = files.value.filter((f) => f.path !== path)
}

// 粘贴图片：截图后 Ctrl+V 直接进附件列表
function onPaste(e: ClipboardEvent) {
  const all = Array.from(e.clipboardData?.files || [])
  const imgs = all.filter((f) => f.type.startsWith('image/'))
  if (!imgs.length) {
    if (all.length) toast.bad(t('chat.attachPickHint'))
    return
  }
  e.preventDefault()
  for (const f of imgs) {
    const reader = new FileReader()
    reader.onload = () => {
      const dataUrl = String(reader.result || '')
      const base64 = dataUrl.slice(dataUrl.indexOf(',') + 1)
      if (!base64) return
      files.value.push({
        path: '',
        name: f.name || `paste-${Date.now()}.png`,
        outside: false,
        image: base64,
      })
    }
    reader.readAsDataURL(f)
  }
}

// ── 输入与键盘 ──
const mode = ref<'steer' | 'queue'>('steer')
const sendOnEnter = () => settings.values['send_on_enter'] !== 'false'
const sendHint = computed(() =>
  sendOnEnter() ? t('chat.enterToSend') : t('chat.ctrlToSend'),
)

watch(
  () => props.running,
  (on) => {
    if (!on) mode.value = 'steer'
    nextTick(() => ta.value?.focus())
  },
)

function clearInput() {
  text.value = ''
  saveDraft()
  menu.value = null
}

function onInput() {
  saveDraft()
  const v = text.value
  if (/@\S*$/.test(v)) {
    menu.value = 'mention'
    if (!browseLoaded.value && !browseLoading.value) void loadDir('')
  } else if (v.startsWith('/') && !/\s/.test(v)) {
    menu.value = 'slash'
    cmdIndex.value = 0
    if (!settings.skills.length && !settings.skillsLoading) void settings.loadSkills()
  } else {
    menu.value = null
  }
}

// 键盘高亮必须跟手：列表超过可视高度时，光标跑下去了但看不见选中项等于导航失效。
// 用容器自己的 scrollTop 算，不用 scrollIntoView——后者会把外层页面也一起滚。
function ensureSlashVisible(i: number) {
  const box = slashList.value
  if (!box) return
  const el = box.querySelector<HTMLElement>(`[data-idx="${i}"]`)
  if (!el) return
  const top = el.offsetTop - box.offsetTop
  const bottom = top + el.offsetHeight
  if (top < box.scrollTop) box.scrollTop = top
  else if (bottom > box.scrollTop + box.clientHeight) box.scrollTop = bottom - box.clientHeight
}

// 菜单里用鼠标滚轮 / 拖滚动条 / 点选项时不能让输入框丢焦点：
// 一旦焦点跑到按钮上，↑↓ 与回车就都不再送到输入框了。
function keepFocus(e: PointerEvent) {
  const el = e.target as HTMLElement | null
  if (el?.closest('input, textarea')) return
  e.preventDefault()
}

const mentionIndex = ref(0)
const mentionList = ref<HTMLElement | null>(null)
watch(
  () => browsePath.value,
  () => (mentionIndex.value = 0),
)
watch(browseFilter, () => (mentionIndex.value = 0))

function ensureMentionVisible(i: number) {
  const box = mentionList.value
  if (!box) return
  const rows = box.querySelectorAll<HTMLElement>('.pop-row')
  const el = rows[i]
  if (!el) return
  const top = el.offsetTop - box.offsetTop
  const bottom = top + el.offsetHeight
  if (top < box.scrollTop) box.scrollTop = top
  else if (bottom > box.scrollTop + box.clientHeight) box.scrollTop = bottom - box.clientHeight
}

function onKeydown(e: KeyboardEvent) {
  if (menu.value === 'slash' && slashItems.value.length) {
    const n = slashItems.value.length
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      cmdIndex.value = (cmdIndex.value + 1) % n
      ensureSlashVisible(cmdIndex.value)
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      cmdIndex.value = (cmdIndex.value - 1 + n) % n
      ensureSlashVisible(cmdIndex.value)
      return
    }
    if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
      e.preventDefault()
      runSlash(slashItems.value[Math.min(cmdIndex.value, n - 1)])
      return
    }
  }
  if (menu.value === 'mention' && browseShown.value.length) {
    const n = browseShown.value.length
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      mentionIndex.value = (mentionIndex.value + 1) % n
      ensureMentionVisible(mentionIndex.value)
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      mentionIndex.value = (mentionIndex.value - 1 + n) % n
      ensureMentionVisible(mentionIndex.value)
      return
    }
    if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
      e.preventDefault()
      pickFromTree(browseShown.value[Math.min(mentionIndex.value, n - 1)])
      return
    }
  }
  if (e.key === 'Escape') {
    if (menu.value) {
      menu.value = null
      return
    }
    // 运行中按 Esc = 停止；只在输入框为空时响应，免得误按清掉上下文
    if (props.running && !text.value.trim()) {
      e.preventDefault()
      emit('stop')
    }
    return
  }
  if (e.key === 'Enter' && !e.isComposing) {
    if (e.shiftKey) return
    const wantSend = e.ctrlKey || sendOnEnter()
    if (!wantSend) return
    e.preventDefault()
    submit()
  }
}

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
  if (!value && !files.value.length) return
  if (outsideCount.value) return
  emit(
    'send',
    value,
    files.value.map((f) => ({
      attach_path: f.path,
      attach_name: f.name,
      attach_image_base64: f.image,
    })),
  )
  clearInput()
  files.value = []
}

const canSend = computed(() => {
  if (props.disabled || props.sending) return false
  if (props.running) return text.value.trim().length > 0
  return (text.value.trim().length > 0 || files.value.length > 0) && outsideCount.value === 0
})

function onDocPointerDown(e: PointerEvent) {
  if (root.value?.contains(e.target as Node)) return
  menu.value = null
}

onMounted(() => {
  loadDraft()
  document.addEventListener('pointerdown', onDocPointerDown)
})
onBeforeUnmount(() => document.removeEventListener('pointerdown', onDocPointerDown))

defineExpose({
  focus: () => ta.value?.focus(),
  attachPath,
})
</script>

<template>
  <div ref="root" class="composer">
    <div class="composer-in">
      <div class="composer-box">
        <!-- 待发送文件：数量不设上限，超过两行就在本区内部滚动，不把输入框顶出屏 -->
        <div v-if="files.length" class="attach-row">
          <span
            v-for="(f, i) in files"
            :key="`${f.path}-${i}`"
            class="attach"
            :class="{ 'is-bad': f.outside }"
            :title="f.outside ? t('chat.attachOutside') : f.image ? f.name : f.path"
          >
            <AppIcon :name="f.image ? 'image' : 'doc'" size="ic-2xs" />
            <span class="attach-n">{{ f.name }}</span>
            <button class="attach-x" type="button" :title="t('chat.attachRemove')" @click="removeFile(f.path)">
              <AppIcon name="close" size="ic-2xs" />
            </button>
          </span>
          <span class="attach-tip">{{ files.length }}</span>
        </div>

        <!-- 运行态提示 -->
        <div v-if="running" class="run-hint">
          <AppIcon name="info" size="ic-2xs" />
          <span>{{ mode === 'queue' ? t('chat.queueHint') : t('chat.steerHint') }}</span>
        </div>

        <!-- 排队条 -->
        <div v-if="chat.queued.length" class="queue-dock">
          <div class="qd-head">
            <AppIcon name="clock" size="ic-2xs" />
            {{ t('chat.queuedTitle', { n: chat.queued.length }) }}
          </div>
          <div v-for="(q, i) in chat.queued" :key="`${i}-${q}`" class="qd-item">{{ q }}</div>
        </div>

        <!-- 输入块：外层给 min-height，空态也有一个大目标可点；
             焦点态在 content.css 里加柔光圈，这里只负责层级。 -->
        <div class="ta-shell">
          <el-input
            ref="ta"
            v-model="text"
            class="ta"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 10 }"
            :disabled="disabled"
            :placeholder="placeholder || t('chat.inputPlaceholder')"
            @keydown="onKeydown"
            @input="onInput"
            @paste="onPaste"
          />
        </div>

        <!-- 工具行：与输入块用一条分隔线切开，不再抢输入框的横向空间 -->
        <div class="composer-bar is-divided">
          <el-tooltip :content="t('chat.attachPick')" placement="top" :show-after="400">
            <button class="bar-btn" type="button" :disabled="disabled" @click="pickAttachment">
              <AppIcon name="folder" size="ic-xs" />
            </button>
          </el-tooltip>
          <WorkspaceChip />
          <SessionChips ref="chipsRef" />
          <ContextMeter
            :used="chat.contextUsed"
            :win="chat.contextWindow"
            :ratio="chat.contextRatio"
            :known="chat.contextKnown"
            :reserve="chat.contextReserve"
            :session-id="session.currentId || ''"
          />
          <span v-if="outsideCount" class="bar-warn">
            {{ t('chat.attachOutsideCount', { n: outsideCount }) }}
          </span>
          <span v-if="!running" class="bar-hint">{{ sendHint }}</span>
          <span class="bar-sp" />

          <el-radio-group v-if="running" v-model="mode" size="small" :disabled="!!acting">
            <el-radio-button value="steer">{{ t('chat.steer') }}</el-radio-button>
            <el-radio-button value="queue">{{ t('chat.queue') }}</el-radio-button>
          </el-radio-group>

          <el-button
            v-if="running"
            size="small"
            type="danger"
            plain
            :loading="stopping"
            :disabled="stopping"
            @click="emit('stop')"
          >
            {{ t('chat.stop') }}
          </el-button>
          <el-button
            v-else
            size="small"
            type="primary"
            :loading="sending"
            :disabled="!canSend"
            @click="submit"
          >
            <AppIcon v-if="!sending" name="chevron-up" size="ic-xs" class="send-ic" />
            {{ t('chat.send') }}
          </el-button>
        </div>
      </div>

      <!-- 斜杠菜单 -->
      <div v-if="menu === 'slash'" class="pop menu-pop" @pointerdown="keepFocus">
        <div v-if="settings.skillsLoading && !settings.skills.length" class="pop-row is-muted">
          <span class="pop-spin" />
          {{ t('chat.slashLoading') }}
        </div>
        <div v-else-if="!slashItems.length" class="pop-row is-muted">{{ t('chat.slashNoMatch') }}</div>
        <div v-else ref="slashList" class="pop-list">
          <button
            v-for="(it, i) in slashItems"
            :key="`${it.kind}-${it.key}`"
            :data-idx="i"
            class="pop-row"
            :class="{ 'is-hi': i === cmdIndex }"
            type="button"
            @mouseenter="cmdIndex = i"
            @click="runSlash(it)"
          >
            <AppIcon v-if="it.kind === 'skill'" name="sparkles" size="ic-xs" />
            <AppIcon v-else name="terminal" size="ic-xs" />
            <span class="pop-grow">
              <b class="pop-t">{{ it.kind === 'cmd' ? `/${it.key}` : it.name }}</b>
              <span class="pop-d">{{ it.kind === 'cmd' ? it.name + ' · ' + it.desc : it.desc }}</span>
            </span>
            <span v-if="it.kind === 'skill'" class="pop-tag">{{ t('chat.slashTagSkill') }}</span>
            <span v-else class="pop-tag is-builtin">{{ t('chat.slashTagBuiltin') }}</span>
          </button>
        </div>
      </div>

      <!-- @ 引用 -->
      <div v-else-if="menu === 'mention'" class="pop menu-pop is-mention" @pointerdown="keepFocus">
        <div class="mb-head">
          <button class="mb-btn" type="button" :title="t('chat.mentionUp')" :disabled="!browsePath" @click="browseUp">
            <AppIcon name="chevron-left" size="ic-xs" />
          </button>
          <div class="crumbs">
            <template v-for="(cr, i) in browseCrumbs" :key="cr.path || '__root__'">
              <span v-if="i" class="crumb-sep">/</span>
              <button class="crumb" :class="{ 'is-cur': i === browseCrumbs.length - 1 }" type="button" @click="loadDir(cr.path)">
                {{ cr.name }}
              </button>
            </template>
          </div>
          <input v-model="browseFilter" class="mb-filter" :placeholder="t('chat.mentionFilter')" />
        </div>
        <div ref="mentionList" class="mb-list">
          <div v-if="browseLoading" class="pop-row is-muted"><span class="pop-spin" />{{ t('common.loading') }}</div>
          <div v-else-if="!browseShown.length" class="pop-row is-muted">
            {{ browseFilter ? t('chat.mentionNoMatch') : t('chat.mentionEmpty') }}
          </div>
          <button
            v-for="(e, i) in browseShown"
            :key="e.entry_name"
            class="pop-row"
            :class="{ 'is-hi': i === mentionIndex }"
            type="button"
            @mouseenter="mentionIndex = i"
            @click="pickFromTree(e)"
          >
            <AppIcon :name="e.entry_is_dir ? 'folder' : 'doc'" size="ic-xs" />
            <span class="pop-grow"><b class="pop-t">{{ e.entry_name }}</b></span>
            <AppIcon v-if="e.entry_is_dir" name="chevron-right" size="ic-xs" />
          </button>
        </div>
        <button class="pop-row mb-out" type="button" @click="pickAttachment">
          <AppIcon name="search" size="ic-xs" />
          <span class="pop-grow">
            <b class="pop-t">{{ t('chat.attachPick') }}</b>
            <span class="pop-d">{{ t('chat.attachPickHint') }}</span>
          </span>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.attach-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3) 0;
  /* 数量不设上限：超过两行就在本区内部滚动，不把输入框顶出屏 */
  max-height: calc(var(--wb-ctl-h-sm) * 2 + var(--wb-sp-4));
  overflow-y: auto;
  overscroll-behavior: contain;
}
.attach {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  height: var(--wb-ctl-h-sm);
  padding: 0 var(--wb-sp-1) 0 var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-hover);
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
  max-width: calc(220px * var(--wb-vw-scale));
}
.attach.is-bad {
  border-color: var(--wb-danger);
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
.attach-n {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.attach-x {
  display: grid;
  place-items: center;
  width: calc(18px * var(--wb-vw-scale));
  height: calc(18px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  color: var(--wb-muted);
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.attach-x:hover {
  background: var(--wb-tint-lg);
  color: var(--wb-danger);
  transform: scale(1.06);
}
.attach-x:active {
  transform: scale(0.92);
}
.attach-tip {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.run-hint {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3) 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.queue-dock {
  margin: var(--wb-sp-2) var(--wb-sp-3) 0;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-hover);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.qd-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-muted);
}
.qd-item {
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* 输入块：撑满高度并让 textarea 也撑满，点击空白区即可落光标 */
.ta-shell {
  display: flex;
  min-height: calc(68px * var(--wb-vw-scale));
  padding: var(--wb-sp-1);
}
.ta {
  flex: 1;
  min-width: 0;
  display: flex;
}
.ta :deep(.el-textarea__inner) {
  border: none;
  box-shadow: none;
  background: transparent;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  font-size: var(--wb-fs-md);
  max-height: calc(240px * var(--wb-vw-scale));
  resize: none;
}
.ta :deep(.el-textarea__inner:focus) {
  box-shadow: none;
}
/* 工具行与输入块用一条分隔线切开 */
.composer-bar.is-divided {
  margin-top: var(--wb-sp-1);
  padding-top: var(--wb-sp-2);
  border-top: 1px solid var(--wb-border);
}
.composer-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3) var(--wb-sp-3);
  /* 工具行不是可复制的正文：拖选输入框里的字时扫过这排，浏览器会把 chip 与水位环
     一起框成一片淡蓝色块。只有输入框里的文字该被选中。 */
  user-select: none;
}
.bar-hint,
.bar-warn {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  white-space: nowrap;
}
.bar-btn {
  display: grid;
  place-items: center;
  width: var(--wb-ctl-h-sm);
  height: var(--wb-ctl-h-sm);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  color: var(--wb-muted);
  flex: none;
  transition:
    background var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.bar-btn:hover:not(:disabled) {
  background: var(--wb-elev-hover);
  border-color: var(--wb-border-strong);
  color: var(--wb-ink);
  transform: scale(1.06);
}
.bar-btn:active:not(:disabled) {
  transform: scale(0.92);
}
.bar-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.bar-warn {
  color: var(--wb-danger);
  font-weight: 600;
}
.bar-sp {
  flex: 1;
  min-width: var(--wb-sp-1);
}
.send-ic {
  margin-right: 2px;
}

/* 菜单面板：EP 弹层同款描边与阴影，含键盘导航与目录浏览，不是表单基元 */
.pop {
  position: absolute;
  left: var(--wb-sp-3);
  right: var(--wb-sp-3);
  bottom: calc(100% - var(--wb-sp-2));
  max-height: calc(340px * var(--wb-vw-scale));
  background: var(--wb-elev-raised);
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-lg);
  box-shadow: var(--wb-shadow-3);
  padding: var(--wb-sp-1);
  animation: pop-in var(--wb-dur) var(--wb-ease);
  z-index: var(--wb-z-pop);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
@keyframes pop-in {
  from {
    opacity: 0;
    transform: translateY(4px);
  }
}
/* 滚动在列表层而不是弹层上：键盘高亮要靠它自己的 scrollTop 算可视区 */
.pop-list {
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
}
.menu-pop.is-mention {
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
}
.pop-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-sm);
  text-align: left;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}
.pop-row:hover:not(:disabled),
.pop-row.is-hi:not(:disabled) {
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
}
.pop-row:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.pop-row.is-muted {
  color: var(--wb-muted);
  cursor: default;
}
.pop-grow {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}
.pop-t {
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pop-row.is-hi .pop-t,
.pop-row:hover .pop-t {
  color: var(--wb-primary);
}
.pop-d {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pop-tag {
  flex: none;
  padding: 1px var(--wb-sp-1);
  border-radius: var(--wb-radius-xs);
  background: var(--wb-elev-hover);
  color: var(--wb-muted);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
}
.pop-tag.is-builtin {
  background: var(--wb-elev-raised);
  border: 1px solid var(--wb-border);
  color: var(--wb-muted);
}
.pop-spin {
  width: calc(12px * var(--wb-vw-scale));
  height: calc(12px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  border: 1.6px solid var(--wb-border-strong);
  border-top-color: var(--wb-primary);
  animation: wb-spin 0.8s linear infinite;
  flex: none;
}
.mb-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2);
  border-bottom: 1px solid var(--wb-border);
  flex: none;
}
.mb-btn {
  display: grid;
  place-items: center;
  width: calc(22px * var(--wb-vw-scale));
  height: calc(22px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  color: var(--wb-muted);
}
.mb-btn:hover:not(:disabled) {
  background: var(--wb-elev-hover);
  color: var(--wb-ink);
}
.mb-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
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
  padding: 2px 4px;
  border-radius: var(--wb-radius-xs);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-muted);
  white-space: nowrap;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}
.crumb:hover {
  background: var(--wb-elev-hover);
  color: var(--wb-ink);
}
.crumb.is-cur {
  color: var(--wb-ink);
}
.crumb-sep {
  color: var(--wb-muted);
  font-size: var(--wb-fs-2xs);
}
.mb-filter {
  width: calc(88px * var(--wb-vw-scale));
  height: var(--wb-ctl-h-sm);
  padding: 0 var(--wb-sp-2);
  flex: none;
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-elev-raised);
  color: var(--wb-ink);
  font-size: var(--wb-fs-2xs);
  outline: none;
}
.mb-filter:focus {
  border-color: var(--wb-primary);
}
.mb-list {
  overflow-y: auto;
  min-height: 0;
  flex: 1;
  padding: var(--wb-sp-1);
}
.mb-out {
  border-top: 1px solid var(--wb-border);
  border-radius: 0;
  flex: none;
}
@media (max-width: 1080px) {
  .bar-hint {
    display: none;
  }
}
</style>
