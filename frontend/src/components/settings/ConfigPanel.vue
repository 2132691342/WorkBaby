<script setup lang="ts">
// 引擎配置原文：只看不改。
// 设置页不写引擎配置——节点级接口各管各的键（模型页 models、MCP 页 mcp.json），
// 多一个写入口必有「这里改了、那里没跟着变」；这里只供对照与排错。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import CodeEditor from '@/components/common/CodeEditor.vue'
import { usePrefsStore } from '@/stores/prefs'
import { useAppStore } from '@/stores/app'
import type { EngineKind } from '@/types/api'

const props = defineProps<{
  /** 当前看哪个引擎的配置 */
  kind: EngineKind
}>()

const emit = defineEmits<{
  pick: [kind: EngineKind]
}>()

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

/** 有多个引擎时才给切换器：只有一个时它只会占地方。 */
const showPicker = computed(() => app.installed.length > 1)
</script>

<template>
  <section v-if="app.installed.length" class="card-pad">
    <div class="wb-section-title">
      {{ t('settings.configTitle') }}
      <span class="push" />
      <div v-if="showPicker" class="seg">
        <button
          v-for="item in app.installed"
          :key="item.kind"
          class="seg-btn"
          :class="{ active: props.kind === item.kind }"
          @click="emit('pick', item.kind)"
        >
          {{ item.kind }}
        </button>
      </div>
    </div>
    <p class="set-hint">{{ t('settings.configHint') }}</p>

    <div class="config-files">
      <button
        v-for="file in prefs.configFiles"
        :key="file.file_path"
        class="config-tab wb-pressable"
        :class="{ active: prefs.configFile === file.file_path, missing: !file.exists }"
        :title="file.file_path"
        @click="prefs.pickConfig(file.file_path)"
      >
        {{ file.file_label }}
      </button>
    </div>

    <div v-if="prefs.loadingText" class="set-hint">{{ t('common.loading') }}</div>
    <div v-else-if="prefs.configText && !prefs.configText.found" class="set-hint">
      {{ t('settings.configMissing') }}
      <span class="mono selectable"> · {{ prefs.configText.file_path }}</span>
    </div>
    <template v-else-if="prefs.configText">
      <div class="set-hint mono selectable config-path">{{ prefs.configText.file_path }}</div>
      <CodeEditor :model-value="prefs.configText.text" :lang="lang" readonly min-height="320px" />
      <div v-if="prefs.configText.truncated" class="set-hint">
        {{ t('settings.configTruncated') }}
      </div>
    </template>
  </section>
</template>

<style scoped>
.push {
  margin-left: auto;
}

.set-hint {
  margin: 0 0 var(--wb-sp-2);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.config-path {
  overflow-wrap: anywhere;
}

.config-files {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-2);
  margin-bottom: var(--wb-sp-3);
}

.config-tab {
  padding: 3px 9px;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-full);
  background: var(--wb-surface);
  color: var(--wb-mute);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  cursor: pointer;
}

.config-tab.active {
  border-color: var(--wb-brand);
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
}

/* 文件不存在时不显示成「选中的那个」，否则会以为内容为空 */
.config-tab.missing {
  border-style: dashed;
  color: var(--wb-faint);
}
</style>
