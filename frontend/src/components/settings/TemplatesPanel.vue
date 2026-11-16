<script setup lang="ts">
// 提示词模板管理：列表 + 新建/编辑 + 删除。模板存 data\templates 的 md 文件，
// 选中后经斜杠菜单 / 命令面板填进输入框。这里是「怎么存」的入口，不是「怎么用」的。
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { confirmDanger } from '@/utils/confirm'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import WbEmpty from '@/components/common/WbEmpty.vue'
import type { PromptTemplate } from '@/types/api'

const { t } = useI18n()
const toasts = useToasts()

const rows = ref<PromptTemplate[]>([])
const loading = ref(false)
// 首屏取数是否失败：失败时不能显示「还没有模板」——那是一句假断言。
const loadFailed = ref(false)
const dialogOpen = ref(false)
const saving = ref(false)
const editing = ref(false)
// 正在删除的那一条：删除要转圈，别让用户重复点。
const removing = ref('')

// 表单草稿：名字是键，编辑时禁改（改名 = 另存一份）。
const form = ref({ template_name: '', template_desc: '', template_args: '', template_body: '' })

async function load() {
  loading.value = true
  try {
    rows.value = (await api.templates()).rows ?? []
    loadFailed.value = false
  } catch (err) {
    loadFailed.value = true
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
    const result = await api.saveTemplate(f.template_name.trim(), f.template_desc.trim(), f.template_body, f.template_args.trim())
    dialogOpen.value = false
    // 报落盘后的名字（后端会规范化：空格 / 标点 / 大小写）。
    // 报输入值等于骗人——用户下次在斜杠菜单里按那个名字找不到它。
    const stored = result.template_name || f.template_name.trim()
    toasts.push('ok', t('templates.savedAs', { name: stored }))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    saving.value = false
  }
}

async function remove(row: PromptTemplate) {
  const ok = await confirmDanger(
    t('templates.deleteConfirm', { name: row.template_name }),
    t('common.delete'),
    t('common.delete'),
    t('common.cancel'),
  )
  if (!ok) return
  removing.value = row.template_name
  // 与改名同理：确认之后的失败不能落进同一个 catch，否则删失败了界面一声不吭。
  try {
    await api.deleteTemplate(row.template_name)
    toasts.push('ok', t('templates.deleted', { name: row.template_name }))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    removing.value = ''
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
      <button type="button" class="wb-btn sm primary" @click="openCreate">{{ t('common.create') }}</button>
    </div>

    <div v-if="loading" class="tpl-state"><span class="dot busy" /> {{ t('common.loading') }}</div>
    <!-- 取数失败：说清「没取到」并给重试，不说「还没有模板」 -->
    <div v-else-if="loadFailed" class="tpl-state">
      <p class="tpl-failed">{{ t('common.loadFailed') }}</p>
      <button type="button" class="wb-btn sm primary" @click="load">{{ t('common.retry') }}</button>
    </div>
    <WbEmpty
      v-else-if="!rows.length"
      :title="t('templates.emptyTitle')"
      :hint="t('templates.emptyHint')"
    >
      <template #action>
        <button type="button" class="wb-btn sm primary" @click="openCreate">{{ t('templates.create') }}</button>
      </template>
    </WbEmpty>
    <ul v-else class="tpl-list">
      <li v-for="row in rows" :key="row.template_name" class="tpl-row">
        <div class="tpl-main">
          <code class="tpl-name">/{{ row.template_name }}</code>
          <span class="tpl-desc">{{ row.template_desc }}</span>
          <span v-if="row.template_args" class="tpl-args">{{ row.template_args }}</span>
        </div>
        <div class="tpl-actions">
          <button type="button" class="wb-btn sm" @click="openEdit(row)">{{ t('common.edit') }}</button>
          <button type="button"
            class="wb-btn sm danger"
            :class="{ 'is-loading': removing === row.template_name }"
            :disabled="removing === row.template_name"
            @click="remove(row)"
          >
            {{ t('common.delete') }}
          </button>
        </div>
      </li>
    </ul>
  </section>

  <el-dialog
    v-model="dialogOpen"
    :title="editing ? t('templates.edit') : t('templates.create')"
    width="560px"
    :close-on-click-modal="false"
  >
    <el-form label-position="top" size="small" @submit.prevent="save">
      <el-form-item :label="t('templates.name')">
        <el-input v-model="form.template_name" :disabled="editing" :placeholder="t('templates.nameHint')" />
      </el-form-item>
      <el-form-item :label="t('templates.desc')">
        <el-input v-model="form.template_desc" :placeholder="t('templates.descHint')" />
      </el-form-item>
      <el-form-item :label="t('templates.args')">
        <el-input v-model="form.template_args" :placeholder="t('templates.argsHint')" />
      </el-form-item>
      <el-form-item :label="t('templates.body')">
        <el-input
          v-model="form.template_body"
          type="textarea"
          :rows="8"
          :placeholder="t('templates.bodyHint')"
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <button type="button" class="wb-btn sm" :disabled="saving" @click="dialogOpen = false">{{ t('common.cancel') }}</button>
      <button type="button" class="wb-btn sm primary" :class="{ 'is-loading': saving }" :disabled="saving" @click="save">
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
.tpl-failed {
  margin: 0 0 var(--wb-sp-2);
  color: var(--wb-warn);
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
  border-radius: var(--wb-r-md);
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
</style>
