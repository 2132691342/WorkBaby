<script setup lang="ts">
// 工作目录芯片：在输入框底栏直接切目录。
// 切换是双写——全局默认（KV）+ 当前会话；只改一边会出现「点了没换地方」的错觉。
import { computed, ref } from 'vue'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import { OpenDirectoryDialog } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'

const settings = useSettingsStore()
const toast = useToastStore()

const busy = ref(false)

// 芯片直接显示全路径：小白要和文件管理器对得上；太长时 CSS 省略号收住，tooltip 也是全路径
const current = computed(() => settings.values['workspace'] || settings.boot?.workspace || '')
const label = computed(() => current.value || '默认目录')

async function pick() {
  if (busy.value) return
  busy.value = true
  try {
    const dir = await OpenDirectoryDialog('选择工作目录')
    // 没选或选了同一个目录：不动
    if (!dir || dir === current.value) return
    await settings.setValue('workspace', dir)
    await settings.loadBoot()
    toast.ok(`工作目录已切换：${dir}`)
  } catch (e) {
    toast.bad(`切换工作目录失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <button
    class="chip ws-chip"
    type="button"
    :class="{ 'is-loading': busy, 'is-default': !current }"
    :disabled="busy"
    :title="current || '还没设置工作目录；点这里选一个，助手读写文件、跑命令都在这里面'"
    :aria-label="current ? `工作目录：${current}，点击更换` : '还没设置工作目录，点击选择一个'"
    @click="pick"
  >
    <AppIcon name="folder" size="ic-xs" />
    <span class="ws-name">{{ label }}</span>
  </button>
</template>

<style scoped>
/* 底栏 chips 行的一员：全路径过长省略号收住，title 给全路径 */
.ws-chip {
  flex: 0 1 auto;
  min-width: 0;
  max-width: 420px;
}
.ws-chip .ws-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* 未设置时虚线灰态：一眼看出「还没选」，和已选的实线区分开 */
.ws-chip.is-default {
  border-style: dashed;
  color: var(--wb-muted);
}
</style>
