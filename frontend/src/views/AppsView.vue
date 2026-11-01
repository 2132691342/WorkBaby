<script setup lang="ts">
// 微前端页：左栏登记册 + 右侧运行容器。
// 外部应用是「用户自己接进来的东西」，本页只负责登记、启停与隔离运行。
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { pickFolderSafe } from '../api/shell'
import { useAppsStore, type MicroApp } from '../stores/apps'
import AppIcon from '../components/common/AppIcon.vue'
import MicroAppHost from '../components/apps/MicroAppHost.vue'
import PageHeader from '../components/layout/PageHeader.vue'

const { t } = useI18n()
const apps = useAppsStore()

const selected = ref('')
const dialog = ref(false)
const editing = ref('')
const busy = ref(false)

const form = reactive<MicroApp>({
  app_name: '',
  app_entry: '',
  app_enabled: true,
  app_container: '',
  app_note: '',
})

const errors = reactive({ name: '', entry: '' })

const current = computed(() => apps.apps.find((a) => a.app_name === selected.value) ?? null)

function reset() {
  editing.value = ''
  form.app_name = ''
  form.app_entry = ''
  form.app_enabled = true
  form.app_container = ''
  form.app_note = ''
  errors.name = ''
  errors.entry = ''
}

function openCreate() {
  reset()
  dialog.value = true
}

function openEdit(app: MicroApp) {
  reset()
  editing.value = app.app_name
  Object.assign(form, app)
  dialog.value = true
}

/** 校验顺序与表单顺序一致：一次只报第一处，避免来回试。 */
function validate(): boolean {
  errors.name = ''
  errors.entry = ''
  if (!form.app_name.trim()) errors.name = t('apps.nameRequired')
  if (!form.app_entry.trim()) errors.entry = t('apps.entryRequired')
  // 入口支持 http(s) URL 与本地目录；两者都不像就当场拒
  const entry = form.app_entry.trim()
  const looksUrl = /^https?:\/\//i.test(entry)
  const looksPath = /^[a-zA-Z]:[\\/]/.test(entry) || entry.startsWith('/') || entry.startsWith('\\\\')
  if (entry && !looksUrl && !looksPath) {
    errors.entry = t('apps.entryInvalid')
  }
  return !errors.name && !errors.entry
}

async function save() {
  if (busy.value || !validate()) return
  busy.value = true
  try {
    const payload: MicroApp = {
      app_name: form.app_name.trim(),
      app_entry: form.app_entry.trim(),
      app_enabled: form.app_enabled,
      app_container: form.app_container.trim(),
      app_note: form.app_note.trim(),
    }
    if (editing.value) {
      await apps.update(editing.value, payload)
      if (selected.value === editing.value) selected.value = payload.app_name
    } else {
      const ok = await apps.add(payload)
      if (!ok) return
      selected.value = payload.app_name
    }
    dialog.value = false
  } finally {
    busy.value = false
  }
}

async function toggle(app: MicroApp) {
  await apps.update(app.app_name, { app_enabled: !app.app_enabled })
  if (!app.app_enabled && selected.value === app.app_name) selected.value = ''
}

async function remove(app: MicroApp) {
  await apps.remove(app.app_name)
  if (selected.value === app.app_name) selected.value = ''
}

async function browseEntry() {
  const dir = await pickFolderSafe(t('apps.pickEntry'))
  if (dir) form.app_entry = dir
}

/** 运行状态：登记在册且启用才谈得上挂载。 */
function stateOf(name: string) {
  return apps.stateOf(name)
}

onMounted(() => {
  if (!apps.loaded) void apps.load()
})
</script>

<template>
  <div class="page">
    <PageHeader :title="t('apps.title')" :subtitle="t('apps.desc')">
      <template #actions>
        <el-button size="small" type="primary" @click="openCreate">
          <AppIcon name="plus" size="ic-xs" />
          <span class="ml-1">{{ t('apps.add') }}</span>
        </el-button>
      </template>
    </PageHeader>

    <div class="apps-wrap">
      <aside class="apps-side">
        <div v-if="!apps.apps.length" class="side-empty">{{ t('apps.empty') }}</div>
        <button
          v-for="app in apps.apps"
          :key="app.app_name"
          class="app-row"
          :class="{ 'is-on': app.app_name === selected }"
          type="button"
          @click="selected = app.app_name"
        >
          <span class="app-led" :class="{ 'is-live': stateOf(app.app_name).mounted, 'is-bad': !!stateOf(app.app_name).error }" />
          <span class="app-body">
            <b class="app-name">{{ app.app_name }}</b>
            <span class="app-entry">{{ app.app_entry }}</span>
          </span>
          <el-switch
            :model-value="app.app_enabled"
            size="small"
            @click.stop
            @change="toggle(app)"
          />
        </button>
      </aside>

      <section class="apps-main">
        <div v-if="current" class="apps-tools">
          <el-button size="small" @click="openEdit(current)">{{ t('common.edit') }}</el-button>
          <el-popconfirm :title="t('apps.removeTitle')" width="240" @confirm="remove(current)">
            <template #reference>
              <el-button size="small">{{ t('common.delete') }}</el-button>
            </template>
          </el-popconfirm>
          <span class="apps-sp" />
          <span class="apps-note">{{ current.app_note }}</span>
        </div>
        <MicroAppHost :app="current && current.app_enabled ? current : null" />
      </section>
    </div>

    <el-dialog
      v-model="dialog"
      :title="editing ? t('common.edit') : t('apps.add')"
      :width="`calc(560px * var(--wb-vw-scale))`"
      destroy-on-close
    >
      <el-form label-position="top" @submit.prevent>
        <el-form-item :label="t('apps.name')" required :error="errors.name">
          <el-input v-model="form.app_name" :disabled="!!editing || busy" :placeholder="t('apps.namePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('apps.entry')" required :error="errors.entry">
          <div class="entry-row">
            <el-input v-model="form.app_entry" :disabled="busy" :placeholder="t('apps.entryPlaceholder')" />
            <el-button :disabled="busy" @click="browseEntry">{{ t('apps.pick') }}</el-button>
          </div>
          <p class="hint">{{ t('apps.entryHint') }}</p>
        </el-form-item>
        <el-form-item :label="t('apps.container')">
          <el-input v-model="form.app_container" :disabled="busy" :placeholder="t('apps.containerPlaceholder')" />
          <p class="hint">{{ t('apps.containerHint') }}</p>
        </el-form-item>
        <el-form-item :label="t('apps.note')">
          <el-input v-model="form.app_note" :disabled="busy" />
        </el-form-item>
        <el-form-item>
          <el-switch v-model="form.app_enabled" :active-text="t('apps.enabled')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button :disabled="busy" @click="dialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="busy" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  min-height: 0;
}
.apps-wrap {
  display: flex;
  flex: 1;
  min-height: 0;
  gap: var(--wb-sp-3);
  padding: 0 var(--wb-sp-5) var(--wb-sp-5);
}
.apps-side {
  width: calc(300px * var(--wb-vw-scale));
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--wb-sp-2);
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-xl);
  background: var(--wb-elev-raised);
  box-shadow: var(--wb-card-shadow);
  overflow-y: auto;
}
.side-empty {
  padding: var(--wb-sp-8) var(--wb-sp-4);
  text-align: center;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.app-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  text-align: left;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.app-row:hover {
  background: var(--wb-elev-hover);
  transform: scale(1.01);
}
.app-row:active {
  transform: scale(0.99);
}
.app-row.is-on {
  background: var(--wb-accent-tint);
}
.app-led {
  width: 7px;
  height: 7px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-border-strong);
  flex: none;
}
.app-led.is-live {
  background: var(--wb-success);
}
.app-led.is-bad {
  background: var(--wb-danger);
}
.app-body {
  display: flex;
  flex-direction: column;
  gap: 1px;
  flex: 1;
  min-width: 0;
}
.app-name {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.app-entry {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.apps-main {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-xl);
  background: var(--wb-elev-raised);
  box-shadow: var(--wb-card-shadow);
  overflow: hidden;
}
.apps-tools {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-bottom: 1px solid var(--wb-border);
  flex: none;
}
.apps-sp {
  flex: 1;
}
.apps-note {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 40%;
}
.entry-row {
  display: flex;
  gap: var(--wb-sp-2);
  width: 100%;
}
.hint {
  margin-top: var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
}
</style>
