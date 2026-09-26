<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useSettingsStore } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'

/**
 * 设置 · 高级 tab：托盘常驻 / 记忆开关 / 免审授权管理。
 * exec 二进制白名单为后端默认策略（不暴露编辑界面）。
 */
const settings = useSettingsStore()
const toast = useToast()
const { closeToTray, memoryEnabled, error } = storeToRefs(settings)
const { setCloseToTray, loadMemoryEnabled, setMemoryEnabled } = settings

/** 关闭到托盘：立即持久化（无脏检查必要，单项开关）。 */
async function onCloseToTrayChange(v: boolean): Promise<void> {
  if (await setCloseToTray(v)) {
    toast.success(t('settings.closeToTraySaved'))
  } else {
    toast.error(error.value ?? t('common.saveFailed'))
  }
}

/** 记忆开关：立即持久化。关闭后 Agent 不再自动召回/沉淀记忆（历史消息仍保留）。 */
async function onMemoryEnabledChange(v: boolean): Promise<void> {
  if (await setMemoryEnabled(v)) {
    toast.success(v ? t('settings.memoryEnabledOn') : t('settings.memoryEnabledOff'))
  } else {
    toast.error(error.value ?? t('common.saveFailed'))
  }
}

// ===== 免审授权管理：「本会话允许」的持久化授权，可随时撤销 =====
import { apiGet, apiPost } from '@/api/client'
import type { ApprovalGrant } from '@/types/api'

const grants = ref<ApprovalGrant[]>([])
const grantsLoading = ref(false)

async function loadGrants(): Promise<void> {
  grantsLoading.value = true
  try {
    const rows = await apiGet<ApprovalGrant[]>('/api/v1/chat/approval-grants')
    grants.value = Array.isArray(rows) ? rows : []
  } catch {
    grants.value = []
  } finally {
    grantsLoading.value = false
  }
}

async function revokeGrant(id: string): Promise<void> {
  try {
    await apiPost(`/api/v1/chat/approval-grants/${id}/delete`)
    grants.value = grants.value.filter((g) => g.id !== id)
    toast.success(t('settings.grantRevoked'))
  } catch (e) {
    toast.error(e instanceof Error ? e.message : t('common.saveFailed'))
  }
}

function fmtTime(ms: number): string {
  return new Date(ms).toLocaleString()
}

onMounted(async () => {
  await Promise.all([loadMemoryEnabled(), loadGrants()])
})
</script>

<template>
  <!-- 自带滚动与页边距：设置区统一 .set-body/.set-page（见 ProviderSettings 同款） -->
  <div class="set-body wb-ui">
    <div class="set-page">
      <section class="card p-4">
    <!-- 系统行为：托盘常驻 -->
    <h2 class="mb-1 font-display text-sm font-semibold text-wb-ink">
      {{ t('settings.section.system') }}
    </h2>
    <div class="mb-5 mt-2 flex items-center justify-between rounded-lg border border-wb-border bg-wb-surface-2 p-3">
      <div>
        <div class="text-sm font-medium text-wb-ink">{{ t('settings.closeToTray') }}</div>
        <div class="mt-0.5 text-xs text-wb-muted">{{ t('settings.closeToTrayHint') }}</div>
      </div>
      <el-switch :model-value="closeToTray" @change="(v: boolean) => onCloseToTrayChange(v)" />
    </div>

    <h2 class="mb-1 font-display text-sm font-semibold text-wb-ink">
      {{ t('settings.memoryTitle') }}
    </h2>
    <div class="mb-5 mt-2 flex items-center justify-between rounded-lg border border-wb-border bg-wb-surface-2 p-3">
      <div>
        <div class="text-sm font-medium text-wb-ink">{{ t('settings.memoryEnabled') }}</div>
        <div class="mt-0.5 max-w-lg text-xs text-wb-muted">{{ t('settings.memoryEnabledHint') }}</div>
        <div class="mt-1 text-xs2 text-wb-muted">{{ t('settings.memoryPathHint') }}</div>
      </div>
      <el-switch :model-value="memoryEnabled" @change="(v: boolean) => onMemoryEnabledChange(v)" />
    </div>

    <!-- 免审授权：「本会话允许」的持久化授权，跨重启生效，可随时撤销 -->
    <h2 class="mb-1 mt-6 font-display text-sm font-semibold text-wb-ink">
      {{ t('settings.grantsTitle') }}
    </h2>
    <p class="mb-3 text-xs text-wb-muted">{{ t('settings.grantsHint') }}</p>
    <div v-loading="grantsLoading" class="rounded-lg border border-wb-border bg-wb-surface-2 p-3">
      <div v-if="grants.length > 0" class="flex flex-col gap-2">
        <div
          v-for="g in grants"
          :key="g.id"
          class="flex items-center justify-between rounded-lg border border-wb-border bg-wb-surface px-3 py-2"
        >
          <div class="min-w-0">
            <div class="truncate font-mono text-xs text-wb-ink">{{ g.command }}</div>
            <div class="mt-0.5 text-xs2 text-wb-muted">{{ fmtTime(g.created_at) }}</div>
          </div>
          <el-button size="small" type="danger" plain @click="revokeGrant(g.id)">
            {{ t('settings.grantRevoke') }}
          </el-button>
        </div>
      </div>
      <div v-else class="flex items-center gap-2 px-1 py-3 text-xs text-wb-muted">
        <span>—</span>
        <span>{{ t('settings.grantsEmpty') }}</span>
      </div>
    </div>
      </section>
    </div>
  </div>
</template>
