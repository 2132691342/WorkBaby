<script setup lang="ts">
// 输入区底栏：附件按钮 + 工作区 / 会话参数 chip + 上下文水位 + 模式切换 + 发送 / 停止。
// 与输入框分开的理由：这一排是「这一轮用什么参数发」，与编辑正文无关，
// 一起放会把 ChatInput 顶到 500 行以上。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import AppIcon from '../common/AppIcon.vue'
import ContextMeter from './ContextMeter.vue'
import SessionChips from './SessionChips.vue'
import WorkspaceChip from './WorkspaceChip.vue'

defineProps<{
  /** 运行中：底栏换成「插话 / 排队 + 停止」 */
  running: boolean
  sending?: boolean
  stopping?: boolean
  /** 运行中正在发的是哪一类 */
  acting?: 'steer' | 'queue' | ''
  disabled?: boolean
  /** 区外附件条数：>0 时提示且禁止发送 */
  outsideCount: number
  /** 能否发送 */
  canSend: boolean
  /** 回车提示文案 */
  sendHint: string
}>()

const emit = defineEmits<{
  pickFile: []
  send: []
  stop: []
  /** 运行中发送的方式变了 */
  'update:mode': [mode: 'steer' | 'queue']
}>()

const { t } = useI18n()
const chat = useChatStore()
const session = useSessionStore()

const mode = ref<'steer' | 'queue'>('steer')
const chipsRef = ref<InstanceType<typeof SessionChips> | null>(null)

const runMode = computed({
  get: () => mode.value,
  set: (v) => {
    mode.value = v
    emit('update:mode', v)
  },
})

/** 打开某个参数 chip（模型 / 思考档）：命令 `/model`、`/thinking` 的落点。 */
function showChip(kind: 'model' | 'thinking') {
  chipsRef.value?.show(kind)
}

defineExpose({ showChip })
</script>

<template>
  <div class="composer-bar is-divided">
    <el-tooltip :content="t('chat.attachPick')" placement="top" :show-after="400">
      <button class="bar-btn" type="button" :disabled="disabled" @click="emit('pickFile')">
        <AppIcon name="folder" size="ic-xs" />
      </button>
    </el-tooltip>
    <WorkspaceChip />
    <SessionChips ref="chipsRef" />
    <ContextMeter
      :used="chat.contextUsed"
      :win="chat.contextWindow"
      :ratio="chat.contextRatio"
      :known="chat.contextKnown"
      :reserve="chat.contextReserve"
      :session-id="session.currentId || ''"
    />
    <span v-if="outsideCount" class="bar-warn">
      {{ t('chat.attachOutsideCount', { n: outsideCount }) }}
    </span>
    <span v-if="!running" class="bar-hint">{{ sendHint }}</span>
    <span class="bar-sp" />

    <el-radio-group v-if="running" v-model="runMode" size="small" :disabled="!!acting">
      <el-radio-button value="steer">{{ t('chat.steer') }}</el-radio-button>
      <el-radio-button value="queue">{{ t('chat.queue') }}</el-radio-button>
    </el-radio-group>

    <el-button
      v-if="running"
      size="small"
      type="danger"
      plain
      :loading="stopping"
      :disabled="stopping"
      @click="emit('stop')"
    >
      {{ t('chat.stop') }}
    </el-button>
    <el-button v-else size="small" type="primary" :loading="sending" :disabled="!canSend" @click="emit('send')">
      <AppIcon v-if="!sending" name="chevron-up" size="ic-xs" class="send-ic" />
      {{ t('chat.send') }}
    </el-button>
  </div>
</template>

<style scoped>
.composer-bar.is-divided {
  margin-top: var(--wb-sp-1);
  padding-top: var(--wb-sp-2);
  border-top: 1px solid var(--wb-border);
}
.composer-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3) var(--wb-sp-3);
  /* 底栏不是可复制的正文：拖选输入框里的字时扫过这排，浏览器会把 chip 与水位环
     一起框成一片淡蓝色块。只有输入框里的文字该被选中。 */
  user-select: none;
}
.bar-hint,
.bar-warn {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  white-space: nowrap;
}
.bar-warn {
  color: var(--wb-danger);
  font-weight: 600;
}
.bar-btn {
  display: grid;
  place-items: center;
  width: var(--wb-ctl-h-sm);
  height: var(--wb-ctl-h-sm);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  color: var(--wb-muted);
  flex: none;
  transition:
    background var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.bar-btn:hover:not(:disabled) {
  background: var(--wb-elev-hover);
  border-color: var(--wb-border-strong);
  color: var(--wb-ink);
  transform: scale(1.06);
}
.bar-btn:active:not(:disabled) {
  transform: scale(var(--wb-btn-scale-press));
}
.bar-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.bar-sp {
  flex: 1;
  min-width: var(--wb-sp-1);
}
.send-ic {
  margin-right: 2px;
}
@media (max-width: 1080px) {
  .bar-hint {
    display: none;
  }
}
</style>
