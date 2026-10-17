<script setup lang="ts">
// 按模型占比的圆环图：色值只走 --wb-ch-1..4，规则见 DESIGN.md「图表」。
import { computed } from 'vue'
import type { StatsModelItem } from '../../types/api'
import { fmtCount } from '../../utils/num'

const props = defineProps<{ items: StatsModelItem[]; limit?: number }>()

const R = 52
const STROKE = 16
const C = 2 * Math.PI * R

const rows = computed(() => {
  const list = [...props.items].sort((a, b) => (b.total || 0) - (a.total || 0))
  const sum = list.reduce((n, m) => n + (m.total || 0), 0)
  if (!sum) return []
  const top = list.slice(0, props.limit || 4)
  const rest = list.slice(top.length)
  const restTotal = rest.reduce((n, m) => n + (m.total || 0), 0)
  const out = top.map((m, i) => ({
    key: m.model,
    model: m.model,
    total: m.total,
    calls: m.calls,
    pct: (m.total || 0) / sum,
    tone: i,
  }))
  if (rest.length) {
    out.push({
      key: '__rest__',
      model: `其他 ${rest.length} 个模型`,
      total: restTotal,
      calls: 0,
      pct: restTotal / sum,
      tone: out.length,
    })
  }
  return out
})

// 圆环按占比切弧：留 1.5° 缝让相邻两段不糊在一起
const segs = computed(() => {
  let acc = 0
  return rows.value.map((r) => {
    const len = Math.max(0, r.pct * C - 2.6)
    const seg = { key: r.key, tone: r.tone, dash: len, offset: -acc * C }
    acc += r.pct
    return seg
  })
})

const total = computed(() => props.items.reduce((n, m) => n + (m.total || 0), 0))
</script>

<template>
  <div v-if="rows.length" class="ring-wrap">
    <div class="ring" role="img" aria-label="按模型的 token 占比">
      <svg viewBox="0 0 140 140">
        <g transform="rotate(-90 70 70)">
          <circle class="track" cx="70" cy="70" :r="R" fill="none" :stroke-width="STROKE" />
          <circle
            v-for="s in segs"
            :key="s.key"
            class="arc"
            :class="`t${s.tone}`"
            cx="70"
            cy="70"
            :r="R"
            fill="none"
            :stroke-width="STROKE"
            :stroke-dasharray="`${s.dash} ${C - s.dash}`"
            :stroke-dashoffset="s.offset"
            stroke-linecap="butt"
          />
        </g>
      </svg>
      <!-- 环心放总量而不是百分比：用户关心的是「花了多少」，不是「这个圆代表 100%」 -->
      <div class="ring-center">
        <b>{{ fmtCount(total) }}</b>
        <span>token</span>
      </div>
    </div>
    <ul class="legend">
      <li v-for="r in rows" :key="r.key">
        <i class="sw" :class="`t${r.tone}`" />
        <span class="nm" :title="r.model">{{ r.model }}</span>
        <span class="pct">{{ Math.round(r.pct * 100) }}%</span>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.ring-wrap {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-5);
}
.ring {
  position: relative;
  width: 148px;
  height: 148px;
  flex: none;
}
.ring svg {
  display: block;
  width: 100%;
  height: 100%;
}
.track {
  stroke: var(--wb-tint);
}
.arc {
  transition: opacity var(--wb-dur) var(--wb-ease);
}
.arc.t0 {
  stroke: var(--wb-ch-1);
}
.arc.t1 {
  stroke: var(--wb-ch-2);
}
.arc.t2 {
  stroke: var(--wb-ch-3);
}
.arc.t3 {
  stroke: var(--wb-ch-4);
}
.ring-center {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 1px;
  pointer-events: none;
}
.ring-center b {
  font-family: var(--font-display);
  font-size: var(--wb-fs-lg);
  font-weight: 700;
  color: var(--wb-ink);
  line-height: 1.1;
}
.ring-center span {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.legend {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.legend li {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
}
.sw {
  width: 9px;
  height: 9px;
  border-radius: var(--wb-radius-full);
  flex: none;
}
.sw.t0 {
  background: var(--wb-ch-1);
}
.sw.t1 {
  background: var(--wb-ch-2);
}
.sw.t2 {
  background: var(--wb-ch-3);
}
.sw.t3 {
  background: var(--wb-ch-4);
}
.legend .nm {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
}
.legend .pct {
  flex: none;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
</style>
