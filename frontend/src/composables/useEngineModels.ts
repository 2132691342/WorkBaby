// 按引擎分桶的模型清单与思考档：拉清单可能要起进程（omp 冷启动实测 11 秒），
// 不分桶的话「切引擎」这个高频动作每次都付一次冷启动；分桶后只付第一次。
// 独立于 chat store 是因为生命周期不同：这里每个引擎一份，store 里当前会话一份。
import { computed, ref } from 'vue'
import { api } from '@/api/http'
import type { EngineKind, ModelRow } from '@/types/api'

/**
 * 分桶缓存的持有者。
 *
 * 它是一个普通对象而不是一个 store：这个状态只在对话 store 里用，
 * 而对话 store 已经有一个「按引擎分桶」的维度了，再套一层 store 只是多一跳。
 */
export function useEngineModels(kind: () => EngineKind) {
  const modelCache = ref<Record<EngineKind, ModelRow[]>>({ omp: [], pi: [] })
  const levelCache = ref<Record<EngineKind, string[]>>({ omp: [], pi: [] })
  // loading 按引擎分桶，不用一个全局标志：全局 bool 会在上个引擎的拉取
  // 还在飞时让新引擎的清单永远拉不下来——看到的是旧清单，
  // 界面上写的却是新引擎的名字。
  const modelLoading = ref<Record<EngineKind, boolean>>({ omp: false, pi: false })

  /**
   * 当前引擎的模型清单是不是在拉。
   *
   * 它是分桶状态之上的一个「当前引擎视角」：界面问的是「现在这个引擎
   * 还在不在拉」，而不是「有没有哪个引擎在拉」。
   */
  const modelsLoading = computed(() => modelLoading.value[kind()])

  /**
   * 拉可选模型清单。失败不抛：模型清单拉不到时输入区仍应可用，
   * 只是下拉里只剩「当前模型」一项。
   */
  async function loadModels(force = false): Promise<ModelRow[]> {
    const target = kind()
    const cached = modelCache.value[target]
    if (!force && cached.length) return cached
    // 已经在拉这个引擎就不重复拉：两个请求只会互相覆盖。
    if (modelLoading.value[target]) return cached
    modelLoading.value = { ...modelLoading.value, [target]: true }
    try {
      const data = await api.models(target)
      const rows: ModelRow[] = data.models ?? []
      modelCache.value = { ...modelCache.value, [target]: rows }
      return rows
    } catch {
      return cached
    } finally {
      modelLoading.value = { ...modelLoading.value, [target]: false }
    }
  }

  /** 记下某引擎的思考档清单（后端报上来时调用）。 */
  function setLevels(target: EngineKind, values: string[]): void {
    if (!values.length) return
    levelCache.value = { ...levelCache.value, [target]: values }
  }

  /** 取某引擎的思考档清单。 */
  function levelsOf(target: EngineKind): string[] {
    return levelCache.value[target] ?? []
  }

  /** 记下某引擎的模型清单（后端报上来时调用）。 */
  function setModels(target: EngineKind, rows: ModelRow[]): void {
    modelCache.value = { ...modelCache.value, [target]: rows }
  }

  return {
    modelCache,
    levelCache,
    modelsLoading,
    loadModels,
    setLevels,
    levelsOf,
    setModels,
  }
}
