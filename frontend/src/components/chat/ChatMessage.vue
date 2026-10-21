<script setup lang="ts">
// 用户消息：角色徽章交代说话权属，气泡保留右对齐的对话感。
// 助手段的渲染在 AssistantTurn（按内容块顺序内联思考 / 正文 / 工具卡）。
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MessageVO } from '../../types/api'
import { useToastStore } from '../../stores/toast'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{ msg: MessageVO }>()
const { t } = useI18n()
const toast = useToastStore()

const meName = computed(() => t('chat.you'))

const copied = ref(false)
let copiedTimer = 0
onBeforeUnmount(() => clearTimeout(copiedTimer))

async function copy() {
  if (!props.msg.content || copied.value) return
  try {
    await navigator.clipboard.writeText(props.msg.content)
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
</script>

<template>
  <div class="msg-me">
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
.me-act {
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
.me-act:hover:not(:disabled) {
  background: var(--wb-elev-hover);
  color: var(--wb-ink);
  transform: scale(1.06);
}
.me-act:active:not(:disabled) {
  transform: scale(0.92);
}
.me-act:disabled {
  opacity: 0.8;
}
</style>
