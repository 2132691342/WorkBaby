<script setup lang="ts">
// MCP 服务器表：一行一个服务器，开关 / 编辑 / 删除。
// 只有一个引擎，所以没有「装到哪边」这一列；定义来自 mcp.json，那份文件是唯一事实。
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { McpServer } from '@/types/api'

const props = defineProps<{
  rows: McpServer[]
  loading: boolean
  /** 正在操作的那一条的服务器名（空表示空闲） */
  busyName: string
}>()

const emit = defineEmits<{
  create: []
  edit: [row: McpServer]
  remove: [row: McpServer]
  toggle: [row: McpServer, enabled: boolean]
}>()

const { t } = useI18n()

/** 已启用的个数：表头上那个数字，扫一眼就知道开了几个。 */
function enabledCount(): number {
  return props.rows.filter((row) => row.mcp_enabled).length
}

/** 状态标签：启用 / 停用 / 项目级覆盖。 */
function statusTag(row: McpServer): { cls: string; text: string } {
  if (!row.mcp_enabled) return { cls: '', text: t('mcp.disabled') }
  if (row.scope_kind === 'project') return { cls: 'brand', text: t('mcp.projectScope') }
  return { cls: 'ok', text: t('mcp.configured') }
}
</script>

<template>
  <section>
    <div class="wb-sec-head">
      <span class="wb-sec-title">
        {{ t('mcp.servers') }} <span class="count">{{ props.rows.length }}</span>
        <span class="count">· {{ enabledCount() }} {{ t('common.enabled') }}</span>
      </span>
      <button class="wb-btn primary sm" :disabled="props.loading" @click="emit('create')">
        <AppIcon name="plus" :size="13" />
        {{ t('mcp.add') }}
      </button>
    </div>

    <div v-if="!props.rows.length && !props.loading" class="wb-empty">
      <span class="wb-tile wb-tile--lg wb-tile--neutral"><AppIcon name="plug" :size="22" /></span>
      <p class="wb-empty-title">{{ t('mcp.emptyTitle') }}</p>
      <p class="wb-empty-hint">{{ t('mcp.emptyHint') }}</p>
      <button class="wb-btn sm primary" @click="emit('create')">{{ t('mcp.add') }}</button>
    </div>

    <el-table v-else :data="props.rows" size="small">
      <el-table-column :label="t('mcp.name')" min-width="150">
        <template #default="{ row }">
          <div class="cell-name">
            <span class="name">{{ row.server_name }}</span>
            <span v-if="row.mcp_desc" class="desc">{{ row.mcp_desc }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('mcp.transport')" width="80">
        <template #default="{ row }">
          <span class="wb-mono">{{ row.transport_kind }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('mcp.definition')" min-width="240">
        <template #default="{ row }">
          <span class="wb-mono clip">{{ row.server_url || row.command_line || '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('mcp.state')" width="96">
        <template #default="{ row }">
          <span class="tag" :class="statusTag(row).cls">{{ statusTag(row).text }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('plugins.enabledLabel')" width="80">
        <template #default="{ row }">
          <el-switch
            :model-value="row.mcp_enabled"
            :loading="props.busyName === row.server_name"
            :disabled="Boolean(props.busyName) && props.busyName !== row.server_name"
            @change="(v: string | number | boolean) => emit('toggle', row, Boolean(v))"
          />
        </template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" width="140" align="right">
        <template #default="{ row }">
          <div class="row-actions">
            <button class="wb-btn text sm" :disabled="Boolean(props.busyName)" @click="emit('edit', row)">
              {{ t('common.edit') }}
            </button>
            <button class="wb-btn text sm danger" :disabled="Boolean(props.busyName)" @click="emit('remove', row)">
              {{ t('common.delete') }}
            </button>
          </div>
        </template>
      </el-table-column>
    </el-table>
  </section>
</template>

<style scoped>
.count {
  font-family: var(--wb-font-mono);
  color: var(--wb-faint);
  letter-spacing: 0;
  text-transform: none;
}

.cell-name {
  display: flex;
  flex-direction: column;
  min-width: 0;
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

.clip {
  display: block;
  max-width: 320px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--wb-sp-2);
}
</style>
