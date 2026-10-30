// 全局设置与引导数据：外观取值、模型服务与首屏预热清单的共享状态。
import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as api from '../api'
import { i18n } from '../i18n'
import type { BootVO, FeatureVO, ProviderVO, SkillVO, ToolVO } from '../types/api'
import { useToastStore } from './toast'

export const useSettingsStore = defineStore('settings', () => {
  const boot = ref<BootVO | null>(null)
  /** 模型服务清单：providers.list() 与 warmup() 都落这里（扁平 VO，不带默认值信封） */
  const providers = ref<ProviderVO[]>([])
  const skills = ref<SkillVO[]>([])
  const tools = ref<ToolVO[]>([])
  const features = ref<FeatureVO[]>([])
  const values = ref<Record<string, string>>({})
  // 预热数据：设置页各面板共用，warmup() 一次拉齐后各面板直接读
  const warmed = ref(false)
  /** 引导数据已加载；引擎是否就绪看 boot.boot_ready */
  const ready = ref(false)
  const providersLoading = ref(false)
  const skillsLoading = ref(false)
  const error = ref('')

  function fail(e: unknown) {
    error.value = (e as Error)?.message || i18n.global.t('common.loadFailed', { msg: '' })
  }

  // 只重读设置表：主题、字体、字号只依赖它，不该被别的子项连坐。
  async function syncSettings() {
    try {
      values.value = { ...values.value, ...(await api.settings.all()) }
      return true
    } catch (e) {
      fail(e)
      return false
    }
  }

  async function loadBoot() {
    try {
      const b = await api.bootstrap()
      boot.value = b
      values.value = b.boot_settings || {}
      ready.value = true
      return boot.value
    } catch (e) {
      fail(e)
      await syncSettings()
      throw e
    }
  }

  async function loadProviders() {
    providersLoading.value = true
    error.value = ''
    try {
      const resp = await api.providers.list()
      providers.value = resp.resp_providers ?? []
    } catch (e) {
      fail(e)
    } finally {
      providersLoading.value = false
    }
  }

  /** 默认模型只活在 bootstrap 快照里：改了要重拉 */
  async function refreshBoot() {
    try {
      const b = await api.bootstrap()
      boot.value = b
      values.value = b.boot_settings || {}
    } catch (e) {
      fail(e)
    }
  }

  async function refreshProviderData() {
    await loadProviders()
    await refreshBoot()
  }

  async function loadSkills() {
    skillsLoading.value = true
    error.value = ''
    try {
      skills.value = await api.skills.list()
    } catch (e) {
      fail(e)
    } finally {
      skillsLoading.value = false
    }
  }

  /** 首屏预热：四路清单一次拉齐；失败静默，各面板自带重拉与错误态。 */
  async function warmup() {
    try {
      const r = await api.warmup()
      providers.value = r.providers ?? []
      skills.value = r.skills ?? []
      tools.value = r.tools ?? []
      features.value = r.features ?? []
      warmed.value = true
    } catch {
      /* 预热失败不阻断：面板自己会再拉 */
    }
  }

  // 设置项写入公共出口：本地先落值再发请求，失败弹提示并回滚。
  async function setValue(key: string, value: string) {
    const prev = values.value[key]
    values.value = { ...values.value, [key]: value }
    try {
      await api.settings.set(key, value)
    } catch (e) {
      values.value = { ...values.value, [key]: prev }
      useToastStore().bad(
        i18n.global.t('common.saveFailed', { msg: (e as Error)?.message || '' }),
      )
      throw e
    }
  }

  return {
    boot,
    providers,
    skills,
    tools,
    features,
    values,
    warmed,
    ready,
    providersLoading,
    skillsLoading,
    error,
    loadBoot,
    syncSettings,
    loadProviders,
    refreshBoot,
    refreshProviderData,
    loadSkills,
    warmup,
    setValue,
  }
})
