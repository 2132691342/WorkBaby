<script setup lang="ts">
// 工具面板：清单来自 PI（最近一次会话声明给模型的工具 + MCP 在线工具）。
// 启停写的是 PI settings.json 的 defaultTools，不自己实现任何工具。
// 长清单走分页（默认 10 行，5/10/20/50 可选）；来源用行内标签区分，不再整组铺开。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { ToolListRESP, ToolVO } from '../../types/api'
import { fmtRel } from '../../utils/time'
import AppIcon from '../common/AppIcon.vue'
import SettingsSection from './SettingsSection.vue'

const { t } = useI18n()
const toast = useToastStore()
const settingsStore = useSettingsStore()

const PAGE_SIZES = [5, 10, 20, 50]

const data = ref<ToolListRESP | null>(null)
const busy = ref('')
const filter = ref('')
const source = ref<string>('all')
const page = ref(1)
const pageSize = ref(10)

const SOURCES = ['builtin', 'extension', 'mcp'] as const

// 组内保持 PI 的原序，组间按内置 → 插件 → MCP 排，来源改成行内标签后顺序仍可读
const filtered = computed(() => {
  const q = filter.value.trim().toLowerCase()
  const items = (data.value?.tools ?? []).filter(
    (x) => !q || x.name.toLowerCase().includes(q) || (x.description || '').toLowerCase().includes(q),
  )
  const bySource = SOURCES.map((key) => items.filter((x) => x.source === key)).flat()
  return source.value === 'all' ? bySource : bySource.filter((x) => x.source === source.value)
})

const paged = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filtered.value.slice(start, start + pageSize.value)
})

// 搜索词 / 来源 / 每页行数一变就回第一页；页码越界时收回最后一页
watch([filter, source, pageSize], () => {
  page.value = 1
})
watch(
  () => filtered.value.length,
  (n) => {
    const last = Math.max(1, Math.ceil(n / pageSize.value))
    if (page.value > last) page.value = last
  },
)

const sourceLabel = (s: string) => t((SOURCES as readonly string[]).includes(s) ? `settings.tools.${s}` : 'common.unknown')

const enabledCount = computed(() => (data.value?.tools ?? []).filter((x) => x.enabled).length)
const snapshot = computed(() =>
  data.value?.snapshot === 'session' ? t('settings.tools.snapshotSession') : t('settings.tools.snapshotStatic'),
)

async function reload() {
  try {
    data.value = await api.tools.list()
    settingsStore.tools = data.value
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
  }
}

async function toggle(tool: ToolVO) {
  if (busy.value) return
  busy.value = tool.name
  try {
    await api.tools.toggle(tool.name, !tool.enabled)
    await reload()
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

onMounted(() => {
  if (settingsStore.tools) data.value = settingsStore.tools
  void reload()
})
</script>

<template>
  <div class="panel-wrap">
    <SettingsSection icon="wrench" :title="t('settings.tools.title')" :desc="t('settings.tools.desc')">
      <template #actions>
        <el-button size="small" :disabled="!!busy" @click="reload">{{ t('common.refresh') }}</el-button>
      </template>

      <div class="tool-bar">
        <el-input v-model="filter" size="small" :placeholder="t('common.search')" clearable class="tool-search">
          <template #prefix><AppIcon name="search" size="ic-xs" /></template>
        </el-input>
        <el-radio-group v-model="source" size="small">
          <el-radio-button value="all">{{ t('common.all') }}</el-radio-button>
          <el-radio-button v-for="s in SOURCES" :key="s" :value="s">
            {{ t(`settings.tools.${s}`) }}
          </el-radio-button>
        </el-radio-group>
        <span class="sp" />
        <span class="wb-label">{{ enabledCount }} / {{ data?.tools?.length ?? 0 }}</span>
        <span v-if="data?.snapshot_at" class="tool-snap">
          {{ t('settings.tools.snapshot') }}: {{ snapshot }} · {{ fmtRel(data.snapshot_at) }}
        </span>
      </div>

      <div v-if="!data" class="wb-card pad"><el-skeleton animated :rows="4" /></div>

      <div v-else-if="!(data?.tools ?? []).length" class="wb-card empty-box">
        <span class="wb-tile wb-tile--lg"><AppIcon name="wrench" size="ic-xl" /></span>
        <b>{{ t('settings.tools.empty') }}</b>
        <p>{{ t('settings.tools.emptyHint') }}</p>
      </div>

      <template v-else>
        <div class="tool-list">
          <article v-for="tool in paged" :key="tool.name" class="tool-row" :class="{ 'is-off': !tool.enabled }">
            <span class="tool-tile"><AppIcon name="terminal" size="ic-sm" /></span>
            <div class="tool-tx">
              <span class="tool-name">
                {{ tool.name.replace(/^mcp__/, '') }}
                <el-tag size="small" :class="`is-src-${tool.source}`">{{ sourceLabel(tool.source) }}</el-tag>
                <el-tag v-if="tool.server" size="small" type="info">{{ tool.server }}</el-tag>
              </span>
              <span class="tool-desc">{{ tool.description || '—' }}</span>
            </div>
            <el-switch
              :model-value="tool.enabled"
              :loading="busy === tool.name"
              :disabled="!!busy"
              @change="toggle(tool)"
            />
          </article>

          <div v-if="!paged.length" class="wb-card no-match">{{ t('settings.tools.noMatch') }}</div>
        </div>

        <div class="pager">
          <el-pagination
            v-model:current-page="page"
            v-model:page-size="pageSize"
            size="small"
            background
            layout="total, sizes, prev, pager, next"
            :total="filtered.length"
            :page-sizes="PAGE_SIZES"
          />
        </div>
      </template>
    </SettingsSection>
  </div>
</template>

<style scoped>
.tool-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--wb-sp-3);
}
.tool-search {
  max-width: calc(220px * var(--wb-vw-scale));
}
.tool-snap {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.tool-list {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.tool-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
}
.tool-row.is-off {
  opacity: 0.6;
}
.tool-tile {
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border-radius: var(--wb-radius-sm);
  display: grid;
  place-items: center;
  background: var(--wb-elev-hover);
  color: var(--wb-muted);
  flex: none;
}
.tool-tx {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.tool-name {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
  min-width: 0;
}
.tool-desc {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* 来源标签：内置 / 插件 / MCP 一眼可分，色值仍走令牌 */
.tool-name :deep(.el-tag) {
  font-family: var(--font-sans);
  flex: none;
}
.tool-name :deep(.el-tag.is-src-mcp) {
  background: var(--wb-accent-tint);
  border-color: transparent;
  color: var(--wb-primary);
}
.pager {
  display: flex;
  justify-content: flex-end;
  padding-top: var(--wb-sp-2);
}
.no-match {
  padding: var(--wb-sp-6) 0;
  text-align: center;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
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
.empty-box p {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.sp {
  flex: 1;
}
</style>
