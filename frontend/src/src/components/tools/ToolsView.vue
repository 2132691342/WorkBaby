<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useToast } from '@/composables/useToast'
import { apiGet, apiPost } from '@/api/client'
import { Wrench, File, Terminal, BookOpen, Brain, Cpu, Search, ChevronDown } from '@/components/common/icons'
import { t } from '@/i18n'
import type { Component } from 'vue'

/**
 * 工具清单（照 prd/WorkBaby-UI-Prototype.html 15 屏）：
 * hero + 搜索 + 5 个工具分组（文件与目录 / 命令与网络 / 文档与知识 / 记忆·计划·委派 / 文本·数据·多媒体）。
 * 调 GET /api/v1/tools；group / risk_level / read_only 由后端 tool.MetaOf 推导。
 */
interface ToolParam {
  name: string
  type: string
  required: boolean
  description: string
}
interface ToolItem {
  name: string
  description: string
  risk_level: string
  group: string
  read_only: boolean
  destructive: boolean
  enabled: boolean
  params: ToolParam[]
  schema_json: string
}

const toast = useToast()
const tools = ref<ToolItem[]>([])
const loading = ref(false)
const search = ref('')
const expanded = ref<Set<string>>(new Set())
const toggling = ref<Set<string>>(new Set())

async function load(): Promise<void> {
  loading.value = true
  try {
    tools.value = await apiGet<ToolItem[]>('/api/v1/tools')
  } catch (e) {
    toast.error(e instanceof Error ? e.message : String(e))
    tools.value = []
  } finally {
    loading.value = false
  }
}

onMounted(load)

interface ToolGroup {
  id: string
  titleKey: string
  icon: Component
  items: ToolItem[]
}

/** 固定 5 个分组（原型 15 屏），id 与后端 tool.GroupOf 返回值一一对应。 */
const groupSpecs: { id: string; titleKey: string; icon: Component }[] = [
  { id: 'file', titleKey: 'tools.group.file', icon: File },
  { id: 'exec', titleKey: 'tools.group.exec', icon: Terminal },
  { id: 'doc', titleKey: 'tools.group.doc', icon: BookOpen },
  { id: 'agent', titleKey: 'tools.group.agent', icon: Brain },
  { id: 'media', titleKey: 'tools.group.media', icon: Cpu }
]

function matchQuery(it: ToolItem, q: string): boolean {
  return (
    it.name.toLowerCase().includes(q) ||
    (it.description ?? '').toLowerCase().includes(q) ||
    (it.params ?? []).some((p) => p.name.toLowerCase().includes(q))
  )
}

const groups = computed<ToolGroup[]>(() =>
  groupSpecs
    .map((g) => ({
      ...g,
      items: tools.value.filter((it) => it.group === g.id && matchQuery(it, search.value.trim().toLowerCase()))
    }))
    .filter((g) => g.items.length > 0)
)

/** 有数据但全被搜索过滤掉时给出可恢复的提示，避免整页空白。 */
const noMatch = computed(() => !loading.value && tools.value.length > 0 && groups.value.length === 0)

const total = computed(() => tools.value.length)
const enabledCount = computed(() => tools.value.filter((it) => it.enabled).length)

const riskMeta: Record<string, { class: string; labelKey: string }> = {
  readonly: { class: 'b-neutral', labelKey: 'tools.risk.readonly' },
  write_local: { class: 'b-warning', labelKey: 'tools.risk.writeLocal' },
  exec: { class: 'b-danger', labelKey: 'tools.risk.exec' },
  network: { class: 'b-info', labelKey: 'tools.risk.network' },
  destructive: { class: 'b-danger', labelKey: 'tools.risk.destructive' }
}

function riskBadge(item: ToolItem): { class: string; labelKey: string } {
  return riskMeta[item.risk_level] ?? { class: 'b-neutral', labelKey: 'tools.risk.readonly' }
}

function toggleExpand(name: string): void {
  const next = new Set(expanded.value)
  if (next.has(name)) next.delete(name)
  else next.add(name)
  expanded.value = next
}

async function toggleEnabled(it: ToolItem): Promise<void> {
  if (toggling.value.has(it.name)) return
  const next = it.enabled
  toggling.value = new Set(toggling.value).add(it.name)
  try {
    await apiPost(`/api/v1/tools/${it.name}/enabled`, { enabled: !next })
    it.enabled = !next
  } catch (e) {
    toast.error(e instanceof Error ? e.message : String(e))
  } finally {
    const s = new Set(toggling.value)
    s.delete(it.name)
    toggling.value = s
  }
}
</script>

<template>
  <div class="scroll wb-ui">
    <div class="wrap wrap-md">
      <!-- Hero -->
      <header class="hero">
        <div class="tile"><Wrench class="ic" /></div>
        <div>
          <h1>{{ t('tools.title') }}</h1>
          <p>{{ t('tools.subtitle', total, enabledCount) }}</p>
        </div>
        <span class="sp" />
        <div class="field-wrap" style="width: 230px">
          <Search class="ic ic-sm" />
          <input v-model="search" class="input with-icon" :placeholder="t('tools.search')" />
        </div>
      </header>

      <div v-if="loading" class="empty">{{ t('ui.status.loading') }}</div>
      <div v-else-if="tools.length === 0" class="empty">{{ t('tools.empty') }}</div>
      <div v-else-if="noMatch" class="empty">{{ t('tools.noMatch') }}</div>

      <template v-else>
        <div v-for="g in groups" :key="g.id" class="tgroup mb10">
          <h3>
            <component :is="g.icon" class="ic ic-sm" />
            {{ t(g.titleKey) }}
            <span class="cnt">{{ g.items.length }}</span>
          </h3>
          <div v-for="it in g.items" :key="it.name" class="tcol">
            <div class="titem clickable" @click="toggleExpand(it.name)">
              <ChevronDown class="ic ic-sm chev" :class="{ open: expanded.has(it.name) }" />
              <span class="nm">{{ it.name }}</span>
              <span class="ds">{{ it.description || t('tools.noDesc') }}</span>
              <span class="badge" :class="riskBadge(it).class">{{ t(riskBadge(it).labelKey) }}</span>
              <span v-if="it.read_only" class="badge b-neutral">{{ t('tools.readOnly') }}</span>
              <span class="sw">
                <span
                  class="switch"
                  :class="{ on: it.enabled, busy: toggling.has(it.name) }"
                  :title="it.enabled ? t('tools.enabled') : t('tools.disabled')"
                  @click.stop="toggleEnabled(it)"
                />
              </span>
            </div>
            <div v-if="expanded.has(it.name)" class="tparams">
              <div v-if="!it.params || it.params.length === 0" class="fs11 muted">{{ t('tools.noParams') }}</div>
              <div v-for="p in it.params" :key="p.name" class="tparam">
                <code>{{ p.name }}</code>
                <span class="ty">{{ p.type || 'any' }}</span>
                <span v-if="p.required" class="badge b-danger">{{ t('ui.status.required') }}</span>
                <span class="pd">{{ p.description }}</span>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.tcol {
  border-bottom: 1px solid var(--wb-border);
}
.tcol:last-child {
  border-bottom: 0;
}
.titem.clickable {
  cursor: pointer;
  transition: background 0.12s;
}
.titem.clickable:hover {
  background: var(--wb-surface-hover);
}
.titem .chev {
  flex: none;
  color: var(--wb-muted);
  transition: transform 0.16s;
  transform: rotate(-90deg);
}
.titem .chev.open {
  transform: rotate(0deg);
}
.tparams {
  padding: 8px 12px 10px 34px;
  background: var(--wb-surface-2);
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.tparam {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 11px;
}
.tparam code {
  font-family: var(--font-mono);
  font-size: 11px;
  font-weight: 600;
  min-width: 110px;
}
.tparam .ty {
  font-family: var(--font-mono);
  font-size: 10.5px;
  color: var(--wb-primary);
}
.tparam .pd {
  color: var(--wb-muted);
  flex: 1;
  min-width: 0;
}
.switch.busy {
  opacity: 0.55;
  cursor: progress;
}
</style>
