<script setup lang="ts">
// 输入区上方的状态条：缺引擎提示、引擎状态、排队条目、重试进度、错误原话。
// 它们回答「现在能不能发、刚才为什么不顺」，一堆条件渲染单独拆出，
// 不让输入区的模板反复折叠。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '@/stores/chat'
import { useAppStore } from '@/stores/app'

defineProps<{ attachError: string }>()

const chat = useChatStore()
const app = useAppStore()
const { t } = useI18n()

/** 两条队的总长度。 */
const queuedCount = computed(() => chat.queue.steering.length + chat.queue.follow_up.length)

async function clearQueued(): Promise<void> {
  try {
    await chat.clearQueue()
    chat.pushNotice('info', t('chat.queueCleared'))
  } catch (err) {
    chat.promptError = err instanceof Error ? err.message : String(err)
  }
}
</script>

<template>
  <div v-if="!app.hasEngine" class="no-engine">
    <span class="dot warn" />
    <span>{{ t('chat.noEngine') }}</span>
    <button
      class="wb-btn text sm"
      :class="{ 'is-loading': app.reDetecting }"
      :disabled="app.reDetecting"
      @click="app.detectEngines()"
    >
      {{ t('chat.redetect') }}
    </button>
    <button class="wb-btn text sm" @click="app.openApps()">{{ t('chat.openApps') }}</button>
  </div>

  <div v-if="chat.widgets.length || chat.statusText" class="status-strip">
    <span v-if="chat.statusText" class="status-chip">{{ chat.statusText }}</span>
    <span v-for="widget in chat.widgets" :key="widget.key" class="status-chip">
      {{ widget.lines.join(' · ') }}
    </span>
  </div>

  <div v-if="queuedCount" class="queue-strip">
    <span class="dot busy" />
    {{ t('chat.queuedCount', { n: queuedCount }) }}
    <!-- 引擎只支持整队清空，不支持单条移除：能力位没开，这里就不渲染可点的条目，
         只把排队的内容平铺出来（让用户知道排了什么）。 -->
    <span v-for="(m, i) in chat.queue.steering" :key="'s' + i" class="queue-chip">{{ m }}</span>
    <span v-for="(m, i) in chat.queue.follow_up" :key="'f' + i" class="queue-chip">{{ m }}</span>
    <button v-if="chat.caps?.queue_clear" class="wb-btn text sm" :title="t('chat.clearQueue')" @click="clearQueued">
      {{ t('chat.clearQueue') }}
    </button>
  </div>

  <div v-if="chat.retry" class="retry-strip">
    <span class="dot busy" />
    {{ t('chat.retrying', { attempt: chat.retry.attempt, max: chat.retry.max }) }}
    <span class="retry-delay">{{ Math.round(chat.retry.delay_ms / 1000) }}s</span>
  </div>

  <div v-if="chat.promptError" class="error-strip">
    {{ chat.promptError }}
  </div>

  <div v-if="attachError" class="error-strip">
    {{ attachError }}
    <button class="wb-btn text sm" @click="$emit('clearAttachError')">×</button>
  </div>

  <div v-if="chat.bindErr" class="warn-strip">
    {{ chat.bindErr }}
  </div>
</template>

<style scoped>
.no-engine,
.status-strip,
.queue-strip,
.retry-strip,
.error-strip,
.warn-strip {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
  padding: var(--wb-sp-2) var(--wb-sp-5);
  border-top: 1px solid var(--wb-line);
  font-size: var(--wb-fs-2xs);
  flex: none;
}

.no-engine,
.warn-strip {
  background: var(--wb-warn-soft);
  color: var(--wb-warn);
}

.error-strip {
  background: var(--wb-bad-soft);
  color: var(--wb-bad);
  overflow-wrap: anywhere;
}

.queue-strip,
.retry-strip {
  background: var(--wb-surface);
  color: var(--wb-mute);
}

.status-strip {
  background: var(--wb-surface);
  color: var(--wb-mute);
}

.status-chip,
.queue-chip {
  padding: 1px 6px;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-sm);
  background: var(--wb-hover);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-ink-5);
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.retry-delay {
  font-family: var(--wb-font-mono);
  color: var(--wb-faint);
}

.install-cmd {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
}
</style>
