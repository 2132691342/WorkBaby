<script setup lang="ts">
/**
 * 自定义斜杠命令管理：保存常用提示词为命令，
 * / 面板选中即灌入输入框（$ARGUMENTS 占位就地编辑）。
 */
import { onMounted, ref } from 'vue'
import { Terminal, Plus, Trash2 } from '@/components/common/icons'
import { apiGet, apiPost } from '@/api/client'
import { useDialog } from '@/composables/useDialog'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'
import FormDialog from '@/components/common/FormDialog.vue'
import Field from '@/components/common/Field.vue'
import type { UserCommand, UserCommandReq } from '@/types/api'

const dialog = useDialog()
const toast = useToast()
const list = ref<UserCommand[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const editingName = ref<string | null>(null)
const showForm = ref(false)
const saving = ref(false)
const form = ref<UserCommandReq>(blank())

function blank(): UserCommandReq {
  return { name: '', prompt: '', description: '' }
}

onMounted(load)

async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    list.value = await apiGet<UserCommand[]>('/api/v1/chat/commands/custom')
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  editingName.value = null
  form.value = blank()
  error.value = null
  showForm.value = true
}
function openEdit(c: UserCommand): void {
  editingName.value = c.name
  form.value = { name: c.name, prompt: c.prompt, description: c.description ?? '' }
  error.value = null
  showForm.value = true
}
function closeForm(): void {
  if (saving.value) return
  showForm.value = false
}

async function submitForm(): Promise<void> {
  saving.value = true
  error.value = null
  try {
    await apiPost('/api/v1/chat/commands/custom', form.value)
    showForm.value = false
    await load()
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    error.value = msg
    toast.error(t('common.saveFailed'), msg)
  } finally {
    saving.value = false
  }
}

async function remove(c: UserCommand): Promise<void> {
  const ok = await dialog.confirm({ title: t('common.confirmDeleteTitle'), content: t('commands.deleteConfirm', c.name), danger: true })
  if (!ok) return
  try {
    await apiPost(`/api/v1/chat/commands/custom/${encodeURIComponent(c.name)}/delete`)
    await load()
  } catch (e) {
    toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
  }
}
</script>

<template>
  <div class="scroll wb-ui">
    <div class="wrap wrap-md">
      <header class="hero">
        <div class="tile"><Terminal class="ic" /></div>
        <div>
          <h1>{{ t('commands.title') }}</h1>
          <p>{{ t('commands.subtitle') }}</p>
        </div>
        <span class="sp" />
        <button class="btn btn-primary" @click="openCreate">
          <Plus class="ic ic-sm" />
          {{ t('commands.new') }}
        </button>
      </header>

      <div v-if="error && !showForm" class="alert a-danger">{{ error }}</div>

      <section class="card p-5">
        <h2 class="mb-3 font-display text-sm font-semibold text-wb-ink">{{ t('commands.list') }}</h2>
        <div v-if="loading" class="text-sm text-wb-muted">{{ t('ui.status.loading') }}</div>
        <el-table v-else-if="list.length > 0" :data="list" stripe class="wb-el-table">
          <el-table-column :label="t('commands.name')" min-width="140">
            <template #default="{ row }">
              <span class="font-medium text-wb-ink">/{{ (row as UserCommand).name }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('commands.description')" min-width="180" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="text-xs text-wb-muted">{{ (row as UserCommand).description || t('commands.noDescription') }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('commands.prompt')" min-width="260" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="font-mono text-xs text-wb-muted">{{ (row as UserCommand).prompt }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('memory.center.col.actions')" width="110" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" size="small" @click="openEdit(row as UserCommand)">{{ t('ui.btn.edit') }}</el-button>
              <el-button link type="danger" size="small" @click="remove(row as UserCommand)">
                <Trash2 class="h-3.5 w-3.5" />
              </el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-else :description="t('commands.empty')" :image-size="80" class="py-6">
          <el-button type="primary" size="small" @click="openCreate">
            <Plus class="h-3.5 w-3.5" />
            {{ t('commands.new') }}
          </el-button>
        </el-empty>
        <p class="fs11 muted" style="margin: 8px 0 0">{{ t('commands.usageHint') }}</p>
        <p class="fs11 muted" style="margin: 4px 0 0">{{ t('commands.fileHint') }}</p>
      </section>
    </div>

    <FormDialog
      v-model="showForm"
      :title="editingName ? t('commands.edit') : t('commands.new')"
      :submitting="saving"
      :confirm-text="editingName ? t('ui.btn.save') : t('ui.btn.create')"
      @confirm="submitForm"
      @close="closeForm"
    >
      <div class="wb-fgrid">
        <Field :label="t('commands.name')" required>
          <input v-model="form.name" class="input mono" :placeholder="t('commands.nameHint')" />
        </Field>
        <Field :label="t('commands.description')" full>
          <input v-model="form.description" class="input" :placeholder="t('commands.descriptionHint')" />
        </Field>
        <Field :label="t('commands.prompt')" required full>
          <textarea v-model="form.prompt" class="input mono" rows="8" :placeholder="t('commands.promptHint')" />
        </Field>
      </div>
      <p class="fs11 muted" style="margin: 6px 0 0">{{ t('commands.formHint') }}</p>
    </FormDialog>
  </div>
</template>
