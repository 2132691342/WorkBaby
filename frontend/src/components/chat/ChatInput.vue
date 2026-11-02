<script setup lang="ts">
// 输入区：多行输入 + 技能芯片 + 待发送图片 + @ 引用 + / 命令菜单；底栏在 ComposerBar。
// 运行中发送即插话（steer）或排队（follow_up），Esc 在输入框为空时停下当前回合。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import { useComposerCommands } from '../../composables/useComposerCommands'
import { useComposerDraft } from '../../composables/useComposerDraft'
import { readFileBase64, useComposerPayload } from '../../composables/useComposerPayload'
import { pickFile } from '../../api/shell'
import type { FileEntryVO } from '../../types/api'
import { parseSkillCommand, type BuiltinKey, type SlashItem } from '../../utils/composerMenu'
import AppIcon from '../common/AppIcon.vue'
import ComposerBar from './ComposerBar.vue'
import ComposerImages from './ComposerImages.vue'
import ComposerMentionMenu from './ComposerMentionMenu.vue'
import ComposerSkill from './ComposerSkill.vue'
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
  send: [text: string, skill: string, images: { mime: string; base64: string }[]]
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
const payload = useComposerPayload()

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
const fileEl = ref<HTMLInputElement | null>(null)
const ta = ref<{ focus: () => void; textarea?: HTMLTextAreaElement; ref?: HTMLTextAreaElement } | null>(
  null,
)

/** 已挂技能的描述：从技能清单里查，清单没加载到就只显示名字。 */
const skillDesc = computed(
  () => settings.skills.find((s) => s.skill_name === payload.skill.value)?.skill_description || '',
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
  // 技能不是一段要发出去的文字：挂成芯片，正文留空给人写诉求
  clearInput()
  mountSkill(item.key)
}

/** 挂技能：清单里没有这个名字就直接拒绝，否则发出去才会失败。 */
function mountSkill(name: string) {
  if (settings.skills.length && !settings.skills.some((s) => s.skill_name === name)) {
    toast.bad(t('chat.skillUnknown', { name }))
    return
  }
  payload.setSkill(name)
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

// ── @ 引用：选定后在正文里内联 @相对路径，由模型用 read 工具自己取内容 ──
/** 把相对路径插到触发菜单的那个 @ 位置，光标跟在路径后面。 */
function insertReference(rel: string) {
  text.value = text.value.replace(/@\S*$/, `@${rel} `)
  saveDraft()
  menu.value = null
  nextTick(() => ta.value?.focus())
}

function pickFromTree(entry: FileEntryVO) {
  if (entry.entry_is_dir) return
  insertReference(mentionRef.value?.relativeOf(entry) || entry.entry_name)
}

/** 工作区之外的文件没法用相对路径引用：给绝对路径，模型按绝对路径读。 */
async function pickExternal() {
  menu.value = null
  text.value = text.value.replace(/@\S*$/, '').trimEnd()
  const picked = await pickFile('', '')
  if (!picked) return
  insertReference(picked.replace(/\\/g, '/'))
}

// ── 图片：粘贴与选择两条入口，都落到同一份待发送列表 ──
async function addImageFiles(files: FileList | File[]) {
  for (const f of Array.from(files)) {
    try {
      const img = await readFileBase64(f)
      const err = payload.addImage(img.name, img.mime, img.base64)
      if (err) toast.bad(err)
    } catch (e) {
      toast.bad(e instanceof Error ? e.message : String(e))
    }
  }
}

async function pickImage() {
  menu.value = null
  // 浏览器文件选择框：读原始字节，不绕一圈去求后端给路径（路径副本会丢 EXIF 与 mime）
  fileEl.value?.click()
}

function onFilePicked(e: Event) {
  const el = e.target as HTMLInputElement
  if (el.files?.length) void addImageFiles(el.files)
  // 置空才能重复选同一个文件（change 只在值变化时触发）
  el.value = ''
  nextTick(() => ta.value?.focus())
}

function onPaste(e: ClipboardEvent) {
  const all = Array.from(e.clipboardData?.files || [])
  const imgs = all.filter((f) => f.type.startsWith('image/'))
  // 没有图片就放行：普通文本粘进输入框是正常操作
  if (!imgs.length) return
  e.preventDefault()
  void addImageFiles(imgs)
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

// 切会话要丢掉上一段的技能与图片：它们属于那一条消息，不是输入框的持久状态
watch(
  () => session.currentId,
  () => payload.clear(),
)

function onInput() {
  saveDraft()
  const v = text.value
  // 手打 /skill:名 在提交时折成芯片：这里只负责把菜单让开，不然菜单会挡住正文
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

/** 手打的 /skill:名 也要折成芯片（否则命令字会混进人话里）。 */
function foldSkillCommand() {
  const parsed = parseSkillCommand(text.value)
  if (!parsed) return
  mountSkill(parsed.name)
  text.value = parsed.rest
  saveDraft()
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
  return text.value.trim().length > 0 || !payload.empty.value
})

function submit() {
  if (props.disabled || props.sending) return
  // 手打 /skill:名 先折成芯片：正文里不留命令字
  foldSkillCommand()
  const value = text.value.trim()
  if (!props.running) {
    // 非运行态只发正文与图片；芯片在发送那一刻被定格进这条消息
    if (!value && payload.empty.value) return
    emit('send', value, payload.skill.value, payload.imagePayload())
    clearInput()
    payload.clear()
    return
  }
  // 运行态：插话 / 排队走的是纯文本通道，芯片带不过去，所以明确告知
  if (payload.skill.value || payload.images.value.length) {
    toast.info(t('chat.attachOnlyIdle'))
  }
  if (!value) return
  if (mode.value === 'queue') emit('queue', value)
  else emit('steer', value)
  clearInput()
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
  /** 桌面壳从外部打开文件时把绝对路径内联进正文 */
  attachPath: (p: string) => insertReference(p.replace(/\\/g, '/')),
})
</script>

<template>
  <div ref="root" class="composer">
    <div class="composer-in">
      <div class="composer-box">
        <!-- 技能与图片各占一行，都在正文之上：正文那一块只放人话 -->
        <ComposerSkill :skill="payload.skill.value" :description="skillDesc" @clear="payload.clearSkill()" />
        <ComposerImages :images="payload.images.value" @remove="payload.removeImage" />

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
          :can-send="canSend"
          :send-hint="sendHint"
          @pick-file="pickExternal"
          @pick-image="pickImage"
          @send="submit"
          @stop="emit('stop')"
        />
        <!-- 图片选择的真实入口：隐藏的 input 比拦系统对话框可靠 -->
        <input
          ref="fileEl"
          class="file-hidden"
          type="file"
          accept="image/*"
          multiple
          @change="onFilePicked"
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
.file-hidden {
  display: none;
}
</style>
