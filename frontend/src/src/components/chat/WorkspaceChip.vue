<script setup lang="ts">
// 工作目录芯片：聊天输入框底栏随手切，不用跑去行为设置。
// 切换 = 全局默认（KV）+ 当前会话（会话级端点）双写：只改一边就会骗人——
// 当前对话还在旧目录里读写，新手会以为「点了没反应」。
import { computed, ref } from 'vue'
import * as api from '../../api'
import { useSessionStore } from '../../stores/session'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import { OpenDirectoryDialog } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'

const settings = useSettingsStore()
const session = useSessionStore()
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
    if (session.currentId) {
      try {
        await api.sessions.setWorkspace(session.currentId, dir)
      } catch (e) {
        // 全局默认已切、当前会话没切成：必须说清楚，不能装作都成功了
        toast.bad(`全局默认已切换，但当前会话切换失败：${(e as Error)?.message || '请重试'}`)
        return
      }
    }
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
