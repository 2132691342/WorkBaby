<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import { ChevronDown, Settings, Archive } from '@/components/common/icons'
import type { Message } from '@/types/api'
import { t } from '@/i18n'
import { Sparkles, FileText, Code2, Clock, BookOpen } from '@/components/common/icons'
import MessageItem from '@/components/chat/MessageItem.vue'
import StreamingBubble from '@/components/chat/StreamingBubble.vue'
import ApprovalInline from '@/components/chat/ApprovalInline.vue'
import StopReasonBanner from '@/components/chat/StopReasonBanner.vue'
import AssistantAvatar from '@/components/chat/AssistantAvatar.vue'
import { useToast } from '@/composables/useToast'
import { RESUMABLE_STOP_REASONS } from '@/chat/models/blocks'
import { useChatStore } from '@/stores/chat'

/** 空状态的一键示例 prompt，点击直接填入输入框（文案走 i18n）。 */
const quickPrompts: { title: string; text: string; icon: unknown }[] = [
  {
    title: t('chat.quick.weeklyTitle'),
    text: t('chat.quick.weeklyText'),
    icon: FileText
  },
  {
    title: t('chat.quick.explainTitle'),
    text: t('chat.quick.explainText'),
    icon: Code2
  },
  {
    title: t('chat.quick.cronTitle'),
    text: t('chat.quick.cronText'),
    icon: Clock
  },
  {
    title: t('chat.quick.kdocsTitle'),
    text: t('chat.quick.kdocsText'),
    icon: BookOpen
  }
]

const emit = defineEmits<{
  useQuickPrompt: [text: string]
  /** 划选引用：把选中的对话文字作为引用追加到输入框。 */
  quote: [text: string]
  /** 划选引用 → 辅助对话：选中文字带到右栏辅助会话提问（不打断主任务）。 */
  'quote-side': [text: string]
}>()

/**
 * 消息列表：滚动锚定 + 虚拟渲染窗口 + 空状态 + 条目流。
 *
 * <p>单条历史消息渲染内聚在 {@link MessageItem}，流式气泡在 {@link StreamingBubble}，
 * 审批卡在 {@link ApprovalInline}，终止横幅在 {@link StopReasonBanner}。
 * `messages`（当前会话快照）与 `streaming`（状态机信号）由 ChatView 传入，
 * 流式状态直接读 chat store。
 */
const props = defineProps<{
  messages: Message[]
  streaming: boolean
}>()

const chat = useChatStore()
const { pendingApprovals, stopReason } = storeToRefs(chat)
const { approveAllPending } = chat

/** 可批量放行的审批数（不可逆与补充输入必须逐条处理，不参与批量）。 */
const approvableCount = computed(
  () => pendingApprovals.value.filter((p) => p.risk === 'needs_approval').length
)
const router = useRouter()

/** 尚未配置任何可用模型：空态卡给出直达设置页的引导，杜绝「发了消息没反应」的懵态。 */
const noModels = computed(() => chat.models.length === 0)
function goSettings(): void {
  void router.push('/settings?tab=models')
}

const toast = useToast()

const scrollEl = ref<HTMLElement | null>(null)
// 用户是否停留在底部（上翻阅读历史时不强制拉底，F5）
const pinned = ref(true)
// 当前 hover 的消息 ID
const hoveredID = ref<string | null>(null)

function onScroll(): void {
  const el = scrollEl.value
  if (!el) return
  pinned.value = el.scrollHeight - el.scrollTop - el.clientHeight < 80
}

/**
 * 可恢复检查点：这些终态意味着「任务没做完，但可以接着做」。
 *
 * <p>「继续」= 发一条明确的续跑指令，走正常发送链路即可——会话历史整体回灌，
 * 模型看得见自己上一轮做到哪儿，不需要额外接口。
 * 集合定义统一收口在 blocks.ts 的 RESUMABLE_STOP_REASONS。
 */
const canContinue = computed(
  () => !props.streaming && !!stopReason.value && RESUMABLE_STOP_REASONS.has(stopReason.value)
)

async function onContinue(): Promise<void> {
  if (!canContinue.value) return
  try {
    // 崩溃/中断恢复优先走检查点续跑（同一 runID，已完成工具经 StepRecords 复用不重放）
    if (stopReason.value === 'interrupted') {
      const last = [...props.messages].reverse().find((m) => m.role === 'assistant')
      if (last?.run_id && last.status === 'failed') {
        await chat.resumeRun(last.run_id)
        return
      }
    }
    await chat.sendMessage(t('chat.continuePrompt'))
  } catch (e) {
    toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
  }
}

function scrollToBottom(): void {
  const el = scrollEl.value
  if (el) el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
  pinned.value = true
}

// 会话切换（messages 整体替换）时重置为贴底
watch(
  () => props.messages,
  () => {
    pinned.value = true
  }
)

// 内容变化时，仅当用户停留在底部才自动滚底。
// 关键：双 rAF —— 第一次 rAF 等到 batcher flush + streamingContent ref 写完，
// 第二次 rAF 强制等到 DOM 完成 layout（MarkdownRenderer 100ms 节流刚更新过的高度），再 scrollTo。
// 否则单 rAF 时 scrollHeight 仍是上次 layout 的旧值，滚不到底。
let scrollRafID: number | null = null
function scheduleScrollToBottom(): void {
  if (scrollRafID != null) return
  scrollRafID = requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      scrollRafID = null
      const el = scrollEl.value
      if (el && pinned.value) {
        el.scrollTo({ top: el.scrollHeight, behavior: 'auto' })
      }
    })
  })
}
watch(
  () => [props.messages.length, chat.streamingContent.length, props.streaming, chat.streamingBlocks.length],
  scheduleScrollToBottom
)

/**
 * 判断第 i 条消息是否在「当前可视线附近」（流式期只保留最后 1 条 + 上下 5 条，
 * 其余靠 content-visibility 跳过渲染）。
 */
/**
 * 轮次分组：一条 user 消息开启一轮，其后的 assistant / tool 归属同一轮；组内紧凑、组间宽松。
 */
const groups = computed<{ key: string; items: { m: Message; index: number }[] }[]>(() => {
  const out: { key: string; items: { m: Message; index: number }[] }[] = []
  props.messages.forEach((m, index) => {
    if (m.role === 'user' || out.length === 0) {
      out.push({ key: m.id, items: [{ m, index }] })
      return
    }
    out[out.length - 1]!.items.push({ m, index })
  })
  return out
})

const NEAR_WINDOW = 5
function isNearCurrent(i: number): boolean {
  if (!props.streaming) return true
  const total = props.messages.length
  // 流式 + 最近添加 → 最后一条附近；空消息阶段（流开但还没首 token）放空也无所谓
  return i >= total - 1 - NEAR_WINDOW && i <= total - 1
}

// ===== 划选引用 =====
// 选中对话里的任意文字 → 选区旁浮出「添加到当前任务」→ 以引用块追加进输入框。
const quoteBar = ref<{ x: number; y: number; text: string } | null>(null)

/** 压缩纪要展开态与逐行拆分（六段纪要 [目标]/[进度]… 每行一条）。 */
const sysOpen = ref(false)
const compressedLines = computed(() =>
  (chat.compressionNotice?.summary ?? '').split('\n').map((l) => l.trim()).filter(Boolean)
)

function onSelectionChange(): void {
  const sel = window.getSelection()
  if (!sel || sel.isCollapsed || !scrollEl.value) {
    quoteBar.value = null
    return
  }
  const text = sel.toString().replace(/\s+/g, ' ').trim()
  // 过短或过长都不浮出（引用是「追问锚点」，不是全文复制）
  if (text.length < 2 || text.length > 500 || !scrollEl.value.contains(sel.anchorNode)) {
    quoteBar.value = null
    return
  }
  const rect = sel.getRangeAt(0).getBoundingClientRect()
  const host = scrollEl.value.getBoundingClientRect()
  quoteBar.value = {
    x: Math.min(rect.left - host.left + rect.width / 2, host.width - 80),
    y: rect.top - host.top - 34,
    text
  }
}

function onQuotePick(): void {
  if (!quoteBar.value) return
  emit('quote', quoteBar.value.text)
  quoteBar.value = null
  window.getSelection()?.removeAllRanges()
}

function onQuoteSidePick(): void {
  if (!quoteBar.value) return
  emit('quote-side', quoteBar.value.text)
  quoteBar.value = null
  window.getSelection()?.removeAllRanges()
}

onMounted(() => document.addEventListener('selectionchange', onSelectionChange))
onBeforeUnmount(() => document.removeEventListener('selectionchange', onSelectionChange))
</script>

<template>
  <div ref="scrollEl" class="min-h-0 flex-1 overflow-y-auto px-6 py-6 relative" @scroll="onScroll">
    <!-- 空状态：克制卡片（实底 + 细边框，无渐变扫光）+ 4 个一键示例 prompt。 -->
    <div v-if="messages.length === 0 && !streaming" class="flex h-full items-center justify-center px-4">
      <div class="w-full max-w-xl">
        <div class="card-pop p-8 text-center">
          <div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-xl bg-wb-primary/10 text-wb-primary">
            <Sparkles class="h-6 w-6" />
          </div>
          <h3 class="text-base font-semibold text-wb-ink">{{ t('chat.emptyTitle') }}</h3>
          <p class="mt-2 text-sm text-wb-muted">{{ t('chat.emptyPrompt') }}</p>
          <p class="mt-1 text-xs text-wb-muted/70">{{ t('chat.emptyHint') }}</p>
          <!-- 无可用模型：直达设置页引导 -->
          <div
            v-if="noModels"
            class="mx-auto mt-4 flex max-w-sm items-center justify-center gap-2 rounded-lg border border-wb-warning/30 bg-wb-warning/10 px-3 py-2 text-xs text-wb-ink"
          >
            <span>{{ t('chat.noModelsGuide') }}</span>
            <el-button link type="primary" size="small" @click="goSettings">
              <el-icon class="mr-0.5"><Settings /></el-icon>{{ t('chat.goSettings') }}
            </el-button>
          </div>
        </div>
        <div class="mt-4 grid grid-cols-1 gap-2 sm:grid-cols-2">
          <button
            v-for="(s, i) in quickPrompts"
            :key="i"
            type="button"
            class="card-pop group flex items-start gap-3 px-4 py-3 text-left transition-all hover:border-wb-primary/40 hover:shadow-sm"
            @click="$emit('useQuickPrompt', s.text)"
          >
            <span class="mt-0.5 inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-wb-primary/10 text-wb-primary">
              <component :is="s.icon" class="h-3.5 w-3.5" />
            </span>
            <span class="min-w-0">
              <span class="block text-sm font-medium text-wb-ink">{{ s.title }}</span>
              <span class="mt-0.5 block truncate text-xs text-wb-muted">{{ s.text }}</span>
            </span>
          </button>
        </div>
      </div>
    </div>

    <div class="mx-auto max-w-[800px] space-y-7">
      <!-- 一轮 = 一条 user + 其后的 assistant / tool：组内紧凑、组间宽松 -->
      <div v-for="g in groups" :key="g.key" class="space-y-3">
        <div
          v-for="it in g.items"
          :key="it.m.id"
          class="wb-msg-item group flex wb-msg-enter"
          :class="[
            it.m.role === 'user' ? 'justify-end' : 'justify-start',
            { 'wb-msg-item-near': isNearCurrent(it.index) },
            { 'wb-msg-item-lead': it.m.role === 'user' }
          ]"
          :style="{ animationDelay: (it.index % 5) * 60 + 'ms' }"
          @mouseenter="hoveredID = it.m.id"
          @mouseleave="hoveredID = null"
        >
          <!-- assistant 头像（桌宠形象，与桌宠窗口同源） -->
          <AssistantAvatar v-if="it.m.role !== 'user'" class="mr-3" />

          <MessageItem
            :message="it.m"
            :index="it.index"
            :is-last="it.index === messages.length - 1"
            :streaming="streaming"
            :hovered="hoveredID === it.m.id"
          />
        </div>
      </div>

      <!-- 上下文自动压缩分隔线：压缩发生在会话流中间，用居中系统行标记「这里之前的历史已被折叠」，
           下次发送或切会话清除。auto 压缩器带交接纪要 → 行可展开，逐行看「被折叠的轮次做了什么」。 -->
      <div v-if="chat.compressionNotice" class="wb-sys-wrap">
        <component
          :is="chat.compressionNotice.summary ? 'button' : 'div'"
          :type="chat.compressionNotice.summary ? 'button' : undefined"
          class="wb-sysline"
          :class="{ 'is-clickable': !!chat.compressionNotice.summary }"
          @click="chat.compressionNotice.summary && (sysOpen = !sysOpen)"
        >
          <Archive class="h-3 w-3" />
          <span>{{ t('chat.compressedSeparator', chat.compressionNotice.removed) }}</span>
          <template v-if="chat.compressionNotice.summary">
            <span class="wb-sys-act">{{ sysOpen ? t('chat.compressedHide') : t('chat.compressedView') }}</span>
            <ChevronDown class="h-3 w-3 transition-transform" :class="sysOpen ? 'rotate-180' : ''" />
          </template>
        </component>
        <div v-if="sysOpen && chat.compressionNotice.summary" class="wb-sys-detail">
          <p v-for="(line, li) in compressedLines" :key="li" class="wb-sys-line">{{ line }}</p>
        </div>
      </div>

      <!-- 流式中的 assistant 气泡 -->
      <div v-if="streaming" class="flex justify-start">
        <AssistantAvatar class="mr-3" speaking />
        <div class="w-full min-w-0">
          <div class="mb-1 flex items-center gap-2 px-1 text-xs text-wb-muted">
            <span class="font-medium text-wb-ink">WorkBaby</span>
          </div>
          <StreamingBubble />
        </div>
      </div>

      <!-- 审批内联：从页面顶部下移至此，贴近当前运行消息，
           与触发审批的工具调用保持空间上下文（红色=不可逆，警告=可恢复） -->
      <div v-if="pendingApprovals.length" class="flex flex-col gap-2 justify-start">
        <div class="w-full min-w-0">
          <ApprovalInline />
        </div>
        <!-- 队列中还有未展示的请求：批量放行可恢复项，不可逆项仍需逐条确认 -->
        <div v-if="approvableCount > 1" class="flex items-center gap-2">
          <button type="button" class="btn" @click="approveAllPending">
            {{ t('chat.approveAll', approvableCount) }}
          </button>
          <span class="fs11 muted">{{ t('chat.approvalQueue', pendingApprovals.length) }}</span>
        </div>
      </div>

      <!-- 终止原因横幅：中性终态（非 completed 才展示）——
           用户主动停止 / 工具失败限额是正常收场，区别于 error（error 有独立红色 alert）。
           可恢复终态（cancelled / tool_error_limit / max_turns / token_budget）附「继续」按钮，
           一键接着上轮未完成的部分做，不必让用户重新描述任务。 -->
      <StopReasonBanner
        :stop-reason="stopReason"
        :streaming="streaming"
        :elapsed-ms="chat.stopElapsedMs"
        @continue="onContinue"
        @dismiss="chat.dismissStopReason()"
      />
    </div>

    <!-- 划选引用浮层：选中对话文字后出现在选区上方（追问 / 辅助对话两个动作） -->
    <div
      v-if="quoteBar"
      class="quote-bar"
      :style="{ left: quoteBar.x + 'px', top: quoteBar.y + 'px' }"
    >
      <button type="button" class="quote-bar-btn" @mousedown.prevent @click="onQuotePick">
        {{ t('chat.quoteAdd') }}
      </button>
      <button type="button" class="quote-bar-btn is-side" @mousedown.prevent @click="onQuoteSidePick">
        {{ t('side.quoteAsk') }}
      </button>
    </div>

    <!-- 滚动到底部按钮：fixed 定位确保不被 overflow 容器裁剪，bottom-24 抬到 composer 上方（不重叠） -->
    <el-button
      v-if="!pinned"
      size="small"
      round
      class="fixed bottom-24 left-1/2 z-30 -translate-x-1/2 shadow-[var(--wb-shadow-lg)]"
      @click="scrollToBottom"
    >
      <el-icon class="mr-1"><ChevronDown /></el-icon>
      {{ t('chat.scrollToBottom') }}
    </el-button>
  </div>
</template>

<style scoped>
/* CSS-only 虚拟滚动：屏外消息跳过渲染；流式最后 1 条 + 上下 5 条强制可见。
   M2：contain-intrinsic-size 加 auto 关键字——Chromium 会记住每个条目
   最后一次渲染的实际高度作为占位估算，消除「估算 96px 与真实高度偏差
   导致的滚动条跳动」，长会话滚动稳定性显著提升（零 JS 成本）。 */
.wb-msg-item {
  content-visibility: auto;
  contain-intrinsic-size: auto 96px;
}
.wb-msg-item-near {
  content-visibility: visible;
}
/* 轮首（user 消息）是一轮的锚点：跳转定位时留出顶部余量，标题不被滚动位置压住 */
.wb-msg-item-lead {
  scroll-margin-top: 1.5rem;
}
/* 划选引用浮层：小工具条（两个动作：追加到当前任务 / 辅助对话提问），
   Mousedown prevent 防止点击时清掉选区 */
.quote-bar {
  position: absolute;
  z-index: 30;
  transform: translateX(-50%);
  display: flex;
  gap: 2px;
  padding: 2px;
  border-radius: 999px;
  border: 1px solid var(--wb-border-strong);
  background: var(--wb-surface);
  box-shadow: var(--wb-shadow-lg);
  white-space: nowrap;
}
.quote-bar-btn {
  border: 0;
  background: transparent;
  padding: 3px 10px;
  border-radius: 999px;
  color: var(--wb-ink);
  font-size: 11px;
  cursor: pointer;
}
.quote-bar-btn:hover {
  background: var(--wb-surface-hover);
  color: var(--wb-primary-strong);
}
.quote-bar-btn.is-side:hover {
  color: var(--wb-mint);
}
</style>
