// 可选模型清单：对话页的模型下拉由它提供。
// 本壳只有一个引擎，所以不再按引擎分桶——但保留「同一时间只飞一个请求」的纪律：
// 两个请求只会互相覆盖，而下拉里看到哪一份是不可预期的。
import { ref } from 'vue'
import { api } from '@/api/http'
import type { ModelRow } from '@/types/api'

/** 模型清单的持有者。 */
export function useEngineModels() {
  const modelCache = ref<ModelRow[]>([])
  const loading = ref(false)

  // 拉可选模型清单，失败不抛：拉不到时输入区仍应可用，只是下拉里只剩「当前模型」。
  async function loadModels(force = false): Promise<ModelRow[]> {
    if (!force && modelCache.value.length) return modelCache.value
    if (loading.value) return modelCache.value
    loading.value = true
    try {
      const data = await api.models()
      modelCache.value = data.models ?? []
      return modelCache.value
    } catch {
      return modelCache.value
    } finally {
      loading.value = false
    }
  }

  /** 直接覆盖清单（后端报上新的时用）。 */
  function setModels(rows: ModelRow[]): void {
    modelCache.value = rows
  }

  return { modelCache, loading, loadModels, setModels }
}
