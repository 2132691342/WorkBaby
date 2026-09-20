<script setup lang="ts">
import { computed, onMounted, ref, watch, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import {
  MessageSquare, LayoutDashboard, Brain, BookOpen,
  Folder, Server, Zap, ScrollText, Settings, FileText,
  Cat, Sparkles, Eraser, RefreshCw, Crosshair, Shield, Wrench, Activity
} from '@/components/common/icons'
import { t } from '@/i18n'
import { apiGet, apiPost } from '@/api/client'
import { useToast } from '@/composables/useToast'
import { useChatStore } from '@/stores/chat'
import { useFocusMode } from '@/composables/useFocusMode'
import {
  paletteOpen,
  closePalette,
  registerCommands,
  listCommands,
  filterCommands
} from '@/composables/useCommandPalette'
import type { CommandItem } from '@/composables/useCommandPalette'

/**
 * 全局命令面板：Ctrl+K 呼出。命令注册协议收口在 useCommandPalette；
 * 分组：nav 全站导航 / action 全局操作 / session 会话切换 / mode 权限切换 / tool 工具启停。
 */
const router = useRouter()
const chat = useChatStore()
const toast = useToast()
const focus = useFocusMode()
const open = paletteOpen()

const query = ref('')
const active = ref(0)
const inputRef = ref<HTMLInputElement | null>(null)
const listRef = ref<HTMLElement | null>(null)

// 不显式标注元素类型：lucide 自绘图标每个都是具体 DefineComponent<{size,color,strokeWidth,absoluteStrokeWidth}>
// 的独立实例，Vue 的 Component 是 17 个重载的 union；用 typeof MessageSquare 之类的窄类型会导致 icon 字段不兼容，
// 干脆让 TS 推为最宽类型（icon 为 any 透出），nav 数据本身在 import 处类型已强。
const NAV_ITEMS = [
  { to: '/chat', labelKey: 'nav.chat', icon: MessageSquare },
  { to: '/dashboard', labelKey: 'nav.dashboard', icon: LayoutDashboard },
  { to: '/runs', labelKey: 'nav.runs', icon: Activity },
  { to: '/memory', labelKey: 'nav.memory', icon: Brain },
  { to: '/kdocs', labelKey: 'nav.knowledge', icon: BookOpen },
  { to: '/folders', labelKey: 'nav.folders', icon: Folder },
  { to: '/files', labelKey: 'nav.files', icon: FileText },
  { to: '/pet', labelKey: 'nav.pet', icon: Cat },
  { to: '/tools', labelKey: 'nav.tools', icon: Wrench },
  { to: '/mcp', labelKey: 'nav.mcp', icon: Server },
  { to: '/skills', labelKey: 'nav.skills', icon: Zap },
  { to: '/docs', labelKey: 'nav.docs', icon: ScrollText },
  { to: '/settings', labelKey: 'nav.settings', icon: Settings }
]

// ===== 权限模式 =====

const MODES = [
  { value: 'default', labelKey: 'cmdk.modeDefault' },
  { value: 'auto_edit', labelKey: 'cmdk.modeAutoEdit' },
  { value: 'yolo', labelKey: 'cmdk.modeYolo' }
] as const

const currentMode = ref<string>('default')

async function loadCurrentMode(): Promise<void> {
  try {
    const row = await apiGet<{ k: string; v: string }>('/api/v1/kv/agent.session_mode')
    if (row?.v && MODES.some((m) => m.value === row.v)) currentMode.value = row.v
  } catch {
    /* 未配置走默认 */
  }
}

function registerModeCommands(): void {
  for (const m of MODES) {
    registerCommands([
      {
        id: `mode:${m.value}`,
        title: `${currentMode.value === m.value ? '✓ ' : ''}${t(m.labelKey)}`,
        group: 'mode',
        icon: Shield,
        run: async () => {
          await apiPost(`/api/v1/kv/agent.session_mode`, { value: m.value })
          currentMode.value = m.value
          registerModeCommands()
          toast.success(t('cmdk.modeSwitched', t(m.labelKey)))
        }
      }
    ])
  }
}

// ===== 工具启停 =====

interface ToolMeta {
  name: string
  description: string
  risk_level: string
  enabled: boolean
}

function registerToolCommands(tools: ToolMeta[]): void {
  const items: CommandItem[] = tools.map((tool) => ({
    id: `tool:${tool.name}`,
    title: `${t('cmdk.toolToggle')}：${tool.name}`,
    group: 'tool',
    hint: tool.enabled ? t('cmdk.toolOn') : t('cmdk.toolOff'),
    icon: Wrench,
    run: async () => {
      await apiPost(`/api/v1/tools/${tool.name}/enabled`, { enabled: !tool.enabled })
      toast.success(
        tool.enabled ? t('cmdk.toolDisabled', tool.name) : t('cmdk.toolEnabled', tool.name)
      )
      void refreshDynamicCommands()
    }
  }))
  registerCommands(items)
}

/** 打开面板时刷新动态命令（会话 ✓ / 模式 ✓ / 工具启停态）。 */
async function refreshDynamicCommands(): Promise<void> {
  registerModeCommands()
  const [s, tools] = await Promise.allSettled([
    loadCurrentMode(),
    apiGet<ToolMeta[]>('/api/v1/tools')
  ])
  if (s.status === 'fulfilled') registerModeCommands()
  if (tools.status === 'fulfilled' && Array.isArray(tools.value)) {
    registerToolCommands(tools.value)
  }
}

onMounted(() => {
  const items: CommandItem[] = NAV_ITEMS.map((n) => ({
    id: `nav:${n.to}`,
    title: t(n.labelKey),
    group: 'nav',
    hint: n.to,
    icon: n.icon,
    run: () => void router.push(n.to)
  }))
  items.push({
    id: 'action:new-session',
    title: t('chat.newSession'),
    group: 'action',
    icon: Sparkles,
    run: async () => {
      const s = await chat.createSession(chat.selectedModelID)
      await router.push(`/chat/${s.id}`)
    }
  })
  items.push({
    id: 'action:clear-messages',
    title: t('chat.clearSession'),
    group: 'action',
    icon: Eraser,
    run: () => void chat.clearMessages()
  })
  items.push({
    id: 'action:compact',
    title: t('chat.compact'),
    group: 'action',
    icon: RefreshCw,
    run: () => {
      // 通过自定义事件让 ChatView 打开压缩对话框；面板关闭后再触发
      window.dispatchEvent(new CustomEvent('workbaby:open-compact'))
    }
  })
  items.push({
    id: 'view:focus-toggle',
    title: t('chat.focusMode'),
    group: 'action',
    icon: Crosshair,
    run: () => focus.toggle()
  })
  // 切换会话（动态命令：每条会话一行；当前会话标注「✓」）
  for (const s of chat.sessions) {
    items.push({
      id: `session:${s.id}`,
      title: `${chat.currentID === s.id ? '✓ ' : ''}${s.name || t('chat.unnamed')}`,
      group: 'session',
      hint: s.model,
      icon: MessageSquare,
      run: () => void router.push(`/chat/${s.id}`)
    })
  }
  registerCommands(items)
  void refreshDynamicCommands()
})

const filtered = computed<CommandItem[]>(() => filterCommands(listCommands(), query.value))

const actionGroup = computed(() => filtered.value.filter((c) => c.group === 'action'))
const modeGroup = computed(() => filtered.value.filter((c) => c.group === 'mode'))
const toolGroup = computed(() => filtered.value.filter((c) => c.group === 'tool'))
const sessionGroup = computed(() => filtered.value.filter((c) => c.group === 'session'))
const navGroup = computed(() => filtered.value.filter((c) => c.group === 'nav'))

watch(
  () => open.value,
  async (v) => {
    if (v) {
      query.value = ''
      active.value = 0
      void refreshDynamicCommands()
      await nextTick()
      inputRef.value?.focus()
    }
  }
)

watch(query, () => {
  active.value = 0
})

function onKeydown(e: KeyboardEvent): void {
  const total = filtered.value.length
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    active.value = total > 0 ? (active.value + 1) % total : 0
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    active.value = total > 0 ? (active.value - 1 + total) % total : 0
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const hit = filtered.value[active.value]
    if (hit) runCommand(hit)
  } else if (e.key === 'Escape') {
    e.preventDefault()
    closePalette()
  }
}

async function runCommand(cmd: CommandItem): Promise<void> {
  closePalette()
  try {
    await cmd.run()
  } catch {
    /* 动作失败静默（新建会话等已有各自错误提示路径） */
  }
}

/** 渲染扁平序号：active 在各组之间连续定位。 */
function flatIndex(cmd: CommandItem): number {
  return filtered.value.indexOf(cmd)
}
</script>

<template>
  <Teleport to="body">
    <Transition name="wb-cmdk">
      <div
        v-if="open"
        class="fixed inset-0 z-[2000] flex items-start justify-center bg-black/30 pt-[12vh] backdrop-blur-[2px]"
        @click.self="closePalette"
      >
        <div
          class="w-[560px] max-w-[92vw] overflow-hidden rounded-xl border border-wb-border bg-wb-surface shadow-[var(--wb-shadow-pop)]"
          role="dialog"
          aria-label="command palette"
        >
          <input
            ref="inputRef"
            v-model="query"
            class="w-full border-b border-wb-border bg-transparent px-4 py-3.5 text-sm text-wb-ink outline-none placeholder:text-wb-muted/60"
            :placeholder="t('cmdk.placeholder')"
            spellcheck="false"
            @keydown="onKeydown"
          />
          <div ref="listRef" class="max-h-[46vh] overflow-y-auto p-2">
            <template v-if="actionGroup.length > 0">
              <p class="px-2 pb-1 pt-2 text-xs2 font-semibold text-wb-muted">{{ t('cmdk.groupActions') }}</p>
              <button
                v-for="cmd in actionGroup"
                :key="cmd.id"
                type="button"
                class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm transition-colors"
                :class="flatIndex(cmd) === active ? 'bg-wb-primary/10 text-wb-ink' : 'text-wb-ink hover:bg-wb-surface-hover'"
                @mousemove="active = flatIndex(cmd)"
                @click="runCommand(cmd)"
              >
                <component :is="cmd.icon" class="h-4 w-4 shrink-0 text-wb-primary-strong" />
                <span class="truncate">{{ cmd.title }}</span>
              </button>
            </template>

            <!-- 权限模式切换 -->
            <template v-if="modeGroup.length > 0">
              <p class="px-2 pb-1 pt-2 text-xs2 font-semibold text-wb-muted">{{ t('cmdk.groupMode') }}</p>
              <button
                v-for="cmd in modeGroup"
                :key="cmd.id"
                type="button"
                class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm transition-colors"
                :class="flatIndex(cmd) === active ? 'bg-wb-primary/10 text-wb-ink' : 'text-wb-ink hover:bg-wb-surface-hover'"
                @mousemove="active = flatIndex(cmd)"
                @click="runCommand(cmd)"
              >
                <component :is="cmd.icon" class="h-4 w-4 shrink-0 text-wb-info" />
                <span class="truncate">{{ cmd.title }}</span>
              </button>
            </template>

            <!-- 工具启停 -->
            <template v-if="toolGroup.length > 0">
              <p class="px-2 pb-1 pt-2 text-xs2 font-semibold text-wb-muted">{{ t('cmdk.groupTools') }}</p>
              <button
                v-for="cmd in toolGroup"
                :key="cmd.id"
                type="button"
                class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm transition-colors"
                :class="flatIndex(cmd) === active ? 'bg-wb-primary/10 text-wb-ink' : 'text-wb-ink hover:bg-wb-surface-hover'"
                @mousemove="active = flatIndex(cmd)"
                @click="runCommand(cmd)"
              >
                <component :is="cmd.icon" class="h-4 w-4 shrink-0 text-wb-muted" />
                <span class="truncate">{{ cmd.title }}</span>
                <span v-if="cmd.hint" class="ml-auto shrink-0 font-mono text-2xs text-wb-muted/70">{{ cmd.hint }}</span>
              </button>
            </template>

            <!-- 会话切换（动态列表：当前会话以 ✓ 前缀标记） -->
            <template v-if="sessionGroup.length > 0">
              <p class="px-2 pb-1 pt-2 text-xs2 font-semibold text-wb-muted">{{ t('cmdk.groupSessions') }}</p>
              <button
                v-for="cmd in sessionGroup"
                :key="cmd.id"
                type="button"
                class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm transition-colors"
                :class="flatIndex(cmd) === active ? 'bg-wb-primary/10 text-wb-ink' : 'text-wb-ink hover:bg-wb-surface-hover'"
                @mousemove="active = flatIndex(cmd)"
                @click="runCommand(cmd)"
              >
                <component :is="cmd.icon" class="h-4 w-4 shrink-0 text-wb-primary" />
                <span class="truncate">{{ cmd.title }}</span>
                <span v-if="cmd.hint" class="ml-auto shrink-0 font-mono text-2xs text-wb-muted/70">{{ cmd.hint }}</span>
              </button>
            </template>

            <template v-if="navGroup.length > 0">
              <p class="px-2 pb-1 pt-2 text-xs2 font-semibold text-wb-muted">{{ t('cmdk.groupNav') }}</p>
              <button
                v-for="cmd in navGroup"
                :key="cmd.id"
                type="button"
                class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm transition-colors"
                :class="flatIndex(cmd) === active ? 'bg-wb-primary/10 text-wb-ink' : 'text-wb-ink hover:bg-wb-surface-hover'"
                @mousemove="active = flatIndex(cmd)"
                @click="runCommand(cmd)"
              >
                <component :is="cmd.icon" class="h-4 w-4 shrink-0 text-wb-muted" />
                <span class="truncate">{{ cmd.title }}</span>
                <span v-if="cmd.hint" class="ml-auto shrink-0 font-mono text-2xs text-wb-muted/70">{{ cmd.hint }}</span>
              </button>
            </template>
            <p v-if="filtered.length === 0" class="px-3 py-8 text-center text-sm text-wb-muted">
              {{ t('cmdk.empty') }}
            </p>
          </div>
          <div class="border-t border-wb-border px-4 py-2 text-xs2 text-wb-muted/80">
            {{ t('cmdk.hint') }}
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.wb-cmdk-enter-active,
.wb-cmdk-leave-active {
  transition: opacity 0.15s cubic-bezier(0.16, 1, 0.3, 1);
}
.wb-cmdk-enter-active > div,
.wb-cmdk-leave-active > div {
  transition: transform 0.15s cubic-bezier(0.16, 1, 0.3, 1);
}
.wb-cmdk-enter-from,
.wb-cmdk-leave-to {
  opacity: 0;
}
.wb-cmdk-enter-from > div,
.wb-cmdk-leave-to > div {
  transform: translateY(-8px) scale(0.98);
}
</style>
