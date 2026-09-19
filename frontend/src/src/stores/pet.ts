import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import { useDialog } from '@/composables/useDialog'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'
import { UploadPetSprite, OpenFileDialog, ReadLocalImage, SavePetSpriteImage } from '@/wailsjs/go/main/App'
import type { PetConfig, PetConfigReq, PetSprite, PetSpriteReq } from '@/types/api'

/**
 * 桌宠 store：配置读写 + 形象资产管理（上传 / 编辑 / 删除），端点见 doc/16。
 */
export const usePetStore = defineStore('pet', () => {
  const config = ref<PetConfig | null>(null)
  const sprites = ref<PetSprite[]>([])
  const error = ref<string | null>(null)
  const info = ref<string | null>(null)
  const loading = ref(false)

  const form = ref<PetConfigReq>({ enabled: false, mode: 'swing', scale: 1, bubble_enabled: true, bubble_duration_ms: 3000, click_through: false })
  const spriteName = ref('')
  /** 图片编辑器待处理的源图（data URL）；非空时 PetSpaceView 打开裁剪弹窗。 */
  const editSource = ref('')
  /** 本次编辑对应的默认名称（取自原文件名）。 */
  const editName = ref('')

  /** 同时加载配置与 sprite 列表，并把配置字段回填到表单。 */
  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const [c, s] = await Promise.all([apiGet<PetConfig>('/api/v1/pet/config'), apiGet<PetSprite[]>('/api/v1/pet/sprites')])
      config.value = c
      sprites.value = s
      form.value = { enabled: c.enabled, mode: c.mode, sprite_id: c.sprite_id, position_x: c.position_x, position_y: c.position_y, scale: c.scale, bubble_enabled: c.bubble_enabled, bubble_duration_ms: c.bubble_duration_ms, click_through: c.click_through }
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  /** 保存桌宠配置。反馈统一走 toast（页内横幅会被滚走，且与其他页面不一致）。 */
  async function save(): Promise<void> {
    const toast = useToast()
    error.value = null
    info.value = null
    try {
      config.value = await apiPost<PetConfig>('/api/v1/pet/config/update', form.value)
      toast.success(t('common.saved'))
      window.dispatchEvent(new Event('wb-avatar-refresh'))
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      error.value = msg
      toast.error(t('common.saveFailed'), msg)
    }
  }

  /** 新增 sprite（内置标记恒为 false）。 */
  async function addSprite(): Promise<void> {
    error.value = null
    if (!spriteName.value.trim()) {
      error.value = t('common.nameRequired')
      return
    }
    try {
      await apiPost<PetSprite>('/api/v1/pet/sprites', { name: spriteName.value.trim(), is_builtin: false } satisfies PetSpriteReq)
      spriteName.value = ''
      await load()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  /** 删除指定 sprite。 */
  async function removeSprite(id: string): Promise<void> {
    const dialog = useDialog()
    if (!await dialog.confirm({ title: t('common.confirmDeleteTitle'), content: t('common.confirmDelete'), danger: true })) return
    try {
      await apiPost(`/api/v1/pet/sprites/${id}/delete`)
      await load()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  /** 从本地路径取文件名（去扩展名），作为上传 sprite 的默认名。 */
  function basenameFromPath(p: string): string {
    const seg = p.replace(/\\/g, '/').split('/').pop() ?? 'sprite'
    return seg.replace(/\.[^.]+$/, '') || 'sprite'
  }

  /**
   * 上传自定义 sprite：原生文件对话框选本地图片 → UploadPetSprite(name, srcPath)。
   * 成功后增量插入列表并自动选中，不整体 load()（避免表单被覆盖、预览闪断）。
   */
  async function uploadSpriteFile(_file?: File, name?: string): Promise<PetSprite | null> {
    error.value = null
    info.value = null
    try {
      const selected = await OpenFileDialog(t('pet.pickSprite'), '*.png;*.jpg;*.jpeg;*.gif;*.webp')
      if (!selected) return null
      const created = await UploadPetSprite(name?.trim() || basenameFromPath(selected), selected)
      sprites.value = [created, ...sprites.value.filter((s) => s.id !== created.id)]
      // 自动选中刚上传的形象，预览立即更新（不触发整体 load，避免表单被 config 覆盖）
      if (form.value) form.value.sprite_id = created.id
      info.value = t('pet.uploaded', created.name)
      return created
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return null
    }
  }

  /**
   * 选本地图 → 后端代读成 data URL → 交给图片编辑器（裁剪 / 旋转 / 抠图）。
   * 返回 false 表示用户取消或读取失败，调用方据此决定是否打开编辑器。
   */
  async function pickImageForEdit(): Promise<boolean> {
    const toast = useToast()
    error.value = null
    try {
      const selected = await OpenFileDialog(t('pet.pickSprite'), '*.png;*.jpg;*.jpeg;*.gif;*.webp')
      if (!selected) return false
      editName.value = basenameFromPath(selected)
      editSource.value = await ReadLocalImage(selected)
      return true
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      error.value = msg
      toast.error(t('pet.uploadFailed'), msg)
      return false
    }
  }

  /** 保存编辑器产出的图片（data URL）为新 sprite 并自动选中。 */
  async function saveEditedSprite(dataURL: string, name?: string): Promise<PetSprite | null> {
    const toast = useToast()
    error.value = null
    try {
      const created = await SavePetSpriteImage((name ?? '').trim() || editName.value || 'sprite', dataURL)
      sprites.value = [created, ...sprites.value.filter((s) => s.id !== created.id)]
      form.value.sprite_id = created.id
      toast.success(t('pet.uploaded', created.name))
      window.dispatchEvent(new Event('wb-avatar-refresh'))
      return created as unknown as PetSprite
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      error.value = msg
      toast.error(t('pet.uploadFailed'), msg)
      return null
    }
  }

  return {
    config,
    sprites,
    error,
    info,
    loading,
    form,
    spriteName,
    editSource,
    editName,
    load,
    save,
    addSprite,
    removeSprite,
    uploadSpriteFile,
    pickImageForEdit,
    saveEditedSprite
  }
})
