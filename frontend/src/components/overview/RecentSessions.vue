<script setup lang="ts">
// 概览页的「最近会话」卡：一行一条，点开继续。
// 「还在核对」必须显示成还没数完，不能是空列表 + 「还没有会话」——
// 那是一句断言，而磁盘上可能躺着一堆会话文件。
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { SessionRow } from '@/types/api'

defineProps<{
  rows: SessionRow[]
  /** 这一屏的事实是否已经全部到手；为假时不说「还没有会话」 */
  ready: boolean
}>()

const emit = defineEmits<{
  open: [path: string]
  all: []
  start: []
}>()

const { t } = useI18n()

/** 毫秒时间戳 → 「月-日 时:分」。时间戳是 number，模版里不能直接 slice。 */
function stamp(ms: number): string {
  if (!ms) return ''
  const date = new Date(ms)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}
</script>

<template>
  <section class="wb-card card-pad">
    <div class="wb-sec-head">
      <span class="wb-sec-title">{{ t('overview.recent') }}</span>
      <button class="wb-btn text sm" @click="emit('all')">
        {{ t('overview.all') }}
        <AppIcon name="chevron" :size="12" />
      </button>
    </div>

    <!-- 还没数完：不给空态，只说一句在核对 -->
    <div v-if="!ready && !rows.length" class="recent-pending">
      <span class="dot busy" />
      {{ t('overview.collecting') }}
    </div>

    <div v-else-if="rows.length" class="recent-list">
      <button v-for="row in rows" :key="row.session_id" class="recent-row wb-row-click" @click="emit('open', row.session_path)">
        <span class="wb-tile wb-tile--sm wb-tile--neutral">
          <AppIcon name="chat" :size="14" />
        </span>
        <span class="recent-title">{{ row.session_title || t('common.untitled') }}</span>
        <span class="recent-meta wb-mono">{{ row.session_model || '—' }}</span>
        <span class="recent-time wb-mono">{{ stamp(row.updated_at) }}</span>
      </button>
    </div>

    <div v-else class="wb-empty">
      <p class="wb-empty-title">{{ t('overview.noSessions') }}</p>
      <p class="wb-empty-hint">{{ t('overview.noSessionsHint') }}</p>
      <button class="wb-btn sm primary" @click="emit('start')">{{ t('overview.startChat') }}</button>
    </div>
  </section>
</template>

<style scoped>
.recent-list {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.recent-pending {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3) var(--wb-sp-2);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
}

.recent-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: var(--wb-sp-2);
  border: none;
  background: transparent;
  text-align: left;
  border-radius: var(--wb-r-sm);
}

.recent-title {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recent-meta {
  flex: none;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.recent-time {
  flex: none;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.wb-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-6);
  text-align: center;
}

.wb-empty-title {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}

.wb-empty-hint {
  margin: 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
}
</style>
