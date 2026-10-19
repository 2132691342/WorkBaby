<script setup lang="ts">
// 单条消息：用户气泡 / 助手回答（markdown + 思考折叠 + 用量脚注）。
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MessageVO } from '../../types/api'
import { useToastStore } from '../../stores/toast'
import { renderMarkdown } from '../../utils/md'
import { fmtCount } from '../../utils/num'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{ msg: MessageVO; plain?: boolean }>()
const { t } = useI18n()
const toast = useToastStore()

// 角色名走 i18n：硬编码「你 / WB」在切换语言后会留下一半中文
const meName = computed(() => t('chat.you'))
const assistantName = computed(() => t('chat.assistant'))

// 正文里可能混着 <think> 标签（没有单独 thinking 字段的消息）：渲染时拆开，
// 正文干净、思考归位——用户看到裸标签才会以为程序坏了。
function splitThink(s: string): { think: string; text: string } {
  if (!s.includes('<think>')) return { think: '', text: s }
  const think = [...s.matchAll(/<think>([\s\S]*?)<\/think>/g)]
    .map((m) => m[1].trim())
    .filter(Boolean)
    .join('\n\n')
  const text = s
    .replace(/<think>[\s\S]*?<\/think>/g, '')
    .replace(/<\/?think>/g, '')
    .trim()
  return { think, text }
}

const split = computed(() => splitThink(props.msg.content || ''))
const html = computed(() => (split.value.text ? renderMarkdown(split.value.text) : ''))
const hasText = computed(() => split.value.text.trim().length > 0)
const thinking = computed(() => props.msg.thinking || split.value.think)
const thinkOpen = ref(false)

// 只有 toolCall 的助手消息没有可读正文：工具卡已经单独列出，
// 再给它画一个空壳就是一个没内容的框（从命令行跑出来的会话满屏都是）。
const visible = computed(
  () => hasText.value || thinking.value.trim().length > 0 || !!props.msg.images?.length,
)

const copied = ref(false)
let copiedTimer = 0
onBeforeUnmount(() => clearTimeout(copiedTimer))

async function copy() {
  if (!split.value.text || copied.value) return
  try {
    await navigator.clipboard.writeText(split.value.text)
    copied.value = true
    clearTimeout(copiedTimer)
    copiedTimer = window.setTimeout(() => (copied.value = false), 1200)
  } catch {
    // 剪贴板被系统策略拒绝时不能静默：用户以为复制成功了
    toast.bad(t('common.copyFailed'))
  }
}

function fmtTime(ms: number) {
  const d = new Date(ms)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

// 用量读数四件：入 / 出 / 缓存命中（决定贵不贵）/ 上下文占用（决定还能聊多久）
const metrics = computed(() => {
  const u = props.msg.usage
  if (!u) return []
  const input = u.cached > u.input ? u.input + u.cached : u.input
  const context = Math.max(u.context, input)
  const out: { k: string; v: string; hi?: boolean }[] = [
    { k: '↓', v: fmtCount(input) },
    { k: '↑', v: fmtCount(u.output) },
  ]
  if (input > 0) {
    const rate = u.cached > 0 ? Math.min(100, Math.round((u.cached / input) * 100)) : 0
    out.push({ k: 'cache', v: rate > 0 ? `${rate}%` : '—', hi: rate > 0 })
  }
  if (context > 0) out.push({ k: 'ctx', v: fmtCount(context) })
  return out
})
</script>

<template>
  <!-- 用户消息：角色徽章统一交代说话权属，气泡保留右对齐的对话感 -->
  <div v-if="msg.role === 'user'" class="msg-me">
    <div class="msg-me-col">
      <div class="msg-badge"><span>{{ meName }}</span></div>
      <div class="msg-me-bubble">
        <div v-if="msg.images?.length" class="me-imgs">
          <img
            v-for="(im, i) in msg.images"
            :key="i"
            class="me-img"
            :src="`data:${im.mime};base64,${im.base64}`"
            :alt="`${i + 1}`"
          />
        </div>
        <div v-if="msg.content">{{ msg.content }}</div>
      </div>
      <div class="msg-acts is-end">
        <time class="me-time">{{ fmtTime(msg.created_at) }}</time>
        <el-tooltip :content="copied ? t('common.copied') : t('common.copy')" placement="top" :show-after="300">
          <button class="me-act" type="button" :disabled="copied || !msg.content" @click="copy">
            <AppIcon :name="copied ? 'check' : 'copy'" size="ic-2xs" />
          </button>
        </el-tooltip>
      </div>
    </div>
  </div>

  <!-- 助手回答：容器代替头像做分区，纯工具声明回合不重复打徽章 -->
  <div v-else-if="visible" class="msg-ai" :class="{ 'is-plain': plain }">
    <div v-if="hasText && !plain" class="msg-badge" :class="{ 'is-bad': msg.is_error }">
      <span>{{ assistantName }}</span>
      <AppIcon v-if="msg.is_error" name="alert" size="ic-2xs" />
    </div>
    <div class="msg-ai-body">
      <div v-if="thinking" class="think">
        <button class="think-head" type="button" :aria-expanded="thinkOpen" @click="thinkOpen = !thinkOpen">
          <AppIcon name="brain" size="ic-xs" />
          <span>{{ t('chat.thinkingBlock') }}</span>
          <span class="think-count">{{ t('chat.thinkChars', { n: thinking.length }) }}</span>
          <AppIcon class="think-chev" :name="thinkOpen ? 'chevron-down' : 'chevron-right'" size="ic-xs" />
        </button>
        <div v-if="thinkOpen" class="think-body">{{ thinking }}</div>
      </div>

      <div v-if="hasText" class="prose" :class="{ 'is-error': msg.is_error }" v-html="html" />

      <div v-if="hasText" class="msg-acts">
        <el-tooltip :content="copied ? t('common.copied') : t('common.copy')" placement="top" :show-after="300">
          <button class="ai-act" type="button" :disabled="copied" @click="copy">
            <AppIcon :name="copied ? 'check' : 'copy'" size="ic-2xs" />
            <span>{{ copied ? t('common.copied') : t('common.copy') }}</span>
          </button>
        </el-tooltip>
        <span v-for="m in metrics" :key="m.k" class="ai-metric" :class="{ 'is-hit': m.hi }">
          <b>{{ m.k }}</b>{{ m.v }}
        </span>
        <time class="ai-time">{{ fmtTime(msg.created_at) }}</time>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 用户列整体右对齐，但徽章要占满宽度后在内部靠右——
   否则 align-items:flex-end 会把徽章压成与气泡同宽的窄条。 */
.msg-me-col {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: var(--wb-sp-1);
  width: 100%;
}
.msg-me-col > .msg-badge {
  align-self: stretch;
}
.me-imgs {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-2);
  margin-bottom: var(--wb-sp-2);
}
.me-img {
  max-width: calc(180px * var(--wb-vw-scale));
  max-height: calc(120px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  object-fit: cover;
}
.me-time {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.msg-acts.is-end {
  justify-content: flex-end;
}
.me-act,
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
.me-act:hover:not(:disabled),
.ai-act:hover:not(:disabled) {
  background: var(--wb-elev-hover);
  color: var(--wb-ink);
  transform: scale(1.06);
}
.me-act:active:not(:disabled),
.ai-act:active:not(:disabled) {
  transform: scale(0.92);
}
.me-act:disabled,
.ai-act:disabled {
  opacity: 0.8;
}
.msg-ai-body.is-indent {
  padding-left: calc(var(--wb-avatar-size) + var(--wb-sp-3));
}
.msg-ai.is-plain .msg-ai-body.is-indent {
  padding-left: 0;
}
.think-count {
  font-variant-numeric: tabular-nums;
  opacity: 0.75;
  font-weight: 500;
}
.think-chev {
  margin-left: auto;
  transition: transform var(--wb-dur-fast) var(--wb-ease);
}
.prose.is-error {
  color: var(--wb-danger);
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
