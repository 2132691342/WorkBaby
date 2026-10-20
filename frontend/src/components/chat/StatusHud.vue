<script setup lang="ts">
// 底部状态条：模型 / 思考档 / 上下文水位 / 本轮耗时常驻可见。
// 这些读数放在输入区里会被工具按钮挤走，独立成条后任何时候都能一眼看到。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import { useSettingsStore } from '../../stores/settings'
import { fmtCount } from '../../utils/num'
import AppIcon from '../common/AppIcon.vue'

const { t } = useI18n()
const chat = useChatStore()
const session = useSessionStore()
const settings = useSettingsStore()

const model = computed(() => {
  const s = session.current
  return s?.model || settings.boot?.default_model || ''
})
const provider = computed(() => {
  const s = session.current
  return s?.provider || settings.boot?.default_provider || ''
})

const thinking = computed(() => {
  const level = session.current?.thinking_level
  return level ? t(`thinking.${level}`) : ''
})

const ctxTone = computed(() => {
  const r = chat.contextRatio
  if (r >= 90) return 'is-bad'
  if (r >= 70) return 'is-warn'
  return ''
})

const elapsed = computed(() => {
  const s = chat.elapsedSec
  if (!s) return ''
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m${s % 60}s`
})
</script>

<template>
  <div class="hud">
    <span class="hud-seg hud-model" :title="`${provider}/${model}`">
      <AppIcon name="cpu" size="ic-2xs" />
      <span class="hud-tx">{{ provider }}/{{ model || '—' }}</span>
    </span>

    <span v-if="thinking" class="hud-seg">
      <AppIcon name="brain" size="ic-2xs" />
      <span class="hud-tx">{{ thinking }}</span>
    </span>

    <span class="hud-sp" />

    <span v-if="chat.running && elapsed" class="hud-seg">
      <AppIcon name="clock" size="ic-2xs" />
      <span class="hud-tx">{{ elapsed }}</span>
    </span>

    <span
      v-if="chat.contextKnown && chat.contextWindow > 0"
      class="hud-seg hud-ctx"
      :class="ctxTone"
      :title="t('chat.contextMeter')"
    >
      <AppIcon name="chart" size="ic-2xs" />
      <span class="hud-tx">{{ chat.contextRatio }}%</span>
      <span class="hud-dim">/ {{ fmtCount(chat.contextWindow) }}</span>
    </span>
  </div>
</template>

<style scoped>
.hud {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  height: calc(30px * var(--wb-vw-scale));
  flex: none;
  padding: 0 var(--wb-sp-5);
  border-top: 1px solid var(--wb-border);
  background: var(--wb-elev-workspace);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.hud-seg {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  min-width: 0;
}
.hud-model {
  max-width: calc(280px * var(--wb-vw-scale));
}
.hud-tx {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hud-dim {
  opacity: 0.65;
}
.hud-sp {
  flex: 1;
  min-width: var(--wb-sp-2);
}
.hud-ctx.is-warn {
  color: var(--wb-warning);
}
.hud-ctx.is-bad {
  color: var(--wb-danger);
}
</style>
