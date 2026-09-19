<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useSettingsStore } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'

/**
 * 设置 · 高级 tab：exec agent 二进制白名单（脏检查 + 回滚）。
 */
const settings = useSettingsStore()
const toast = useToast()
const { execWhitelist, closeToTray, memoryEnabled, error } = storeToRefs(settings)
const { loadExecWhitelist, saveExecWhitelist, setCloseToTray, loadMemoryEnabled, setMemoryEnabled } = settings

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

const newBinary = ref('')
/** 原始白名单（后端拉到的），用于判断「脏」并支持回滚 */
const originalExecWhitelist = ref<string[]>([])
/** 白名单是否被本地修改（控制保存按钮可用性 + 顶部角标提示） */
const execWhitelistDirty = computed(() => {
  const a = originalExecWhitelist.value
  const b = execWhitelist.value
  if (a.length !== b.length) return true
  const sa = [...a].sort()
  const sb = [...b].sort()
  for (let i = 0; i < sa.length; i++) if (sa[i] !== sb[i]) return true
  return false
})

async function addBinary(): Promise<void> {
  const v = newBinary.value.trim()
  if (!v || execWhitelist.value.includes(v)) return
  execWhitelist.value = [...execWhitelist.value, v]
  newBinary.value = ''
}
function removeBinary(b: string): void {
  execWhitelist.value = execWhitelist.value.filter((x) => x !== b)
}
/** 放弃本地修改，恢复到原始值 */
function resetExecWhitelistLocal(): void {
  execWhitelist.value = [...originalExecWhitelist.value]
}
async function handleSaveExecWhitelist(): Promise<void> {
  if (await saveExecWhitelist(execWhitelist.value)) {
    originalExecWhitelist.value = [...execWhitelist.value]
    toast.success(t('settings.execWhitelistSaved'))
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
  await Promise.all([loadMemoryEnabled(), loadExecWhitelist(), loadGrants()])
  originalExecWhitelist.value = [...execWhitelist.value]
})
</script>

<template>
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
        <div class="mt-1 text-[11px] text-wb-muted">{{ t('settings.memoryPathHint') }}</div>
      </div>
      <el-switch :model-value="memoryEnabled" @change="(v: boolean) => onMemoryEnabledChange(v)" />
    </div>

    <h2 class="mb-1 font-display text-sm font-semibold text-wb-ink">
      {{ t('settings.execWhitelist') }}
    </h2>
    <p class="mb-3 text-xs text-wb-muted">{{ t('settings.execWhitelistHint') }}</p>

    <h3 class="wb-form-section__title">{{ t('settings.section.security') }}</h3>
    <div class="mb-3 rounded-lg border border-wb-border bg-wb-surface-2 p-3">
      <div class="mb-2 flex items-center justify-between">
        <span class="text-xs text-wb-muted">
          {{ t('settings.execWhitelistCount', execWhitelist.length) }}
        </span>
        <span v-if="execWhitelistDirty" class="text-[10px] font-medium text-wb-warning">
          ● {{ t('settings.execWhitelistDirty') }}
        </span>
      </div>
      <div v-if="execWhitelist.length > 0" class="flex flex-wrap gap-2">
        <div
          v-for="b in execWhitelist"
          :key="b"
          class="group inline-flex items-center gap-2 rounded-lg border border-wb-border bg-wb-surface px-3 py-1.5 font-mono text-xs text-wb-ink transition-colors hover:border-wb-danger/40"
        >
          <span class="h-1.5 w-1.5 rounded-full bg-wb-mint" />
          <span>{{ b }}</span>
          <button
            type="button"
            class="ml-1 inline-flex h-4 w-4 items-center justify-center rounded text-wb-muted transition-colors hover:bg-wb-danger/10 hover:text-wb-danger"
            :title="t('ui.btn.delete')"
            @click="removeBinary(b)"
          >
            ✕
          </button>
        </div>
      </div>
      <div v-else class="flex items-center gap-2 rounded-lg border border-dashed border-wb-border px-3 py-4 text-xs text-wb-muted">
        <span>—</span>
        <span>{{ t('settings.execWhitelistEmpty') }}</span>
      </div>
    </div>

    <div class="flex flex-wrap items-center gap-2">
      <el-input
        v-model="newBinary"
        :placeholder="t('settings.execWhitelistAddPlaceholder')"
        class="!w-72"
        clearable
        @keydown.enter="addBinary"
      />
      <el-button :disabled="!newBinary.trim()" @click="addBinary">+ {{ t('common.add') }}</el-button>
      <div class="ml-auto flex items-center gap-2">
        <el-button @click="resetExecWhitelistLocal" :disabled="!execWhitelistDirty">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :disabled="!execWhitelistDirty" @click="handleSaveExecWhitelist">
          {{ t('settings.execWhitelistSave') }}
        </el-button>
      </div>
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
            <div class="mt-0.5 text-[11px] text-wb-muted">{{ fmtTime(g.created_at) }}</div>
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
</template>
