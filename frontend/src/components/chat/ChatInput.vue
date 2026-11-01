<script setup lang="ts">
// 输入区：多行输入 + 待发送附件 + @ 引用 + / 命令菜单；底栏在 ComposerBar。
// 运行中发送即插话（steer）或排队（follow_up），Esc 在输入框为空时停下当前回合。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import { useComposerCommands } from '../../composables/useComposerCommands'
import { useComposerDraft } from '../../composables/useComposerDraft'
import { pickFile } from '../../api/shell'
import type { AttachmentREQ, FileEntryVO } from '../../types/api'
import type { BuiltinKey, PendingFile, SlashItem } from '../../utils/composerMenu'
import AppIcon from '../common/AppIcon.vue'
import ComposerAttach from './ComposerAttach.vue'
import ComposerBar from './ComposerBar.vue'
import ComposerMentionMenu from './ComposerMentionMenu.vue'
import ComposerSlashMenu from './ComposerSlashMenu.vue'

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
const chat = useChatStore()
const session = useSessionStore()
const settings = useSettingsStore()
const toast = useToastStore()

const { text, save: saveDraft, loadCurrent } = useComposerDraft(() => session.currentId)

const barRef = ref<InstanceType<typeof ComposerBar> | null>(null)
const slashRef = ref<InstanceType<typeof ComposerSlashMenu> | null>(null)
const mentionRef = ref<InstanceType<typeof ComposerMentionMenu> | null>(null)

const { run: runCommand } = useComposerCommands({
  newSession: () => emit('newSession'),
  showModel: () => barRef.value?.showChip('model'),
  showThinking: () => barRef.value?.showChip('thinking'),
})

// ── 菜单与键盘高亮 ──
const menu = ref<'slash' | 'mention' | null>(null)
const cmdIndex = ref(0)
const root = ref<HTMLElement | null>(null)
const ta = ref<{ focus: () => void; textarea?: HTMLTextAreaElement; ref?: HTMLTextAreaElement } | null>(
  null,
)

function clearInput() {
  text.value = ''
  saveDraft()
  menu.value = null
}

function runSlash(item: SlashItem) {
  if (item.kind === 'cmd') {
    clearInput()
    void runCommand(item.key as BuiltinKey)
    return
  }
  clearInput()
  text.value = `/skill:${item.key} `
  nextTick(() => ta.value?.focus())
}

// 高亮必须跟手：列表超过可视高度时，光标跑下去了但看不见选中项等于导航失效。
// 用容器自己的 scrollTop 算，不用 scrollIntoView——后者会把外层页面也一起滚。
function ensureVisible(box: HTMLElement | null, i: number) {
  if (!box) return
  const el = box.querySelector<HTMLElement>(`[data-idx="${i}"]`)
  if (!el) return
  const top = el.offsetTop - box.offsetTop
  const bottom = top + el.offsetHeight
  if (top < box.scrollTop) box.scrollTop = top
  else if (bottom > box.scrollTop + box.clientHeight) box.scrollTop = bottom - box.clientHeight
}

// 菜单里用鼠标滚轮 / 点选项时不能让输入框丢焦点：
// 一旦焦点跑到按钮上，↑↓ 与回车就都不再送到输入框了。
function keepFocus(e: PointerEvent) {
  const el = e.target as HTMLElement | null
  if (el?.closest('input, textarea')) return
  e.preventDefault()
}

// ── 附件 ──
const files = ref<PendingFile[]>([])
const outsideCount = computed(() => files.value.filter((f) => f.outside).length)

function baseName(p: string): string {
  const parts = p.split(/[\\/]/)
  return parts[parts.length - 1] || p
}

// 逐段比大小写再拼前缀：盘符大小写与分隔符混用时不至于判错
function toRelative(base: string, fullPath: string): string | null {
  const rootDir = base.replace(/[\\/]+$/, '')
  if (!rootDir) return fullPath
  const key = (p: string) => p.replace(/\//g, '\\').toLowerCase()
  if (!key(fullPath).startsWith(key(rootDir) + '\\')) return null
  return fullPath.slice(rootDir.length + 1)
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

function pickFromTree(entry: FileEntryVO) {
  const rel = mentionRef.value?.relativeOf(entry) || entry.entry_name
  if (!files.value.some((f) => f.path === rel)) {
    files.value.push({ path: rel, name: entry.entry_name, outside: false })
  }
  text.value = text.value.replace(/@\S*$/, '').trimEnd()
  menu.value = null
  saveDraft()
  nextTick(() => ta.value?.focus())
}

async function pickExternal() {
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
const sendHint = computed(() => (sendOnEnter() ? t('chat.enterToSend') : t('chat.ctrlToSend')))

watch(
  () => props.running,
  (on) => {
    if (!on) mode.value = 'steer'
    nextTick(() => ta.value?.focus())
  },
)

function onInput() {
  saveDraft()
  const v = text.value
  if (/@\S*$/.test(v)) {
    menu.value = 'mention'
    void mentionRef.value?.loadDir('')
  } else if (v.startsWith('/') && !/\s/.test(v)) {
    menu.value = 'slash'
    cmdIndex.value = 0
    if (!settings.skills.length && !settings.skillsLoading) void settings.loadSkills()
  } else {
    menu.value = null
  }
}

function onKeydown(e: KeyboardEvent) {
  if (menu.value === 'slash' && slashRef.value?.items.length) {
    const n = slashRef.value.items.length
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      const dir = e.key === 'ArrowDown' ? 1 : -1
      cmdIndex.value = (cmdIndex.value + dir + n) % n
      ensureVisible(root.value?.querySelector<HTMLElement>('.pop-list') ?? null, cmdIndex.value)
      return
    }
    if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
      e.preventDefault()
      const item = slashRef.value.itemAt(cmdIndex.value)
      if (item) runSlash(item)
      return
    }
  }
  if (menu.value === 'mention' && mentionRef.value?.shown.length) {
    const n = mentionRef.value.shown.length
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      const dir = e.key === 'ArrowDown' ? 1 : -1
      const next = (mentionRef.value.currentIndex() + dir + n) % n
      mentionRef.value.setIndex(next)
      ensureVisible(root.value?.querySelector<HTMLElement>('.mb-list') ?? null, next)
      return
    }
    if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
      e.preventDefault()
      mentionRef.value.pickAt(mentionRef.value.currentIndex())
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
    if (!(e.ctrlKey || sendOnEnter())) return
    e.preventDefault()
    submit()
  }
}

const canSend = computed(() => {
  if (props.disabled || props.sending) return false
  if (props.running) return text.value.trim().length > 0
  return (text.value.trim().length > 0 || files.value.length > 0) && outsideCount.value === 0
})

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

function onDocPointerDown(e: PointerEvent) {
  if (root.value?.contains(e.target as Node)) return
  menu.value = null
}

onMounted(() => {
  loadCurrent()
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
        <ComposerAttach :files="files" :outside-count="outsideCount" @remove="removeFile" />

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

        <ComposerBar
          ref="barRef"
          v-model:mode="mode"
          :running="running"
          :sending="sending"
          :stopping="stopping"
          :acting="acting"
          :disabled="disabled"
          :outside-count="outsideCount"
          :can-send="canSend"
          :send-hint="sendHint"
          @pick-file="pickExternal"
          @send="submit"
          @stop="emit('stop')"
        />
      </div>

      <ComposerSlashMenu
        v-if="menu === 'slash'"
        ref="slashRef"
        :query="text"
        :skills="settings.skills"
        :loading="settings.skillsLoading"
        :index="cmdIndex"
        @pick="runSlash"
        @hover="cmdIndex = $event"
        @pointerdown="keepFocus"
      />
      <ComposerMentionMenu
        v-else-if="menu === 'mention'"
        ref="mentionRef"
        @pick="pickFromTree"
        @pick-external="pickExternal"
        @pointerdown="keepFocus"
      />
    </div>
  </div>
</template>

<style scoped>
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
</style>
