<script setup lang="ts">
import { computed } from 'vue'
import { t } from '@/i18n'
import { formatDuration } from '@/utils/time'

/**
 * 消息用量徽标：从 MessageList 抽出的小型纯展示组件。
 *
 * <p>只接受单条消息 + isLast 标记（控制「重新生成」按钮的禁用），其余 UI 完全内聚。
 * 复杂格式（紧凑数字、缓存命中率 dot、悬浮详情）由父组件作为 props 传入，
 * 本组件不再重复业务逻辑——保持 MessageList 拆分的纯度（CLAUDE §2.3.1：内联快速解析不暴露）。
 */
interface UsageShape {
  input_tokens?: number | null
  output_tokens?: number | null
  cache_read_tokens?: number | null
  total_tokens?: number | null
  latency_ms?: number | null
  cost?: string | null
}

const props = defineProps<{
  usage: UsageShape | null | undefined
  /** 流式进行中：加一枚呼吸圆点提示「本轮正在计量」。 */
  live?: boolean
}>()

function hasUsage(u?: UsageShape | null): boolean {
  if (!u) return false
  return (
    (u.input_tokens ?? 0) > 0 ||
    (u.output_tokens ?? 0) > 0 ||
    (u.cache_read_tokens ?? 0) > 0 ||
    (u.total_tokens ?? 0) > 0
  )
}

function compact(n?: number | null): string {
  const v = n ?? 0
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}k`
  return String(v)
}

function cacheHitRate(u?: UsageShape | null): number | null {
  if (!u) return null
  const read = u.cache_read_tokens ?? 0
  const input = u.input_tokens ?? 0
  // 输入为 0 说明本轮没有可用分母，隐藏；否则一律展示——
  // 之前 read=0 时整行隐藏，新会话前几轮缓存未预热时用户会以为功能丢了。
  if (input <= 0) return null
  return Math.round((read / input) * 100)
}

/** 耗时统一走 utils/time（口径全站一致）。 */
const fmtDuration = formatDuration

function fmtUsage(u: UsageShape | null | undefined): string {
  if (!u) return ''
  const parts: string[] = []
  if (u.input_tokens) parts.push(`↑${compact(u.input_tokens)}`)
  if (u.output_tokens) parts.push(`↓${compact(u.output_tokens)}`)
  const hit = cacheHitRate(u)
  if (hit != null) parts.push(`缓存 ${hit}%`)
  const dur = fmtDuration(u.latency_ms)
  if (dur) parts.push(dur)
  return parts.join(' · ')
}

function usageDetail(u: UsageShape | null | undefined): string {
  if (!u) return ''
  const rows: string[] = []
  if (u.input_tokens != null) rows.push(`输入 ${u.input_tokens}`)
  if (u.output_tokens != null) rows.push(`输出 ${u.output_tokens}`)
  // 命中率必须与分母一起给：只显示「缓存 99%」会被误读成全局口径，
  // 而它实际是「本轮」——与仪表盘的窗口聚合值天然不同。
  if (u.input_tokens != null) {
    const read = u.cache_read_tokens ?? 0
    if (read > 0) {
      rows.push(`${t('chat.usageCacheRead')} ${read} / ${t('chat.usageInput')} ${u.input_tokens}（${t('chat.usageThisTurn')} ${cacheHitRate(u)}%）`)
    } else {
      rows.push(t('chat.usageCacheCold'))
    }
  }
  if (u.total_tokens != null) rows.push(`合计 ${u.total_tokens}`)
  const dur = fmtDuration(u.latency_ms)
  if (dur) rows.push(`耗时 ${dur}`)
  if (u.cost) rows.push(`费用 ${u.cost}`)
  return rows.join('\n')
}

const show = computed(() => hasUsage(props.usage))
</script>

<template>
  <div v-if="show" class="mt-1 px-1">
    <el-tooltip :content="usageDetail(usage)" placement="top">
      <span class="inline-flex cursor-help items-center gap-1 rounded-full border border-wb-border/70 bg-wb-surface px-1.5 py-0.5 font-mono text-[10px] tabular-nums text-wb-muted transition-colors hover:border-wb-primary/40">
        <span v-if="live" class="h-1.5 w-1.5 animate-pulse rounded-full bg-wb-primary" />
        {{ fmtUsage(usage) }}
        <span
          v-if="cacheHitRate(usage) != null"
          class="h-1.5 w-1.5 rounded-full"
          :style="{ background: (cacheHitRate(usage) ?? 0) >= 50 ? 'var(--wb-mint)' : 'var(--wb-warning)' }"
        />
      </span>
    </el-tooltip>
  </div>
</template>
