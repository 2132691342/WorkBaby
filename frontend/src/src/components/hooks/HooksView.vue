<script setup lang="ts">
/**
 * 用户钩子管理（设置中心 tab）：生命周期事件触发用户命令的子进程协议。
 * 七类事件；纯名称名单 / 正则 matcher；PreToolUse 可 deny，Stop 可 block 续跑。
 */
import { onMounted, ref } from 'vue'
import { Webhook, Plus, Trash2, Play } from '@/components/common/icons'
import { apiGet, apiPost } from '@/api/client'
import { useDialog } from '@/composables/useDialog'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'
import FormDialog from '@/components/common/FormDialog.vue'
import Field from '@/components/common/Field.vue'

interface UserHook {
  id: string
  name: string
  event: string
  matcher: string
  command: string
  timeout_ms: number
  enabled: boolean
  sort: number
}
interface HookTest { decision: string; reason: string; err: string; duration_ms: number }

const dialog = useDialog()
const toast = useToast()
const list = ref<UserHook[]>([])
const loading = ref(false)
const showForm = ref(false)
const saving = ref(false)
const testing = ref<string | null>(null)
const editingId = ref<string | null>(null)
const form = ref(blank())

const EVENTS = [
  'SessionStart',
  'UserPromptSubmit',
  'PreToolUse',
  'PermissionRequest',
  'PostToolUse',
  'PostToolUseFailure',
  'Stop',
] as const

/** 参与 matcher 过滤的事件（其余事件的匹配式被后端忽略）。 */
const MATCHER_EVENTS = ['PreToolUse', 'PermissionRequest', 'PostToolUse', 'PostToolUseFailure']

function blank() {
  return { id: '', name: '', event: 'PreToolUse', matcher: '', command: '', timeout_ms: 10000, enabled: true, sort: 0 }
}

onMounted(load)

async function load(): Promise<void> {
  loading.value = true
  try {
    list.value = await apiGet<UserHook[]>('/api/v1/hooks')
  } catch (e) {
    toast.error(t('hooks.loadFailed'), e instanceof Error ? e.message : String(e))
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  editingId.value = null
  form.value = blank()
  showForm.value = true
}
function openEdit(h: UserHook): void {
  editingId.value = h.id
  form.value = { ...h }
  showForm.value = true
}

async function submit(): Promise<void> {
  saving.value = true
  try {
    await apiPost('/api/v1/hooks', form.value)
    showForm.value = false
    await load()
  } catch (e) {
    toast.error(t('common.saveFailed'), e instanceof Error ? e.message : String(e))
  } finally {
    saving.value = false
  }
}

async function toggle(h: UserHook): Promise<void> {
  try {
    await apiPost('/api/v1/hooks', { ...h, enabled: !h.enabled })
    await load()
  } catch (e) {
    toast.error(t('common.saveFailed'), e instanceof Error ? e.message : String(e))
  }
}

async function remove(h: UserHook): Promise<void> {
  const ok = await dialog.confirm({
    title: t('common.confirmDeleteTitle'),
    content: t('hooks.deleteConfirm', h.name),
    danger: true,
  })
  if (!ok) return
  try {
    await apiPost(`/api/v1/hooks/${encodeURIComponent(h.id)}/delete`)
    await load()
  } catch (e) {
    toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
  }
}

/** 试跑：样例载荷执行一次，回执直接弹提示。 */
async function test(h: UserHook): Promise<void> {
  testing.value = h.id
  try {
    const r = await apiPost<HookTest>(`/api/v1/hooks/${encodeURIComponent(h.id)}/test`)
    if (r.err) toast.error(t('hooks.testFailed'), r.err)
    else if (r.decision === 'deny') toast.warning(t('hooks.testDenied'), r.reason || '')
    else toast.success(t('hooks.testAllowed'), `${r.duration_ms}ms`)
  } catch (e) {
    toast.error(t('hooks.testFailed'), e instanceof Error ? e.message : String(e))
  } finally {
    testing.value = null
  }
}

function eventLabel(e: string): string {
  const map: Record<string, string> = {
    SessionStart: t('hooks.eventSessionStart'),
    UserPromptSubmit: t('hooks.eventUserPromptSubmit'),
    PreToolUse: t('hooks.eventPreToolUse'),
    PermissionRequest: t('hooks.eventPermissionRequest'),
    PostToolUse: t('hooks.eventPostToolUse'),
    PostToolUseFailure: t('hooks.eventPostToolUseFailure'),
    Stop: t('hooks.eventStop'),
  }
  return map[e] ?? e
}

/** 可拦截 / 可续跑的事件在列表里用警示色区分：它们真的会改变执行结果。 */
function eventTagType(e: string): 'warning' | 'danger' | 'info' {
  if (e === 'PreToolUse') return 'danger'
  if (e === 'PermissionRequest' || e === 'Stop' || e === 'UserPromptSubmit') return 'warning'
  return 'info'
}

/** 切换事件时清掉不生效的 matcher，避免保存后被后端拒绝。 */
function onEventChange(): void {
  if (!MATCHER_EVENTS.includes(form.value.event)) form.value.matcher = ''
}
</script>

<template>
  <div class="scroll wb-ui">
    <div class="wrap wrap-md">
      <header class="hero">
        <div class="tile"><Webhook class="ic" /></div>
        <div>
          <h1>{{ t('hooks.title') }}</h1>
          <p>{{ t('hooks.subtitle') }}</p>
        </div>
        <span class="sp" />
        <button class="btn btn-primary" @click="openCreate">
          <Plus class="ic ic-sm" />
          {{ t('hooks.new') }}
        </button>
      </header>

      <section class="card p-5">
        <div v-if="loading" class="text-sm text-wb-muted">{{ t('ui.status.loading') }}</div>
        <el-table v-else-if="list.length > 0" :data="list" stripe class="wb-el-table">
          <el-table-column :label="t('hooks.name')" min-width="120">
            <template #default="{ row }">
              <span class="font-medium text-wb-ink">{{ (row as UserHook).name }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('hooks.event')" width="130">
            <template #default="{ row }">
              <el-tag size="small" :type="eventTagType((row as UserHook).event)">
                {{ eventLabel((row as UserHook).event) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('hooks.command')" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="font-mono text-xs text-wb-muted">{{ (row as UserHook).command }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('hooks.enabled')" width="80">
            <template #default="{ row }">
              <el-switch :model-value="(row as UserHook).enabled" size="small" @change="toggle(row as UserHook)" />
            </template>
          </el-table-column>
          <el-table-column :label="t('memory.center.col.actions')" width="150" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" size="small" :loading="testing === (row as UserHook).id" @click="test(row as UserHook)">
                <Play class="h-3.5 w-3.5" />
              </el-button>
              <el-button link type="primary" size="small" @click="openEdit(row as UserHook)">{{ t('ui.btn.edit') }}</el-button>
              <el-button link type="danger" size="small" @click="remove(row as UserHook)">
                <Trash2 class="h-3.5 w-3.5" />
              </el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-else :description="t('hooks.empty')" :image-size="80" class="py-6" />
        <p class="fs11 muted" style="margin: 8px 0 0; white-space: pre-line">{{ t('hooks.protocolHint') }}</p>
      </section>
    </div>

    <FormDialog
      v-model="showForm"
      :title="editingId ? t('hooks.edit') : t('hooks.new')"
      :submitting="saving"
      :confirm-text="editingId ? t('ui.btn.save') : t('ui.btn.create')"
      @confirm="submit"
    >
      <div class="wb-fgrid">
        <Field :label="t('hooks.name')" required>
          <input v-model="form.name" class="input" :placeholder="t('hooks.nameHint')" />
        </Field>
        <Field :label="t('hooks.event')" required>
          <select v-model="form.event" class="input" @change="onEventChange">
            <option v-for="e in EVENTS" :key="e" :value="e">{{ eventLabel(e) }}</option>
          </select>
        </Field>
        <Field :label="t('hooks.matcher')" full>
          <input
            v-model="form.matcher"
            class="input mono"
            :disabled="!MATCHER_EVENTS.includes(form.event)"
            :placeholder="MATCHER_EVENTS.includes(form.event) ? t('hooks.matcherHint') : t('hooks.matcherNa')"
          />
        </Field>
        <Field :label="t('hooks.command')" required full>
          <input v-model="form.command" class="input mono" :placeholder="t('hooks.commandHint')" />
        </Field>
        <Field :label="t('hooks.timeout')">
          <input v-model.number="form.timeout_ms" type="number" class="input" min="1000" max="60000" step="1000" />
        </Field>
        <Field :label="t('hooks.enabled')">
          <el-switch v-model="form.enabled" />
        </Field>
      </div>
      <p class="fs11 muted" style="margin: 6px 0 0; white-space: pre-line">{{ t('hooks.protocolHint') }}</p>
    </FormDialog>
  </div>
</template>
