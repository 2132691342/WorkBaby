<script setup lang="ts">
import { errorText } from '../../utils/errors'
// 用户消息：角色徽章交代说话权属，气泡保留右对齐的对话感。
// 助手段的渲染在 AssistantTurn（按内容块顺序内联思考 / 正文 / 工具卡）。
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import type { MessageImageVO, MessageVO } from '../../types/api'
import { useToastStore } from '../../stores/toast'
import { useSessionStore } from '../../stores/session'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{ msg: MessageVO }>()
const { t } = useI18n()
const toast = useToastStore()
const session = useSessionStore()

// 只有用户消息能当分支点：从助手回复之后重开会留下半截对话
const canBranch = computed(
  () =>
    Boolean(props.msg.message_entry_id) &&
    session.forkEntryIds.includes(props.msg.message_entry_id ?? ''),
)

async function branchHere() {
  const entryId = props.msg.message_entry_id
  if (!canBranch.value || !entryId) return
  try {
    await ElMessageBox.confirm(t('chat.branchConfirm'), t('chat.branchTitle'), {
      confirmButtonText: t('chat.branchGo'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    await session.branchFrom(entryId)
    toast.ok(t('chat.branchDone'))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  }
}

const meName = computed(() => t('chat.you'))

const copied = ref(false)
let copiedTimer = 0
onBeforeUnmount(() => clearTimeout(copiedTimer))

async function copy() {
  if (!props.msg.message_content || copied.value) return
  try {
    await navigator.clipboard.writeText(props.msg.message_content)
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

/** 点图放大：直接在容器里开一个新标签页看原图，不另做预览层。 */
function preview(img: MessageImageVO) {
  const win = window.open('', '_blank')
  if (!win) return
  win.document.write(
    `<title>${img.image_mime}</title><body style="margin:0;background:#0b1020">` +
      `<img src="data:${img.image_mime};base64,${img.image_base64}" style="max-width:100%;display:block;margin:0 auto">`,
  )
  win.document.close()
}
</script>

<template>
  <div class="msg-me">
    <div class="msg-me-col">
      <div class="msg-badge"><span>{{ meName }}</span></div>
      <div class="msg-me-bubble">
        <!-- 技能芯片：回放时也显形，看得出这条消息是哪个技能带的 -->
        <span v-if="msg.message_skill" class="me-skill">
          <AppIcon name="sparkles" size="ic-2xs" />
          <b>{{ msg.message_skill }}</b>
        </span>
        <div v-if="msg.message_images?.length" class="me-imgs">
          <img
            v-for="(img, i) in msg.message_images"
            :key="i"
            class="me-img"
            :src="`data:${img.image_mime};base64,${img.image_base64}`"
            :alt="t('chat.attachTitle')"
            @click="preview(img)"
          />
        </div>
        <div v-if="msg.message_content" class="me-text">{{ msg.message_content }}</div>
      </div>
      <div class="msg-acts is-end">
        <time class="me-time">{{ fmtTime(msg.message_created_at) }}</time>
        <el-tooltip :content="copied ? t('common.copied') : t('common.copy')" placement="top" :show-after="300">
          <button class="me-act" type="button" :disabled="copied || !msg.message_content" @click="copy">
            <AppIcon :name="copied ? 'check' : 'copy'" size="ic-2xs" />
          </button>
        </el-tooltip>
        <el-tooltip v-if="canBranch" :content="t('chat.branchHere')" placement="top" :show-after="300">
          <button class="me-act" type="button" @click="branchHere">
            <AppIcon name="refresh" size="ic-2xs" />
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
.me-time {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
/* 技能芯片与图片都在气泡内顶部：它们是这条消息的一部分，不是旁注 */
.me-skill {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  margin-bottom: var(--wb-sp-2);
  padding: 1px var(--wb-sp-2);
  border-radius: var(--wb-radius-full);
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
  font-size: var(--wb-fs-2xs);
}
.me-imgs {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-2);
  margin-bottom: var(--wb-sp-2);
}
.me-img {
  width: calc(120px * var(--wb-vw-scale));
  max-height: calc(160px * var(--wb-vw-scale));
  object-fit: cover;
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  cursor: zoom-in;
  transition:
    transform var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease);
}
.me-img:hover {
  transform: scale(1.02);
  border-color: var(--wb-primary);
}
.me-text {
  white-space: pre-wrap;
  word-break: break-word;
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
  transform: scale(var(--wb-btn-scale-press));
}
.me-act:disabled {
  opacity: 0.8;
}
</style>
