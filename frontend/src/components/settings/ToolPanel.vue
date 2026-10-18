<script setup lang="ts">
// 工具面板：清单来自 PI（最近一次会话声明给模型的工具 + MCP 在线工具）。
// 启停写的是 PI settings.json 的 defaultTools，不自己实现任何工具。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { ToolListRESP, ToolVO } from '../../types/api'
import { fmtRel } from '../../utils/time'
import AppIcon from '../common/AppIcon.vue'
import PnHead from '../common/PnHead.vue'

const toast = useToastStore()
const settingsStore = useSettingsStore()
const data = ref<ToolListRESP | null>(null)
const busy = ref('')
const filter = ref('')

const SOURCE: Record<string, string> = { builtin: '内置', extension: '插件', mcp: 'MCP' }

const groups = computed(() => {
  const q = filter.value.trim().toLowerCase()
  const items = (data.value?.tools ?? []).filter(
    (t) => !q || t.name.toLowerCase().includes(q) || (t.description || '').toLowerCase().includes(q),
  )
  const out: Array<{ key: string; items: ToolVO[] }> = []
  for (const key of ['builtin', 'extension', 'mcp']) {
    const list = items.filter((t) => t.source === key)
    if (list.length) out.push({ key, items: list })
  }
  return out
})

async function reload() {
  try {
    data.value = await api.tools.list()
    settingsStore.tools = data.value
  } catch (e) {
    toast.bad(`读取工具失败：${(e as Error)?.message || '请重试'}`)
  }
}

async function toggle(t: ToolVO) {
  if (busy.value) return
  busy.value = t.name
  try {
    await api.tools.toggle(t.name, !t.enabled)
    await reload()
  } catch (e) {
    toast.bad(`切换失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

onMounted(() => {
  // 预热缓存先上屏：有就直接渲染，后台再刷一次对齐最新；没有才等网络
  if (settingsStore.tools) {
    data.value = settingsStore.tools
    void reload()
  } else void reload()
})
</script>

<template>
  <div class="panel">
    <PnHead icon="wrench" title="工具">
      <template #sub>
        <template v-if="data?.snapshot === 'session'">
          来自最近一次会话的工具声明（{{ fmtRel(data.snapshot_at) }}）：这就是模型此刻真能调到的工具。
        </template>
        <template v-else>
          还没有会话快照，这里按 PI 的内置工具表推断；聊过一轮之后会显示真实声明。
        </template>
      </template>
      <el-input v-model="filter" class="search" placeholder="搜索工具" clearable />
      <el-button :disabled="!!busy" @click="reload">刷新</el-button>
    </PnHead>

    <section v-for="g in groups" :key="g.key" class="group">
      <div class="g-head">
        <b>{{ SOURCE[g.key] }}</b>
        <span class="sp" />
        <span class="muted">{{ g.items.length }} 个</span>
      </div>
      <div class="rows">
        <article v-for="t in g.items" :key="t.name" class="row card" :class="{ 'is-off': !t.enabled }">
          <div class="r-top">
            <span class="rk"><AppIcon name="wrench" size="ic-sm" /></span>
            <b class="mono">{{ t.name }}</b>
          </div>
          <span class="r-desc">{{ t.description }}</span>
          <div class="r-acts">
            <span v-if="g.key === 'mcp'" class="muted sm">{{ t.server }}</span>
            <span class="sp" />
            <el-switch
              v-if="g.key !== 'mcp'"
              :model-value="t.enabled"
              :loading="busy === t.name"
              :disabled="!!busy"
              @change="toggle(t)"
            />
          </div>
        </article>
      </div>
    </section>

    <p v-if="!groups.length" class="muted">
      没有工具：先配好模型并聊一轮，PI 会在会话里写下它声明的工具。
    </p>
  </div>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}
.search {
  width: 180px;
  height: var(--wb-ctl-h-sm);
}
.group {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.g-head {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.g-head b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.sp {
  flex: 1;
}
.rows {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(300px, 100%), 1fr));
  gap: var(--wb-sp-3);
}
.row {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3) var(--wb-sp-4);
}
.row.is-off {
  opacity: 0.6;
}
.r-top {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
}
.r-top b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rk {
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border-radius: var(--wb-radius-sm);
  display: grid;
  place-items: center;
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
  flex: none;
}
.r-desc {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}
.r-acts {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin-top: auto;
}
.sm {
  font-size: var(--wb-fs-2xs);
}
</style>
