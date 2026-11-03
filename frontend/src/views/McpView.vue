<script setup lang="ts">
// MCP 页：一份清单 × 每引擎一列开关的矩阵（借鉴 cc-switch）。
// 两引擎共用 mcp.json 的 mcpServers 形状，读写一份代码；各自的文件仍是唯一事实——
// 矩阵只是视图与同步动作：装到另一边 = 把那边现成的定义复制过来。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import AppIcon from '@/components/common/AppIcon.vue'
import McpDialog from '@/components/mcp/McpDialog.vue'
import McpTable from '@/components/mcp/McpTable.vue'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'
import type { EngineKind, McpListData, McpMatrixRow, McpServer } from '@/types/api'

const { t } = useI18n()
const app = useAppStore()
const toasts = useToasts()

const data = ref<McpListData | null>(null)
const loading = ref(false)
const saving = ref(false)
const busyName = ref('')
const testing = ref(false)
const dialog = ref(false)
const editingName = ref('')
// 写入目标：编辑时默认勾上已有定义的引擎，新建时默认两个都装
const targets = ref<{ omp: boolean; pi: boolean }>({ omp: true, pi: true })

// 表单：一条命令或一个 URL，二选一
const form = ref({
  server_name: '',
  mode: 'command' as 'command' | 'url',
  command_text: '',
  server_url: '',
  transport_kind: 'http',
  desc: '',
  work_dir: '',
  env_text: '',
})

const installedKinds = computed(() => app.installed.map((e) => e.kind))
const piInstalled = computed(() => installedKinds.value.includes('pi'))
const servers = computed<McpServer[]>(() => data.value?.servers ?? [])

/** 矩阵：按名合并两引擎的行。同名在两边各有一条时各占一格。 */
const matrix = computed<McpMatrixRow[]>(() => {
  const map = new Map<string, McpMatrixRow>()
  for (const row of servers.value) {
    const kind = row.engine_kind as EngineKind
    let entry = map.get(row.server_name)
    if (!entry) {
      entry = {
        server_name: row.server_name,
        transport_kind: row.transport_kind,
        command_line: row.command_line,
        server_url: row.server_url,
        desc: row.desc,
        cells: {},
      }
      map.set(row.server_name, entry)
    }
    entry.cells[kind] = row
  }
  return [...map.values()].sort((a, b) => a.server_name.localeCompare(b.server_name))
})

/** 各引擎的用户级配置路径：从行上的 config_path 取（该引擎一条服务器都没有时显示 —）。 */
function userPathOf(kind: EngineKind): string {
  return servers.value.find((s) => s.engine_kind === kind && s.scope_kind === 'user')?.config_path ?? ''
}

async function load(live = false) {
  loading.value = true
  try {
    data.value = await api.mcpList(undefined, live && piInstalled.value)
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingName.value = ''
  targets.value = { omp: installedKinds.value.includes('omp'), pi: piInstalled.value }
  form.value = {
    server_name: '',
    mode: 'command',
    command_text: '',
    server_url: '',
    transport_kind: 'http',
    desc: '',
    work_dir: '',
    env_text: '',
  }
  dialog.value = true
}

function openEdit(row: McpMatrixRow) {
  // 表单初值取优先引擎的那份定义（omp 优先）
  const cell = row.cells.omp ?? row.cells.pi
  if (!cell) return
  editingName.value = row.server_name
  targets.value = { omp: Boolean(row.cells.omp), pi: Boolean(row.cells.pi) }
  form.value = {
    server_name: row.server_name,
    mode: cell.server_url ? 'url' : 'command',
    command_text: cell.command_line ?? '',
    server_url: cell.server_url ?? '',
    transport_kind: cell.transport_kind === 'sse' ? 'sse' : 'http',
    desc: cell.desc ?? '',
    work_dir: '',
    env_text: '',
  }
  dialog.value = true
}

async function save() {
  if (!form.value.server_name.trim()) return
  const kinds = (['omp', 'pi'] as EngineKind[]).filter((k) => targets.value[k])
  if (!kinds.length) return
  saving.value = true
  try {
    const envList = form.value.env_text
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)
    for (const kind of kinds) {
      await api.mcpSave({
        engine_kind: kind,
        server_name: form.value.server_name.trim(),
        command_text: form.value.mode === 'command' ? form.value.command_text : undefined,
        server_url: form.value.mode === 'url' ? form.value.server_url : undefined,
        transport_kind: form.value.mode === 'url' ? form.value.transport_kind : undefined,
        desc: form.value.desc || undefined,
        env_list: envList.length ? envList : undefined,
        work_dir: form.value.work_dir || undefined,
      })
    }
    dialog.value = false
    ElMessage.success(t('common.saved'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    saving.value = false
  }
}

async function toggle(row: McpMatrixRow, kind: EngineKind, enabled: boolean) {
  busyName.value = row.server_name
  try {
    if (row.cells[kind]) {
      await api.mcpToggle(row.server_name, enabled, kind)
    } else {
      // 那边没有定义：把另一边现成的复制过去再启停
      await api.mcpCopyTo(row.server_name, enabled, kind)
    }
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyName.value = ''
  }
}

async function copyTo(row: McpMatrixRow, kind: EngineKind) {
  busyName.value = row.server_name
  try {
    await api.mcpCopyTo(row.server_name, true, kind)
    ElMessage.success(t('mcp.copied'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyName.value = ''
  }
}

async function remove(row: McpMatrixRow) {
  const kinds = (['omp', 'pi'] as EngineKind[]).filter((k) => row.cells[k])
  try {
    await ElMessageBox.confirm(
      t('mcp.removeConfirm', { name: row.server_name, engines: kinds.join(' / ') }),
      t('mcp.removeTitle'),
      { confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel'), type: 'warning' },
    )
  } catch {
    return
  }
  busyName.value = row.server_name
  try {
    for (const kind of kinds) {
      await api.mcpRemove(row.server_name, kind)
    }
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyName.value = ''
  }
}

// 探测要起一次引擎子进程（几秒），单独一个按钮；只有 pi 提供 `mcp list --json`，
// 探测结果并进矩阵的 pi 格，不整表替换（omp 的行不在这次响应里）。
async function probe() {
  testing.value = true
  try {
    const fresh = await api.mcpList('pi', true)
    if (!fresh.live_checked) {
      ElMessage.info(t('mcp.probeUnavailable'))
      return
    }
    for (const row of fresh.servers) {
      const cell = servers.value.find(
        (s) => s.engine_kind === 'pi' && s.server_name === row.server_name,
      )
      if (cell) {
        cell.live_status = row.live_status
        cell.live_error = row.live_error
        cell.tool_names = row.tool_names
      }
    }
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    testing.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('mcp.title') }}</h1>
        <p class="page-sub">{{ t('mcp.matrixSubtitle') }}</p>
      </div>
      <div class="head-actions">
        <button v-if="piInstalled" class="wb-btn" :class="{ 'is-loading': testing }" :disabled="testing" @click="probe">
          <AppIcon name="search" :size="14" />
          {{ t('mcp.probe') }}
        </button>
        <button class="wb-btn primary" @click="openCreate">
          <AppIcon name="plus" :size="14" />
          {{ t('mcp.add') }}
        </button>
        <button class="wb-btn icon ghost" :class="{ 'is-loading': loading }" :title="t('common.refresh')" @click="load()">
          <AppIcon name="refresh" :size="15" />
        </button>
      </div>
    </header>

    <div class="page-body">
      <section class="wb-card card-pad path-card">
        <div class="wb-sec-head">
          <span class="wb-sec-title">{{ t('mcp.file') }}</span>
        </div>
        <dl class="path-list">
          <div class="wb-kv">
            <dt>omp</dt>
            <dd class="wb-mono">{{ userPathOf('omp') || '—' }}</dd>
          </div>
          <div class="wb-kv">
            <dt>pi</dt>
            <dd class="wb-mono">{{ userPathOf('pi') || '—' }}</dd>
          </div>
        </dl>
        <p class="hint">{{ t('mcp.fileHint') }}</p>
      </section>

      <p v-if="data?.read_errors?.length" class="notice">
        <AppIcon name="alert" :size="14" />
        {{ data.read_errors.join('；') }}
      </p>

      <McpTable
        :rows="matrix"
        :loading="loading"
        :busy-name="busyName"
        :installed="installedKinds"
        @create="openCreate"
        @edit="openEdit"
        @remove="remove"
        @toggle="toggle"
        @copy="copyTo"
      />
    </div>

    <McpDialog v-model:open="dialog" :form="form" :editing="editingName" :saving="saving" @save="save">
      <template #extra>
        <div class="target-row">
          <span class="target-label">{{ t('mcp.writeTargets') }}</span>
          <label class="target-check"><input v-model="targets.omp" type="checkbox" :disabled="!installedKinds.includes('omp')" /> omp</label>
          <label class="target-check"><input v-model="targets.pi" type="checkbox" :disabled="!piInstalled" /> pi</label>
        </div>
      </template>
    </McpDialog>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--wb-sp-4);
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-4);
  border-bottom: 1px solid var(--wb-line);
  flex: none;
}

.page-title {
  margin: 0;
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 650;
  color: var(--wb-ink);
}

.page-sub {
  margin: 4px 0 0;
  max-width: var(--wb-w-desc);
  font-size: var(--wb-fs-sm);
  line-height: 1.65;
  color: var(--wb-mute);
}

.head-actions {
  display: flex;
  gap: var(--wb-sp-2);
}

.page-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-12);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}

.path-card {
  max-width: 720px;
}

.path-list {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  margin: 0;
}

.hint {
  margin: var(--wb-sp-2) 0 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
  line-height: var(--wb-lh-base);
}

.notice {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin: 0;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-r);
  background: var(--wb-warn-soft);
  color: var(--wb-warn);
  font-size: var(--wb-fs-xs);
}

.target-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding-top: var(--wb-sp-2);
}

.target-label {
  font-size: var(--wb-fs-2xs);
  font-weight: 650;
  color: var(--wb-mute);
}

.target-check {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink);
  cursor: pointer;
}

.target-check:has(input:disabled) {
  opacity: 0.45;
  cursor: not-allowed;
}
</style>
