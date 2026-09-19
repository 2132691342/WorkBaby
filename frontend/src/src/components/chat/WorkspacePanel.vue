<script setup lang="ts">
import { computed, ref, watch, type Component } from 'vue'
import {
  FileText,
  FolderOpen,
  Image,
  RefreshCw,
  X as Close
} from '@/components/common/icons'
import { apiGet } from '@/api/client'
import { openExternal } from '@/api/shellBridge'
import type { WorkspaceFile } from '@/types/api'
import { t } from '@/i18n'
import Skeleton from '@/components/common/Skeleton.vue'
import WorkspaceFileTree from '@/components/chat/WorkspaceFileTree.vue'

/**
 * 工作区侧栏：两个 tab 展示当前会话的工作区内容。
 *
 * <ul>
 *   <li><b>tree</b>：工作区真实磁盘目录树（懒加载，默认 tab）——用户打开工作区想看的是文件，不是标签</li>
 *   <li><b>artifacts</b>：可内嵌预览的产物（html / url / text / pdf / image）</li>
 * </ul>
 */
const props = defineProps<{
  session_id: string | null
  /** 会话绑定的外部工作目录（空 = 默认工作区）；仅用于头部展示。 */
  workspace_path?: string | null
  /** 打开目录选择器（头部「更换」按钮）。 */
  pick?: () => void
}>()

const emit = defineEmits<{ attach: [path: string] }>()

type TabID = 'tree' | 'artifacts'

interface TabDef {
  id: TabID
  labelKey: string
  icon: Component
}

const tabs: TabDef[] = [
  { id: 'tree', labelKey: 'chat.tabDir', icon: FolderOpen },
  { id: 'artifacts', labelKey: 'chat.tabArtifacts', icon: Image }
]

const activeTab = ref<TabID>('tree')
const files = ref<WorkspaceFile[]>([])
const loading = ref(false)
const selected = ref<WorkspaceFile | null>(null)
const treeRef = ref<InstanceType<typeof WorkspaceFileTree> | null>(null)

/** 头部目录展示：外部目录取末两级，默认工作区给固定文案。 */
const rootDisplay = computed(() => {
  const wp = props.workspace_path
  if (!wp) return t('chat.workspace.default')
  const parts = wp.replace(/\\/g, '/').split('/').filter(Boolean)
  return parts.length <= 2 ? wp : `…/${parts.slice(-2).join('/')}`
})

/** 可预览产物：html / url / text / pdf / image。 */
const previewableFiles = computed(() =>
  files.value.filter((f) => ['html', 'url', 'text', 'pdf', 'image'].includes(f.kind))
)

watch(
  () => [props.session_id, props.workspace_path] as [string | null, string | null | undefined],
  ([id], old) => {
    const [prevId] = old ?? [null]
    selected.value = null
    if (id && id !== prevId) void load(id)
  },
  { immediate: true }
)

/** 产物清单：仅 artifacts tab 用（真实目录树由 WorkspaceFileTree 自行拉取）。 */
async function load(id: string): Promise<void> {
  loading.value = true
  try {
    // 路径与后端路由一致：/chat/workspace/:id/files（此前段序写反，被 catch 吞成空清单）
    files.value = await apiGet<WorkspaceFile[]>(`/api/v1/chat/workspace/${id}/files`)
  } catch {
    files.value = []
  } finally {
    loading.value = false
  }
}

function selectFile(f: WorkspaceFile): void {
  selected.value = f
}

function onTabChange(name: string | number): void {
  activeTab.value = name as TabID
}

function refresh(): void {
  if (!props.session_id) return
  if (activeTab.value === 'tree') treeRef.value?.refresh()
  else void load(props.session_id)
}
</script>

<template>
  <aside class="flex h-full w-full flex-col bg-wb-surface/50">
    <!-- 根目录条：让「当前工作区是哪个目录」一眼可见 -->
    <div class="flex shrink-0 items-center gap-1.5 border-b border-wb-border px-3 py-2">
      <FolderOpen class="h-3.5 w-3.5 shrink-0 text-wb-primary" />
      <span
        class="min-w-0 flex-1 truncate text-xs font-medium text-wb-ink"
        :title="workspace_path || t('chat.workspace.defaultFull')"
      >{{ rootDisplay }}</span>
      <button
        v-if="pick"
        type="button"
        class="shrink-0 rounded px-1.5 py-0.5 text-[11px] text-wb-muted transition-colors hover:bg-wb-surface-hover hover:text-wb-primary-strong"
        :title="t('chat.workspacePicker.title')"
        @click="pick"
      >
        {{ t('chat.workspace.change') }}
      </button>
      <button
        type="button"
        class="flex h-6 w-6 shrink-0 items-center justify-center rounded text-wb-muted transition-colors hover:bg-wb-surface-hover hover:text-wb-ink"
        :disabled="!session_id || loading"
        :title="t('chat.refresh')"
        @click="refresh"
      >
        <RefreshCw class="h-3.5 w-3.5" :class="loading ? 'animate-spin' : ''" />
      </button>
    </div>

    <!-- tab 栏（独立一行，不再与刷新按钮绝对定位互相覆盖） -->
    <el-tabs v-model="activeTab" class="wb-panel-tabs shrink-0" @tab-change="onTabChange">
      <el-tab-pane v-for="tab in tabs" :key="tab.id" :name="tab.id">
        <template #label>
          <span class="flex items-center gap-1.5 text-xs">
            <el-icon :size="14"><component :is="tab.icon" /></el-icon>
            {{ t(tab.labelKey) }}
          </span>
        </template>
      </el-tab-pane>
    </el-tabs>

    <!-- 内容区：tree tab 自带工具行与滚动，外层不套 padding；artifacts 自己滚动 -->
    <div class="min-h-0 flex-1 overflow-hidden">
      <Skeleton v-if="loading && activeTab === 'artifacts'" class="p-2" :lines="4" />

      <!-- tree：工作区真实磁盘目录树（懒加载 + 筛选 + 添加到聊天） -->
      <WorkspaceFileTree
        v-else-if="activeTab === 'tree'"
        ref="treeRef"
        class="h-full"
        :session_id="session_id"
        @attach="(p: string) => emit('attach', p)"
      />

      <!-- artifacts：可内嵌预览的产物 -->
      <div v-else class="h-full overflow-y-auto p-2">
        <el-empty
          v-if="previewableFiles.length === 0"
          :description="t('chat.noArtifacts')"
          :image-size="48"
        />
        <ul v-else class="space-y-0.5">
          <li
            v-for="f in previewableFiles"
            :key="f.path"
            class="group flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-xs transition-colors"
            :class="selected?.path === f.path ? 'bg-wb-primary/10 text-wb-primary-strong' : 'text-wb-ink hover:bg-wb-primary/5'"
            @click="selectFile(f)"
          >
            <component :is="f.kind === 'image' ? Image : FileText" class="h-3.5 w-3.5 shrink-0" />
            <span class="min-w-0 flex-1 truncate">{{ f.name || f.path }}</span>
          </li>
        </ul>
      </div>
    </div>

    <!-- 选中文件预览 -->
    <div v-if="selected" class="shrink-0 border-t border-wb-border p-2">
      <div class="mb-1 flex items-center justify-between">
        <span class="truncate text-xs text-wb-ink">{{ selected.name || selected.path }}</span>
        <button
          type="button"
          class="flex h-5 w-5 items-center justify-center rounded text-wb-muted transition-colors hover:bg-wb-surface-hover hover:text-wb-ink"
          @click="selected = null"
        >
          <Close class="h-3 w-3" />
        </button>
      </div>
      <iframe
        v-if="['html', 'url', 'text', 'pdf'].includes(selected.kind)"
        :src="selected.url"
        :title="selected.name"
        class="h-40 w-full rounded-md border border-wb-border bg-wb-surface"
        sandbox="allow-scripts allow-popups allow-forms"
      />
      <img
        v-else-if="selected.kind === 'image'"
        :src="selected.url"
        :alt="selected.name"
        class="max-h-40 w-full rounded-md border border-wb-border bg-wb-surface object-contain"
      />
      <div v-else class="flex items-center justify-between rounded-md border border-wb-border bg-wb-surface px-2 py-1.5 text-xs text-wb-muted">
        <span class="truncate">{{ selected.name }}</span>
        <a :href="selected.url" :download="selected.name" class="cursor-pointer text-wb-primary-strong hover:underline" @click.prevent="openExternal(selected.url)">{{ t('chat.download') }}</a>
      </div>
    </div>
  </aside>
</template>

<style scoped>
/* 只取 el-tabs 的 header 作为 tab 栏，内容区由上方自定义区域承载 */
.wb-panel-tabs :deep(.el-tabs__header) {
  margin: 0;
  padding: 0 4px;
  border-bottom: 1px solid var(--wb-border);
}
.wb-panel-tabs :deep(.el-tabs__content) {
  display: none;
}
.wb-panel-tabs :deep(.el-tabs__item) {
  height: 36px;
  line-height: 36px;
  font-size: 13px;
  padding: 0 10px;
}
.wb-panel-tabs :deep(.el-tabs__nav-wrap)::after {
  display: none;
}
</style>
