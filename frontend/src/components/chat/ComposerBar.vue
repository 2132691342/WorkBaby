<script setup lang="ts">
// 输入框底栏一行：左到右是「这句话的参数」（工作目录 → 引擎 → 模型 → 思考档）、
// 两个低频开关（附件、自动压缩）、行末读数与动作（水位 …… 排队 / 停止 / 发送）。
// 参数与动作是同一句话的属性，所以一行装下；新对话属于会话列表，不在这行。
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import ContextMeter from '@/components/chat/ContextMeter.vue'
import EngineChip from '@/components/chat/EngineChip.vue'
import ModelPicker from '@/components/chat/ModelPicker.vue'
import ThinkingChip from '@/components/chat/ThinkingChip.vue'
import WorkspaceChip from '@/components/chat/WorkspaceChip.vue'
import { useChatStore } from '@/stores/chat'
import { useAppStore } from '@/stores/app'

const props = defineProps<{
  /** 有没有内容可发（正文或附件） */
  canSend: boolean
  sending: boolean
  queueing: boolean
  stopping: boolean
  /** 正在执行的内置命令（为空表示空闲） */
  busyCmd: string
  /** 当前模型收不收图片 */
  imagesAllowed: boolean
  /** 字符计数（0 时不显示；直播中也不显示——行末那几位要留给动作） */
  count: number
}>()

const emit = defineEmits<{
  browseFiles: []
  queue: []
  toggleCompact: [enabled: boolean]
  stop: []
  submit: []
}>()

const chat = useChatStore()
const app = useAppStore()
const { t } = useI18n()
</script>

<template>
  <div class="composer-bar">
    <div class="bar-row">
      <!-- 参数组：允许被裁剪的一侧。窗口窄了先收参数，
           「排队 / 停止 / 发送」永远在——动作被顶出去就是按钮消失。 -->
      <div class="bar-params">
        <WorkspaceChip />
        <EngineChip />
        <ModelPicker />
        <ThinkingChip />

        <button
          class="wb-btn icon ghost"
          :title="props.imagesAllowed ? t('chat.attachPick') : t('chat.attachNoImageModel')"
          :disabled="!app.hasEngine || !props.imagesAllowed"
          @click="emit('browseFiles')"
        >
          <AppIcon name="file" :size="14" />
        </button>

        <!-- 自动压缩是个开关：图标不能与刷新共用，否则「压缩开着」和
             「刷新一下」两个完全不同的动作长得一样。开着时走 .wb-btn.on 底色，
             文字也换成「自动压缩」——纯图标开关的状态只能靠悬停猜。 -->
        <button
          class="wb-btn sm ghost compact-btn"
          :class="{ on: chat.autoCompaction, 'is-loading': chat.switching === 'compaction' }"
          :disabled="chat.busy || chat.switching === 'compaction' || !app.hasEngine"
          :title="chat.autoCompaction ? t('chat.autoCompactOn') : t('chat.autoCompactOff')"
          :aria-pressed="chat.autoCompaction"
          @click="emit('toggleCompact', !chat.autoCompaction)"
        >
          <AppIcon name="dice" :size="13" />
          <span class="compact-label">{{ t('chat.autoCompact') }}</span>
        </button>

        <span v-if="props.count && !chat.streaming" class="composer-hint">{{ props.count }}</span>
      </div>

      <div class="bar-acts">
        <ContextMeter />

        <button
          v-if="chat.streaming"
          class="wb-btn sm ghost"
          :class="{ 'is-loading': props.queueing }"
          :disabled="!props.canSend || props.queueing"
          :title="t('chat.queueHint')"
          @click="emit('queue')"
        >
          <AppIcon name="clock" :size="13" />
          {{ t('chat.queue') }}
        </button>

        <button
          v-if="chat.streaming"
          class="wb-btn sm"
          :class="{ 'is-loading': props.stopping }"
          :disabled="props.stopping"
          :title="t('chat.stop')"
          @click="emit('stop')"
        >
          <AppIcon name="stop" :size="13" />
          {{ t('chat.stop') }}
        </button>

        <!-- 发送键在 sending 期间也保持可点，点它等于「喊停」。
             等首字的那几秒里，停止是用户唯一的出路；禁用它等于
             让人盯着一个按不动的按钮，直到引擎自己出错。
             注意：这一态**不能**挂 .is-loading——那个类带 pointer-events: none，
             转圈的同时也把「停下」这一下吃掉了。忙碌的内置命令才转圈。 -->
        <button
          class="wb-btn primary sm"
          :class="{ 'is-loading': !props.sending && props.busyCmd !== '' }"
          :disabled="(!props.canSend && !props.sending) || !app.hasEngine"
          :title="props.sending ? t('chat.stop') : undefined"
          @click="props.sending ? emit('stop') : emit('submit')"
        >
          <AppIcon :name="props.sending ? 'stop' : 'send'" :size="13" />
          {{ props.sending ? t('chat.stop') : chat.streaming ? t('chat.sendSteer') : t('chat.send') }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.composer-bar {
  min-width: 0;
}

/* 一行两段：参数组可收缩（内部裁剪），动作组恒定可见。
   不换行——换行会让「发送」按钮的位置随内容长短漂移。 */
.bar-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
}

/* 参数组：吃掉剩余宽度；装不下时由各芯片按自己的省略号收缩。
   不给这组加 overflow: hidden——芯片的下拉面板就渲染在组里，
   裁剪会把弹出层一起裁掉（表现为「点了没反应」）。 */
.bar-params {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: 1 1 auto;
  min-width: 0;
}

.bar-params > * {
  flex: 0 1 auto;
  min-width: 0;
}

.bar-params > .wb-btn {
  flex: none;
}

/* 动作组：不参与收缩，始终完整可见 */
.bar-acts {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: none;
}

.composer-hint {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  font-family: var(--wb-font-mono);
  font-variant-numeric: tabular-nums;
}

/* 窄档下这一排参数先被压缩：标签收掉，图标还在（悬停有 title） */
.compact-btn {
  flex: none;
}

@container app (max-width: 1180px) {
  .compact-label {
    display: none;
  }
}
</style>
