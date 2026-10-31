<script setup lang="ts">
// 内置功能清单：独立功能页与设置页共用同一份渲染与开关逻辑。
// 开关即注册表装配——关掉的功能不再把它带的工具并入工具表。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { get, post } from '../../api/http'
import { useToastStore } from '../../stores/toast'
import type { FeatureVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'

const { t } = useI18n()
const toast = useToastStore()

const busy = ref('')
const loading = ref(true)
const items = ref<FeatureVO[]>([])

// 组间按 p1 → p2 → 其余排，组内保持后端原序
const PHASE_ORDER = ['p1', 'p2']
const groups = computed(() => {
  const byPhase = new Map<string, FeatureVO[]>()
  for (const f of items.value) {
    const list = byPhase.get(f.feature_phase) ?? []
    list.push(f)
    byPhase.set(f.feature_phase, list)
  }
  const rank = (p: string) => {
    const i = PHASE_ORDER.indexOf(p)
    return i < 0 ? PHASE_ORDER.length : i
  }
  return [...byPhase.keys()]
    .sort((a, b) => rank(a) - rank(b))
    .map((phase) => ({ phase, items: byPhase.get(phase) ?? [] }))
})

const phaseLabel = (p: string) => p.toUpperCase()

const enabledCount = computed(() => items.value.filter((f) => f.feature_enabled).length)

async function reload() {
  loading.value = true
  try {
    items.value = (await get<FeatureVO[]>('/features')) ?? []
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    loading.value = false
  }
}

async function toggle(f: FeatureVO) {
  if (busy.value) return
  busy.value = f.feature_code
  try {
    await post<boolean>('/features/toggle', {
      feature_code: f.feature_code,
      feature_enabled: !f.feature_enabled,
    })
    await reload()
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

onMounted(() => {
  void reload()
})

defineExpose({ reload, items, enabledCount })
</script>

<template>
  <div class="feat-wrap">
    <div v-if="loading && !items.length" class="wb-card pad">
      <el-skeleton animated :rows="4" />
    </div>

    <div v-else-if="!items.length" class="wb-card empty-box">
      <span class="wb-tile wb-tile--lg"><AppIcon name="zap" size="ic-xl" /></span>
      <b>{{ t('common.empty') }}</b>
    </div>

    <template v-else>
      <section v-for="g in groups" :key="g.phase" class="phase-group">
        <div class="wb-label phase-cap">{{ phaseLabel(g.phase) }}</div>
        <div class="feat-list">
          <article
            v-for="f in g.items"
            :key="f.feature_code"
            class="feat"
            :class="{ 'is-off': !f.feature_enabled }"
          >
            <div class="feat-main">
              <span class="feat-name">{{ f.feature_name }}</span>
              <span class="feat-desc">{{ f.feature_description || '—' }}</span>
              <span v-if="f.feature_tools.length" class="feat-tools">
                <el-tag v-for="tool in f.feature_tools" :key="tool" size="small" class="feat-tool">
                  {{ tool }}
                </el-tag>
              </span>
            </div>
            <el-switch
              :model-value="f.feature_enabled"
              :loading="busy === f.feature_code"
              :disabled="!!busy"
              @change="toggle(f)"
            />
          </article>
        </div>
      </section>
    </template>
  </div>
</template>

<style scoped>
.feat-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.phase-group {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.phase-cap {
  padding: var(--wb-sp-1) 0 0;
}
.feat-list {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.feat {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  transition:
    border-color var(--wb-dur) var(--wb-ease),
    box-shadow var(--wb-dur) var(--wb-ease),
    opacity var(--wb-dur) var(--wb-ease);
}
.feat:hover {
  border-color: var(--wb-border-strong);
  box-shadow: var(--wb-card-shadow);
}
.feat.is-off {
  opacity: 0.62;
}
.feat-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.feat-name {
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
}
.feat-desc {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}
.feat-tools {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-1);
  margin-top: 2px;
}
.feat-tool {
  font-family: var(--font-mono);
}
.pad {
  padding: var(--wb-sp-4);
}
.empty-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8) var(--wb-sp-6);
  text-align: center;
}
.empty-box b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
</style>
