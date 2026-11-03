<script setup lang="ts">
// MCP 矩阵表：一行一个服务器（按名合并两引擎的定义），每个引擎一列开关。
// 借鉴 cc-switch 的管理方式：一份清单，装到哪边、开在哪边一眼看全。
// 某个引擎没有这个服务器时，那一列是「装过去」——把另一边现成的定义复制过来。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { EngineKind, McpMatrixRow } from '@/types/api'

const props = defineProps<{
  rows: McpMatrixRow[]
  loading: boolean
  /** 正在操作的那一条的服务器名（空表示空闲） */
  busyName: string
  /** 探测到的引擎：没装的引擎那一列不渲染开关（不渲染灰开关） */
  installed: EngineKind[]
}>()

const emit = defineEmits<{
  create: []
  edit: [row: McpMatrixRow]
  remove: [row: McpMatrixRow]
  toggle: [row: McpMatrixRow, kind: EngineKind, enabled: boolean]
  copy: [row: McpMatrixRow, kind: EngineKind]
}>()

const { t } = useI18n()

const ENGINES: EngineKind[] = ['omp', 'pi']

function cellOf(row: McpMatrixRow, kind: EngineKind) {
  return row.cells[kind]
}

function hasEngine(kind: EngineKind): boolean {
  return props.installed.includes(kind)
}

/** 一列里已启用的个数：表头上那个数字，扫一眼就知道每边开了几个。 */
function enabledCount(kind: EngineKind): number {
  return props.rows.filter((r) => r.cells[kind]?.enabled).length
}

function statusTag(cell: NonNullable<McpMatrixRow['cells'][EngineKind]>): { cls: string; text: string } {
  if (cell.live_status === 'connected') return { cls: 'ok', text: t('mcp.connected') }
  if (cell.live_status === 'failed') return { cls: 'bad', text: t('mcp.failed') }
  if (cell.live_status) return { cls: 'warn', text: cell.live_status }
  if (!cell.enabled) return { cls: '', text: t('mcp.disabled') }
  return { cls: 'brand', text: t('mcp.configured') }
}

const anyDenied = computed(() => props.rows.some((r) => Object.values(r.cells).some((c) => c?.disabled_by_deny)))
</script>

<template>
  <section>
    <div class="wb-sec-head">
      <span class="wb-sec-title">
        {{ t('mcp.servers') }} <span class="count">{{ props.rows.length }}</span>
      </span>
    </div>

    <div v-if="!props.rows.length && !props.loading" class="wb-empty">
      <span class="wb-tile wb-tile--lg wb-tile--neutral"><AppIcon name="server" :size="22" /></span>
      <p class="wb-empty-title">{{ t('mcp.emptyTitle') }}</p>
      <p class="wb-empty-hint">{{ t('mcp.emptyHint') }}</p>
      <button class="wb-btn sm primary" @click="emit('create')">{{ t('mcp.add') }}</button>
    </div>

    <el-table v-else :data="props.rows" size="small">
      <el-table-column :label="t('mcp.name')" min-width="150">
        <template #default="{ row }">
          <div class="cell-name">
            <span class="name">{{ row.server_name }}</span>
            <span class="desc">{{ row.desc || row.command_line || row.server_url || '—' }}</span>
          </div>
        </template>
      </el-table-column>

      <el-table-column
        v-for="kind in ENGINES"
        :key="kind"
        min-width="110"
        align="center"
      >
        <template #header>
          <span class="col-head" :class="{ off: !hasEngine(kind) }">
            {{ kind }}
            <span class="col-count">{{ enabledCount(kind) }}</span>
          </span>
        </template>
        <template #default="{ row }">
          <template v-if="hasEngine(kind)">
            <!-- 有定义：开关；被名单压住时点不动并说明原因 -->
            <el-switch
              v-if="cellOf(row, kind)"
              :model-value="cellOf(row, kind)!.enabled"
              :disabled="cellOf(row, kind)!.disabled_by_deny || Boolean(props.busyName)"
              :title="cellOf(row, kind)!.disabled_by_deny ? t('mcp.denyHint') : cellOf(row, kind)!.config_path"
              @change="(v: string | number | boolean) => emit('toggle', row, kind, Boolean(v))"
            />
            <!-- 没定义：把另一边现成的定义装过来 -->
            <button
              v-else
              class="wb-btn text sm install"
              :disabled="Boolean(props.busyName)"
              :title="t('mcp.copyHint', { engine: kind })"
              @click="emit('copy', row, kind)"
            >
              <AppIcon name="plus" :size="12" />
              {{ t('mcp.installTo', { engine: kind }) }}
            </button>
          </template>
          <span v-else class="tag muted">{{ t('mcp.notInstalled') }}</span>
        </template>
      </el-table-column>

      <el-table-column :label="t('mcp.status')" width="120">
        <template #default="{ row }">
          <div class="cell-status">
            <template v-for="kind in ENGINES" :key="kind">
              <span
                v-if="cellOf(row, kind)?.live_status"
                class="tag"
                :class="statusTag(cellOf(row, kind)!).cls"
                :title="cellOf(row, kind)!.live_error || kind"
              >
                {{ kind }} · {{ statusTag(cellOf(row, kind)!).text }}
              </span>
            </template>
          </div>
        </template>
      </el-table-column>

      <el-table-column :label="t('common.actions')" width="150" align="right">
        <template #default="{ row }">
          <div class="row-actions">
            <button class="wb-btn text sm" :disabled="Boolean(props.busyName)" @click="emit('edit', row)">
              {{ t('common.edit') }}
            </button>
            <button
              class="wb-btn text sm danger"
              :class="{ 'is-loading': props.busyName === row.server_name }"
              :disabled="Boolean(props.busyName)"
              @click="emit('remove', row)"
            >
              {{ t('common.delete') }}
            </button>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <!-- 停用名单压住时说明原因：否则用户只会以为开关坏了 -->
    <p v-if="anyDenied" class="hint">{{ t('mcp.denyHint') }}</p>
  </section>
</template>

<style scoped>
.count {
  font-family: var(--wb-font-mono);
  color: var(--wb-faint);
  letter-spacing: 0;
  text-transform: none;
}

.col-head {
  font-family: var(--wb-font-mono);
  font-weight: 650;
  text-transform: none;
  letter-spacing: 0.02em;
}

.col-head.off {
  opacity: 0.4;
}

.col-count {
  margin-left: 4px;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
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

.install {
  color: var(--wb-brand);
}

.tag.muted {
  opacity: 0.5;
}

.cell-status {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
}

.row-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--wb-sp-2);
}

.hint {
  margin: var(--wb-sp-2) 0 0;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}
</style>
