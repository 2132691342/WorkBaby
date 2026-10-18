<script setup lang="ts">
// 工具面板：清单来自 PI（最近一次会话声明给模型的工具 + MCP 在线工具）。
// 启停写的是 PI settings.json 的 defaultTools，不自己实现任何工具。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import type { ToolListRESP, ToolVO } from '../../types/api'
import { fmtRel } from '../../utils/time'
import AppIcon from '../common/AppIcon.vue'

const toast = useToastStore()
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

onMounted(() => void reload())
</script>

<template>
  <div class="panel">
    <header class="pn-head">
      <div>
        <h3>工具</h3>
        <p class="pn-sub">
          <template v-if="data?.snapshot === 'session'">
            来自最近一次会话的工具声明（{{ fmtRel(data.snapshot_at) }}）：这就是模型此刻真能调到的工具。
          </template>
          <template v-else>
            还没有会话快照，这里按 PI 的内置工具表推断；聊过一轮之后会显示真实声明。
          </template>
        </p>
      </div>
      <div class="acts">
        <input v-model="filter" class="input search" placeholder="搜索工具" />
        <button class="btn" type="button" @click="reload">刷新</button>
      </div>
    </header>

    <section v-for="g in groups" :key="g.key" class="group">
      <div class="g-head">
        <b>{{ SOURCE[g.key] }}</b>
        <span class="sp" />
        <span class="muted">{{ g.items.length }} 个</span>
      </div>
      <div class="rows">
        <article v-for="t in g.items" :key="t.name" class="row card" :class="{ 'is-off': !t.enabled }">
          <span class="rk"><AppIcon name="wrench" size="ic-sm" /></span>
          <div class="rr">
            <b class="mono">{{ t.name }}</b>
            <span class="r-desc">{{ t.description }}</span>
          </div>
          <span v-if="g.key === 'mcp'" class="muted sm">{{ t.server }}</span>
          <button
            v-else
            class="btn btn-sm"
            type="button"
            :class="{ 'is-loading': busy === t.name }"
            :disabled="!!busy"
            @click="toggle(t)"
          >
            {{ t.enabled ? '停用' : '启用' }}
          </button>
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
.pn-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
}
.pn-head h3 {
  font-size: var(--wb-fs-lg);
  color: var(--wb-ink);
}
.pn-sub {
  margin-top: 4px;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  max-width: 640px;
  line-height: 1.7;
}
.acts {
  display: flex;
  gap: var(--wb-sp-2);
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
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: 9px var(--wb-sp-4);
}
.row.is-off {
  opacity: 0.6;
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
.rr {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.rr b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.r-desc {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sm {
  font-size: var(--wb-fs-2xs);
}
</style>
