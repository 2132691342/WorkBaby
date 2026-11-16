<script setup lang="ts">
// 对话页在没有消息可画时的三块落地区：缺引擎 / 正在切会话 / 打不开这一条。
// 它们共用同一副居中骨架（图标 + 一句话 + 一条出去的路），所以收在一个组件里；
// 空态（新对话的开场建议）留在 ChatView，那是「可以开始」而不是「出了问题」。
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import WbSkeleton from '@/components/common/WbSkeleton.vue'

defineProps<{
  /** 引擎没就绪：连输入框都发不出去 */
  noEngine: boolean
  /** 正在从后端投影一条会话 */
  loading: boolean
  /** 打开失败的原话（空串表示没有错误） */
  error: string
  /** 还没选工作目录：此时能聊，但助手没法读写文件——先说清这件事 */
  noWorkspace: boolean
}>()

const emit = defineEmits<{ openLatest: []; openSettings: []; pickWorkspace: [] }>()

const { t } = useI18n()
</script>

<template>
  <div v-if="loading" class="gate-loading">
    <WbSkeleton :rows="5" />
  </div>

  <!-- 缺工作目录优先于缺引擎：没目录时连「在哪儿干活」都没定 -->
  <div v-else-if="noWorkspace" class="gate">
    <AppIcon name="folder" :size="22" class="gate-ic gate-ic--brand" />
    <p class="gate-title">{{ t('chat.noWorkspaceTitle') }}</p>
    <p class="gate-hint">{{ t('chat.noWorkspace') }}</p>
    <button class="wb-btn primary" type="button" @click="emit('pickWorkspace')">{{ t('chat.pickWorkspace') }}</button>
  </div>

  <div v-else-if="noEngine" class="gate">
    <p class="gate-title">{{ t('chat.noEngine') }}</p>
    <button class="wb-btn primary" type="button" @click="emit('openSettings')">{{ t('nav.settings') }}</button>
  </div>

  <div v-else-if="error" class="gate">
    <AppIcon name="alert" :size="22" class="gate-ic" />
    <p class="gate-title">{{ t('chat.sessionOpenFailed') }}</p>
    <p class="gate-detail selectable">{{ error }}</p>
    <button class="wb-btn" type="button" @click="emit('openLatest')">{{ t('chat.openLatest') }}</button>
  </div>
</template>

<style scoped>
.gate-loading {
  max-width: var(--wb-col);
  margin: 0 auto;
  padding: var(--wb-sp-6) var(--wb-sp-5);
}

.gate {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--wb-sp-3);
  min-height: 100%;
  padding: var(--wb-sp-10);
  text-align: center;
}

.gate-ic {
  color: var(--wb-warn);
}

.gate-ic--brand {
  color: var(--wb-brand);
}

.gate-hint {
  margin: 0;
  max-width: 560px;
  font-size: var(--wb-fs-2xs);
  line-height: 1.7;
  color: var(--wb-faint);
}

.gate-title {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-mute);
}

/* 失败原话用等宽字：里面常带路径与错误码，用户要能一眼复制出去 */
.gate-detail {
  margin: 0;
  max-width: 560px;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-mute);
  overflow-wrap: anywhere;
}
</style>
