<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { apiGet } from '@/api/client'
import PageState from '@/components/common/PageState.vue'
import { t } from '@/i18n'
import { formatDateTime } from '@/utils/time'
import type { RunRecord, RunRecordList } from '@/types/api'

/**
 * 运行历史：run_records 索引（GET /api/v1/chat/runs）+ JSONL 事件明细回放。
 *
 * <p>事件明细走 {home}/runs/{runID}.jsonl，点「回放」按 seq 逐条回看。
 */
const items = ref<RunRecord[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const r = await apiGet<RunRecordList>('/api/v1/chat/runs?limit=100')
    items.value = r.items ?? []
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

onMounted(load)

function statusLabel(s: RunRecord['status']): string {
  if (s === 'running') return t('runs.statusRunning')
  if (s === 'error') return t('runs.statusError')
  return t('runs.statusDone')
}

function statusCls(s: RunRecord['status']): string {
  if (s === 'running') return 'b-warning'
  if (s === 'error') return 'b-danger'
  return 'b-success'
}

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return String(n)
}

/** 毫秒 → 秒/毫秒（run 级别跨越秒级，统一用 ms 会把列撑得很难读）。 */
function fmtMs(ms: number): string {
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`
  return `${ms}ms`
}

/**
 * 分段耗时归因：等模型 / 跑工具。
 * 回答「这次 run 慢在哪」——工具全算进等模型会把长工具链误判成模型慢。
 */
function timingOf(r: RunRecord): string {
  return `${fmtMs(r.llm_ms ?? 0)} / ${fmtMs(r.tools_ms ?? 0)}`
}

// ===== 事件回放 =====
const replayOpen = ref(false)
const replayRunID = ref('')
const replayRaw = ref('')
const replayLoading = ref(false)

interface ReplayRow {
  seq: number
  name: string
  ts: number
}

/** JSONL 逐行解析为时间线；单行损坏时降级为原文，不整段失败。 */
const replayRows = computed<ReplayRow[]>(() =>
  replayRaw.value
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
    .map((l) => {
      try {
        const o = JSON.parse(l) as { seq?: number; name?: string; ts?: number }
        return { seq: o.seq ?? 0, name: o.name ?? '?', ts: o.ts ?? 0 }
      } catch {
        return { seq: 0, name: l.slice(0, 120), ts: 0 }
      }
    })
)

async function openReplay(runID: string): Promise<void> {
  replayRunID.value = runID
  replayRaw.value = ''
  replayOpen.value = true
  replayLoading.value = true
  try {
    const raw = await apiGet<string>(`/api/v1/chat/runs/${runID}/events`)
    replayRaw.value = String(raw ?? '')
  } catch (e) {
    replayRaw.value = ''
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    replayLoading.value = false
  }
}
</script>

<template>
  <div class="wb-ui" style="display: flex; flex-direction: column; gap: 20px">
    <section class="card p-sm">
      <h2 class="mb4">{{ t('runs.title') }}</h2>
      <p class="fs11 muted mb10">{{ t('runs.desc') }}</p>

      <PageState :loading="loading" :empty="items.length === 0" :error="error" @retry="load" />

      <div v-if="!loading && items.length > 0" class="tbl-wrap">
        <table class="tbl">
          <thead>
            <tr>
              <th>{{ t('runs.colStarted') }}</th>
              <th>{{ t('runs.colModel') }}</th>
              <th>{{ t('common.status') }}</th>
              <th>{{ t('runs.colReason') }}</th>
              <th class="ta-r">{{ t('runs.colTurns') }}</th>
              <th class="ta-r">{{ t('runs.colTokens') }}</th>
              <th class="ta-r">{{ t('runs.colTimings') }}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in items" :key="r.run_id">
              <td class="nowrap mono">{{ formatDateTime(r.started_at) }}</td>
              <td class="mono">{{ r.model || '—' }}</td>
              <td><span class="badge" :class="statusCls(r.status)">{{ statusLabel(r.status) }}</span></td>
              <td class="muted">{{ r.reason || '—' }}</td>
              <td class="ta-r mono">{{ r.turns }}</td>
              <td class="ta-r mono">
                ↑{{ fmtTokens(r.input_tokens) }} ↓{{ fmtTokens(r.output_tokens) }}
              </td>
              <td class="ta-r mono muted" :title="t('runs.timingsHint')">{{ timingOf(r) }}</td>
              <td>
                <div class="tbl-actions">
                  <button class="btn" style="height: 24px; padding: 0 8px" @click="openReplay(r.run_id)">
                    {{ t('runs.replay') }}
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <el-dialog v-model="replayOpen" :title="t('runs.replayTitle')" width="720px" append-to-body>
      <p class="fs11 muted mono mb10">{{ replayRunID }}</p>
      <div v-if="replayLoading" class="py-8">
        <el-skeleton :rows="4" animated />
      </div>
      <p v-else-if="replayRows.length === 0" class="fs11 muted">{{ t('runs.replayEmpty') }}</p>
      <div v-else class="tbl-wrap" style="max-height: 56vh; overflow: auto">
        <table class="tbl">
          <thead>
            <tr>
              <th style="width: 72px">seq</th>
              <th>event</th>
              <th style="width: 160px">{{ t('runs.colStarted') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, i) in replayRows" :key="i">
              <td class="mono muted">{{ row.seq }}</td>
              <td class="mono">{{ row.name }}</td>
              <td class="mono muted nowrap">{{ row.ts ? formatDateTime(row.ts) : '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </el-dialog>
  </div>
</template>
