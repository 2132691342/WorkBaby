<script setup lang="ts">
/**
 * 文件变更 + 工件侧栏。
 *
 * <p>双标签：左侧「变更」（文件每次写入的快照/diff/回滚点），右侧「工件」（会话产出物清单）。
 * 数据源：chat store 的 fileChanges / artifacts 流式事件 + 切会话时全量回灌。
 */
import { computed, ref, watch, type Component } from 'vue'
import { storeToRefs } from 'pinia'
import { useChatStore } from '@/stores/chat'
import { useDialog } from '@/composables/useDialog'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'
import { looksLikeDiff, parseDiffLines, diffLineClass } from '@/chat/models/blocks'
import { formatDateTime } from '@/utils/time'
import type { FileChange, FileChangeDetail } from '@/types/api'
import {
  FilePlus2,
  Pencil,
  Trash2,
  RotateCcw,
  FileText,
  Image as ImageIcon,
  Code2,
  Database,
  File as FileIcon
} from '@/components/common/icons'

const chat = useChatStore()
const { fileChanges, artifacts } = storeToRefs(chat)
const dialog = useDialog()
const toast = useToast()

const tab = ref<'changes' | 'artifacts'>('changes')
const detail = ref<FileChangeDetail | null>(null)
const detailLoading = ref(false)

const changes = computed(() => fileChanges.value)
const arts = computed(() => artifacts.value)

/** 详情是否为 unified diff（决定着色渲染还是裸文本）。 */
const isDiff = computed(() => looksLikeDiff(detail.value?.diff ?? ''))
const diffLines = computed(() => parseDiffLines(detail.value?.diff ?? ''))

/**
 * 变更按 run 分组：run_id 相邻相同的归为一组（列表按时间倒序，分组保序）。
 * 组标题展示首条时间——用户回滚时能对上「那一轮改了哪些文件」。
 */
const groupedChanges = computed<{ runID: string; label: string; items: FileChange[] }[]>(() => {
  const groups: { runID: string; label: string; items: FileChange[] }[] = []
  for (const c of changes.value) {
    const last = groups[groups.length - 1]
    if (last && last.runID === c.run_id) {
      last.items.push(c)
    } else {
      groups.push({ runID: c.run_id, label: fmtTime(c.created_at), items: [c] })
    }
  }
  return groups
})

function fmtBytes(n: number): string {
  if (n < 1024) return `${n}B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)}KB`
  return `${(n / 1024 / 1024).toFixed(2)}MB`
}

function fmtTime(ms: number): string {
  if (!ms) return ''
  return formatDateTime(ms).replace(/^\d{4}-/, '')
}

function actionIcon(action: string): Component {
  switch (action) {
    case 'create':
      return FilePlus2 as unknown as Component
    case 'delete':
      return Trash2 as unknown as Component
    default:
      return Pencil as unknown as Component
  }
}

function actionLabel(action: string): string {
  switch (action) {
    case 'create':
      return t('changes.action.create')
    case 'modify':
      return t('changes.action.modify')
    case 'delete':
      return t('changes.action.delete')
    default:
      return action
  }
}

function artifactIcon(kind: string): Component {
  if (kind === 'image') return ImageIcon as unknown as Component
  if (kind === 'code') return Code2 as unknown as Component
  if (kind === 'data') return Database as unknown as Component
  if (kind === 'doc') return FileText as unknown as Component
  return FileIcon as unknown as Component
}

async function showDetail(c: FileChange): Promise<void> {
  detailLoading.value = true
  try {
    const d = await chat.loadFileChangeDetail(c.id)
    detail.value = d
  } finally {
    detailLoading.value = false
  }
}

function closeDetail(): void {
  detail.value = null
}

async function rollback(c: FileChange): Promise<void> {
  const ok = await dialog.confirm({
    title: t('changes.rollbackTitle'),
    content: t('changes.rollbackConfirm', c.rel_path),
    danger: true
  })
  if (!ok) return
  if (await chat.rollbackFileChange(c.id)) {
    toast.success(t('changes.rolledBack', c.rel_path))
  } else {
    toast.error(t('changes.rollbackFailed'))
  }
}

async function removeArtifact(id: string, name: string): Promise<void> {
  const ok = await dialog.confirm({
    title: t('artifacts.deleteTitle'),
    content: t('artifacts.deleteConfirm', name),
    danger: true
  })
  if (!ok) return
  if (await chat.deleteArtifact(id)) {
    toast.success(t('artifacts.deleted', name))
  } else {
    toast.error(t('artifacts.deleteFailed'))
  }
}

watch(tab, (v) => {
  if (v === 'changes' && changes.value.length === 0 && chat.currentID) {
    void chat.loadFileChanges(chat.currentID)
  } else if (v === 'artifacts' && arts.value.length === 0 && chat.currentID) {
    void chat.loadArtifacts(chat.currentID)
  }
})
</script>

<template>
  <div class="flex h-full flex-col text-wb-ink">
    <!-- 双标签 -->
    <div class="flex shrink-0 gap-1 border-b border-wb-border px-2 pt-2">
      <button
        type="button"
        class="flex-1 rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
        :class="tab === 'changes' ? 'bg-wb-primary/10 text-wb-primary-strong' : 'text-wb-muted hover:bg-wb-surface-2'"
        @click="tab = 'changes'"
      >
        {{ t('changes.title') }}
        <span class="ml-1 text-[10px] tabular-nums text-wb-muted">{{ changes.length }}</span>
      </button>
      <button
        type="button"
        class="flex-1 rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
        :class="tab === 'artifacts' ? 'bg-wb-primary/10 text-wb-primary-strong' : 'text-wb-muted hover:bg-wb-surface-2'"
        @click="tab = 'artifacts'"
      >
        {{ t('artifacts.title') }}
        <span class="ml-1 text-[10px] tabular-nums text-wb-muted">{{ arts.length }}</span>
      </button>
    </div>

    <!-- 内容区 -->
    <div class="min-h-0 flex-1 overflow-y-auto p-2">
      <!-- 变更详情（占满面板） -->
      <div v-if="tab === 'changes' && detail" class="flex h-full flex-col">
        <div class="mb-2 flex items-center justify-between">
          <button class="text-xs text-wb-primary hover:underline" @click="closeDetail">← {{ t('changes.back') }}</button>
          <span class="truncate text-[10px] text-wb-muted">{{ detail.rel_path }}</span>
        </div>
        <div v-if="detailLoading" class="text-xs text-wb-muted">{{ t('common.loading') }}</div>
        <!-- diff 着色渲染（与工具时间线同一套 parseDiffLines，裸 pre 只在非 diff 文本时兜底） -->
        <pre
          v-else-if="detail.diff && isDiff"
          class="min-h-0 flex-1 overflow-auto rounded-md border border-wb-border bg-wb-surface-2 p-2 font-mono text-[11px] leading-relaxed"
        ><span
          v-for="(ln, li) in diffLines"
          :key="li"
          :class="diffLineClass(ln.type)"
        >{{ ln.text }}
</span></pre>
        <pre v-else-if="detail.diff" class="min-h-0 flex-1 overflow-auto rounded-md border border-wb-border bg-wb-surface-2 p-2 font-mono text-[11px] leading-relaxed text-wb-ink">{{ detail.diff }}</pre>
        <div v-else class="rounded-md border border-wb-border bg-wb-surface-2 p-3 text-xs text-wb-muted">{{ t('changes.diffEmpty') }}</div>
    </div>

      <!-- 变更列表：按 run 分组（组标题 = 该轮首次变更时间） -->
      <div v-else-if="tab === 'changes'" class="space-y-3">
        <div v-if="changes.length === 0" class="px-2 py-8 text-center text-xs text-wb-muted">
          {{ t('changes.empty') }}
        </div>
        <div v-for="g in groupedChanges" :key="g.runID || g.label" class="space-y-1.5">
          <p v-if="g.runID" class="px-1 text-[10px] font-medium uppercase tracking-wider text-wb-muted">
            {{ t('changes.runGroup', g.label, g.items.length) }}
          </p>
          <button
            v-for="c in g.items"
            :key="c.id"
            type="button"
            class="group block w-full rounded-md border border-wb-border bg-wb-surface px-2 py-2 text-left transition-colors hover:border-wb-primary/40"
            :class="{ 'opacity-60': c.rolled_back }"
            @click="showDetail(c)"
          >
            <div class="flex items-center gap-2">
              <component :is="actionIcon(c.action)" class="h-3.5 w-3.5 shrink-0" :class="c.action === 'create' ? 'text-wb-mint' : c.action === 'delete' ? 'text-wb-danger' : 'text-wb-info'" />
              <span class="truncate text-xs font-medium text-wb-ink">{{ c.rel_path }}</span>
              <span v-if="c.rolled_back" class="ml-auto rounded border border-wb-border px-1 py-0.5 text-[9px] text-wb-muted">{{ t('changes.rolledBackBadge') }}</span>
            </div>
            <div class="mt-1 flex items-center gap-2 text-[10px] text-wb-muted">
              <span>{{ actionLabel(c.action) }}</span>
              <span>·</span>
              <span class="font-mono">{{ fmtBytes(c.bytes_before) }} → {{ fmtBytes(c.bytes_after) }}</span>
              <span v-if="c.added_lines > 0 || c.removed_lines > 0" class="ml-auto font-mono">
                <span class="text-wb-mint">+{{ c.added_lines }}</span>
                <span class="ml-1 text-wb-danger">-{{ c.removed_lines }}</span>
              </span>
            </div>
            <div v-if="!c.rolled_back" class="mt-1.5 flex justify-end opacity-0 transition-opacity group-hover:opacity-100">
              <el-button size="small" text type="primary" @click.stop="rollback(c)">
                <el-icon class="mr-1"><RotateCcw /></el-icon>
                <span class="text-[10px]">{{ t('changes.rollback') }}</span>
              </el-button>
            </div>
          </button>
        </div>
      </div>

      <!-- 工件列表 -->
      <div v-else class="space-y-1.5">
        <div v-if="arts.length === 0" class="px-2 py-8 text-center text-xs text-wb-muted">
          {{ t('artifacts.empty') }}
        </div>
        <div
          v-for="a in arts"
          :key="a.id"
          class="group flex items-center gap-2 rounded-md border border-wb-border bg-wb-surface px-2 py-2"
        >
          <component :is="artifactIcon(a.kind)" class="h-3.5 w-3.5 shrink-0 text-wb-primary" />
          <div class="min-w-0 flex-1">
            <div class="truncate text-xs font-medium text-wb-ink" :title="a.rel_path">{{ a.name }}</div>
            <div class="truncate text-[10px] text-wb-muted">
              <span>{{ a.rel_path }}</span>
              <span class="ml-2 font-mono">{{ fmtBytes(a.size) }}</span>
              <span class="ml-2">{{ fmtTime(a.updated_at) }}</span>
            </div>
          </div>
          <el-button
            text
            size="small"
            class="opacity-0 transition-opacity group-hover:opacity-100"
            @click="removeArtifact(a.id, a.name)"
          >
            <el-icon color="var(--wb-danger)"><Trash2 /></el-icon>
          </el-button>
        </div>
      </div>
    </div>
  </div>
</template>