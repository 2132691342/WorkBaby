// 应用偏好：开机自启、恢复上次会话、数据目录、引擎配置原文。
// 不塞进 app store：那里在管 SSE 与事件分流，这里的状态只有设置页一个消费方。
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '@/api/http'
import type { AppPrefs, ConfigFile, ConfigText, EngineKind } from '@/types/api'

export const usePrefsStore = defineStore('prefs', () => {
  const prefs = ref<AppPrefs | null>(null)
  const configFiles = ref<ConfigFile[]>([])
  const configText = ref<ConfigText | null>(null)
  const configKind = ref<EngineKind>('omp')
  const configFile = ref('')
  const loading = ref(false)
  const loadingText = ref(false)

  /**
   * 拉一次偏好。
   *
   * 开机自启的真实值在注册表里，所以这个接口每次都要真读一次，
   * 不能靠本地缓存——否则用户在任务管理器里禁掉启动项之后，
   * 开关会一直显示「开」。
   */
  async function load() {
    loading.value = true
    try {
      prefs.value = await api.prefs()
      return prefs.value
    } finally {
      loading.value = false
    }
  }

  async function setAutoStart(enabled: boolean) {
    prefs.value = await api.setAutoStart(enabled)
    return prefs.value
  }

  async function setRestoreSession(enabled: boolean) {
    prefs.value = await api.setRestoreSession(enabled)
    return prefs.value
  }

  /** 改关窗行为：tray / quit。 */
  async function setCloseAction(action: 'tray' | 'quit') {
    prefs.value = await api.setCloseAction(action)
    return prefs.value
  }

  /** 选一份配置文件并读它的原文。file 为空表示「这个引擎的主配置」。 */
  async function openConfig(kind: EngineKind, file = '') {
    configKind.value = kind
    loadingText.value = true
    try {
      const list = await api.configFiles(kind)
      configFiles.value = list.entries ?? []
      const target = file || configFiles.value[0]?.file_path || ''
      configFile.value = target
      configText.value = target ? await api.configText(target, kind) : null
      return configText.value
    } finally {
      loadingText.value = false
    }
  }

  /** 切到清单里的另一份文件。 */
  async function pickConfig(file: string) {
    configFile.value = file
    loadingText.value = true
    try {
      configText.value = await api.configText(file, configKind.value)
      return configText.value
    } finally {
      loadingText.value = false
    }
  }

  return {
    prefs,
    configFiles,
    configText,
    configKind,
    configFile,
    loading,
    loadingText,
    load,
    setAutoStart,
    setRestoreSession,
    setCloseAction,
    openConfig,
    pickConfig,
  }
})
