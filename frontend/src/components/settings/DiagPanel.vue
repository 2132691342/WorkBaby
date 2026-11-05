<script setup lang="ts">
// 诊断：后端日志 + 最近失败的请求，回答「刚才那一下到底哪一层出的错」。
// 日志回答「引擎/后端怎么了」，失败现场回答「哪一次调用失败了」。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { formatFailure, useDiagnostics } from '@/utils/diag'
import { useToasts } from '@/composables/useToasts'

const props = defineProps<{
  /** 后端日志（新的在后） */
  logs: string[]
}>()

const emit = defineEmits<{ refreshLogs: [] }>()

const { t } = useI18n()
const toasts = useToasts()
const diag = useDiagnostics()

// 默认只看失败现场：日志是长文本，展开会把这一节挤到一屏之外。
const tab = ref<'failures' | 'logs'>('failures')

/** 复制用的文本：把一处失败的全部线索拼在一起，粘给人看不用额外解释。 */
const failureText = computed(() =>
  diag.entries.value.map(formatFailure).join('\n'),
)

async function copyFailures(): Promise<void> {
  if (!failureText.value) return
  try {
    await navigator.clipboard.writeText(failureText.value)
    toasts.push('ok', t('common.copied'))
  } catch {
    // 剪贴板不可用时不打扰：用户仍能手动选中
  }
}

/** 日志文件路径（后端日志按天切）：告诉用户去哪里找完整记录。 */
function logTail(): string {
  return props.logs.slice(-200).join('\n')
}

async function copyLogs(): Promise<void> {
  if (!props.logs.length) return
  try {
    await navigator.clipboard.writeText(logTail())
    toasts.push('ok', t('common.copied'))
  } catch {
    /* 同上 */
  }
}

/** 时刻显示：只到秒，日期在悬停里。 */
function clockOf(at: number): string {
  return new Date(at).toLocaleTimeString()
}
</script>

<template>
  <section class="card-pad">
    <div class="wb-section-title">
      {{ t('settings.groups.diag') }}
      <span class="seg diag-tabs">
        <button
          class="seg-btn"
          :class="{ active: tab === 'failures' }"
          @click="tab = 'failures'"
        >
          {{ t('settings.diagFailures', { n: diag.entries.value.length }) }}
        </button>
        <button class="seg-btn" :class="{ active: tab === 'logs' }" @click="tab = 'logs'">
          {{ t('settings.diagLogs') }}
        </button>
      </span>
      <button
        v-if="tab === 'failures'"
        class="wb-btn text sm push"
        :disabled="!diag.entries.value.length"
        @click="copyFailures"
      >
        {{ t('common.copy') }}
      </button>
      <template v-else>
        <button class="wb-btn text sm push" @click="emit('refreshLogs')">
          {{ t('common.refresh') }}
        </button>
        <button class="wb-btn text sm" :disabled="!props.logs.length" @click="copyLogs">
          {{ t('common.copy') }}
        </button>
      </template>
    </div>

    <p class="wb-hint">{{ t('settings.diagHint') }}</p>

    <!-- 失败现场 -->
    <template v-if="tab === 'failures'">
      <p v-if="!diag.entries.value.length" class="diag-empty">{{ t('settings.diagNoFailures') }}</p>
      <div v-else class="diag-list">
        <button
          v-for="(item, index) in diag.entries.value"
          :key="`${item.at}-${index}`"
          class="diag-row wb-row-click"
          :title="item.detail || item.message"
        >
          <span class="diag-time mono">{{ clockOf(item.at) }}</span>
          <span class="tag warn">{{ item.code }}</span>
          <span class="diag-where mono">{{ item.where }}</span>
          <span class="diag-msg">{{ item.message }}</span>
        </button>
        <button class="wb-btn text sm danger diag-clear" @click="diag.clear()">
          <AppIcon name="trash" :size="12" />
          {{ t('settings.diagClear') }}
        </button>
      </div>
    </template>

    <!-- 后端日志 -->
    <pre v-else class="log selectable">{{ logTail() || t('common.empty') }}</pre>
  </section>
</template>

<style scoped>
.diag-tabs {
  margin-left: var(--wb-sp-2);
}

.push {
  margin-left: auto;
}

.diag-empty {
  margin: var(--wb-sp-2) 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
}

.diag-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: var(--wb-sp-2);
}

/* 一行一条失败：时间 / 码 / 位置 / 原话。原话允许被截断，其余固定。 */
.diag-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: 3px var(--wb-sp-2);
  border: 0;
  border-radius: var(--wb-r-sm);
  background: transparent;
  color: var(--wb-ink-2);
  font-family: inherit;
  font-size: var(--wb-fs-2xs);
  text-align: left;
  cursor: pointer;
}

.diag-row:hover {
  background: var(--wb-hover);
}

.diag-time {
  flex: none;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  font-variant-numeric: tabular-nums;
}

.diag-where {
  flex: none;
  max-width: 12em;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.diag-msg {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.diag-clear {
  align-self: flex-start;
  margin-top: var(--wb-sp-1);
}

.log {
  margin: var(--wb-sp-2) 0 0;
  max-height: 260px;
  overflow: auto;
  padding: var(--wb-sp-3);
  border-radius: var(--wb-r);
  background: var(--wb-code-bg);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  line-height: 1.6;
  color: var(--wb-ink-5);
  white-space: pre-wrap;
}
</style>
