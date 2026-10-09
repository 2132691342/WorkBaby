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

  // 所有设置项写入的公共出口：失败必须弹出，否则开关类操作「点了没反应」。
  // 成功不弹——开关自身的状态变化已经说明了结果。
  // 本地先落值再发请求：开关/单选这类控件的「点亮」必须是瞬时的，
  // 等一个 HTTP 往返才变色，用户会以为没点上而重复点。
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
