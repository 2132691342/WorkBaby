<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { Folder, Plus } from '@/components/common/icons'
import { useFoldersStore } from '@/stores/folders'
import { useDialog } from '@/composables/useDialog'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'
import type { FolderTreeNode } from '@/types/api'
import { formatDateTime } from '@/utils/time'

/**
 * 文件夹管理页。
 *
 * <p>el-tree 树形展示 + 新建 / 删除 / 重命名 / 移动（el-dialog）；后端维护 path 一致性。
 * <p>色系统一走 wb-* token。
 */
const folders = useFoldersStore()
const dialog = useDialog()
const toast = useToast()
const { tree, selected, error, showCreate, showRename, showMove, newName, newDesc, newParentID, renameName, moveTarget } = storeToRefs(folders)
const { selectFolder, createFolder, doRename, doMove, doDelete } = folders

function flatten(nodes: FolderTreeNode[], depth = 0, out: { node: FolderTreeNode; depth: number }[] = []): typeof out {
  for (const n of nodes) {
    out.push({ node: n, depth })
    flatten(n.children, depth + 1, out)
  }
  return out
}

const flat = computed(() => flatten(tree.value))

/** el-tree 平铺节点（folder 直挂，便于 slot 访问 path/name）。 */
interface TreeItem {
  key: string
  label: string
  folder: Folder
  children: TreeItem[]
}
type Folder = FolderTreeNode['folder']

const treeData = computed<TreeItem[]>(() => toTree(tree.value))

function toTree(nodes: FolderTreeNode[]): TreeItem[] {
  return nodes.map((n) => ({
    key: n.folder.id,
    label: n.folder.name,
    folder: n.folder,
    children: toTree(n.children ?? [])
  }))
}

/** 删除带 confirm（替代 silent delete）。 */
async function confirmDelete(): Promise<void> {
  if (!selected.value) return
  const ok = await dialog.confirm({
    title: t('folder.deleteTitle'),
    content: t('folder.deleteConfirm', selected.value.name),
    danger: true
  })
  if (ok) {
    try {
      await doDelete(selected.value)
      toast.success(t('folder.deletedSuccess'))
    } catch (e) {
      toast.error(t('folder.deleteFailed'), e instanceof Error ? e.message : String(e))
    }
  }
}

onMounted(folders.load)
</script>

<template>
  <div class="wb-ui" style="display: flex; flex: 1; min-height: 0">
    <!-- 左：树（原型 pane-l） -->
    <aside class="pane-l" style="width: 380px">
      <div class="flex-r mb10">
        <div>
          <h2 style="font-size: 13.5px; font-weight: 600">{{ t('folder.title') }}</h2>
          <p class="fs11 muted">{{ t('folder.subtitle') }}</p>
        </div>
        <span class="sp" />
        <button class="btn btn-sm btn-primary" @click="showCreate = true">
          <Plus class="ic ic-sm" />
          <span>{{ t('folder.new') }}</span>
        </button>
      </div>
      <div v-if="error" class="mb-2 rounded-md bg-wb-danger/15 px-2 py-1 text-xs text-wb-danger">
        {{ error }}
      </div>
      <div class="min-h-0 flex-1 overflow-y-auto">
        <el-tree
          v-if="flat.length > 0"
          :data="treeData"
          node-key="key"
          default-expand-all
          :expand-on-click-node="false"
          :current-node-key="selected?.id ?? undefined"
          :props="{ label: 'label', children: 'children' }"
          class="folder-tree"
          @current-change="(data?: TreeItem) => data && selectFolder(data.folder)"
        >
          <template #default="{ data }">
            <span class="flex items-center gap-1.5 text-xs" :title="data.folder.path">
              <Folder class="h-3.5 w-3.5 shrink-0 text-wb-muted" />
              <span class="min-w-0 flex-1 truncate text-wb-ink">{{ data.folder.name }}</span>
              <el-tag v-if="data.children?.length" size="small" type="info" effect="plain">{{ data.children.length }}</el-tag>
            </span>
          </template>
        </el-tree>
        <div v-else class="rounded-xl border border-dashed border-wb-border px-4 py-6 text-center text-xs text-wb-muted">
          <Folder class="mx-auto mb-2 h-5 w-5 text-wb-muted/60" />
          <p>{{ t('folder.empty') }}</p>
        </div>
      </div>
    </aside>

    <!-- 右：详情（原型 pane-r） -->
    <main class="pane-r" style="flex: 1; overflow-y: auto; padding: 22px 24px">
      <div v-if="!selected" class="flex h-full items-center justify-center">
        <div class="text-center">
          <div class="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-wb-primary/10 text-wb-primary">
            <Folder class="h-7 w-7" />
          </div>
          <h3 class="mt-4 font-display text-sm font-semibold text-wb-ink">{{ t('folder.selectHintTitle') }}</h3>
          <p class="mt-1 max-w-xs text-xs text-wb-muted">{{ t('folder.selectHint') }}</p>
          <el-button v-if="tree.length === 0" type="primary" class="mt-5" plain @click="showCreate = true">
            <Plus class="h-3.5 w-3.5" />
            <span>{{ t('folder.new') }}</span>
          </el-button>
        </div>
      </div>
      <template v-else>
        <div class="card p-6">
          <div class="flex items-start gap-3">
            <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-wb-primary/10 text-wb-primary">
              <Folder class="h-5 w-5" />
            </div>
            <div class="min-w-0 flex-1">
              <h2 class="font-display text-lg font-semibold text-wb-ink">{{ selected.name }}</h2>
              <div class="mt-1 flex items-center gap-2 text-xs text-wb-muted">
                <span class="badge-neutral font-mono">{{ selected.path }}</span>
              </div>
            </div>
            <div class="flex shrink-0 gap-2">
              <el-button @click="showRename = true; renameName = selected.name">{{ t('folder.rename') }}</el-button>
              <el-button @click="showMove = true">{{ t('folder.move') }}</el-button>
              <el-button type="danger" @click="confirmDelete">{{ t('folder.delete') }}</el-button>
            </div>
          </div>
          <dl class="mt-6 grid grid-cols-2 gap-4 text-sm md:grid-cols-3">
            <div>
              <dt class="text-xs text-wb-muted">{{ t('folder.id') }}</dt>
              <dd class="mt-1 truncate font-mono text-xs text-wb-ink">{{ selected.id }}</dd>
            </div>
            <div>
              <dt class="text-xs text-wb-muted">{{ t('folder.parent') }}</dt>
              <dd class="mt-1 text-wb-ink">{{ selected.parent_id ?? t('folder.root') }}</dd>
            </div>
            <div>
              <dt class="text-xs text-wb-muted">{{ t('folder.child_count') }}</dt>
              <dd class="mt-1 text-wb-ink">{{ selected.child_count }}</dd>
            </div>
            <div>
              <dt class="text-xs text-wb-muted">{{ t('folder.created') }}</dt>
              <dd class="mt-1 text-wb-ink">{{ formatDateTime(selected.created_at) }}</dd>
            </div>
            <div>
              <dt class="text-xs text-wb-muted">{{ t('folder.updated') }}</dt>
              <dd class="mt-1 text-wb-ink">{{ formatDateTime(selected.updated_at) }}</dd>
            </div>
          </dl>
        </div>
      </template>
    </main>

    <!-- 新建对话框（el-dialog） -->
    <el-dialog v-model="showCreate" :title="t('folder.newTitle')" width="420px" append-to-body>
      <el-form class="wb-el-form" label-position="top" @submit.prevent="createFolder">
        <el-form-item :label="t('folder.name')">
          <el-input v-model="newName" :placeholder="t('folder.name')" />
        </el-form-item>
        <el-form-item :label="t('folder.parent')">
          <el-select v-model="newParentID" :placeholder="t('folder.rootOption')" clearable class="w-full">
            <el-option v-for="entry in flat" :key="'p-' + entry.node.folder.id" :value="entry.node.folder.id" :label="entry.node.folder.name" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('folder.desc')">
          <el-input v-model="newDesc" :placeholder="t('folder.desc')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">{{ t('ui.btn.cancel') }}</el-button>
        <el-button type="primary" @click="createFolder">{{ t('ui.btn.create') }}</el-button>
      </template>
    </el-dialog>

    <!-- 重命名对话框 -->
    <el-dialog v-model="showRename" :title="t('folder.renameTitle')" width="420px" append-to-body>
      <el-input v-model="renameName" />
      <template #footer>
        <el-button @click="showRename = false">{{ t('ui.btn.cancel') }}</el-button>
        <el-button type="primary" @click="doRename">{{ t('ui.btn.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 移动对话框 -->
    <el-dialog v-model="showMove" :title="t('folder.moveTo')" width="420px" append-to-body>
      <el-select v-model="moveTarget" :placeholder="t('folder.rootOption')" clearable class="w-full">
        <el-option
          v-for="entry in flat"
          :key="'m-' + entry.node.folder.id"
          :value="entry.node.folder.id"
          :label="entry.node.folder.name"
          :disabled="entry.node.folder.id === selected?.id"
        />
      </el-select>
      <template #footer>
        <el-button @click="showMove = false">{{ t('ui.btn.cancel') }}</el-button>
        <el-button type="primary" @click="doMove">{{ t('folder.moveBtn') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* el-tree 融入 wb 主题 */
.folder-tree {
  --el-tree-node-hover-bg-color: color-mix(in srgb, var(--wb-primary) 8%, transparent);
  --el-tree-text-color: var(--wb-ink);
  background: transparent;
}
.folder-tree :deep(.el-tree-node__content) {
  border-radius: 8px;
  height: 30px;
}
</style>