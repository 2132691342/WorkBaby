<script setup lang="ts">
/**
 * Skill 管理视图（技能包编辑）。
 *
 * <p><b>技能包</b> = SKILL.md 正文(markdown) + 元信息(description / when_to_use / allowed_tools)
 * + scripts（可执行脚本，经 run_skill_script 工具运行）。
 * 来源：内置（只读）/ 手动编辑 / 本地 zip 批量导入。
 */
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { Zap, Upload, FileText, Plus, Trash2 } from '@/components/common/icons'
import { OpenFileDialog } from '@/wailsjs/go/main/App'
import { apiPost } from '@/api/client'
import { useSkillsStore } from '@/stores/skills'
import { t } from '@/i18n'
import { useToast } from '@/composables/useToast'
import FormDialog from '@/components/common/FormDialog.vue'
import Field from '@/components/common/Field.vue'
import type { Skill, SkillScript } from '@/types/api'

const skills = useSkillsStore()
const { skills: list, loading, importingZip, error, info, editingID, editingSource, form } = storeToRefs(skills)
const toast = useToast()

onMounted(() => {
  skills.load()
})

// ===== 新建 / 编辑统一弹窗 =====
const showForm = ref(false)
const saving = ref(false)

function openCreate(): void {
  skills.startCreate()
  showForm.value = true
}
function openEdit(s: Skill): void {
  skills.startEdit(s)
  showForm.value = true
}
function closeForm(): void {
  if (saving.value) return
  skills.cancel()
  showForm.value = false
}
async function submitForm(): Promise<void> {
  saving.value = true
  try {
    const saved = await skills.submit()
    if (saved) showForm.value = false
  } finally {
    saving.value = false
  }
}

/** 列表行内启停开关。 */
async function toggleEnabled(s: Skill, enabled: boolean): Promise<void> {
  try {
    await apiPost(`/api/v1/skills/${encodeURIComponent(s.name)}/enabled`, { enabled })
    await skills.load()
  } catch (e) {
    toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
  }
}

/** 内置技能只读（后端 UpdateCustom 也会拒绝）。 */
const readOnly = computed(() => editingSource.value === 'builtin')

// ===== 本地 zip 批量导入 =====
async function pickZipAndImport(): Promise<void> {
  try {
    const path = await OpenFileDialog(t('skill.zipPickTitle'), '*.zip')
    if (!path) return
    await skills.importZip(path)
  } catch (e) {
    toast.error(t('skill.zipImportFailed'), e instanceof Error ? e.message : String(e))
  }
}

// ===== scripts 包内脚本编辑 =====
const LANG_OPTIONS = [
  { value: 'javascript', label: t('skill.scriptLang.javascript') },
  { value: 'python', label: t('skill.scriptLang.python') },
  { value: 'powershell', label: t('skill.scriptLang.powershell') },
  { value: 'bash', label: t('skill.scriptLang.bash') }
]
const scriptDialogOpen = ref(false)
const scriptIdx = ref(-1)
const scriptForm = ref<SkillScript>({ name: '', language: 'javascript', code: '' })

function openScriptCreate(): void {
  skills.addScript()
  scriptIdx.value = (form.value.scripts ?? []).length - 1
  scriptForm.value = { name: '', language: 'javascript', code: '' }
  scriptDialogOpen.value = true
}
function openScriptEdit(i: number): void {
  const s = (form.value.scripts ?? [])[i]
  if (!s) return
  scriptIdx.value = i
  scriptForm.value = { name: s.name, language: s.language || 'javascript', code: s.code ?? '' }
  scriptDialogOpen.value = true
}
function saveScript(): void {
  const scripts = (form.value.scripts ?? []).slice()
  if (!scriptForm.value.name.trim()) {
    toast.warning(t('skill.scriptNameRequired'))
    return
  }
  scripts[scriptIdx.value] = { name: scriptForm.value.name.trim(), language: scriptForm.value.language, code: scriptForm.value.code }
  form.value.scripts = scripts
  scriptDialogOpen.value = false
}
function removeScript(i: number): void {
  skills.removeScriptAt(i)
}
</script>

<template>
  <div class="scroll wb-ui">
    <div class="wrap wrap-md">
      <!-- Hero header（原型 10 屏） -->
      <header class="hero">
        <div class="tile"><Zap class="ic" /></div>
        <div>
          <h1>{{ t('skill.title') }}</h1>
          <p>{{ t('skill.subtitle') }}</p>
        </div>
        <span class="sp" />
        <button class="btn" :disabled="importingZip" @click="pickZipAndImport">
          <Upload class="ic ic-sm" />
          {{ t('skill.importZip') }}
        </button>
        <button class="btn btn-primary" @click="openCreate">
          <Plus class="ic ic-sm" />
          {{ t('skill.new') }}
        </button>
      </header>

      <div v-if="error && !showForm" class="alert a-danger">{{ error }}</div>
      <div v-if="info" class="alert a-success">{{ info }}</div>

      <!-- Skill 列表 -->
      <section class="card p-5">
        <h2 class="mb-3 font-display text-sm font-semibold text-wb-ink">{{ t('skill.list') }}</h2>
        <div v-if="loading" class="text-sm text-wb-muted">{{ t('ui.status.loading') }}</div>
        <el-table v-else-if="list.length > 0" :data="list" stripe class="wb-el-table">
          <el-table-column :label="t('skill.name')" min-width="200">
            <template #default="{ row }">
              <span class="font-medium text-wb-ink">{{ (row as Skill).name }}</span>
              <el-tag
                v-if="(row as Skill).source_kind"
                size="small"
                effect="plain"
                class="ml-2"
                :type="(row as Skill).source_kind === 'builtin' ? 'info' : 'warning'"
              >
                {{ (row as Skill).source_kind === 'builtin' ? t('pet.builtin') : t('skill.tagImported') }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('skill.description')" min-width="240" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="text-xs text-wb-muted">{{ (row as Skill).description || t('skill.noDescription') }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('skill.scripts')" width="90" align="center">
            <template #default="{ row }">
              <span class="text-xs text-wb-muted">{{ (row as Skill).scripts?.length ?? 0 }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.enabled')" width="80">
            <template #default="{ row }">
              <el-switch
                size="small"
                :model-value="(row as Skill).enabled"
                @change="(v: string | number | boolean) => void toggleEnabled(row as Skill, Boolean(v))"
              />
            </template>
          </el-table-column>
          <el-table-column :label="t('memory.center.col.actions')" width="120" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" size="small" @click="openEdit(row as Skill)">{{ t('ui.btn.edit') }}</el-button>
              <el-button
                v-if="(row as Skill).source_kind !== 'builtin'"
                link
                type="danger"
                size="small"
                @click="skills.remove(row as Skill)"
              >
                {{ t('ui.btn.delete') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-else :description="t('skill.empty')" :image-size="80" class="py-6">
          <el-button type="primary" size="small" @click="openCreate">
            <Plus class="h-3.5 w-3.5" />
            {{ t('skill.new') }}
          </el-button>
        </el-empty>
      </section>
    </div>

    <!-- 新建 / 编辑统一弹窗（技能包：meta + body + scripts） -->
    <FormDialog
      v-model="showForm"
      :title="editingID ? t('skill.edit') : t('skill.new')"
      :submitting="saving"
      :confirm-disabled="readOnly"
      :confirm-text="editingID ? t('ui.btn.save') : t('ui.btn.create')"
      @confirm="submitForm"
      @close="closeForm"
    >
      <el-alert v-if="readOnly" type="info" :closable="false" class="mb-3" :title="t('skill.builtinReadonly')" />
      <div class="wb-fgrid">
        <Field :label="t('skill.name')" required>
          <input v-model="form.name" class="input" :disabled="readOnly" :placeholder="t('skill.name')" />
        </Field>
        <Field :label="t('skill.enabledLabel')">
          <div class="flex items-center" style="height: 28px">
            <span
              class="switch"
              :class="{ on: form.enabled }"
              :style="{ cursor: readOnly ? 'not-allowed' : 'pointer', opacity: readOnly ? 0.5 : 1 }"
              @click="!readOnly && (form.enabled = !form.enabled)"
            />
          </div>
        </Field>
        <Field :label="t('skill.description')" full>
          <input v-model="form.description" class="input" :disabled="readOnly" :placeholder="t('skill.descriptionHint')" />
        </Field>
        <Field :label="t('skill.whenToUse')" full>
          <textarea v-model="form.when_to_use" class="input" rows="2" :disabled="readOnly" :placeholder="t('skill.whenToUseHint')" />
        </Field>
        <Field :label="t('skill.content')" required full>
          <textarea v-model="form.body" class="input mono" rows="10" :disabled="readOnly" :placeholder="t('skill.contentHint')" />
        </Field>
      </div>

      <div class="wb-fsect mt4">
        <p class="wb-fsect__title">{{ t('skill.allowedTools') }}</p>
        <el-select
          v-model="form.allowed_tools"
          :disabled="readOnly"
          multiple
          filterable
          allow-create
          default-first-option
          :reserve-keyword="false"
          class="w-full"
          :placeholder="t('skill.allowedToolsHint')"
        >
          <el-option v-for="tn in form.allowed_tools || []" :key="tn" :value="tn" :label="tn" />
        </el-select>
      </div>

      <!-- 技能包 scripts（如 SKILL.md 包 scripts/ 目录；run_skill_script 执行） -->
      <div class="mt4 rounded-xl border border-wb-border bg-wb-surface/60 p-3">
        <div class="mb-2 flex items-center justify-between">
          <div class="flex items-center gap-1.5 text-xs font-medium text-wb-ink">
            <FileText class="h-3.5 w-3.5 text-wb-mint" />
            {{ t('skill.scripts') }}
            <span class="rounded bg-wb-mint/10 px-1.5 text-[10px] text-wb-mint">{{ (form.scripts ?? []).length }}</span>
          </div>
          <button class="btn btn-sm" :disabled="readOnly" @click="openScriptCreate">
            <Plus class="ic ic-sm" />
            {{ t('skill.scriptAdd') }}
          </button>
        </div>
        <div v-if="(form.scripts ?? []).length === 0" class="text-xs text-wb-muted">{{ t('skill.scriptsEmpty') }}</div>
        <ul v-else class="space-y-1">
          <li
            v-for="(sc, i) in (form.scripts ?? [])"
            :key="`${sc.name}-${i}`"
            class="flex items-center gap-2 rounded-lg bg-wb-surface px-2 py-1.5 text-xs"
          >
            <span class="min-w-0 flex-1 truncate font-mono text-wb-ink">{{ sc.name || '(unnamed)' }}</span>
            <span class="rounded bg-wb-primary/10 px-1.5 text-[10px] text-wb-primary-strong">{{ sc.language }}</span>
            <span class="text-[10px] tabular-nums text-wb-muted">{{ (sc.code ?? '').length }}B</span>
            <button class="btn-icon" :disabled="readOnly" @click="openScriptEdit(i)">{{ t('ui.btn.edit') }}</button>
            <button class="btn-icon" style="color: var(--wb-danger)" :disabled="readOnly" @click="removeScript(i)">
              <Trash2 class="ic ic-sm" />
            </button>
          </li>
        </ul>
      </div>
      <p class="fs11 muted" style="margin: 6px 0 0">{{ t('skill.triggerHint') }}</p>
    </FormDialog>

    <!-- 脚本编辑对话框 -->
    <el-dialog
      :model-value="scriptDialogOpen"
      :title="t('skill.scriptEdit')"
      width="640px"
      append-to-body
      @update:model-value="(v: boolean) => (scriptDialogOpen = v)"
    >
      <div class="space-y-3">
        <div class="grid grid-cols-2 gap-3">
          <el-form-item :label="t('skill.scriptName')" class="mb-0">
            <el-input v-model="scriptForm.name" :placeholder="t('skill.scriptNamePlaceholder')" />
          </el-form-item>
          <el-form-item :label="t('skill.scriptLanguage')" class="mb-0">
            <el-select v-model="scriptForm.language" class="w-full">
              <el-option v-for="o in LANG_OPTIONS" :key="o.value" :value="o.value" :label="o.label" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item :label="t('skill.scriptCode')" class="mb-0">
          <el-input
            v-model="scriptForm.code"
            type="textarea"
            :rows="14"
            class="font-mono"
            :placeholder="t('skill.scriptCodeHint')"
          />
        </el-form-item>
      </div>
      <template #footer>
        <el-button @click="scriptDialogOpen = false">{{ t('ui.btn.cancel') }}</el-button>
        <el-button type="primary" @click="saveScript">{{ t('ui.btn.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>
