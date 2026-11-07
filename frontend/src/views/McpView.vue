<script setup lang="ts">
// MCP 页：一份清单，增删改查 + 开关。
// 定义落在 pi 的 mcp.json（用户级）与工作区 .pi/mcp.json（项目级），那份文件是唯一事实。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import AppIcon from '@/components/common/AppIcon.vue'
import McpDialog from '@/components/mcp/McpDialog.vue'
import McpTable from '@/components/mcp/McpTable.vue'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'
import type { McpListData, McpServer } from '@/types/api'

const { t } = useI18n()
const app = useAppStore()
const toasts = useToasts()

const data = ref<McpListData | null>(null)
const loading = ref(false)
const saving = ref(false)
const busyName = ref('')
const probingName = ref('')
const dialog = ref(false)
const editingName = ref('')

// 表单：一条命令或一个 URL，二选一；scope 决定落到哪份 mcp.json。
const form = ref({
  server_name: '',
  mode: 'command' as 'command' | 'url',
  command_text: '',
  server_url: '',
  transport_kind: 'http',
  desc: '',
  work_dir: '',
  env_text: '',
  /** 连接超时（秒）；空串表示沿用默认。留成字符串才能区分「没填」与「填 0」 */
  timeout: '',
  scope: 'user' as 'user' | 'project',
})

const servers = computed<McpServer[]>(() =>
  [...(data.value?.servers ?? [])].sort((a, b) => a.server_name.localeCompare(b.server_name)),
)

/** pi 就绪才谈得上「装 MCP」。 */
const piInstalled = computed(() => Boolean(app.runtime?.pi_ready))

async function load() {
  loading.value = true
  try {
    data.value = await api.mcpList()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

function resetForm() {
  form.value = {
    server_name: '',
    mode: 'command',
    command_text: '',
    server_url: '',
    transport_kind: 'http',
    desc: '',
    work_dir: '',
    env_text: '',
    timeout: '',
    scope: 'user',
  }
}

function openCreate() {
  editingName.value = ''
  resetForm()
  dialog.value = true
}

function openEdit(row: McpServer) {
  editingName.value = row.server_name
  form.value = {
    server_name: row.server_name,
    mode: row.server_url ? 'url' : 'command',
    command_text: row.command_line ?? '',
    server_url: row.server_url ?? '',
    transport_kind: row.transport_kind === 'http' ? 'http' : 'stdio',
    desc: row.mcp_desc ?? '',
    work_dir: row.mcp_work_dir ?? '',
    env_text: row.mcp_env_text ?? '',
    timeout: row.mcp_timeout ? String(row.mcp_timeout) : '',
    scope: row.scope_kind === 'project' ? 'project' : 'user',
  }
  dialog.value = true
}

async function save() {
  if (!form.value.server_name.trim()) return
  saving.value = true
  try {
    const envList = form.value.env_text
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)
    await api.mcpSave({
      server_name: form.value.server_name.trim(),
      command_text: form.value.mode === 'command' ? form.value.command_text : undefined,
      server_url: form.value.mode === 'url' ? form.value.server_url : undefined,
      transport_kind: form.value.mode === 'url' ? form.value.transport_kind : undefined,
      mcp_desc: form.value.desc || undefined,
      work_dir: form.value.mode === 'command' ? form.value.work_dir || undefined : undefined,
      // 空数组语义是「清掉环境变量」，不是「保持不动」
      env_list: envList,
      // 超时是秒；空串 = 沿用默认，0 也表示不配置。不传这两个语义就没法区分。
      mcp_timeout: form.value.timeout.trim() ? Number(form.value.timeout) : 0,
      mcp_scope: form.value.scope,
    })
    dialog.value = false
    ElMessage.success(t('common.saved'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    saving.value = false
  }
}

// 连接测试：结果用消息条直说，不改行内状态——探测结论是一次性的，不该常驻表格。
async function probe(row: McpServer) {
  if (probingName.value) return
  probingName.value = row.server_name
  try {
    const out = await api.mcpProbe(row.server_name)
    if (out.probe_ok) {
      const tail = out.probe_tools > 0 ? t('mcp.probeTools', { n: out.probe_tools }) : ''
      ElMessage.success(`${out.probe_message}${tail}`)
    } else {
      ElMessage.error(`${t('mcp.probeFail')}: ${out.probe_message}`)
    }
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    probingName.value = ''
  }
}

async function toggle(row: McpServer, enabled: boolean) {
  busyName.value = row.server_name
  try {
    await api.mcpToggle(row.server_name, enabled)
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyName.value = ''
  }
}

async function remove(row: McpServer) {
  try {
    await ElMessageBox.confirm(t('mcp.removeConfirm', { name: row.server_name }), t('mcp.removeTitle'), {
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
    })
  } catch {
    return
  }
  busyName.value = row.server_name
  try {
    await api.mcpRemove(row.server_name)
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyName.value = ''
  }
}

onMounted(() => void load())
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('mcp.title') }}</h1>
        <p class="page-sub">{{ t('mcp.subtitle') }}</p>
      </div>
      <div class="head-actions">
        <button class="wb-btn primary" @click="openCreate">
          <AppIcon name="plus" :size="14" />
          {{ t('mcp.add') }}
        </button>
        <button class="wb-btn icon ghost" :class="{ 'is-loading': loading }" :title="t('common.refresh')" @click="load">
          <AppIcon name="refresh" :size="15" />
        </button>
      </div>
    </header>

    <div class="page-body">
      <section v-if="piInstalled" class="wb-card card-pad path-card">
        <div class="wb-sec-head">
          <span class="wb-sec-title">{{ t('mcp.file') }}</span>
        </div>
        <dl class="path-list">
          <div class="wb-kv">
            <dt>{{ t('mcp.userScope') }}</dt>
            <dd class="wb-mono">{{ data?.user_path || '—' }}</dd>
          </div>
          <div v-if="data?.project_path" class="wb-kv">
            <dt>{{ t('mcp.projectScope') }}</dt>
            <dd class="wb-mono">{{ data.project_path }}</dd>
          </div>
        </dl>
        <p class="hint">{{ t('mcp.fileHint') }}</p>
      </section>

      <p v-if="data?.read_errors?.length" class="notice">
        <AppIcon name="alert" :size="14" />
        {{ data.read_errors.join('；') }}
      </p>

      <McpTable
        :rows="servers"
        :loading="loading"
        :busy-name="busyName"
        :probing-name="probingName"
        @create="openCreate"
        @edit="openEdit"
        @remove="remove"
        @toggle="toggle"
        @probe="probe"
      />
    </div>

    <McpDialog
      v-model:open="dialog"
      :form="form"
      :editing="editingName"
      :saving="saving"
      :project-path="data?.project_path || ''"
      @save="save"
    />
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
</style>
