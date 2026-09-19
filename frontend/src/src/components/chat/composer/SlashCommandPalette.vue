<script setup lang="ts">
/**
 * `/` 命令面板：输入框内输入 `/` 触发，键盘 ↑↓ 导航、Enter 执行。
 *
 * <p>通过 `defineExpose` 暴露 `pickAt` / `moveActive`，由 ChatInput 的 keydown 驱动。
 *
 * <p>数据源：父组件传入后端命令列表 + 内置 client-only 兜底（new/clear/focus/regenerate/attach/...）。
 * 后端命令元数据是单一真相源，UI 不重复枚举。
 */
import { computed, nextTick, ref, watch, type Component } from 'vue'
import {
  Plus,
  Eraser,
  Crosshair,
  FolderOpen,
  RefreshCw,
  Palette,
  Paperclip,
  ListTree,
  Bot,
  Shield,
  Cpu,
  Sparkles,
  MessageSquare
} from '@/components/common/icons'
import { t } from '@/i18n'

/** 暴露给父组件（ChatInput）的最小命令形状；ChatInput 决定如何处理。 */
export interface SlashCommand {
  id: string
  /** i18n 标签 key（命令名显示）。 */
  labelKey?: string
  /** i18n 描述 key（副标题）。 */
  descKey?: string
  /** 命令名直接文案（用于后端命令，无 i18n 时）。 */
  label?: string
  /** 命令描述直接文案（用于后端命令，无 i18n 时）。 */
  desc?: string
  /** 命令图标。 */
  icon: Component
  /** 快捷键展示（可选）。 */
  shortcut?: string
  /** 命令参数提示（后端元数据中的 args）。 */
  args?: string
  /** 是否纯前端命令（无后端接口）。 */
  clientOnly?: boolean
  /** 命令分组（session/model/agent/system/custom）。 */
  group?: string
  /** 自定义命令的提示词模板（group=custom；选中即灌入输入框）。 */
  prompt?: string
  /** 命令来源：builtin / user（设置页）/ file（{home}/commands）/ workspace（<ws>/.workbaby/commands）。 */
  source?: string
}

const props = defineProps<{
  visible: boolean
  query: string
  /** 后端斜杠命令元数据（GET /api/v1/chat/commands），与本地兜底合并。 */
  backendCommands?: {
    name: string
    args: string
    desc: string
    group: string
    client_only: boolean
    prompt?: string
    source?: string
  }[]
}>()

const emit = defineEmits<{
  pick: [cmd: SlashCommand]
  close: []
  updateActive: [idx: number]
}>()

/**
 * 内置客户端兜底命令——不依赖后端的本地面板行为。
 * 优先级低于后端同名命令（如用户同时配了 /model，则后端的优先）。
 */
const LOCAL_COMMANDS: SlashCommand[] = [
  { id: 'new', labelKey: 'slash.new', descKey: 'slash.newDesc', icon: Plus, shortcut: 'Ctrl+N', group: 'session' },
  { id: 'clear', labelKey: 'slash.clear', descKey: 'slash.clearDesc', icon: Eraser, group: 'session' },
  { id: 'focus', labelKey: 'slash.focus', descKey: 'slash.focusDesc', icon: Crosshair, shortcut: 'Ctrl+/', group: 'system' },
  { id: 'regenerate', labelKey: 'slash.regenerate', descKey: 'slash.regenerateDesc', icon: RefreshCw, group: 'session' },
  { id: 'attach', labelKey: 'slash.attach', descKey: 'slash.attachDesc', icon: Paperclip, group: 'session' },
  { id: 'workspace', labelKey: 'slash.workspace', descKey: 'slash.workspaceDesc', icon: FolderOpen, group: 'session' },
  { id: 'model', labelKey: 'slash.model', descKey: 'slash.modelDesc', icon: Cpu, group: 'model' },
  { id: 'theme', labelKey: 'slash.theme', descKey: 'slash.themeDesc', icon: Palette, group: 'system' },
  { id: 'tasks', labelKey: 'slash.tasks', descKey: 'slash.tasksDesc', icon: ListTree, group: 'system' },
  { id: 'compact', labelKey: 'slash.compact', descKey: 'slash.compactDesc', icon: Eraser, group: 'session' },
  { id: 'export', labelKey: 'slash.export', descKey: 'slash.exportDesc', icon: ListTree, group: 'session' },
  { id: 'context', labelKey: 'slash.context', descKey: 'slash.contextDesc', icon: Cpu, group: 'system' },
  { id: 'help', labelKey: 'slash.help', descKey: 'slash.helpDesc', icon: Sparkles, group: 'system' },
  { id: 'agent', labelKey: 'slash.agent', descKey: 'slash.agentDesc', icon: Bot, group: 'agent' },
  { id: 'trust', labelKey: 'slash.trust', descKey: 'slash.trustDesc', icon: Shield, group: 'agent' },
  { id: 'side', labelKey: 'slash.side', descKey: 'slash.sideDesc', icon: MessageSquare, group: 'agent' }
]

/** 后端命令 → SlashCommand（无 descKey/labelKey，用直接的 desc 字符串透传）。 */
function backendToSlash(cmd: {
  name: string
  args: string
  desc: string
  group: string
  client_only: boolean
  prompt?: string
  source?: string
}): SlashCommand {
  const icon = groupIcon(cmd.group)
  return {
    id: cmd.name,
    labelKey: '',
    descKey: '',
    icon,
    args: cmd.args,
    clientOnly: cmd.client_only,
    group: cmd.group,
    label: cmd.name,
    desc: cmd.desc,
    prompt: cmd.prompt,
    source: cmd.source
  } as SlashCommand
}

/**
 * 来源徽标：让用户一眼看出「这条命令在哪改」。
 * 同名命令有四级来源，没有徽标时「改了文件却不生效」是无法自证的。
 */
function sourceBadge(cmd: SlashCommand): { text: string; cls: string } {
  if (cmd.source === 'workspace') return { text: t('slash.src.workspace'), cls: 'bg-wb-primary/15 text-wb-primary-strong' }
  if (cmd.source === 'file') return { text: t('slash.src.file'), cls: 'bg-wb-primary/10 text-wb-primary' }
  if (cmd.source === 'user') return { text: t('slash.src.user'), cls: 'bg-wb-primary/10 text-wb-primary' }
  if (cmd.clientOnly) return { text: t('slash.local'), cls: 'border border-wb-border text-wb-muted' }
  return { text: t('slash.server'), cls: 'bg-wb-primary/10 text-wb-primary' }
}

/** 合并命令：后端优先（同名本地命令隐藏）。 */
const merged = computed<SlashCommand[]>(() => {
  const backend = (props.backendCommands ?? []).map(backendToSlash)
  const backendIds = new Set(backend.map((b) => b.id))
  return [...backend, ...LOCAL_COMMANDS.filter((c) => !backendIds.has(c.id))]
})

function groupIcon(group: string): Component {
  if (group === 'agent') return Bot
  if (group === 'model') return Cpu
  if (group === 'session') return FolderOpen
  return Sparkles
}

const activeIndex = ref(0)

const filtered = computed(() => {
  const q = props.query.trim().toLowerCase()
  if (!q) return merged.value
  return merged.value.filter((c) => {
    const id = c.id.toLowerCase()
    const desc = (c.desc ?? '').toLowerCase()
    const label = (c.label ?? '').toLowerCase()
    const args = (c.args ?? '').toLowerCase()
    const i18n = (c.labelKey ? t(c.labelKey).toLowerCase() : '') + (c.descKey ? t(c.descKey).toLowerCase() : '')
    return id.includes(q) || desc.includes(q) || label.includes(q) || args.includes(q) || i18n.includes(q)
  })
})

function reset(): void {
  activeIndex.value = 0
}

function pickAt(idx: number): void {
  const cmd = filtered.value[idx]
  if (!cmd) return
  emit('pick', cmd)
}

function moveActive(delta: number): void {
  const len = filtered.value.length
  if (len === 0) return
  activeIndex.value = (activeIndex.value + delta + len) % len
}

/** 高亮项自动滚入视野：列表固定高度内键盘导航超出可见区时不再「看不见选中的项」。 */
const listEl = ref<HTMLElement | null>(null)
watch(activeIndex, async () => {
  await nextTick()
  const el = listEl.value?.querySelector<HTMLElement>('[data-active="true"]')
  el?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
})

defineExpose({ reset, pickAt, moveActive, filtered })
</script>

<template>
  <transition
    enter-active-class="transition-all duration-150 ease-out"
    enter-from-class="scale-95 opacity-0 translate-y-1"
    enter-to-class="scale-100 opacity-100 translate-y-0"
    leave-active-class="transition-all duration-100 ease-in"
    leave-from-class="scale-100 opacity-100"
    leave-to-class="scale-95 opacity-0"
  >
    <div
      v-if="props.visible && filtered.length > 0"
      class="wb-glass absolute bottom-full left-0 z-50 mb-2 w-80 rounded-2xl border border-wb-border bg-wb-surface/95 p-2 shadow-[var(--wb-shadow-lg)] backdrop-blur"
    >
      <p class="px-2 pb-1 pt-1 text-[10px] font-medium uppercase tracking-wider text-wb-muted">
        {{ t('slash.title') }}
      </p>
      <div ref="listEl" class="wb-scroll max-h-72 space-y-0.5 overflow-y-auto overscroll-contain">
        <button
          v-for="(cmd, idx) in filtered"
          :key="cmd.id"
          type="button"
          :data-active="idx === activeIndex || undefined"
          class="flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-left text-xs transition-colors"
          :class="idx === activeIndex ? 'bg-wb-primary/10 text-wb-primary-strong' : 'text-wb-ink hover:bg-wb-primary/5'"
          @mouseenter="activeIndex = idx"
          @click="pickAt(idx)"
        >
          <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-wb-primary/10 text-wb-primary-strong">
            <component :is="cmd.icon" class="h-3 w-3" />
          </span>
          <span class="min-w-0 flex-1">
            <span class="flex items-center gap-1.5">
              <span class="truncate font-medium">/{{ cmd.id }}</span>
              <span v-if="cmd.args" class="truncate text-[10px] text-wb-muted font-mono">{{ cmd.args }}</span>
              <span class="ml-auto shrink-0 rounded px-1 py-0 text-[9px]" :class="sourceBadge(cmd).cls">
                {{ sourceBadge(cmd).text }}
              </span>
            </span>
            <span class="block truncate text-[10px] text-wb-muted">{{ cmd.descKey ? t(cmd.descKey) : cmd.desc }}</span>
          </span>
        </button>
      </div>
    </div>
  </transition>
</template>