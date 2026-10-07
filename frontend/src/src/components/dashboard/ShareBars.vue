<script setup lang="ts">
// 按模型占比：几条横向占比条，语义色循环用，不引图表库。
import { computed } from 'vue'
import type { StatsModelItem } from '../../types/api'
import { fmtCount } from '../../utils/num'

const props = defineProps<{ items: StatsModelItem[]; limit?: number }>()

// 只列前几个模型：再往下对「用得最多的是谁」这个问题没有增量信息
const top = computed(() => props.items.slice(0, props.limit || 5))
const sum = computed(() => props.items.reduce((n, m) => n + (m.total || 0), 0))

const rows = computed(() =>
  top.value.map((m, i) => ({
    key: m.model,
    model: m.model,
    total: m.total,
    pct: sum.value ? Math.round(((m.total || 0) / sum.value) * 100) : 0,
    tone: i % 3,
    tip: `${m.model}：${fmtCount(m.total)} token · 调用 ${m.calls} 次`,
  })),
)
</script>

<template>
  <div class="rowlist">
    <div v-for="r in rows" :key="r.key" class="rli share" :title="r.tip">
      <span class="nm">{{ r.model }}</span>
      <span class="bar"><i :class="`t${r.tone}`" :style="{ width: `${r.pct}%` }" /></span>
      <span class="pct">{{ r.pct }}%</span>
    </div>
  </div>
</template>

<style scoped>
.share {
  gap: var(--wb-sp-3);
  cursor: default;
}
.nm {
  flex: 0 1 auto;
  min-width: 0;
  max-width: 46%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
}
.share .bar {
  flex: 1 1 auto;
  min-width: 0;
}
.pct {
  flex: none;
  width: 4ch;
  text-align: right;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.t0 {
  background: var(--wb-primary);
}
.t1 {
  background: var(--wb-live);
}
.t2 {
  background: var(--wb-lavender);
}
</style>
