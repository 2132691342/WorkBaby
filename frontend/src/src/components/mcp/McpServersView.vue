<script setup lang="ts">
/**
 * MCP 服务管理。
 *
 * <p>直接编辑 {@code mcp.json} 原始 JSON：保存后后端校验 servers 数组 + 原子写 + 触发热重载。
 */
import { onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useMcpStore } from '@/stores/mcp'
import { useToast } from '@/composables/useToast'
import { apiPost } from '@/api/client'
import { t } from '@/i18n'
import { Server, FolderOpen, RefreshCw } from '@/components/common/icons'
import type { McpServer } from '@/types/api'

const mcp = useMcpStore()
const toast = useToast()
const { servers, loading, error, activeCount, rawContent, savingRaw } = storeToRefs(mcp)

async function handleSaveRaw(): Promise<void> {
  if (await mcp.saveRaw()) {
    toast.success(t('mcp.savedJson', activeCount.value))
  } else {
    toast.error(error.value ?? t('common.saveFailed'))
  }
}

/** 重载 MCP：成功/失败统一走 message 提示（不再用页内信息条）。 */
async function handleReload(): Promise<void> {
  const r = await mcp.reload()
  if (r.ok) {
    toast.success(t('mcp.reloaded', r.active))
  } else {
    toast.error(error.value ?? t('chat.operationFailed'))
  }
}

/** 打开 mcp.json 所在目录（桌面壳 + 本地后端）——让用户能直接编辑原始文件。 */
async function revealFile(): Promise<void> {
  try {
    await apiPost('/api/v1/mcp/servers/reveal', {})
    toast.info(t('mcp.revealHint'))
  } catch (e) {
    toast.error(e instanceof Error ? e.message : String(e))
  }
}

onMounted(() => {
  mcp.load()
  mcp.loadRaw()
})
</script>

<template>
  <div class="scroll wb-ui">
    <div class="wrap wrap-md">
      <!-- Hero（原型 11 屏） -->
      <header class="hero">
        <div class="tile"><Server class="ic" /></div>
        <div>
          <h1>{{ t('mcp.title') }}</h1>
          <p>{{ t('mcp.jsonSubtitle') }}</p>
        </div>
        <span class="sp" />
        <button class="btn" :title="t('mcp.revealHint')" @click="revealFile">
          <FolderOpen class="ic ic-sm" />
          {{ t('mcp.reveal') }}
        </button>
        <button class="btn btn-primary" @click="handleReload">
          <RefreshCw class="ic ic-sm" />
          {{ t('mcp.reload') }}
          <span v-if="activeCount > 0" class="badge b-success" style="margin-left: 4px">{{ activeCount }}</span>
        </button>
      </header>

      <div v-if="error" class="alert a-danger">{{ error }}</div>

      <!-- JSON 编辑器（直接编辑 mcp.json） -->
      <section class="card p-5">
        <div class="mb-3 flex items-center justify-between">
          <h2 class="font-display text-sm font-semibold text-wb-ink">{{ t('mcp.jsonEditor') }}</h2>
          <div class="flex items-center gap-2">
            <span class="font-mono text-[10px] text-wb-muted">mcp.json</span>
            <el-tooltip placement="top" :content="t('mcp.revealHint')" :show-after="200">
              <button
                type="button"
                class="inline-flex h-6 w-6 items-center justify-center rounded-md border border-wb-border bg-wb-surface text-wb-muted transition-colors hover:bg-wb-surface-hover hover:text-wb-ink"
                :title="t('mcp.reveal')"
                @click="revealFile"
              >
                📂
              </button>
            </el-tooltip>
          </div>
        </div>
        <p class="mb-3 text-[11px] leading-relaxed text-wb-muted">{{ t('mcp.jsonHint') }}</p>

        <el-input
          v-model="rawContent"
          type="textarea"
          :rows="16"
          spellcheck="false"
          resize="vertical"
          class="font-mono"
          placeholder='{ "mcpServers": {} }'
        />
        <div class="mt-3 flex items-center justify-between gap-2">
          <p class="text-[11px] text-wb-muted">{{ t('mcp.jsonSaveHint') }}</p>
          <el-button type="primary" :loading="savingRaw" @click="handleSaveRaw">
            {{ savingRaw ? t('ui.status.saving') : t('ui.btn.save') }}
          </el-button>
        </div>
      </section>

      <!-- 已配置 servers 状态（el-table） -->
      <section class="card p-5">
        <div class="mb-3 flex items-center justify-between">
          <h2 class="font-display text-sm font-semibold text-wb-ink">
            {{ t('mcp.list') }}
            <span v-if="servers.length > 0" class="ml-2 text-xs font-normal text-wb-muted">
              ({{ servers.length }})
            </span>
          </h2>
          <span
            class="rounded-full px-2 py-0.5 text-xs font-medium"
            :class="activeCount > 0 ? 'bg-wb-mint/12 text-wb-mint' : 'bg-wb-muted/12 text-wb-muted'"
          >
            {{ t('mcp.activeServers', activeCount) }}
          </span>
        </div>
        <div v-if="loading" class="text-sm text-wb-muted">{{ t('ui.status.loading') }}</div>
        <el-table v-else-if="servers.length > 0" :data="servers" stripe class="wb-el-table">
          <el-table-column :label="t('mcp.name')" min-width="180">
            <template #default="{ row }">
              <span class="font-medium text-wb-ink">{{ (row as McpServer).name }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('mcp.transport')" width="100">
            <template #default="{ row }">
              <el-tag size="small" type="info" effect="plain">{{ (row as McpServer).transport }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.enabled')" width="90">
            <template #default="{ row }">
              <el-tag size="small" :type="(row as McpServer).enabled ? 'success' : 'info'" effect="plain">
                {{ (row as McpServer).enabled ? t('common.enabled') : t('common.disabled') }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('mcp.command')" min-width="260" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="font-mono text-xs text-wb-muted">{{ (row as McpServer).command || (row as McpServer).base_url || '—' }}</span>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-else :description="t('mcp.empty')" :image-size="80" class="py-6" />
      </section>
    </div>
  </div>
</template>

