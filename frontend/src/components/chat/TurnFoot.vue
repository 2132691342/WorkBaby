<script setup lang="ts">
// 历史回合的脚注：复制 + 用量读数（提示词 / 输出 / 缓存命中率 / 上下文）+ 时间。
// 抽出来是因为它只是「读数行」，与消息渲染无关；MessageList 已到 500 行。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useToastStore } from '../../stores/toast'
import type { MessageVO, UsageVO } from '../../types/api'
import { fmtCount } from '../../utils/num'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{
  /** 这一回回合的所有消息（取最后一条带用量的） */
  parts: { msg: MessageVO }[]
  /** 回合键：复制反馈按它记忆 */
  turnKey: string
}>()

const { t } = useI18n()
const toast = useToastStore()

const usage = computed<UsageVO | null>(() => {
  for (let i = props.parts.length - 1; i >= 0; i--) {
    const u = props.parts[i].msg.message_usage
    if (u) return u
  }
  return null
})

/** 用量四件：提示词总量 / 输出 / 缓存命中率 / 上下文占用。 */
const metrics = computed(() => {
  const u = usage.value
  if (!u) return []
  const prompt = u.usage_input + u.usage_cached + (u.usage_cache_write || 0)
  const out: { k: string; v: string; hi?: boolean }[] = [
    { k: '↓', v: fmtCount(prompt) },
    { k: '↑', v: fmtCount(u.usage_output) },
  ]
  if (prompt > 0) {
    const rate = Math.round((u.usage_cached / prompt) * 100)
    out.push({ k: 'cache', v: `${rate}%`, hi: rate > 0 })
  }
  if (u.usage_context > 0) out.push({ k: 'ctx', v: fmtCount(u.usage_context) })
  return out
})

/** 这一回合最后一消息的时间。 */
const at = computed(() => props.parts[props.parts.length - 1]?.msg.message_created_at ?? 0)

const copied = ref(false)

async function copy() {
  const text = props.parts
    .map((p) => p.msg.message_content || '')
    .filter(Boolean)
    .join('\n\n')
  if (!text.trim()) return
  try {
    await navigator.clipboard.writeText(text)
    copied.value = true
    window.setTimeout(() => (copied.value = false), 1200)
  } catch {
    toast.bad(t('common.copyFailed'))
  }
}

function fmtTime(ms: number) {
  if (!ms) return ''
  const d = new Date(ms)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}
</script>

<template>
  <div v-if="metrics.length" class="msg-acts is-on turn-foot">
    <button class="ai-act" type="button" :disabled="copied" @click="copy">
      <AppIcon :name="copied ? 'check' : 'copy'" size="ic-2xs" />
      <span>{{ copied ? t('common.copied') : t('common.copy') }}</span>
    </button>
    <span
      v-for="m in metrics"
      :key="m.k"
      class="ai-metric"
      :class="{ 'is-hit': m.hi }"
    >
      <b>{{ m.k }}</b>{{ m.v }}
    </span>
    <time v-if="at" class="ai-time">{{ fmtTime(at) }}</time>
  </div>
</template>

<style scoped>
.turn-foot {
  margin-top: var(--wb-sp-1);
  padding-top: var(--wb-sp-1);
  border-top: 1px solid var(--wb-border);
}
.ai-act {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  height: calc(22px * var(--wb-vw-scale));
  padding: 0 var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
  color: var(--wb-muted);
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.ai-act:hover:not(:disabled) {
  background: var(--wb-elev-hover);
  color: var(--wb-ink);
  transform: scale(1.06);
}
.ai-act:active:not(:disabled) {
  transform: scale(var(--wb-btn-scale-press));
}
.ai-metric {
  display: inline-flex;
  align-items: baseline;
  gap: 2px;
  padding: 0 var(--wb-sp-2);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
  border-left: 1px solid var(--wb-border);
}
.ai-metric b {
  font-weight: 600;
  opacity: 0.7;
}
.ai-metric.is-hit {
  color: var(--wb-success);
}
.ai-time {
  margin-left: auto;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
</style>
