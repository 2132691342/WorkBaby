import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as api from '../api'
import type { BootstrapVO, KnowledgeDocVO, ProviderVO, SkillVO } from '../types/api'
import { useToastStore } from './toast'

export const useSettingsStore = defineStore('settings', () => {
  const boot = ref<BootstrapVO | null>(null)
  const providers = ref<ProviderVO[]>([])
  const skills = ref<SkillVO[]>([])
  const docs = ref<KnowledgeDocVO[]>([])
  const values = ref<Record<string, string>>({})
  const ready = ref(false)
  const providersLoading = ref(false)
  const skillsLoading = ref(false)
  const docsLoading = ref(false)
  const modelsLoading = ref(false)
  const error = ref('')

  function fail(e: unknown) {
    error.value = (e as Error)?.message || '加载失败，请稍后再试'
  }

  async function loadBoot() {
    try {
      boot.value = await api.bootstrap()
      values.value = boot.value.settings || {}
      ready.value = true
      return boot.value
    } catch (e) {
      fail(e)
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

  async function loadDocs() {
    docsLoading.value = true
    error.value = ''
    try {
      docs.value = await api.knowledge.docs()
    } catch (e) {
      fail(e)
    } finally {
      docsLoading.value = false
    }
  }

  // 拉取上游模型列表：不让用户手打模型名
  async function loadModels(providerId: string) {
    if (!providerId) return []
    modelsLoading.value = true
    try {
      return await api.providers.models(providerId)
    } catch (e) {
      fail(e)
      return []
    } finally {
      modelsLoading.value = false
    }
  }

  // 设置项写入公共出口：本地先落值再发请求（等往返才点亮会被当成没点上）、
  // 失败弹提示并回滚，成功不弹（开关自身颜色已说明结果）。
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
    docs,
    values,
    ready,
    providersLoading,
    skillsLoading,
    docsLoading,
    modelsLoading,
    error,
    loadBoot,
    loadProviders,
    loadSkills,
    loadDocs,
    loadModels,
    setValue,
  }
})
