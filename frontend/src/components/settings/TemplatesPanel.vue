<script setup lang="ts">
// 提示词模板管理：列表 + 新建/编辑 + 删除。模板存 data\templates 的 md 文件，
// 选中后经斜杠菜单 / 命令面板填进输入框。这里是「怎么存」的入口，不是「怎么用」的。
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import type { PromptTemplate } from '@/types/api'

const { t } = useI18n()
const toasts = useToasts()

const rows = ref<PromptTemplate[]>([])
const loading = ref(false)
const dialogOpen = ref(false)
const saving = ref(false)
const editing = ref(false)

// 表单草稿：名字是键，编辑时禁改（改名 = 另存一份）。
const form = ref({ template_name: '', template_desc: '', template_args: '', template_body: '' })

async function load() {
  loading.value = true
  try {
    rows.value = (await api.templates()).rows ?? []
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = false
  form.value = { template_name: '', template_desc: '', template_args: '', template_body: '' }
  dialogOpen.value = true
}

function openEdit(row: PromptTemplate) {
  editing.value = true
  form.value = {
    template_name: row.template_name,
    template_desc: row.template_desc,
    template_args: row.template_args,
    template_body: row.template_body,
  }
  dialogOpen.value = true
}

async function save() {
  const f = form.value
  if (!f.template_name.trim() || !f.template_body.trim()) {
    toasts.push('error', t('templates.needNameBody'))
    return
  }
  saving.value = true
  try {
    await api.saveTemplate(f.template_name.trim(), f.template_desc.trim(), f.template_body, f.template_args.trim())
    dialogOpen.value = false
    toasts.push('ok', t('common.saved'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    saving.value = false
  }
}

async function remove(row: PromptTemplate) {
  try {
    await ElMessageBox.confirm(t('templates.deleteConfirm', { name: row.template_name }), t('common.delete'), {
      type: 'warning',
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }
  try {
    await api.deleteTemplate(row.template_name)
    toasts.push('ok', t('common.saved'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

onMounted(load)
</script>

<template>
  <section class="wb-card card-pad">
    <div class="wb-sec-head">
      <div>
        <span class="wb-sec-title">{{ t('templates.title') }}</span>
        <p class="tpl-sub">{{ t('templates.subtitle') }}</p>
      </div>
      <button class="wb-btn sm primary" @click="openCreate">{{ t('common.create') }}</button>
    </div>

    <div v-if="loading" class="tpl-state"><span class="dot busy" /> {{ t('common.loading') }}</div>
    <div v-else-if="!rows.length" class="tpl-state">{{ t('templates.empty') }}</div>
    <ul v-else class="tpl-list">
      <li v-for="row in rows" :key="row.template_name" class="tpl-row">
        <div class="tpl-main">
          <code class="tpl-name">/{{ row.template_name }}</code>
          <span class="tpl-desc">{{ row.template_desc }}</span>
          <span v-if="row.template_args" class="tpl-args">{{ row.template_args }}</span>
        </div>
        <div class="tpl-actions">
          <button class="wb-btn sm" @click="openEdit(row)">{{ t('common.edit') }}</button>
          <button class="wb-btn sm danger" @click="remove(row)">{{ t('common.delete') }}</button>
        </div>
      </li>
    </ul>
  </section>

  <el-dialog v-model="dialogOpen" :title="editing ? t('templates.edit') : t('templates.create')" width="560px">
    <div class="tpl-form">
      <label class="tpl-label">
        {{ t('templates.name') }}
        <input v-model="form.template_name" class="tpl-input" :disabled="editing" :placeholder="t('templates.nameHint')" />
      </label>
      <label class="tpl-label">
        {{ t('templates.desc') }}
        <input v-model="form.template_desc" class="tpl-input" :placeholder="t('templates.descHint')" />
      </label>
      <label class="tpl-label">
        {{ t('templates.args') }}
        <input v-model="form.template_args" class="tpl-input" :placeholder="t('templates.argsHint')" />
      </label>
      <label class="tpl-label">
        {{ t('templates.body') }}
        <textarea v-model="form.template_body" class="tpl-body" rows="8" :placeholder="t('templates.bodyHint')" />
      </label>
    </div>
    <template #footer>
      <button class="wb-btn sm" :disabled="saving" @click="dialogOpen = false">{{ t('common.cancel') }}</button>
      <button class="wb-btn sm primary" :class="{ 'is-loading': saving }" :disabled="saving" @click="save">
        {{ t('common.save') }}
      </button>
    </template>
  </el-dialog>
</template>

<style scoped>
.tpl-sub {
  margin: 2px 0 0;
  font-size: var(--wb-fs-xs);
  color: var(--wb-faint);
}
.tpl-state {
  padding: var(--wb-sp-4);
  font-size: var(--wb-fs-sm);
  color: var(--wb-faint);
}
.tpl-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}
.tpl-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-md);
}
.tpl-main {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.tpl-name {
  flex: none;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-brand);
}
.tpl-desc {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-sm);
  color: var(--wb-mute);
}
.tpl-args {
  flex: none;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}
.tpl-actions {
  flex: none;
  display: flex;
  gap: var(--wb-sp-1);
}
.tpl-form {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.tpl-label {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  font-size: var(--wb-fs-xs);
  color: var(--wb-mute);
}
.tpl-input,
.tpl-body {
  padding: var(--wb-sp-2);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-md);
  background: var(--wb-control-bg);
  color: var(--wb-ink);
  font-size: var(--wb-fs-sm);
}
.tpl-body {
  font-family: var(--wb-font-mono);
  resize: vertical;
}
</style>
