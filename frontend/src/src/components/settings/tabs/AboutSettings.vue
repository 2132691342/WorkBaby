<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useAdminStore } from '@/stores/admin'
import { t } from '@/i18n'

/**
 * 设置 · 关于 tab：版本 / 运行环境 / 数据目录 / 内置运行时。
 */
const adminStore = useAdminStore()
const { overview, runtimeStatus } = storeToRefs(adminStore)

/** 仅展示数字/布尔型聚合计数（过滤掉版本号、路径等已知字段）。 */
const aboutCounts = computed(() => {
  if (!overview.value) return []
  const skip = new Set(['version', 'phase', 'home', 'dbEnabled', 'toolCount'])
  return Object.entries(overview.value).filter(([k, v]) => !skip.has(k) && (typeof v === 'number' || typeof v === 'boolean'))
})
</script>

<template>
  <div class="space-y-4">
    <section class="card p-4">
      <h2 class="mb-3 font-display text-sm font-semibold text-wb-ink">{{ t('admin.title') }}</h2>
      <div v-if="overview" class="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
        <div class="rounded-xl bg-wb-primary/5 p-4">
          <div class="text-xs text-wb-muted">{{ t('admin.version') }}</div>
          <div class="mt-1 font-semibold text-wb-ink">{{ overview.version }}</div>
        </div>
        <div class="rounded-xl bg-wb-primary/5 p-4">
          <div class="text-xs text-wb-muted">{{ t('admin.phase') }}</div>
          <div class="mt-1 font-semibold text-wb-ink">{{ overview.phase }}</div>
        </div>
        <div class="rounded-xl bg-wb-primary/5 p-4">
          <div class="text-xs text-wb-muted">{{ t('admin.tool_count') }}</div>
          <div class="mt-1 font-semibold text-wb-mint">{{ overview.tool_count }}</div>
        </div>
        <div class="rounded-xl bg-wb-primary/5 p-4">
          <div class="text-xs text-wb-muted">{{ t('admin.sqlite') }}</div>
          <div class="mt-1 font-semibold" :class="overview.db_enabled ? 'text-wb-mint' : 'text-wb-muted'">
            {{ overview.db_enabled ? t('common.enabled') : t('admin.off') }}
          </div>
        </div>
      </div>
      <div v-if="overview" class="mt-3 rounded-xl bg-wb-primary/5 p-4 text-xs text-wb-muted">
        <div class="font-semibold text-wb-ink">{{ t('admin.dataDir') }}</div>
        <div class="mt-1 break-all font-mono">{{ overview.home }}</div>
      </div>
      <div v-if="runtimeStatus" class="mt-3 rounded-xl bg-wb-primary/5 p-4 text-xs text-wb-muted">
        <div class="flex items-center justify-between">
          <span class="font-semibold text-wb-ink">{{ t('admin.runtime') }}</span>
          <span :class="runtimeStatus.ready ? 'text-wb-mint' : 'text-wb-muted'">
            {{ runtimeStatus.ready ? t('admin.runtimeReady') : t('admin.runtimeNotReady') }}
          </span>
        </div>
        <div v-if="runtimeStatus.error" class="mt-1">{{ runtimeStatus.error }}</div>
        <div v-else class="mt-1 break-all font-mono">{{ runtimeStatus.bundled_dir }}</div>
        <div v-if="runtimeStatus.assets.length > 0" class="mt-3 space-y-2">
          <div v-for="asset in runtimeStatus.assets" :key="asset.id" class="rounded-lg bg-wb-bg/60 p-2">
            <div class="flex items-center justify-between text-wb-ink">
              <span class="font-medium">{{ asset.id }} {{ asset.version }}</span>
              <span :class="asset.ready ? 'text-wb-mint' : 'text-wb-muted'">
                {{ asset.ready ? t('admin.runtimeReady') : t('admin.runtimeNotReady') }}
              </span>
            </div>
            <div class="mt-1 break-all">{{ asset.error || asset.executable }}</div>
          </div>
        </div>
      </div>
      <div v-if="aboutCounts.length > 0" class="mt-5">
        <h3 class="mb-2 text-xs font-semibold uppercase tracking-wider text-wb-muted">
          {{ t('admin.counts') }}
        </h3>
        <div class="grid grid-cols-2 gap-2 text-sm sm:grid-cols-4">
          <div v-for="[k, v] in aboutCounts" :key="k" class="flex items-center justify-between rounded-lg bg-wb-primary/5 px-3 py-2">
            <span class="text-wb-muted">{{ k }}</span>
            <span class="font-medium tabular-nums text-wb-ink">{{ String(v) }}</span>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
