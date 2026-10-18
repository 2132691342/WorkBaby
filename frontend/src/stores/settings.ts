// 全局设置与引导数据：外观、模型服务、技能与 PI 配置快照的共享状态。
import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as api from '../api'
import type { BootstrapVO, ExtensionListRESP, MCPListRESP, PISettingsVO, ProviderListRESP, SkillVO, ToolListRESP } from '../types/api'
import { useToastStore } from './toast'

export const useSettingsStore = defineStore('settings', () => {
  const boot = ref<BootstrapVO | null>(null)
  const providers = ref<ProviderListRESP | null>(null)
  const skills = ref<SkillVO[]>([])
  const piSettings = ref<PISettingsVO | null>(null)
  const values = ref<Record<string, string>>({})
  // 预热数据：工具 / 插件 / MCP 面板共用，warmup() 一次拉齐后各面板直接读
  const tools = ref<ToolListRESP | null>(null)
  const extensions = ref<ExtensionListRESP | null>(null)
  const mcp = ref<MCPListRESP | null>(null)
  const warmed = ref(false)
  const ready = ref(false)
  const providersLoading = ref(false)
  const skillsLoading = ref(false)
  const error = ref('')

  function fail(e: unknown) {
    error.value = (e as Error)?.message || '加载失败，请稍后再试'
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
      boot.value = await api.bootstrap()
      values.value = boot.value.settings || {}
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
      providers.value = await api.providers.list()
    } catch (e) {
      fail(e)
    } finally {
      providersLoading.value = false
    }
  }

  /** 默认模型只活在 bootstrap 快照里：改了要重拉 */
  async function refreshBoot() {
    try {
      boot.value = await api.bootstrap()
      values.value = boot.value.settings || {}
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

  /** 首屏预热：五路清单一次拉齐；失败静默，各面板自带重拉与错误态。 */
  async function warmup() {
    try {
      const r = await api.warmup()
      skills.value = r.skills ?? []
      if (r.providers) providers.value = r.providers
      if (r.tools) tools.value = r.tools
      if (r.extensions) extensions.value = r.extensions
      if (r.mcp) mcp.value = r.mcp
      warmed.value = true
    } catch {
      /* 预热失败不阻断：面板自己会再拉 */
    }
  }

  async function loadPISettings() {
    try {
      piSettings.value = await api.settings.pi()
    } catch (e) {
      fail(e)
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
      useToastStore().bad(`保存设置失败：${(e as Error)?.message || '请重试'}`)
      throw e
    }
  }

  return {
    boot,
    providers,
    skills,
    piSettings,
    values,
    tools,
    extensions,
    mcp,
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
    loadPISettings,
    warmup,
    setValue,
  }
})
