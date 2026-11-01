// 微前端应用册：登记的外部应用持久化在设置 KV（键 microapps），与对话无关。
// 只描述「哪里有什么应用」，生命周期由 AppsView 用 loadMicroApp 管。
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as api from '../api'
import { i18n } from '../i18n'
import { useToastStore } from './toast'

/** 登记的一个微前端应用。 */
export interface MicroApp {
  /** 应用名（唯一，同时用作 qiankun 的 name） */
  app_name: string
  /** 入口地址（http(s) URL 或本地目录，目录由本地服务托管） */
  app_entry: string
  /** 是否启用（停用的不启动，也不占容器） */
  app_enabled: boolean
  /** 容器选择器（空则用默认容器） */
  app_container: string
  /** 备注 */
  app_note: string
}

/** 设置 KV 里的存储键。 */
const KV_KEY = 'microapps'

/** 运行状态：与持久化无关，只在本次进程里有效。 */
export interface AppRuntime {
  /** 加载中 */
  loading: boolean
  /** 已挂载 */
  mounted: boolean
  /** 最近一条错误（空 = 无错） */
  error: string
}

export const useAppsStore = defineStore('apps', () => {
  const apps = ref<MicroApp[]>([])
  const loaded = ref(false)
  /** 应用名 → 运行状态 */
  const runtime = ref<Record<string, AppRuntime>>({})

  const enabled = computed(() => apps.value.filter((a) => a.app_enabled))

  function stateOf(name: string): AppRuntime {
    if (!runtime.value[name]) {
      runtime.value[name] = { loading: false, mounted: false, error: '' }
    }
    return runtime.value[name]
  }

  function setState(name: string, patch: Partial<AppRuntime>) {
    runtime.value[name] = { ...stateOf(name), ...patch }
  }

  /** 从设置 KV 读取；坏 JSON 视为空册（不挡界面）。 */
  async function load() {
    try {
      const all = await api.settings.all()
      const raw = all[KV_KEY]
      apps.value = raw ? (JSON.parse(raw) as MicroApp[]) : []
    } catch {
      apps.value = []
    } finally {
      loaded.value = true
    }
  }

  /** 落盘：整册写回一个 KV 键，保持单点真相。 */
  async function persist() {
    await api.settings.set(KV_KEY, JSON.stringify(apps.value))
  }

  async function add(app: MicroApp) {
    if (apps.value.some((a) => a.app_name === app.app_name)) {
      useToastStore().bad(i18n.global.t('apps.dupName', { name: app.app_name }))
      return false
    }
    apps.value = [...apps.value, app]
    await persist()
    return true
  }

  async function update(name: string, patch: Partial<MicroApp>) {
    apps.value = apps.value.map((a) => (a.app_name === name ? { ...a, ...patch } : a))
    await persist()
  }

  async function remove(name: string) {
    apps.value = apps.value.filter((a) => a.app_name !== name)
    delete runtime.value[name]
    await persist()
  }

  function clear() {
    apps.value = []
    loaded.value = false
    runtime.value = {}
  }

  return { apps, enabled, loaded, runtime, load, add, update, remove, clear, setState, stateOf }
})
