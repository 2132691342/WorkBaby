<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import {
  Check, Send, AlertTriangle, Lock, File as FileIcon
} from '@/components/common/icons'
import { t } from '@/i18n'
import { useChatStore } from '@/stores/chat'
import { useToast } from '@/composables/useToast'
import { diffLineClass, parseDiffLines, type DiffLine } from '@/chat/models/blocks'
import { synthesizePatch } from '@/chat/models/editPatch'

/**
 * 审批语义卡：按工具类型渲染不同预览（文件写入/编辑 → 路径 + 内容 diff；exec → 白名单状态 + 参数；其余 → args JSON）。
 * 风险徽标区分可恢复与不可逆；决策后按钮区置为已完成并禁用，防重复提交。
 */
const chat = useChatStore()
const toast = useToast()
const { pendingApproval } = storeToRefs(chat)

/** 已响应标记（approved / denied）：防重复提交 + 绿勾反馈。 */
const settled = ref<'' | 'approved' | 'denied'>('')
const deciding = ref(false)

/**
 * 等待秒数：run 正阻塞在审批上，用户看不到任何进度提示会以为卡死。
 * 卡片本身只在待决期存在，挂载即开始计时。
 */
const waitSec = ref(0)
const waitTimer = window.setInterval(() => { waitSec.value += 1 }, 1000)
onUnmounted(() => { window.clearInterval(waitTimer) })

/** 补充输入模式（request_input 工具）：卡片变为问答形态而非批准/拒绝。 */
const isInput = computed(() => pendingApproval.value?.risk === 'input_required')
const inputDraft = ref('')

const parsed = computed<ParsedCommand | null>(() => {
  const cmd = pendingApproval.value?.command ?? ''
  return parseCommand(cmd)
})

const isIrreversible = computed(() => pendingApproval.value?.risk === 'irreversible')

const riskLabel = computed(() => {
  if (isIrreversible.value) return t('chat.approvalRiskIrreversible')
  return t('chat.approvalRiskNeeds')
})

const riskType = computed<'error' | 'warning'>(() =>
  isIrreversible.value ? 'error' : 'warning'
)

const reasonText = computed(() => pendingApproval.value?.reason ?? '')

/**
 * 是否提供「本会话允许」：以服务端声明为准（不可逆操作恒 false）。
 * 前端不再自行按 risk 判断——两边各判一次就会出现「按钮显示但后端不认」的裂缝。
 */
const canRemember = computed(() => pendingApproval.value?.canRemember === true)

interface ParsedArgs { [k: string]: unknown }

interface ParsedCommand {
  name: string
  args: ParsedArgs
  /** 原 args JSON（pretty 用）。 */
  rawArgs: string
}

function parseCommand(cmd: string): ParsedCommand | null {
  const trimmed = cmd.trim()
  if (!trimmed) return null
  const m = /^(.+?)\((.*)\)$/s.exec(trimmed)
  if (!m) return { name: trimmed, args: {}, rawArgs: '' }
  const name = m[1].trim()
  const rawArgs = m[2].trim()
  if (!rawArgs) return { name, args: {}, rawArgs: '' }
  try {
    return { name, args: JSON.parse(rawArgs), rawArgs }
  } catch {
    return { name, args: {}, rawArgs }
  }
}

const toolKind = computed(() => {
  const n = parsed.value?.name ?? ''
  if (n === 'file_write' || n === 'file_edit' || n === 'file_read' || n === 'file_list') return 'file'
  if (n === 'exec' || n === 'run_skill_script') return 'exec'
  if (n === 'webfetch' || n === 'http') return 'http'
  return 'generic'
})

const toolTitle = computed(() => {
  switch (parsed.value?.name) {
    case 'file_write': return t('chat.approvalToolFileWrite')
    case 'file_edit': return t('chat.approvalToolFileEdit')
    case 'file_read': return t('chat.approvalToolFileRead')
    case 'file_list': return t('chat.approvalToolFileList')
    case 'exec': return t('chat.approvalToolExec')
    case 'run_skill_script': return t('chat.approvalToolSkill')
    case 'http': return t('chat.approvalToolHttp')
    case 'webfetch': return t('chat.approvalToolWebfetch')
    default: return parsed.value?.name || t('chat.approvalToolGeneric')
  }
})

const fileWriteArgs = computed(() => {
  if (toolKind.value !== 'file') return null
  const name = parsed.value?.name
  if (name !== 'file_write' && name !== 'file_edit') return null
  const a = parsed.value!.args
  return {
    edit: name === 'file_edit',
    path: typeof a.path === 'string' ? a.path : '',
    content: name === 'file_edit' ? (typeof a.new_string === 'string' ? a.new_string : '') : typeof a.content === 'string' ? a.content : '',
    oldContent: name === 'file_edit' && typeof a.old_string === 'string' ? a.old_string : '',
    append: a.append === true
  }
})

const execArgs = computed(() => {
  if (parsed.value?.name !== 'exec') return null
  const a = parsed.value!.args
  return {
    command: typeof a.command === 'string' ? a.command : '',
    args: Array.isArray(a.args) ? a.args.map(String) : [],
    cwd: typeof a.cwd === 'string' ? a.cwd : ''
  }
})

/** 预览字符上限：超过的折叠为占位。 */
const PREVIEW_LIMIT = 600

function previewContent(content: string): { text: string; truncated: boolean } {
  if (content.length <= PREVIEW_LIMIT) return { text: content, truncated: false }
  return {
    text: content.slice(0, PREVIEW_LIMIT) + '\n…',
    truncated: true
  }
}

const fileWritePreview = computed(() => {
  const a = fileWriteArgs.value
  if (!a) return null
  return previewContent(a.content)
})

/** file_edit 的本地补丁行（非编辑 / 缺 old 时为空，模板据此退回普通预览）。 */
const editPatchLines = computed<DiffLine[]>(() => {
  const a = fileWriteArgs.value
  if (!a?.edit || a.content == null) return []
  const patch = synthesizePatch(a.oldContent ?? '', a.content)
  if (!patch) return []
  return parseDiffLines(previewContent(patch).text)
})

const argsJson = computed(() => {
  const p = parsed.value
  if (!p || !p.rawArgs) return null
  if (toolKind.value === 'file' && parsed.value?.name === 'file_write') return null
  return previewContent(p.rawArgs)
})

async function approve(): Promise<void> {
  if (deciding.value || settled.value) return
  deciding.value = true
  try {
    await chat.approveApproval()
    settled.value = 'approved'
    toast.success(t('chat.approvalApproved'))
  } finally {
    deciding.value = false
  }
}

/** 本会话允许：放行并记住该命令（后端 scope=session；不可逆操作不提供该选项）。 */
async function approveForSession(): Promise<void> {
  if (deciding.value || settled.value) return
  deciding.value = true
  try {
    await chat.approveApprovalForSession()
    settled.value = 'approved'
    toast.success(t('chat.approvalAllowedSession'))
  } finally {
    deciding.value = false
  }
}

/** 拒绝执行（安全审批语义）。 */
async function deny(): Promise<void> {
  if (deciding.value || settled.value) return
  deciding.value = true
  try {
    await chat.denyApproval()
    settled.value = 'denied'
  } finally {
    deciding.value = false
  }
}

/** 跳过补充输入：走后端 /skip；补充输入用 /decide 会因通道不匹配报 4003。 */
async function skip(): Promise<void> {
  await chat.skipApproval()
  toast.success(t('chat.approvalSkipped'))
}

async function sendAnswer(): Promise<void> {
  const text = inputDraft.value.trim()
  if (!text) return
  inputDraft.value = ''
  await chat.answerApproval(text)
}
</script>

<template>
  <el-alert
    v-if="pendingApproval"
    :type="isInput ? 'info' : riskType"
    :closable="false"
    show-icon
    class="approval-card"
  >
    <template #title>
      <div class="flex items-center gap-2">
        <span class="text-sm font-semibold">{{ isInput ? t('chat.inputTitle') : toolTitle }}</span>
        <el-tag
          size="small"
          :type="isInput ? 'primary' : isIrreversible ? 'danger' : 'warning'"
          effect="dark"
          round
        >
          <el-icon class="mr-0.5 align-[-2px]">
            <AlertTriangle />
          </el-icon>
          {{ isInput ? t('chat.inputTag') : riskLabel }}
        </el-tag>
        <!-- 等待计时：run 正阻塞在这里，没有进度感用户会以为卡死 -->
        <span v-if="!settled" class="text-[11px] text-wb-muted">{{ t('chat.waitingConfirm', waitSec) }}</span>
      </div>
    </template>

    <!-- 补充输入模式：问题 + 回复框 -->
    <template v-if="isInput">
      <p class="mb-2 whitespace-pre-wrap text-xs text-wb-ink">{{ pendingApproval.command }}</p>
      <el-input
        v-model="inputDraft"
        type="textarea"
        :rows="2"
        :placeholder="t('chat.inputPlaceholder')"
        @keydown.enter.ctrl="sendAnswer"
      />
      <div class="mt-3 flex gap-2">
        <el-button type="primary" size="small" :disabled="!inputDraft.trim()" @click="sendAnswer">
          <el-icon class="mr-1"><Send /></el-icon>
          {{ t('chat.inputSend') }}
        </el-button>
        <el-button size="small" @click="skip">{{ t('chat.inputSkip') }}</el-button>
      </div>
      <p class="mt-1 text-[11px] text-wb-muted">{{ t('chat.inputHint') }}</p>
    </template>

    <!-- file_write / file_edit：路径 + 内容预览（编辑态展示替换前→替换后） -->
    <template v-else-if="fileWriteArgs">
      <div class="mb-2 flex items-center gap-1.5 text-xs text-wb-muted">
        <el-icon class="text-wb-primary"><FileIcon /></el-icon>
        <span class="font-mono break-all">{{ fileWriteArgs.path || '?' }}</span>
        <el-tag v-if="fileWriteArgs.append" size="small" type="info">
          {{ t('chat.approvalAppend') }}
        </el-tag>
      </div>
      <!-- file_edit：本地合成补丁——执行前就能看清「改哪一行、改成什么」，
           而不是让用户对两段文本自行比对 -->
      <pre
        v-if="editPatchLines.length > 0"
        class="mb-2 overflow-x-auto rounded-md border border-wb-border bg-wb-surface-2 px-3 py-2 font-mono text-[11px] leading-relaxed"
      ><span
          v-for="(line, i) in editPatchLines"
          :key="i"
          class="block"
          :class="diffLineClass(line.type)"
        >{{ line.text }}</span></pre>
      <div v-if="fileWriteArgs.content === ''" class="rounded-md border border-dashed border-wb-border bg-wb-surface-2 px-3 py-2 text-xs text-wb-muted">
        {{ t('chat.approvalEmptyWrite') }}
      </div>
      <pre
        v-else
        class="overflow-x-auto rounded-md border border-wb-border bg-wb-surface-2 px-3 py-2 font-mono text-[11px] leading-relaxed text-wb-ink"
      ><span class="block text-wb-muted">/* {{ fileWriteArgs.edit ? t('chat.approvalNewPreview') : t('chat.approvalAfterPreview') }} */</span>{{ fileWritePreview?.text }}</pre>
      <p v-if="fileWritePreview?.truncated" class="mt-1 text-[10px] text-wb-muted">
        {{ t('chat.approvalTruncated', PREVIEW_LIMIT) }}
      </p>
    </template>

    <!-- exec：命令 + 参数 + 白名单说明 -->
    <template v-else-if="execArgs">
      <div class="mb-2 flex items-center gap-1.5 text-xs text-wb-muted">
        <el-icon class="text-wb-primary"><Send /></el-icon>
        <span class="font-mono">{{ execArgs.command || '?' }}</span>
      </div>
      <div v-if="execArgs.args.length" class="mb-2 flex flex-wrap gap-1">
        <el-tag
          v-for="(arg, i) in execArgs.args"
          :key="i"
          size="small"
          effect="plain"
          class="font-mono"
        >
          {{ arg }}
        </el-tag>
      </div>
      <div v-if="execArgs.cwd" class="mb-2 flex items-center gap-1.5 text-[11px] text-wb-muted">
        <span class="text-wb-muted">cwd</span>
        <span class="font-mono">{{ execArgs.cwd }}</span>
      </div>
    </template>

    <!-- 其它工具：args JSON pretty-print -->
    <pre
      v-else-if="argsJson"
      class="overflow-x-auto rounded-md border border-wb-border bg-wb-surface-2 px-3 py-2 font-mono text-[11px] leading-relaxed text-wb-ink"
    >{{ argsJson.text }}</pre>

    <!-- 兜底：原始命令 -->
    <pre
      v-else
      class="overflow-x-auto rounded-md bg-wb-surface-2 px-3 py-2 font-mono text-xs text-wb-ink"
    >{{ pendingApproval.command }}</pre>

    <template v-if="!isInput">
      <p class="mt-2 flex items-center gap-1 text-xs text-wb-muted">
        <el-icon><Lock /></el-icon>
        {{ reasonText }}
      </p>

      <p class="mt-1 text-[11px] text-wb-muted">
        <span v-if="isIrreversible">{{ t('chat.approvalIrreversibleHint') }}</span>
        <span v-else>{{ t('chat.approvalSessionHint') }}</span>
      </p>

      <!-- 已响应态：绿勾 + 禁用（决策后卡片不再可点，杜绝重复提交） -->
      <div v-if="settled" class="mt-3 flex items-center gap-2 rounded-md border border-wb-mint/40 bg-wb-mint/10 px-3 py-2 text-xs text-wb-ink">
        <el-icon class="text-wb-mint"><Check /></el-icon>
        {{ settled === 'approved' ? t('chat.approvalApproved') : t('chat.approvalDenied') }}
      </div>
      <div v-else class="mt-3 flex flex-wrap gap-2">
        <el-button
          :type="isIrreversible ? 'danger' : 'primary'"
          size="small"
          :loading="deciding"
          @click="approve"
        >
          {{ isIrreversible ? t('chat.approvalApproveIrreversible') : t('chat.approvalAllowOnce') }}
        </el-button>
        <!-- 本会话允许：只有后端声明可记住时出现（不可逆操作恒不出现，避免「选了却无效」） -->
        <el-button
          v-if="canRemember"
          size="small"
          :disabled="deciding"
          @click="approveForSession"
        >
          {{ t('chat.approvalAllowSession') }}
        </el-button>
        <el-button size="small" :disabled="deciding" @click="deny">{{ t('chat.approvalDeny') }}</el-button>
      </div>
    </template>
  </el-alert>
</template>

<style scoped>
.approval-card :deep(.el-alert__content) {
  padding: 4px 0;
}
</style>
