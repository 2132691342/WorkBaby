<script setup lang="ts">
// 对话页的页头与授权链接条：页头回答「在跟谁说话」，链接条回答
// 「引擎要我去浏览器点一下，地址在这儿」。两者都只读状态不持有状态。
// 消息参数（工作区 / 模型 / 水位）在输入框底栏，页头只留引擎与摘要。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { useAppStore } from '@/stores/app'
import { useChatStore } from '@/stores/chat'
import { useToasts } from '@/composables/useToasts'

const app = useAppStore()
const chat = useChatStore()
const toasts = useToasts()
const { t } = useI18n()

/** 当前这一行：服务商 · 模型 · 速率。 */
const modelLine = computed(() => {
  if (!chat.model) return ''
  const parts = [chat.model.provider_name, chat.model.model_id]
  return parts.join(' · ')
})

/** 压缩失败必须出声：store 里是 rethrow 语义，静默会让按钮看起来像坏了。 */
async function compact(): Promise<void> {
  try {
    await chat.compact()
  } catch (err) {
    toasts.push('error', err instanceof Error && err.message ? err.message : t('chat.compactFailed'))
  }
}

/** 计划模式是否可用：引擎把内置斜杠命令清单报上来。 */
const planAvailable = computed(() => chat.commands.some((c) => c.command_name === 'plan'))

// 切换计划模式走引擎的 /plan 命令，不本地记开关：拦截发生在工具注册表上，
// 本地说反了会出现「界面说只读、实际上还能写」。
async function togglePlan(): Promise<void> {
  try {
    await chat.send('/plan')
  } catch (err) {
    toasts.push('error', err instanceof Error && err.message ? err.message : t('chat.planFailed'))
  }
}

</script>

<template>
  <header class="chat-head">
    <div class="chat-head-left">
      <h1 class="chat-title">{{ t('nav.chat') }}</h1>
      <span v-if="modelLine" class="chat-sub">{{ modelLine }}</span>
    </div>
    <div class="chat-head-right">
      <button
        v-if="planAvailable"
        class="wb-btn sm"
        :class="chat.planMode ? 'primary' : 'ghost'"
        :title="chat.planMode ? t('chat.planOff') : t('chat.planOn')"
        :disabled="chat.busy || !app.hasEngine"
        @click="togglePlan"
      >
        {{ chat.planMode ? t('chat.planActive') : t('chat.planMode') }}
      </button>
      <button
        class="wb-btn icon ghost"
        :class="{ 'is-loading': chat.compacting }"
        :title="t('chat.compact')"
        :disabled="chat.busy || !app.hasEngine"
        @click="compact"
      >
        <AppIcon name="refresh" :size="15" />
      </button>
    </div>
  </header>

</template>

<style scoped>
.chat-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-3) var(--wb-sp-5);
  border-bottom: 1px solid var(--wb-line);
  flex: none;
}

.chat-head-left {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-3);
  min-width: 0;
}

.chat-title {
  margin: 0;
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-lg);
  font-weight: 650;
  color: var(--wb-ink);
  flex: none;
}

.chat-sub {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chat-head-right {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: none;
}

</style>
