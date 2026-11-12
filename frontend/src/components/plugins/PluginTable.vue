<script setup lang="ts">
// 插件表：一行一个插件，启停 / 升级 / 重试 / 卸载。
// 只管表格与行内按钮的呈现；动作发给父页面统一挂 loading 与重拉。
import { useI18n } from 'vue-i18n'
import type { PluginRow } from '@/types/api'

const props = defineProps<{
  rows: PluginRow[]
  loading: boolean
  /** 后台有长任务（装配 / 升级全部）：全表按钮跟着哑掉 */
  busy: boolean
  /** 正在卸载 / 升级的那一行 */
  busyName: string
  /** 正在启停的那一行 */
  toggling: string
  /** 正在重试的那一行 */
  upgrading: string
}>()

const emit = defineEmits<{
  toggle: [row: PluginRow, enabled: boolean]
  upgrade: [row: PluginRow]
  retry: [row: PluginRow]
  uninstall: [row: PluginRow]
}>()

const { t } = useI18n()

/** 资源种类拼成一行；没有给破折号，不给空白。 */
function resourceLabel(kinds: string[]): string {
  if (!kinds?.length) return '—'
  return kinds.join(' · ')
}

/** 状态标签：已装 / 已停用 / 缺包 / 失败。 */
function stateLabel(row: PluginRow): string {
  if (row.plugin_state === 'failed') return t('plugins.failedState')
  if (row.plugin_state === 'disabled') return t('plugins.disabledState')
  if (row.plugin_installed) return t('plugins.installedState')
  return t('plugins.missingState')
}

function stateClass(row: PluginRow): string {
  if (row.plugin_state === 'failed') return 'bad'
  if (row.plugin_state === 'disabled') return 'warn'
  if (row.plugin_installed) return 'ok'
  return 'warn'
}
</script>

<template>
  <el-table :data="props.rows" size="small">
    <el-table-column :label="t('plugins.name')" min-width="200">
      <template #default="{ row }">
        <div class="cell-name">
          <span class="name">
            {{ row.plugin_name }}
            <span v-if="row.plugin_recommended" class="tag brand-tag">{{ t('plugins.builtin') }}</span>
          </span>
          <span v-if="row.plugin_desc" class="desc">{{ row.plugin_desc }}</span>
        </div>
      </template>
    </el-table-column>
    <el-table-column :label="t('plugins.version')" width="130">
      <template #default="{ row }">
        <span class="wb-mono">{{ row.plugin_version || '—' }}</span>
        <span
          v-if="(row.plugin_target_version || row.plugin_latest) && row.plugin_version !== (row.plugin_target_version || row.plugin_latest)"
          class="wb-mono target"
          :title="row.plugin_target_version ? t('plugins.targetIs', { v: row.plugin_target_version }) : t('plugins.latestIs', { v: row.plugin_latest })"
        >
          → {{ row.plugin_target_version || row.plugin_latest }}
        </span>
      </template>
    </el-table-column>
    <el-table-column :label="t('plugins.resources')" width="120">
      <template #default="{ row }">
        <span class="wb-mono">{{ resourceLabel(row.resource_kinds) }}</span>
      </template>
    </el-table-column>
    <el-table-column :label="t('plugins.state')" width="96">
      <template #default="{ row }">
        <span class="tag" :class="stateClass(row)" :title="row.plugin_error || ''">{{ stateLabel(row) }}</span>
      </template>
    </el-table-column>
    <el-table-column :label="t('plugins.enabledLabel')" width="80">
      <template #default="{ row }">
        <el-switch
          :model-value="row.plugin_enabled"
          :loading="props.toggling === row.plugin_name"
          :disabled="(Boolean(props.toggling) && props.toggling !== row.plugin_name) || !row.plugin_installed"
          @change="(v: string | number | boolean) => emit('toggle', row, Boolean(v))"
        />
      </template>
    </el-table-column>
    <el-table-column :label="t('common.actions')" width="170" align="right">
      <template #default="{ row }">
        <div class="row-actions">
          <button
            v-if="row.plugin_recommended && !row.plugin_installed"
            class="wb-btn text sm"
            :class="{ 'is-loading': props.upgrading === row.plugin_name }"
            :disabled="Boolean(props.upgrading) || props.busy"
            @click="emit('retry', row)"
          >
            {{ t('plugins.retry') }}
          </button>
          <button
            v-else
            class="wb-btn text sm"
            :class="{ 'is-loading': props.busyName === row.plugin_name }"
            :disabled="Boolean(props.busyName) || props.busy"
            @click="emit('upgrade', row)"
          >
            {{ t('plugins.upgrade') }}
          </button>
          <button
            class="wb-btn text sm danger"
            :class="{ 'is-loading': props.busyName === row.plugin_name }"
            :disabled="Boolean(props.busyName) || props.busy"
            @click="emit('uninstall', row)"
          >
            {{ t('common.uninstall') }}
          </button>
        </div>
      </template>
    </el-table-column>
  </el-table>
</template>

<style scoped>
.cell-name {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.name {
  font-weight: 600;
  color: var(--wb-ink);
}

.desc {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.target {
  margin-left: 6px;
  color: var(--wb-warn);
}

.row-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--wb-sp-2);
}
</style>
