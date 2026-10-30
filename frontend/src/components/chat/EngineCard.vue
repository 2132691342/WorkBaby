<script setup lang="ts">
// 侧栏底部的引擎状态卡：一眼看清「谁在跑、什么模型、上下文多满」。
// 点进去是设置页——它是状态条，也是最短的那条去配置的路。
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import { useSettingsStore } from '../../stores/settings'
import { fmtCount } from '../../utils/num'
import AppIcon from '../common/AppIcon.vue'

const { t } = useI18n()
const router = useRouter()
const chat = useChatStore()
const session = useSessionStore()
const settings = useSettingsStore()

const provider = computed(
  () => session.current?.session_provider || settings.boot?.boot_default_provider || '',
)
const model = computed(() => session.current?.session_model || settings.boot?.boot_default_model || '')
const modelName = computed(() => {
  const list = settings.providers ?? []
  const p = list.find((x) => x.provider_id === provider.value)
  return (
    p?.provider_models.find((m) => m.model_id === model.value)?.model_name ||
    model.value ||
    t('engine.noModel')
  )
})
const thinking = computed(() => t(`thinking.${session.current?.session_thinking_level || 'medium'}`))
const contextPct = computed(() => {
  if (!chat.contextKnown) return 0
  return Math.max(0, Math.min(100, chat.contextRatio))
})
const statusText = computed(() => (chat.running ? t('engine.running') : t('engine.idle')))

function open() {
  void router.push('/settings/providers')
}
</script>

<template>
  <button class="engine" type="button" @click="open">
    <div class="eg-head">
      <span class="wb-led" :class="chat.running ? 'wb-led--live' : 'wb-led--ok'" />
      <span class="wb-label">{{ t('engine.title') }}</span>
      <span class="eg-state" :class="{ 'is-live': chat.running }">{{ statusText }}</span>
      <AppIcon name="chevron-right" size="ic-2xs" class="eg-go" />
    </div>

    <div class="eg-model" :title="modelName">{{ modelName }}</div>

    <dl class="eg-rows">
      <div class="eg-row">
        <dt>{{ t('engine.thinking') }}</dt>
        <dd>{{ thinking }}</dd>
      </div>
      <div class="eg-row">
        <dt>{{ t('engine.context') }}</dt>
        <dd>
          {{ fmtCount(chat.contextUsed) }}
          <span v-if="chat.contextWindow">/ {{ fmtCount(chat.contextWindow) }}</span>
        </dd>
      </div>
    </dl>

    <div class="eg-bar">
      <span class="eg-fill" :style="{ width: `${contextPct}%` }" />
    </div>
  </button>
</template>

<style scoped>
.engine {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2);
  border-radius: var(--wb-radius-lg);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  text-align: left;
  transition:
    border-color var(--wb-dur) var(--wb-ease),
    box-shadow var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.engine:hover {
  border-color: var(--wb-primary-line);
  box-shadow: var(--wb-card-shadow-hover);
  transform: scale(1.01);
}
.engine:active {
  transform: scale(0.985);
}
.eg-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.eg-state {
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-success);
}
.eg-state.is-live {
  color: var(--wb-live);
}
.eg-go {
  margin-left: auto;
  color: var(--wb-muted);
}
.eg-model {
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.eg-rows {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.eg-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-2xs);
}
.eg-row dt {
  color: var(--wb-muted);
}
.eg-row dd {
  margin: 0;
  color: var(--wb-ink-2);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.eg-bar {
  height: 3px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-elev-hover);
  overflow: hidden;
}
.eg-fill {
  display: block;
  height: 100%;
  border-radius: var(--wb-radius-full);
  background: var(--wb-primary);
  transition: width var(--wb-dur) var(--wb-ease);
}
</style>
