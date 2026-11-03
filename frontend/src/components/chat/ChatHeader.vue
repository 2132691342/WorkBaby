<script setup lang="ts">
// 对话页的页头与授权链接条：页头回答「在跟谁说话」，链接条回答
// 「引擎要我去浏览器点一下，地址在这儿」。两者都只读状态不持有状态。
// 消息参数（工作区 / 模型 / 水位）在输入框底栏，页头只留引擎与摘要。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { useAppStore } from '@/stores/app'
import { useChatStore } from '@/stores/chat'
import { openExternal } from '@/api/shell'

const app = useAppStore()
const chat = useChatStore()
const { t } = useI18n()

/** 当前这一行：服务商 · 模型 · 速率。 */
const modelLine = computed(() => {
  if (!chat.model) return ''
  const parts = [chat.model.provider, chat.model.id]
  if (chat.tokensPerSecond) parts.push(`${chat.tokensPerSecond.toFixed(1)} tok/s`)
  return parts.join(' · ')
})

/**
 * 引擎要求浏览器完成授权时，把地址挂在这里给用户复制。
 *
 * 不自动打开：默认浏览器可能打不开，而用户看不到原文就无从补救。
 */
async function copyLink(url: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(url)
    app.pendingLink = null
  } catch {
    // 剪贴板不可用时保留这一条，用户仍能手动选中复制
  }
}

async function openLink(url: string): Promise<void> {
  try {
    await openExternal(url)
  } finally {
    app.pendingLink = null
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
        class="wb-btn icon ghost"
        :class="{ 'is-loading': chat.compacting }"
        :title="t('chat.compact')"
        :disabled="chat.busy || !app.hasEngine"
        @click="chat.compact()"
      >
        <AppIcon name="refresh" :size="15" />
      </button>
    </div>
  </header>

  <div v-if="app.pendingLink" class="link-strip">
    <div class="link-text">
      <strong>{{ t('chat.openLinkTitle') }}</strong>
      <span class="link-hint">{{ app.pendingLink.instructions || t('chat.openLinkHint') }}</span>
      <code class="link-url selectable">{{ app.pendingLink.url }}</code>
    </div>
    <div class="link-actions">
      <button class="wb-btn sm ghost" @click="copyLink(app.pendingLink.url)">
        {{ t('chat.openLinkCopy') }}
      </button>
      <button class="wb-btn primary sm" @click="openLink(app.pendingLink.url)">
        {{ t('chat.openLinkBrowser') }}
      </button>
      <button class="wb-btn sm ghost" @click="app.pendingLink = null">
        {{ t('chat.openLinkDismiss') }}
      </button>
    </div>
  </div>
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

/* 授权链接条：贴在页头下方，不遮消息。用清单底的淡强调色，
   它是一条「请去别处办一件事」的指引，不是错误。 */
.link-strip {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
  padding: var(--wb-sp-2) var(--wb-sp-5);
  border-bottom: 1px solid var(--wb-line);
  background: var(--wb-brand-soft);
  flex: none;
}

.link-text {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
  min-width: 0;
  flex: 1;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink);
}

.link-hint {
  color: var(--wb-mute);
}

.link-url {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  padding: 2px 6px;
  border-radius: var(--wb-r-xs);
  background: var(--wb-surface);
  border: 1px solid var(--wb-line);
  overflow-wrap: anywhere;
}

.link-actions {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: none;
}
</style>
