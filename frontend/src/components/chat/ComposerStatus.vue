<script setup lang="ts">
// 输入区上方的状态条：缺引擎提示、引擎状态、排队条目、重试进度、错误原话。
// 它们回答「现在能不能发、刚才为什么不顺」，一堆条件渲染单独拆出，
// 不让输入区的模板反复折叠；引擎选择在底栏（那是输入的一部分）。
import { useI18n } from 'vue-i18n'
import { useChatStore } from '@/stores/chat'
import { useAppStore } from '@/stores/app'

const emit = defineEmits<{
  clearAttachError: []
}>()

const chat = useChatStore()
const app = useAppStore()
const { t } = useI18n()

const props = defineProps<{ attachError: string }>()


async function clearQueued(): Promise<void> {
  try {
    await chat.clearQueue()
    chat.pushNotice('info', t('chat.queueCleared'))
  } catch (err) {
    chat.promptError = err instanceof Error ? err.message : String(err)
  }
}

async function dropQueued(message: string, queueName: 'steering' | 'followUp'): Promise<void> {
  try {
    if (!(await chat.removeQueued(message, queueName))) {
      chat.pushNotice('info', t('chat.queueGone'))
    }
  } catch (err) {
    chat.promptError = err instanceof Error ? err.message : String(err)
  }
}
</script>

<template>
  <div v-if="!app.hasEngine" class="no-engine">
    <span class="dot warn" />
    <span>{{ t('chat.noEngine') }}</span>
    <code class="install-cmd selectable">{{ app.installHint(chat.kind) }}</code>
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

  <div v-if="chat.queue.steering.length || chat.queue.followUp.length" class="queue-strip">
    <span class="dot busy" />
    {{ t('chat.queuedCount', { n: chat.queue.steering.length + chat.queue.followUp.length }) }}
    <template v-if="chat.caps?.queue_edit">
      <button
        v-for="(m, i) in chat.queue.steering"
        :key="'s' + i"
        class="queue-chip wb-pressable"
        :title="t('chat.removeQueued')"
        @click="dropQueued(m, 'steering')"
      >
        {{ m }}<span class="x">×</span>
      </button>
      <button
        v-for="(m, i) in chat.queue.followUp"
        :key="'f' + i"
        class="queue-chip wb-pressable"
        :title="t('chat.removeQueued')"
        @click="dropQueued(m, 'followUp')"
      >
        {{ m }}<span class="x">×</span>
      </button>
    </template>
    <button
      v-if="chat.caps?.queue_clear"
      class="wb-btn text sm"
      :title="t('chat.clearQueue')"
      @click="clearQueued"
    >
      {{ t('chat.clearQueue') }}
    </button>
  </div>

  <div v-if="chat.retry" class="retry-strip">
    <span class="dot warn" />
    {{ t('chat.retrying') }} {{ chat.retry.attempt }}/{{ chat.retry.max }}
  </div>

  <!-- 切不到请求的会话：历史看得到，但下一句话会写到另一条里去。
       不能只说“失败了”：要说清“看一下没问题、发消息会到哪里”。 -->
  <div v-if="chat.bindErr" class="bind-strip">
    <span class="dot warn" />
    <span class="bind-text selectable">{{ t('chat.bindFailed', { reason: chat.bindErr }) }}</span>
    <button class="wb-btn text sm" @click="chat.bindErr = ''">{{ t('common.close') }}</button>
  </div>

  <!-- 错误必须把原话说出来：只把面板染红等于没告诉用户发生了什么 -->
  <div v-if="chat.promptError" class="error-strip">
    <span class="dot warn" />
    <span class="error-text selectable">{{ chat.promptError }}</span>
    <button class="wb-btn text sm" @click="chat.promptError = ''">{{ t('common.close') }}</button>
  </div>

  <div v-if="props.attachError" class="error-strip">
    <span class="dot warn" />
    <span class="error-text selectable">{{ props.attachError }}</span>
    <button class="wb-btn text sm" @click="emit('clearAttachError')">
      {{ t('common.close') }}
    </button>
  </div>
</template>

<style scoped>
.no-engine {
  max-width: var(--wb-col);
  margin: 0 auto var(--wb-sp-2);
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border: 1px solid var(--wb-warn);
  border-radius: var(--wb-r);
  background: var(--wb-warn-soft);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink);
}

.install-cmd {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  padding: 2px 6px;
  border-radius: var(--wb-r-xs);
  background: var(--wb-surface);
  border: 1px solid var(--wb-line);
  user-select: all;
}

.queue-strip {
  max-width: var(--wb-col);
  margin: 0 auto var(--wb-sp-2);
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
}

/* 排队条目：点一下移除这一条。截断到 18em，长句子不把整行撑开。 */
.queue-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: 18em;
  padding: 2px 7px;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-full);
  background: var(--wb-surface);
  color: var(--wb-ink-2);
  font: inherit;
  line-height: 1.5;
  cursor: pointer;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.queue-chip:hover {
  background: var(--wb-hover);
  border-color: var(--wb-line-strong);
}

.queue-chip .x {
  color: var(--wb-mute);
  font-size: 1.05em;
  line-height: 1;
}

.queue-chip:hover .x {
  color: var(--wb-brand);
}

.retry-strip {
  max-width: var(--wb-col);
  margin: 0 auto var(--wb-sp-2);
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-warn);
}

/* 会话绑定失败：用警告色而不是危险色——
   什么都没坏，只是“下一句话会写到另一条会话里”。 */
.bind-strip {
  max-width: var(--wb-col);
  margin: 0 auto var(--wb-sp-2);
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border: 1px solid var(--wb-warn);
  border-radius: var(--wb-r);
  background: var(--wb-warn-soft);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink);
}

.bind-text {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}

.error-strip {
  max-width: var(--wb-col);
  margin: 0 auto var(--wb-sp-2);
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border: 1px solid var(--wb-bad);
  border-radius: var(--wb-r);
  background: var(--wb-bad-soft);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-bad);
}

.error-text {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}

.status-strip {
  max-width: var(--wb-col);
  margin: 0 auto var(--wb-sp-2);
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}

.status-chip {
  font-size: var(--wb-fs-3xs);
  padding: 1px 7px;
  border-radius: var(--wb-r-full);
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
  font-family: var(--wb-font-mono);
}
</style>
