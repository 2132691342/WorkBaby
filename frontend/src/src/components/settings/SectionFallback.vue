<script setup lang="ts">
/**
 * 异步 section 兜底态（loading / error 共用）。
 *
 * <p>defineAsyncComponent 默认在 chunk 加载失败时静默渲染空白，用户只看到
 * 「点进去一片空白、无任何提示」。这里把两态显式化：加载中给进度条，
 * 失败给出错误原因，便于定位。
 */
import { t } from '@/i18n'

defineProps<{ error?: Error }>()
</script>

<template>
  <div class="flex flex-col items-center gap-3 py-16 text-center">
    <template v-if="error">
      <p class="fs12 text-wb-danger">{{ t('ui.state.error') }}</p>
      <p class="fs11 muted mono max-w-md break-all">{{ error.message }}</p>
    </template>
    <template v-else>
      <div class="bar w-40">
        <div class="h-full w-1/3 animate-pulse rounded-full bg-wb-primary" />
      </div>
      <p class="fs11 muted">{{ t('boot.startingDetail') }}</p>
    </template>
  </div>
</template>
