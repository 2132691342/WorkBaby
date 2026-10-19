<script setup lang="ts">
// 工具面板：清单来自 PI（最近一次会话声明给模型的工具 + MCP 在线工具）。
// 启停写的是 PI settings.json 的 defaultTools，不自己实现任何工具。
import { computed, onMounted, ref } from 'vue'
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

const data = ref<ToolListRESP | null>(null)
const busy = ref('')
const filter = ref('')

const groups = computed(() => {
  const q = filter.value.trim().toLowerCase()
  const items = (data.value?.tools ?? []).filter(
    (x) => !q || x.name.toLowerCase().includes(q) || (x.description || '').toLowerCase().includes(q),
  )
  return ['builtin', 'extension', 'mcp']
    .map((key) => ({ key, items: items.filter((x) => x.source === key) }))
    .filter((g) => g.items.length)
})

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
        <span class="wb-label">{{ enabledCount }} / {{ data?.tools?.length ?? 0 }}</span>
        <span class="sp" />
        <span v-if="data?.snapshot_at" class="tool-snap">
          {{ t('settings.tools.snapshot') }}: {{ snapshot }} · {{ fmtRel(data.snapshot_at) }}
        </span>
      </div>

      <div v-if="!data" class="wb-card pad"><el-skeleton animated :rows="4" /></div>

      <div v-else-if="!groups.length" class="wb-card empty-box">
        <span class="wb-tile wb-tile--lg"><AppIcon name="wrench" size="ic-xl" /></span>
        <b>{{ t('settings.tools.empty') }}</b>
        <p>{{ t('settings.tools.emptyHint') }}</p>
      </div>

      <template v-else>
        <div v-for="g in groups" :key="g.key" class="tool-group">
          <div class="wb-label">{{ t(`settings.tools.${g.key}`) }}</div>
          <div class="tool-list">
            <article v-for="tool in g.items" :key="tool.name" class="tool-row" :class="{ 'is-off': !tool.enabled }">
              <span class="tool-tile"><AppIcon name="terminal" size="ic-sm" /></span>
              <div class="tool-tx">
                <span class="tool-name">
                  {{ tool.name }}
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
          </div>
        </div>
      </template>
    </SettingsSection>
  </div>
</template>

<style scoped>
.tool-bar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
}
.tool-search {
  max-width: calc(280px * var(--wb-vw-scale));
}
.tool-snap {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.tool-group {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
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
}
.tool-desc {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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
