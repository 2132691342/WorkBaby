<script setup lang="ts">
/**
 * 聊天视图：会话头（标题/工作区/操作）+ 消息流 + 输入区 + 可选工作区侧栏。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { Trash2, Eraser, FolderOpen, Pencil, Sparkles, ListTree, RefreshCw, Crosshair, X, Archive, Square, MessageSquare, Sunny } from '@/components/common/icons'
import { useChatStore, backendModeToPermission, type PermissionLevel } from '@/stores/chat'
import { useTrustStore } from '@/stores/trust'
import { useTasksStore } from '@/stores/tasks'
import { useDialog } from '@/composables/useDialog'
import { useToast } from '@/composables/useToast'
import { useFocusMode } from '@/composables/useFocusMode'
import { useTheme } from '@/composables/useTheme'
import { t } from '@/i18n'
import MessageList from '@/components/chat/MessageList.vue'
import GoalCard from '@/components/chat/GoalCard.vue'
import ChatInput from '@/components/chat/ChatInput.vue'
import PetCompanion from '@/components/pet/PetCompanion.vue'
import WorkspacePanel from '@/components/chat/WorkspacePanel.vue'
import SideConversation from '@/components/chat/SideConversation.vue'
import { useSideChatStore } from '@/stores/sideChat'
import WorkspacePickerDialog from '@/components/chat/WorkspacePickerDialog.vue'
import FileChangesPanel from '@/components/chat/FileChangesPanel.vue'
import TaskCenterPanel from '@/components/tasks/TaskCenterPanel.vue'
import PinnedPlan from '@/components/chat/PinnedPlan.vue'
import ContextRing from '@/components/chat/ContextRing.vue'
import ChatBackdrop from '@/components/chat/ChatBackdrop.vue'
import type { AvailableModel, ContextUsageRESP, Session } from '@/types/api'

const chat = useChatStore()
const trust = useTrustStore()
const dialog = useDialog()
const toast = useToast()
const {
  sessions,
  currentID,
  messages,
  loadingMessages,
  models,
  selectedModelID,
  circuitStates,
  streamingStats,
  error,
  contextUsage,
  fileChanges,
  goal
} = storeToRefs(chat)

/**
 * 「当前会话正在生成」：流式状态是单份的，用 streamingSessionID 标记归属。
 * 用户切走后，后台 run 的增量不写视图状态；切回该会话或 run 结束后由快照补齐。
 */
const streaming = computed(() => chat.streaming && chat.streamingSessionID === chat.currentID)

const showWorkspace = ref(false)
const showPicker = ref(false)
/** 右侧面板当前激活的 tab（workspace / changes / tasks / side）。 */
const rightTab = ref<'workspace' | 'changes' | 'tasks' | 'side'>('workspace')
const side = useSideChatStore()

/** ChatInput 实例引用：用于把示例 prompt 灌进去。 */
const chatInputRef = ref<InstanceType<typeof ChatInput> | null>(null)

/**
 * 外部「新建任务」事件（App.vue onCreateSession / Ctrl+N）落点：
 * 不创建空会话，只把输入框聚焦——发首条消息时由 sendMessage 自动建会话。
 */
function focusComposer(): void {
  void nextTick(() => chatInputRef.value?.focusInput())
}
onMounted(() => {
  window.addEventListener('wb:focus-composer', focusComposer)
})
onBeforeUnmount(() => {
  window.removeEventListener('wb:focus-composer', focusComposer)
})

const currentSession = computed(() =>
  currentID.value ? sessions.value.find((s: Session) => s.id === currentID.value) : null
)
const sessionTitle = computed(() => {
  if (!currentID.value) return t('nav.appTitle')
  return currentSession.value?.name || t('chat.unnamed')
})

const currentModel = computed(() => {
  if (!selectedModelID.value) return t('chat.auto')
  const m = models.value.find((x: AvailableModel) => x.id === selectedModelID.value)
  return m ? (m.alias || m.model) : t('chat.auto')
})

/** 提示用户当前 session 的模型是否与所选一致（避免「界面看到 A，实际用了 B」）。 */
const sessionModelMismatch = computed(() => {
  const s = currentSession.value
  if (!s || !selectedModelID.value) return false
  const m = models.value.find((x: AvailableModel) => x.id === selectedModelID.value)
  return !!m && s.model && s.model !== m.model
})

/**
 * 上下文占用估值（优先级取值，杜绝「一直是 0%」）：
 * <ol>
 *   <li>流式 stats 事件（本轮实测 input+output，流中实时）</li>
 *   <li>历史最后一条 assistant 落库的 provider 实测 inputTokens（权威值，流结束/reload 后一直有）</li>
 *   <li>本地粗估兜底（消息内容 chars/3 + thinking，标注估算）</li>
 * </ol>
 */
const lastAssistantUsageTokens = computed(() => {
  for (let i = messages.value.length - 1; i >= 0; i--) {
    const m = messages.value[i]
    if (m.role === 'assistant' && (m.input_tokens ?? 0) > 0) return m.input_tokens as number
  }
  return 0
})
function estimateThreadTokens(): number {
  let total = 0
  for (const m of messages.value) {
    total += Math.floor((m.content?.length ?? 0) / 3)
    if (m.thinking) total += Math.floor(m.thinking.length / 3)
  }
  return total
}
/** 上下文上限：后端权威 context_window 优先，模型配置兜底，再回退 128000。 */
const ctxMax = computed(
  () =>
    contextUsage.value?.context_window ||
    (selectedModelID.value
      ? models.value.find((x: AvailableModel) => x.id === selectedModelID.value)?.context_window
      : 0) ||
    128000
)

/**
 * 上下文占用单一数据源（修「顶栏与输入框占比不一致」）：
 * 顶栏 ContextRing 与输入框 ContextUsagePopover 都消费它。
 * - 非流式：后端权威 /usage/context；无权威值时末条 assistant 实测 / 本地估算兜底（标 estimated）；
 * - 流式：用本轮实测 input_tokens 覆盖（最实时真值），比例同步重算。
 */
const liveContextUsage = computed<ContextUsageRESP | null>(() => {
  const base = contextUsage.value
  const win = ctxMax.value
  if (!base) {
    const used = lastAssistantUsageTokens.value > 0 ? lastAssistantUsageTokens.value : estimateThreadTokens()
    if (used <= 0) return null
    return {
      session_id: currentID.value ?? '', model: currentSession.value?.model ?? '',
      context_window: win, context_budget: 0, used_tokens: used, free_tokens: Math.max(0, win - used),
      used_ratio: Math.min(1000, Math.round((used / win) * 1000)),
      segments: [], message_count: messages.value.length, tool_count: 0, estimated: true
    }
  }
  const s = streamingStats.value
  const live = streaming.value && s && (s.input_tokens ?? 0) > 0 ? (s.input_tokens as number) : 0
  if (!live) return base
  const mergedUsed = Math.max(base.used_tokens, live)
  const bWin = base.context_window || win
  return {
    ...base,
    used_tokens: mergedUsed,
    free_tokens: Math.max(0, bWin - mergedUsed),
    used_ratio: Math.min(1000, Math.round((mergedUsed / bWin) * 1000))
  }
})

const ctxUsed = computed(() => liveContextUsage.value?.used_tokens ?? 0)

// 流结束 → 重拉权威上下文占用，顶栏/输入框同步刷新
watch(streaming, (now, prev) => {
  if (prev && !now && currentID.value) void chat.loadContextUsage(currentID.value)
})

// 当前会话的工作区路径（null = 默认工作区）
const workspacePath = computed(() => currentSession.value?.workspace_path ?? null)

// 活跃后台任务数（pending/running）：任务 tab 徽标的数据源
const tasksStore = useTasksStore()
const taskActiveCount = computed(
  () => tasksStore.items.filter((x) => x.state === 'pending' || x.state === 'running').length
)
const workspaceDisplay = computed(() => {
  const wp = workspacePath.value
  if (!wp) return t('chat.workspace.default')
  const parts = wp.replace(/\\/g, '/').split('/').filter(Boolean)
  return parts.length <= 2 ? wp : parts.slice(-2).join('/')
})

onMounted(() => {
  chat.loadModels()
  // 命令面板触发：打开压缩对话框
  window.addEventListener('workbaby:open-compact', onOpenCompact)
  // 后台任务提交后就地展开任务面板
  window.addEventListener('workbaby:open-tasks', onOpenTasks)
  // /context 命令：打开上下文明细弹窗
  window.addEventListener('workbaby:open-context', onOpenContext)
  // 从设置页改了 provider 温度/思考等参数后回到聊天，重拉当前会话生效参数（避免输入框 chip 显示旧值）
  if (currentID.value) void chat.loadEffectiveParams(currentID.value)
})

onBeforeUnmount(() => {
  window.removeEventListener('workbaby:open-compact', onOpenCompact)
  window.removeEventListener('workbaby:open-tasks', onOpenTasks)
  window.removeEventListener('workbaby:open-context', onOpenContext)
})

/** 展开右侧任务面板（后台任务提交后的落地页）。 */
function onOpenTasks(): void {
  rightTab.value = 'tasks'
  showWorkspace.value = true
}

/** /context：打开上下文明细弹窗（与顶栏 ContextRing 同一数据源）。 */
function onOpenContext(): void {
  if (!currentID.value) return
  void chat.loadContextUsage(currentID.value)
  showContext.value = true
}

async function onClearMessages(): Promise<void> {
  if (!currentID.value) return
  const ok = await dialog.confirm({
    title: t('chat.clearTitle'),
    content: t('chat.clearConfirm'),
    danger: true
  })
  if (!ok) return
  try {
    await chat.clearMessages()
    toast.success(t('chat.clearedSuccess'))
  } catch (e) {
    toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
  }
}

async function onRenameSession(): Promise<void> {
  if (!currentID.value || !currentSession.value) return
  const name = await dialog.prompt({
    title: t('chat.renameTitle'),
    content: t('chat.renamePrompt'),
    defaultValue: currentSession.value?.name || ''
  })
  if (name && name.trim()) {
    try {
      await chat.renameSession(currentID.value, name.trim())
      toast.success(t('chat.renamedSuccess'))
    } catch (e) {
      toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
    }
  }
}

async function onDeleteSession(): Promise<void> {
  if (!currentID.value) return
  const ok = await dialog.confirm({
    title: t('chat.deleteTitle'),
    content: t('chat.deleteConfirm'),
    danger: true
  })
  if (!ok) return
  try {
    await chat.deleteSession(currentID.value)
    toast.success(t('chat.deletedSuccess'))
  } catch (e) {
    toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
  }
}

/**
 * /context 弹窗的优化建议：光看明细用户未必知道该动哪里。
 * 只给两条确定性结论——某一段占比过高（可关对应来源）、总量逼近上限（该压缩了）。
 */
const ctxAdvice = computed<string[]>(() => {
  const u = contextUsage.value
  if (!u) return []
  const out: string[] = []
  const heaviest = u.segments.reduce<{ title: string; ratio: number } | null>(
    (max, s) => (max == null || s.ratio > max.ratio ? { title: s.title, ratio: s.ratio } : max),
    null
  )
  if (heaviest && heaviest.ratio >= 400) {
    out.push(t('ctx.adviceHeavySegment', heaviest.title, (heaviest.ratio / 10).toFixed(0)))
  }
  if (u.used_ratio >= 700) {
    out.push(t('ctx.adviceCompact', (u.used_ratio / 10).toFixed(0)))
  }
  return out
})

/** 手动压缩：可附带「保留指示」与自定义保留窗口。 */
const showCompact = ref(false)
const showContext = ref(false)
const compactInstructions = ref('')
const compactKeepRecent = ref(20)
const { enabled: focusMode, toggle: focusToggle } = useFocusMode()
const theme = useTheme()
/** 主题切换：原型 header 太阳图标，与 useTheme 双向同步 settings 页面。 */
function toggleTheme(): void {
  theme.setTheme(theme.currentTheme.value === 'dark' ? 'light' : 'dark')
}
async function onOpenCompact(): Promise<void> {
  if (!currentID.value || streaming.value) return
  showCompact.value = true
  compactInstructions.value = ''
  compactKeepRecent.value = 20
}
async function onConfirmCompact(): Promise<void> {
  if (!currentID.value) {
    showCompact.value = false
    return
  }
  try {
    const res = await chat.compactSession(currentID.value, {
      instructions: compactInstructions.value.trim() || undefined,
      keep_recent: compactKeepRecent.value
    })
    if (res) {
      const msg = res.pinned
        ? t('chat.compactDonePinned', res.compacted, res.freed_chars, res.freed_tokens)
        : t('chat.compactDone', res.compacted, res.freed_chars, res.freed_tokens)
      toast.success(msg)
      await chat.loadMessages(currentID.value)
      await chat.loadContextUsage(currentID.value)
    }
  } catch (e) {
    toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
  } finally {
    showCompact.value = false
  }
}

async function onRegenerate(): Promise<void> {
  if (streaming.value) return
  const msgs = messages.value
  for (let i = msgs.length - 1; i > 0; i--) {
    if (msgs[i].role === 'assistant' && msgs[i - 1].role === 'user') {
      try {
        await chat.resendFrom(msgs[i - 1].id, msgs[i - 1].content)
      } catch (e) {
        toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
      }
      return
    }
  }
  toast.warning(t('chat.regenerateUnavailable'))
}

/** 空状态示例 prompt → 写入 ChatInput 并聚焦。 */
function onQuickPrompt(text: string): void {
  chatInputRef.value?.setDraft(text)
}

/** 划选引用：把选中的对话文字以引用块追加进输入框。 */
function onQuoteSelection(text: string): void {
  chatInputRef.value?.appendText(`> ${text}\n`)
}

/** 划选「在辅助对话中提问」：打开右栏面板，引用块预填进辅助输入框。 */
function onQuoteSide(text: string): void {
  openSidePanel(`> ${text}\n`)
}

/** 工作区文件树「添加到聊天」：插入文件的<b>绝对路径</b>引用（相对路径解析依赖工作区语义，易歧义）。 */
function onAttachWorkspaceFile(path: string): void {
  chatInputRef.value?.appendText(path.replace(/\/$/, ''))
}

/** 工作区选择器回调：绑定外部目录，传 null 表示解绑回默认工作区。
 *  尚无会话时先创建会话再绑定（选择动作不丢失），避免「选了目录却不知绑到哪」的困惑。 */
async function onPickWorkspace(path: string | null): Promise<void> {
  showPicker.value = false
  // 解绑且原目录是显式绑定的：提供撤销信任登记的选项（用户可能还要继续用，默认保留）
  const prevPath = workspacePath.value
  let revokeTrust = false
  if (!path && prevPath) {
    revokeTrust = await dialog.confirm({
      title: t('chat.workspace.unbindTitle'),
      content: t('chat.workspace.unbindRevokeHint', prevPath),
      confirmText: t('chat.workspace.unbindRevokeYes')
    })
  }
  if (!currentID.value) {
    try {
      await chat.createSession(selectedModelID.value ?? null, path)
      toast.success(
        path
          ? t('chat.workspace.boundSuccess', path)
          : t('chat.workspace.unboundSuccess')
      )
    } catch (e) {
      toast.error(
        t('chat.workspace.bindFailed'),
        e instanceof Error ? e.message : String(e)
      )
    }
    return
  }
  try {
    await chat.updateSessionWorkspace(currentID.value, path)
    toast.success(
      path
        ? t('chat.workspace.boundSuccess', path)
        : t('chat.workspace.unboundSuccess')
    )
    if (revokeTrust) {
      const ok = await trust.revoke(prevPath as string)
      if (ok) toast.success(t('chat.workspace.trustRevoked'))
    }
  } catch (e) {
    toast.error(
      t('chat.workspace.bindFailed'),
      e instanceof Error ? e.message : String(e)
    )
  }
}

/** 工具执行权限级别：会话级持久化（后端 chat_sessions.permission_mode），切换即保存。 */
const permState = ref<PermissionLevel>('confirm')

/** 会话切换时同步权限模式（跟随会话存储值；未设置回落 confirm=后端 default）。 */
watch(
  () => chat.currentID,
  (id) => {
    const ses = chat.sessions.find((s) => s.id === id)
    permState.value = backendModeToPermission(ses?.permission_mode)
  },
  { immediate: true }
)

/** 切换权限：持久化到会话；失败回滚并提示。 */
async function onChangePermission(level: PermissionLevel): Promise<void> {
  const prev = permState.value
  permState.value = level
  if (!chat.currentID) return
  try {
    await chat.setPermissionLevel(chat.currentID, level)
    if (level === 'full') useToast().warning(t('chat.perm.riskWarning'))
  } catch (e) {
    permState.value = prev
    useToast().error(t('common.saveFailed'), e instanceof Error ? e.message : String(e))
  }
}

/** 切右侧面板 tab：点已激活的 tab 收起面板；否则切到目标 tab 并展开。 */
function toggleTab(tab: 'workspace' | 'changes' | 'tasks' | 'side'): void {
  if (showWorkspace.value && rightTab.value === tab) {
    showWorkspace.value = false
    return
  }
  rightTab.value = tab
  showWorkspace.value = true
}

/** 打开辅助对话面板（/side 命令与划选「在辅助对话中提问」共用入口）。 */
function openSidePanel(prefill?: string): void {
  rightTab.value = 'side'
  showWorkspace.value = true
  if (prefill) side.draft = prefill
}

/** header「更多」菜单分发：压缩 / 清空 / 删除（低频危险操作统一入口）。 */
function onHeaderCommand(cmd: string): void {
  if (cmd === 'compact') onOpenCompact()
  else if (cmd === 'clear') void onClearMessages()
  else if (cmd === 'delete') void onDeleteSession()
}
</script>

<template>
  <div class="chat wb-ui">
    <!-- 消息主体（会话历史已归位到左侧栏 40% 区） -->
    <div class="chat-main" style="position: relative">
      <ChatBackdrop />
  <div class="relative z-10 flex h-full flex-col overflow-hidden text-wb-ink">
    <!-- 顶部 ChatHeader：mascot + session title + workspace chip + 模型徽标 + 操作 -->
    <header class="flex items-center justify-between gap-3 border-b border-wb-border bg-wb-surface px-5 py-2.5">
      <div class="flex min-w-0 flex-1 items-center gap-3">
        <div class="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-wb-primary/10 text-wb-primary">
          <Sparkles class="h-3.5 w-3.5" />
        </div>
        <div class="min-w-0 flex-1">
          <h1
            class="truncate text-sm font-medium text-wb-ink"
            :title="t('chat.renameHint')"
            @dblclick="onRenameSession"
          >
            {{ sessionTitle }}
            <Pencil class="ml-1 inline h-3 w-3 text-wb-muted opacity-0 transition-opacity hover-hover:opacity-100" />
          </h1>
          <div class="flex items-center gap-2 text-xs text-wb-muted">
            <!-- 运行态指示：执行中主色脉动，空闲静点 -->
            <span class="relative inline-flex h-1.5 w-1.5">
              <span v-if="streaming" class="absolute inline-flex h-full w-full animate-ping rounded-full bg-wb-primary opacity-60" />
              <span class="relative inline-block h-1.5 w-1.5 rounded-full" :class="streaming ? 'bg-wb-primary' : 'bg-wb-mint'" />
            </span>
            <span>{{ streaming ? t('chat.running') : t('chat.agentIdle') }}</span>
            <span class="text-wb-border">·</span>
            <!-- 模型只读徽标 -->
            <span class="inline-flex items-center gap-1 rounded border border-wb-border bg-wb-surface px-1.5 py-0.5 text-xs2 text-wb-muted">
              {{ currentModel }}
            </span>
          </div>
        </div>
      </div>

      <!-- 中间：workspace chip（点 → picker）；绑定态主色高亮，与「默认工作区」一眼区分 -->
      <button
        type="button"
        class="composer-chip inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs transition-all active:scale-95"
        :class="workspacePath
          ? 'border-wb-primary/40 bg-wb-primary/10 text-wb-primary-strong hover:border-wb-primary'
          : 'border-wb-border bg-wb-surface text-wb-muted hover:border-wb-primary hover:text-wb-primary-strong'"
        :title="workspacePath ?? t('chat.workspace.defaultFull')"
        @click="showPicker = true"
      >
        <FolderOpen
          class="h-3 w-3 shrink-0"
          :class="workspacePath ? 'text-wb-primary-strong' : 'text-wb-primary'"
        />
        <span class="max-w-[220px] truncate font-medium">{{ workspaceDisplay }}</span>
      </button>

      <!-- 运行中：header 常驻停止入口，不必到输入框区寻找 -->
      <el-tooltip v-if="streaming" :content="t('chat.stop')" placement="bottom">
        <el-button type="danger" text circle @click="chat.cancelStream()">
          <el-icon><Square /></el-icon>
        </el-button>
      </el-tooltip>

      <!-- 上下文占用环：与输入框同源 liveContextUsage，悬停展开分段详情 -->
      <ContextRing :usage="liveContextUsage" />

      <!-- 焦点模式：只读正文，隐藏思考与工具时间线 -->
      <el-tooltip :content="t('chat.focusMode')" placement="bottom">
        <el-button
          :type="focusMode ? 'primary' : 'default'"
          text
          circle
          @click="focusToggle"
        >
          <el-icon><Crosshair /></el-icon>
        </el-button>
      </el-tooltip>

      <div class="flex shrink-0 items-center gap-1">
        <el-tooltip :content="t('changes.title')" placement="bottom">
          <el-badge
            :value="fileChanges.length"
            :hidden="rightTab === 'changes' || fileChanges.length === 0"
            :max="99"
            :offset="[2, 6]"
          >
            <el-button
              :type="rightTab === 'changes' ? 'primary' : 'default'"
              text
              circle
              @click="toggleTab('changes')"
            >
              <el-icon><RefreshCw /></el-icon>
            </el-button>
          </el-badge>
        </el-tooltip>
        <el-tooltip :content="t('tasks.title')" placement="bottom">
          <el-badge
            :value="taskActiveCount"
            :hidden="taskActiveCount === 0"
            :max="9"
            :offset="[2, 6]"
          >
            <el-button
              :type="rightTab === 'tasks' ? 'primary' : 'default'"
              text
              circle
              @click="toggleTab('tasks')"
            >
              <el-icon><ListTree /></el-icon>
            </el-button>
          </el-badge>
        </el-tooltip>
        <el-tooltip :content="t('side.title')" placement="bottom">
          <el-button
            :type="rightTab === 'side' ? 'primary' : 'default'"
            text
            circle
            @click="toggleTab('side')"
          >
            <el-icon><MessageSquare /></el-icon>
          </el-button>
        </el-tooltip>
        <el-tooltip :content="t('chat.workspace')" placement="bottom">
          <el-button
            :type="rightTab === 'workspace' ? 'primary' : 'default'"
            text
            circle
            @click="toggleTab('workspace')"
          >
            <el-icon><FolderOpen /></el-icon>
          </el-button>
        </el-tooltip>
        <!-- 主题切换：原型 header 太阳图标，明暗切换与 useTheme 双向同步 -->
        <el-tooltip :content="t('chat.toggleTheme')" placement="bottom">
          <el-button text circle @click="toggleTheme">
            <el-icon><Sunny /></el-icon>
          </el-button>
        </el-tooltip>

        <!-- 低频/危险操作收进「更多」菜单，降低 header 密度与误触 -->
        <el-dropdown trigger="click" @command="onHeaderCommand">
          <el-button text circle :title="t('chat.moreActions')">
            <el-icon><Archive /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="compact" :disabled="!currentID || streaming">
                <el-icon><Archive /></el-icon>{{ t('chat.compact') }}
              </el-dropdown-item>
              <el-dropdown-item command="clear" :disabled="!currentID" divided>
                <el-icon><Eraser /></el-icon>{{ t('chat.clearSession') }}
              </el-dropdown-item>
              <el-dropdown-item command="delete" :disabled="!currentID">
                <el-icon class="text-wb-danger"><Trash2 /></el-icon>
                <span class="text-wb-danger">{{ t('chat.deleteSession') }}</span>
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </header>

    <!-- 工具审批已下移 MessageList，此处不再渲染 -->

    <div v-if="error" class="mx-5 mt-2">
      <el-alert type="error" :title="error" show-icon closable @close="chat.dismissError()">
        <!-- 运行出错可一键重试：重新生成最后一轮（中断的继续走 StopReasonBanner 的「继续」） -->
        <template #default>
          <el-button
            size="small"
            text
            type="primary"
            :disabled="streaming"
            class="mt-1"
            @click="onRegenerate"
          >
            <el-icon class="mr-1"><RefreshCw /></el-icon>
            {{ t('chat.retryLastRun') }}
          </el-button>
        </template>
      </el-alert>
    </div>

    <!-- 当前 session 实际模型与所选不一致时给用户一个明确提示，避免「界面 A、实际 B」困惑 -->
    <div v-if="sessionModelMismatch" class="mx-5 mt-2">
      <el-alert
        type="warning"
        :title="t('chat.sessionModelMismatch', currentSession?.model ?? '', currentModel)"
        :closable="false"
        show-icon
      />
    </div>

    <!-- 主区：消息流 + 输入（左） | 工作区文件树面板（右，可折叠） -->
    <div class="flex min-h-0 flex-1">
      <div class="flex min-w-0 flex-1 flex-col" style="position: relative">
        <!-- 目标模式状态卡（/goal 设定后出现；暂停/完成态常驻可恢复） -->
        <GoalCard v-if="goal && goal.status !== 'done'" :goal="goal" />
        <MessageList
          :messages="messages"
          :streaming="streaming"
          :loading="loadingMessages"
          @use-quick-prompt="onQuickPrompt"
          @quote="onQuoteSelection"
          @quote-side="onQuoteSide"
        />

        <!-- 桌宠陪伴体：抠好的形象浮在输入框上方，跟随会话状态 -->
        <PetCompanion :streaming="streaming" :failed="Boolean(error)" />

        <!-- PinnedPlan：composer 上方的计划胶囊（3/7 常显，hover 展开清单） -->
        <PinnedPlan />

        <ChatInput
          ref="chatInputRef"
          :streaming="streaming"
          :disabled="false"
          :models="models"
          :selectedModelID="selectedModelID"
          :circuit-states="circuitStates"
          :workspace-path="workspacePath"
          :effective-params="chat.effectiveParams"
          :permission="permState"
          :ctx-used="ctxUsed"
          :ctx-max="ctxMax"
          @send="(text, ids, params) => chat.sendMessage(text, ids, params)"
          @select-model="chat.selectModel"
          @stop="chat.cancelStream"
          @send-queued="(text) => chat.sendMessage(text)"
          @regenerate="onRegenerate"
          @compact="onOpenCompact"
          @side="openSidePanel()"
          @pick-workspace="showPicker = true"
          @change-permission="onChangePermission"
        />
      </div>

      <aside
        v-if="showWorkspace"
        class="flex shrink-0 flex-col border-l border-wb-border bg-wb-surface"
        :class="rightTab === 'side' ? 'w-[26rem]' : 'w-80'"
      >
        <div class="flex shrink-0 items-center border-b border-wb-border px-3 py-2 text-2xs font-medium uppercase tracking-wider text-wb-muted">
          <span v-if="rightTab === 'changes'">{{ t('changes.title') }}</span>
          <span v-else-if="rightTab === 'tasks'">{{ t('tasks.title') }}</span>
          <span v-else-if="rightTab === 'side'">{{ t('side.title') }}</span>
          <span v-else>{{ t('chat.workspace') }}</span>
          <button
            type="button"
            class="ml-auto flex h-5 w-5 items-center justify-center rounded text-wb-muted transition-colors hover:bg-wb-surface-hover hover:text-wb-ink"
            :title="t('chat.collapseAll')"
            @click="showWorkspace = false"
          >
            <X class="h-3 w-3" />
          </button>
        </div>
        <div class="min-h-0 flex-1">
          <FileChangesPanel v-if="rightTab === 'changes'" />
          <TaskCenterPanel v-else-if="rightTab === 'tasks'" />
          <SideConversation
            v-else-if="rightTab === 'side'"
            :visible="showWorkspace && rightTab === 'side'"
          />
          <WorkspacePanel
            v-else
            :session_id="currentID"
            :workspace_path="workspacePath"
            :pick="() => (showPicker = true)"
            @attach="onAttachWorkspaceFile"
          />
        </div>
      </aside>
    </div>

    <!-- 工作区路径选择器弹窗（桌面壳 IPC 优先 + 文本兜底） -->
    <WorkspacePickerDialog
      :open="showPicker"
      :current-path="workspacePath"
      @close="showPicker = false"
      @pick="onPickWorkspace"
    />

    <!-- /context：上下文明细弹窗（与 ContextRing / ContextUsagePopover 同一数据源） -->
    <el-dialog v-model="showContext" :title="t('ctx.title')" width="420px" align-center>
      <template v-if="contextUsage">
        <p class="mb-3 text-sm text-wb-ink">
          {{ contextUsage.used_tokens.toLocaleString() }} / {{ ctxMax.toLocaleString() }}
          <span class="text-wb-muted">（{{ Math.round(contextUsage.used_ratio / 10) }}%）</span>
          <span v-if="contextUsage.estimated" class="ml-1 text-wb-muted">~</span>
        </p>
        <div v-if="contextUsage.segments.length > 0" class="space-y-2">
          <div v-for="seg in contextUsage.segments" :key="seg.key">
            <div class="mb-0.5 flex items-center justify-between text-xs">
              <span class="text-wb-ink">{{ seg.title }}</span>
              <span class="font-mono text-wb-muted">{{ seg.tokens.toLocaleString() }} · {{ (seg.ratio / 10).toFixed(1) }}%</span>
            </div>
            <div class="h-1.5 overflow-hidden rounded-full bg-wb-surface-hover">
              <div
                class="h-full rounded-full bg-wb-primary transition-all"
                :style="{ width: Math.min(100, seg.ratio / 10) + '%' }"
              />
            </div>
          </div>
        </div>
        <p v-else class="text-xs text-wb-muted">{{ t('ctx.hintNoSegments') }}</p>
        <!-- 优化建议：只给「看到明细也未必知道该怎么办」的那两条（占比过高 / 总量逼近上限） -->
        <ul v-if="ctxAdvice.length > 0" class="mt-3 space-y-1 text-xs2 text-wb-warn">
          <li v-for="(advice, i) in ctxAdvice" :key="i">· {{ advice }}</li>
        </ul>
        <p class="mt-3 text-xs2 text-wb-muted">
          {{ t('ctx.messages', contextUsage.message_count) }} · {{ t('ctx.tools', contextUsage.tool_count) }}
        </p>
      </template>
      <p v-else class="text-xs text-wb-muted">{{ t('ctx.none') }}</p>
    </el-dialog>

    <!-- 手动压缩：可附保留指示与保留窗口 -->
    <el-dialog v-model="showCompact" :title="t('chat.compact')" width="440px" align-center>
      <p class="mb-3 text-sm text-wb-muted">{{ t('chat.compactDesc') }}</p>
      <el-form label-position="top">
        <el-form-item :label="t('chat.compactKeepRecent')">
          <el-input-number v-model="compactKeepRecent" :min="0" :max="200" :step="5" />
        </el-form-item>
        <el-form-item :label="t('chat.compactInstructions')">
          <el-input
            v-model="compactInstructions"
            type="textarea"
            :rows="3"
            :placeholder="t('chat.compactInstructionsPh')"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCompact = false">{{ t('ui.btn.cancel') }}</el-button>
        <el-button type="primary" :loading="streaming" @click="onConfirmCompact">
          {{ t('chat.compactRun') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
    </div>
  </div>
</template>

<style scoped>
/* chip 悬浮态 — 渐进光晕 */
.composer-chip:hover {
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--wb-primary) 8%, transparent);
}
/* 双击重命名笔图标 hover 显示 */
header h1:hover .hover-hover\:opacity-100 {
  opacity: 1;
}
</style>