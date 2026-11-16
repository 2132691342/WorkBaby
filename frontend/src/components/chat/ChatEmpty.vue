<script setup lang="ts">
// 对话页空态：开场白 + 三条开场建议。点一下把话填进输入框（不直接发），
// 用户还能改两句再发送。从 ChatView 拆出来：那一页贴着 500 行上限。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { useAppStore } from '@/stores/app'
import { useChatStore } from '@/stores/chat'

const app = useAppStore()
const chat = useChatStore()
const { t } = useI18n()

const suggestions = computed(() => [t('chat.suggest1'), t('chat.suggest2'), t('chat.suggest3')])
function useSuggestion(text: string): void {
  chat.draftOverride = text
}
</script>

<template>
  <div class="chat-empty">
    <div class="empty-mark">
      <AppIcon name="spark" :size="26" />
    </div>
    <h2 class="empty-title">{{ t('chat.emptyTitle') }}</h2>
    <p class="empty-hint">{{ t('chat.emptyHint') }}</p>
    <p class="empty-path selectable">{{ app.workspace }}</p>
    <div class="empty-suggest">
      <p class="suggest-title">{{ t('chat.suggestTitle') }}</p>
      <div class="suggest-chips">
        <button v-for="s in suggestions" :key="s" class="suggest-chip" type="button" @click="useSuggestion(s)">
          <AppIcon name="spark" :size="12" />
          {{ s }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--wb-sp-3);
  min-height: 100%;
  padding: var(--wb-sp-10);
  text-align: center;
}

.empty-mark {
  display: grid;
  place-items: center;
  width: var(--wb-tile-lg);
  height: var(--wb-tile-lg);
  border-radius: var(--wb-r-lg);
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
  margin-bottom: var(--wb-sp-1);
}

.empty-title {
  margin: 0;
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 650;
  color: var(--wb-ink);
}

.empty-hint {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-mute);
}

.empty-path {
  margin: var(--wb-sp-2) 0 0;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.empty-suggest {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  margin-top: var(--wb-sp-5);
}

.suggest-title {
  margin: 0;
  font-size: var(--wb-fs-3xs);
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--wb-faint);
}

.suggest-chips {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: var(--wb-sp-2);
  max-width: 560px;
}

.suggest-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 7px 12px;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-full);
  background: var(--wb-raised);
  color: var(--wb-ink-5);
  font-size: var(--wb-fs-sm);
  transition:
    transform var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    background var(--wb-dur-fast) var(--wb-ease);
}

.suggest-chip:hover {
  transform: scale(var(--wb-press-hover));
  border-color: var(--wb-brand-line);
  color: var(--wb-brand);
  background: var(--wb-brand-tint);
}

.suggest-chip:active {
  transform: scale(var(--wb-press-active));
}
</style>
