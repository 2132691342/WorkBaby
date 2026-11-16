<script setup lang="ts">
// 对话页的页头与授权链接条：页头回答「在跟谁说话」，链接条回答
// 「引擎要我去浏览器点一下，地址在这儿」。两者都只读状态不持有状态。
// 消息参数（工作区 / 模型 / 水位）在输入框底栏，页头只留引擎与摘要。
import { computed, ref } from 'vue'
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
    // 压缩是「重写上下文」这种看不见结果的动作：不说一句，用户只知道转了个圈。
    toasts.push('ok', t('chat.compactDone'))
  } catch (err) {
    toasts.push('error', err instanceof Error && err.message ? err.message : t('chat.compactFailed'))
  }
}

/** 计划模式是否可用：引擎把内置斜杠命令清单报上来。 */
const planAvailable = computed(() => chat.commands.some((c) => c.command_name === 'plan'))

// 切换计划模式走专用接口：拦截发生在工具注册表上，本地说反了会出现
// 「界面说只读、实际上还能写」。回执带被拦下的工具名，横幅据此说明后果。
const planBusy = ref(false)
async function togglePlan(): Promise<void> {
  if (planBusy.value) return
  planBusy.value = true
  try {
    const state = await chat.setPlanMode(!chat.planMode)
    if (state.plan_enabled) {
      const names = state.blocked_tools.join('、')
      toasts.push('info', names ? t('chat.planOnWith', { tools: names }) : t('chat.planOn'))
    } else {
      toasts.push('ok', t('chat.planOff'))
    }
  } catch (err) {
    toasts.push('error', err instanceof Error && err.message ? err.message : t('chat.planFailed'))
  } finally {
    planBusy.value = false
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
        :class="[{ 'is-loading': planBusy }, chat.planMode ? 'primary' : 'ghost']"
        :title="chat.planMode ? t('chat.planOff') : t('chat.planOn')"
        :aria-pressed="chat.planMode"
        :disabled="chat.busy || planBusy || !app.hasEngine"
        @click="togglePlan"
      >
        {{ chat.planMode ? t('chat.planActive') : t('chat.planMode') }}
      </button>
      <button
        class="wb-btn sm ghost"
        :class="{ 'is-loading': chat.compacting }"
        :title="t('chat.compactHint')"
        :disabled="chat.busy || !app.hasEngine"
        @click="compact"
      >
        <AppIcon name="refresh" :size="14" />
        {{ t('chat.compact') }}
      </button>
    </div>

    <!-- 计划模式横幅：说明「现在只能读」，并列出被拦下的工具。
         没有它，用户只知道按钮亮了，不知道写文件会失败。 -->
    <div v-if="chat.planMode" class="plan-banner">
      <AppIcon name="eye" :size="14" class="plan-ic" />
      <span class="plan-text">{{ t('chat.planBanner') }}</span>
      <span v-if="chat.blockedTools.length" class="plan-tools" :title="chat.blockedTools.join('、')">
        {{ t('chat.planBannerBlocked', { tools: chat.blockedTools.join('、') }) }}
      </span>
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
  /* 计划模式横幅出现时标题行整体下移：header 允许换行，
     否则横幅会把「对话」标题与按钮挤成一行。 */
  flex-wrap: wrap;
}

/* 计划模式横幅：淡底一行，说明「现在只能读」并列出被拦下的工具。
   占满整行——它是这一页当前所处状态的宣告，不是一条可忽略的提示。 */
.plan-banner {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-basis: 100%;
  padding: var(--wb-sp-1) var(--wb-sp-2);
  border-radius: var(--wb-r-sm);
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
  font-size: var(--wb-fs-2xs);
  min-width: 0;
  animation: plan-in var(--wb-dur) var(--wb-ease);
}

@keyframes plan-in {
  from {
    opacity: 0;
    transform: translateY(-3px);
  }
}

.plan-ic {
  flex: none;
}

.plan-text {
  flex: none;
  font-weight: 600;
}

.plan-tools {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--wb-font-mono);
  color: var(--wb-mute);
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
