import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import { useDialog } from '@/composables/useDialog'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'
import type { Skill, SkillReq, SkillZipResult } from '@/types/api'

/**
 * Skills store（snake_case 契约）。
 *
 * <p>后端以 name 为定位键（/skills/:name/update|delete），前端列表展示 name。
 * 技能包 = SKILL.md(markdown body) + 元信息(description/when_to_use/allowed_tools) + scripts。
 */

const StrUtil = { isBlank: (v: unknown): boolean => v == null || String(v).trim() === '' }

export const useSkillsStore = defineStore('skills', () => {
  const toast = useToast()
  const skills = ref<Skill[]>([])
  const loading = ref(false)
  const importingZip = ref(false)
  const error = ref<string | null>(null)
  const info = ref<string | null>(null)
  const editingID = ref<string | null>(null)
  /** 正在编辑的 Skill 来源（builtin = 只读提示）。 */
  const editingSource = ref<string | null>(null)
  /** 是否处于「新建」模式（列表非空时也显示编辑表单）。 */
  const creating = ref(false)

  const form = ref<SkillReq>(blank())

  function blank(): SkillReq {
    return { name: '', description: '', when_to_use: '', body: '', allowed_tools: [], scripts: [], enabled: true }
  }

  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      skills.value = await apiGet<Skill[]>('/api/v1/skills')
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  function startCreate(): void {
    editingID.value = null
    editingSource.value = null
    creating.value = true
    form.value = blank()
    error.value = null
    info.value = null
  }

  function startEdit(s: Skill): void {
    editingID.value = s.id
    editingSource.value = s.source_kind
    creating.value = false
    form.value = {
      name: s.name,
      description: s.description ?? '',
      when_to_use: s.when_to_use ?? '',
      body: s.body,
      allowed_tools: s.allowed_tools ?? [],
      scripts: s.scripts ?? [],
      enabled: s.enabled
    }
    error.value = null
    info.value = null
  }

  function cancel(): void {
    editingID.value = null
    editingSource.value = null
    creating.value = false
    form.value = blank()
  }

  async function submit(): Promise<Skill | null> {
    error.value = null
    info.value = null
    if (editingSource.value === 'builtin') {
      error.value = t('skill.builtinReadonly')
      return null
    }
    if (StrUtil.isBlank(form.value.name) || StrUtil.isBlank(form.value.body)) {
      error.value = t('skill.errRequired')
      toast.warning(t('skill.errRequired'))
      return null
    }
    try {
      let saved: Skill
      if (editingID.value) {
        saved = await apiPost<Skill>(`/api/v1/skills/${encodeURIComponent(form.value.name)}/update`, form.value)
        info.value = t('common.updated')
      } else {
        saved = await apiPost<Skill>('/api/v1/skills', form.value)
        info.value = t('common.created')
      }
      cancel()
      await load()
      return saved
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      error.value = msg
      toast.error(t('common.saveFailed'), msg)
      return null
    }
  }

  /** 技能包 zip 批量导入（zipPath 由原生文件对话框提供）。 */
  async function importZip(zipPath: string): Promise<boolean> {
    error.value = null
    importingZip.value = true
    try {
      const r = await apiPost<SkillZipResult>('/api/v1/skills/import-zip', { zip_path: zipPath })
      const imported = r?.imported ?? []
      const failed = r?.failed ?? []
      if (imported.length > 0) {
        info.value = t('skill.zipImported', imported.length)
        toast.success(t('skill.zipImported', imported.length))
      }
      if (failed.length > 0) {
        const reasons = failed.map((f) => f.error).join('；')
        error.value = `${t('skill.zipFailed', failed.length)} ${reasons}`
        toast.warning(t('skill.zipFailed', failed.length), reasons)
      }
      await load()
      return imported.length > 0
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      error.value = msg
      toast.error(t('skill.zipImportFailed'), msg)
      return false
    } finally {
      importingZip.value = false
    }
  }

  async function remove(s: Skill): Promise<boolean> {
    const dialog = useDialog()
    if (!await dialog.confirm({ title: t('common.confirmDeleteTitle'), content: t('common.confirmDeleteNamed', s.name), danger: true })) return false
    try {
      await apiPost(`/api/v1/skills/${encodeURIComponent(s.name)}/delete`, {})
      if (editingID.value === s.id) cancel()
      await load()
      info.value = t('common.deleted')
      toast.success(t('common.deleted'))
      return true
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      error.value = msg
      return false
    }
  }

  /** 脚本增删（技能包 scripts 编辑）。 */
  function addScript(): void {
    const scripts = form.value.scripts ?? []
    scripts.push({ name: `script_${scripts.length + 1}`, language: 'javascript', code: '' })
    form.value.scripts = scripts
  }
  function removeScriptAt(i: number): void {
    const scripts = (form.value.scripts ?? []).slice()
    scripts.splice(i, 1)
    form.value.scripts = scripts
  }

  return {
    skills,
    loading,
    importingZip,
    error,
    info,
    editingID,
    editingSource,
    creating,
    form,
    load,
    startCreate,
    startEdit,
    cancel,
    submit,
    importZip,
    remove,
    addScript,
    removeScriptAt
  }
})
