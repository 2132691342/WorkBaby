<script setup lang="ts">
// 引擎配置原文：只看不改。
// 设置页不写引擎配置——节点级接口各管各的键（模型页 models、MCP 页 mcp.json），
// 多一个写入口必有「这里改了、那里没跟着变」；这里只供对照与排错。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import CodeEditor from '@/components/common/CodeEditor.vue'
import { usePrefsStore } from '@/stores/prefs'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const prefs = usePrefsStore()
const app = useAppStore()

/** 配置原文的语法高亮按扩展名选：.yml / .yaml 走 yaml，.json 走 json。 */
const lang = computed<'json' | 'yaml' | 'text'>(() => {
  const path = prefs.configText?.file_path ?? ''
  if (path.endsWith('.json') || path.endsWith('.jsonc')) return 'json'
  if (path.endsWith('.yml') || path.endsWith('.yaml')) return 'yaml'
  return 'text'
})

/** 引擎就绪才谈得上「看它的配置」。 */
const installed = computed(() => Boolean(app.runtime?.pi_ready))
</script>

<template>
  <section v-if="installed" class="card-pad">
    <div class="wb-section-title">{{ t('settings.configTitle') }}</div>
    <p class="set-hint">{{ t('settings.configHint') }}</p>

    <div class="config-files">
      <button
        v-for="file in prefs.configFiles"
        :key="file.config_name"
        class="config-tab wb-pressable"
        :class="{ active: prefs.configText?.file_path === file.file_path }"
        :title="file.file_path"
        @click="prefs.pickConfig(file.config_name)"
      >
        {{ file.config_label }}
        <span v-if="file.file_exists" class="tab-dot" />
      </button>
    </div>

    <div v-if="prefs.configText" class="config-body">
      <p v-if="!prefs.configText.found" class="config-missing">{{ t('settings.configMissing') }}</p>
      <template v-else>
        <CodeEditor :model-value="prefs.configText.file_text" :lang="lang" readonly min-height="52vh" />
        <p v-if="prefs.configText.truncated" class="config-warn">{{ t('settings.configTruncated') }}</p>
      </template>
    </div>
  </section>
</template>

<style scoped>
.config-files {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-1);
  margin: var(--wb-sp-2) 0;
}

.config-tab {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px var(--wb-sp-2);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-sm);
  background: var(--wb-surface);
  color: var(--wb-ink-5);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  cursor: pointer;
  transition:
    border-color var(--wb-dur-fast) var(--wb-ease),
    background-color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.config-tab:hover {
  border-color: var(--wb-brand-3);
  transform: scale(1.02);
}

.config-tab:active {
  transform: scale(0.97);
}

.config-tab.active {
  border-color: var(--wb-brand);
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
}

/* 已生成的文件给一个点：省得一个个点开才发现是空的 */
.tab-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--wb-ok);
}

.config-body {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}

.config-missing,
.config-warn {
  margin: 0;
  padding: var(--wb-sp-3);
  border-radius: var(--wb-r-sm);
  background: var(--wb-hover);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
}

.config-warn {
  color: var(--wb-warn);
  background: var(--wb-warn-soft);
}
</style>
