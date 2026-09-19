<script setup lang="ts">
import { ref } from 'vue'
import { storeToRefs } from 'pinia'
import { Cpu, ChevronDown } from '@/components/common/icons'
import { useDashboardStore } from '@/stores/dashboard'
import { t } from '@/i18n'

/**
 * 仪表盘 · 系统运行时折叠区（原型 07 屏底部，遵循 wb-ui 设计系统）。
 */
const dashboard = useDashboardStore()
const { stats } = storeToRefs(dashboard)

const open = ref(false)
function toggle(): void {
  open.value = !open.value
}
</script>

<template>
  <section v-if="stats?.system" class="card p-sm">
    <div class="flex-r" style="cursor: pointer" @click="toggle">
      <Cpu class="ic ic-sm muted" />
      <h3 style="flex: 1">{{ t('dashboard.runtimeInfo') }}</h3>
      <span class="badge b-success">{{ t('dashboard.devOnly') }}</span>
      <ChevronDown class="ic ic-sm muted" :class="{ 'rotate-180': open }" />
    </div>
    <div v-if="open">
      <div class="divider mt10 mb10" />
      <dl class="kv">
        <dt>{{ t('dashboard.os') }}</dt>
        <dd>{{ stats.system.os_name }} · {{ stats.system.os_arch }}</dd>
        <dt>{{ t('dashboard.cpuCores') }}</dt>
        <dd>{{ stats.system.num_cpu }}</dd>
        <dt>{{ t('dashboard.go_version') }}</dt>
        <dd class="mono">{{ stats.system.go_version }}</dd>
        <dt>{{ t('dashboard.goroutines') }}</dt>
        <dd class="mono">{{ stats.system.goroutines }}</dd>
        <dt>{{ t('dashboard.goHeap') }}</dt>
        <dd class="mono">{{ stats.system.heap_alloc_mb }} MB / {{ stats.system.heap_sys_mb }} MB</dd>
        <dt>{{ t('dashboard.user_home') }}</dt>
        <dd class="mono">{{ stats.system.user_home }}</dd>
      </dl>
    </div>
  </section>
</template>
