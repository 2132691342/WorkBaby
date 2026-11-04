<script setup lang="ts">
// 插件表格：一行一个包，带状态标签、开关与行级动作。
// 纯展示 + 事件上抛：加载、安装、开关、重试都由页面持有，这里不碰 api。
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { PluginRow } from '@/types/api'

const props = defineProps<{
  rows: PluginRow[]
  loading: boolean
  /** 正在跑行级动作的那一行 */
  busyName: string
  toggling: string
  retryingAll: boolean
  /** 每行正在输出的 npm 实时行 */
  liveLines: Record<string, string>
  keyword: string
}>()

const emit = defineEmits<{
  toggle: [row: PluginRow, enabled: boolean]
  upgrade: [row: PluginRow]
  uninstall: [row: PluginRow]
  retry: [row: PluginRow]
}>()

const { t } = useI18n()

/** 状态标签：安装态与启用态是两个维度，分两列说清。 */
function stateLabel(row: PluginRow): string {
  if (row.plugin_state === 'failed') return t('plugins.stateFailed')
  if (row.installed) return t('plugins.stateReady')
  return t('plugins.stateMissing')
}

function stateClass(row: PluginRow): string {
  if (row.plugin_state === 'failed') return 'bad'
  return row.installed ? 'ok' : 'warn'
}

function resourceLabel(kinds: string[]): string {
  return kinds?.length ? kinds.join(' · ') : '—'
}

/** 行内按钮是否该锁：有别的行在忙时全部锁住，避免并发装同一个 npm 前缀。 */
function locked(row: PluginRow): boolean {
  return (Boolean(props.busyName) && props.busyName !== row.plugin_name) || props.retryingAll
}
</script>

<template>
  <div v-if="!rows.length && !loading" class="wb-empty">
    <span class="wb-tile wb-tile--lg wb-tile--neutral"><AppIcon name="plug" :size="22" /></span>
    <p class="wb-empty-title">{{ keyword ? t('common.noMatch') : t('plugins.emptyTitle') }}</p>
    <p class="wb-empty-hint">{{ keyword ? t('common.noMatchHint') : t('plugins.emptyHint') }}</p>
  </div>

  <el-table v-else :data="rows" size="small" row-key="plugin_name">
    <el-table-column :label="t('plugins.name')" min-width="230">
      <template #default="{ row }">
        <div class="cell-name">
          <div class="name-line">
            <span class="name">{{ row.plugin_name }}</span>
            <span v-if="row.builtin" class="badge">{{ t('plugins.builtinBadge') }}</span>
            <span v-if="row.plugin_error" class="badge bad" :title="row.plugin_error">
              {{ t('plugins.failedBadge') }}
            </span>
          </div>
          <span v-if="row.plugin_desc" class="desc">{{ row.plugin_desc }}</span>
          <span v-if="liveLines[row.plugin_name]" class="live wb-mono">{{ liveLines[row.plugin_name] }}</span>
        </div>
      </template>
    </el-table-column>

    <el-table-column :label="t('plugins.version')" width="110">
      <template #default="{ row }">
        <span class="wb-mono">
          {{ row.plugin_version || '—' }}
          <em v-if="row.plugin_target_version && row.plugin_version !== row.plugin_target_version" class="target">
            → {{ row.plugin_target_version }}
          </em>
        </span>
      </template>
    </el-table-column>

    <el-table-column :label="t('plugins.resources')" width="118">
      <template #default="{ row }">
        <span class="wb-mono dim">{{ resourceLabel(row.resource_kinds) }}</span>
      </template>
    </el-table-column>

    <el-table-column :label="t('plugins.state')" width="92">
      <template #default="{ row }">
        <span class="tag" :class="stateClass(row)">{{ stateLabel(row) }}</span>
      </template>
    </el-table-column>

    <el-table-column :label="t('plugins.enabledLabel')" width="76">
      <template #default="{ row }">
        <el-switch
          v-if="row.installed"
          :model-value="row.enabled"
          :loading="toggling === row.plugin_name"
          :disabled="(Boolean(toggling) && toggling !== row.plugin_name) || Boolean(busyName)"
          @change="(v: string | number | boolean) => emit('toggle', row, Boolean(v))"
        />
        <span v-else class="wb-mono dim">—</span>
      </template>
    </el-table-column>

    <el-table-column :label="t('common.actions')" width="176" align="right">
      <template #default="{ row }">
        <div class="row-actions">
          <!-- 未装 / 失败：给重试；已装：给升级 -->
          <button
            v-if="!row.installed"
            class="wb-btn text sm"
            :class="{ 'is-loading': busyName === row.plugin_name }"
            :disabled="locked(row)"
            @click="emit('retry', row)"
          >
            {{ t('plugins.retry') }}
          </button>
          <button
            v-else
            class="wb-btn text sm"
            :class="{ 'is-loading': busyName === row.plugin_name }"
            :disabled="locked(row)"
            @click="emit('upgrade', row)"
          >
            {{ t('plugins.upgrade') }}
          </button>
          <button
            v-if="!row.builtin"
            class="wb-btn text sm danger"
            :class="{ 'is-loading': busyName === row.plugin_name }"
            :disabled="locked(row)"
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
  gap: 2px;
  min-width: 0;
}

.name-line {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}

.name {
  font-weight: 650;
  color: var(--wb-ink);
}

.desc {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.live {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-brand-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.badge {
  flex: none;
  padding: 0 var(--wb-sp-2);
  border-radius: var(--wb-r-sm);
  border: 1px solid var(--wb-line);
  font-size: 10px;
  line-height: 16px;
  color: var(--wb-mute);
}

.badge.bad {
  border-color: var(--wb-danger-soft);
  color: var(--wb-danger);
}

.target {
  font-style: normal;
  color: var(--wb-brand-1);
}

.dim {
  color: var(--wb-faint);
}

.row-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--wb-sp-2);
}
</style>
