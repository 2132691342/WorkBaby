<script setup lang="ts">
// 「应用」页里的一行：一个外部依赖的检测事实。
// 这里只展示与复制，不提供安装——壳不代装用户的环境。
// 没检测到时给的是「该去装什么」，不是一按钮点下去失败。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { AppRow } from '@/types/api'
import type { AppProgressLog } from '@/stores/app'

const props = defineProps<{
  row: AppRow
  /** 这个应用当前的长任务现场；没有就是 null */
  progress: AppProgressLog | null
  /** 正在检测 */
  busy: boolean
  copied: string
}>()

const emit = defineEmits<{
  copy: [text: string]
}>()

const { t } = useI18n()
const detailsOpen = ref(false)

/** 入口路径过长时中间省略，别把卡片撑破。 */
const shortPath = computed(() => {
  const path = props.row.app_path
  if (path.length <= 64) return path
  return `${path.slice(0, 28)}…${path.slice(-32)}`
})
</script>

<template>
  <article class="app-card" :class="{ missing: !row.installed }">
    <div class="app-row">
      <div class="app-ident">
        <span class="app-badge" :class="{ on: row.installed }">{{ row.app_name }}</span>
        <span class="app-ver mono">{{ row.installed ? row.app_version || '—' : t('apps.notDetected') }}</span>
        <span v-if="row.installed" class="tag ok">{{ t('apps.ready') }}</span>
      </div>

      <div class="app-channel">
        <button v-if="row.app_path" class="path-btn mono" :title="row.app_path" @click="emit('copy', row.app_path)">
          {{ shortPath }}
          <AppIcon :name="copied === row.app_path ? 'check' : 'copy'" :size="12" />
        </button>
        <span v-else class="ch-value">{{ row.install_note || t('apps.notDetected') }}</span>
      </div>

      <div class="app-actions">
        <span v-if="row.app_source && row.app_source !== 'none'" class="src mono">{{ row.app_source }}</span>
      </div>
    </div>

    <p v-if="row.blocker" class="app-blocker">
      <AppIcon name="alert" :size="13" />
      {{ row.blocker }}
    </p>
    <p v-else-if="!row.installed && row.install_note" class="app-hint">{{ row.install_note }}</p>

    <details v-if="row.install_note" class="app-details" :open="detailsOpen" @toggle="detailsOpen = !detailsOpen">
      <summary>{{ t('apps.howTo') }}</summary>
      <p class="app-note">{{ row.install_note }}</p>
    </details>

    <div v-if="progress || busy" class="app-progress">
      <span class="spin" v-if="busy || progress?.phase === 'start' || progress?.phase === 'line'" />
      <span class="mono">{{ progress?.message || progress?.lines?.[progress.lines.length - 1] || t('apps.detecting') }}</span>
    </div>
  </article>
</template>

<style scoped>
.app-card {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3) var(--wb-sp-4);
  background: var(--wb-surface);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-lg);
  box-shadow: var(--wb-card-shadow);
  transition:
    border-color var(--wb-dur) var(--wb-ease),
    box-shadow var(--wb-dur) var(--wb-ease);
}

.app-card:hover {
  border-color: var(--wb-line-strong);
  box-shadow: var(--wb-card-shadow-hover);
}

.app-card.missing {
  border-color: color-mix(in srgb, var(--wb-bad) 40%, var(--wb-line));
}

.app-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
}

.app-ident {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: calc(var(--wb-w-input-sm) * 3);
}

.app-badge {
  font-weight: 600;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}

.app-badge.on {
  color: var(--wb-brand-strong);
}

.app-ver {
  font-size: var(--wb-fs-xs);
  color: var(--wb-mute);
}

.app-channel {
  flex: 1;
  min-width: 0;
}

.path-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  max-width: 100%;
  padding: 2px 6px;
  border: 1px solid transparent;
  border-radius: var(--wb-r-sm);
  background: transparent;
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-xs);
  cursor: pointer;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease);
}

.path-btn:hover {
  background: var(--wb-hover);
  border-color: var(--wb-line);
}

.path-btn:active {
  transform: scale(var(--wb-press-active));
}

.ch-value {
  font-size: var(--wb-fs-xs);
  color: var(--wb-mute);
}

.app-actions {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}

.src {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  padding: 2px 6px;
  border-radius: var(--wb-r-xs);
  background: var(--wb-tint);
}

.app-blocker {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  margin: 0;
  font-size: var(--wb-fs-xs);
  color: var(--wb-bad);
}

.app-hint {
  margin: 0;
  font-size: var(--wb-fs-xs);
  color: var(--wb-mute);
}

.app-details {
  font-size: var(--wb-fs-xs);
  color: var(--wb-mute);
}

.app-details summary {
  cursor: pointer;
  user-select: none;
}

.app-note {
  margin: var(--wb-sp-1) 0 0;
  line-height: 1.7;
  color: var(--wb-ink-2);
}

.app-progress {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-xs);
  color: var(--wb-mute);
}

.spin {
  width: 10px;
  height: 10px;
  border: 2px solid var(--wb-brand-line);
  border-top-color: var(--wb-brand);
  border-radius: var(--wb-r-full);
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
