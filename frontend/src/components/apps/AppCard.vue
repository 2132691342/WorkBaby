<script setup lang="ts">
// 一个运行时的「应用」页里的一行：版本、可做的动作、安装进度。
// 不认识后端：能点什么由 AppRow 的 can_* 决定，动作由父组件发。
// 动手前把「会发生什么」讲清楚；没就绪的给一句为什么。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import AppProgress from '@/components/apps/AppProgress.vue'
import type { AppRow } from '@/types/api'
import type { AppProgressLog } from '@/stores/app'

const props = defineProps<{
  row: AppRow
  /** 这个运行时当前的长任务现场；没有就是 null */
  progress: AppProgressLog | null
  /** 正在跑（父组件发起的请求还没回来） */
  busy: boolean
  copied: string
}>()

const emit = defineEmits<{
  run: [action: 'install' | 'update' | 'uninstall']
  copy: [text: string]
}>()

const { t } = useI18n()
const detailsOpen = ref(false)

/** 有现场或正在跑才展开进度；跑完自动收起。 */
const showProgress = computed(() => Boolean(props.progress || props.busy))

/** 版本状态各有各的说法，绝不混用。 */
const versionState = computed<'current' | 'none'>(() => (props.row.installed ? 'current' : 'none'))

/** 内置运行时不需要用户动手，动作只对 pi 出现。 */
const actionable = computed(() => props.row.can_install || props.row.can_update || props.row.can_uninstall)
</script>

<template>
  <article class="app-card">
    <div class="app-row">
      <div class="app-ident">
        <span class="app-badge" :class="{ on: row.installed }">{{ row.app_name }}</span>
        <span class="app-ver mono">{{ row.installed ? row.app_version || '—' : t('apps.notInstalled') }}</span>
        <span v-if="versionState === 'current'" class="tag ok">{{ t('apps.ready') }}</span>
      </div>

      <div class="app-channel">
        <span v-if="row.install_note" class="ch-value">{{ row.install_note }}</span>
      </div>

      <div v-if="actionable" class="app-actions">
        <button
          v-if="!row.installed"
          class="wb-btn primary sm"
          :class="{ 'is-loading': busy }"
          :disabled="!row.can_install || busy"
          @click="emit('run', 'install')"
        >
          <AppIcon name="download" :size="13" />
          {{ t('common.install') }}
        </button>
        <template v-else>
          <button
            v-if="row.can_update"
            class="wb-btn sm"
            :class="{ 'is-loading': busy }"
            :disabled="busy"
            @click="emit('run', 'update')"
          >
            <AppIcon name="refresh" :size="13" />
            {{ t('common.update') }}
          </button>
          <button
            v-if="row.can_uninstall"
            class="wb-btn sm danger"
            :class="{ 'is-loading': busy }"
            :disabled="busy"
            @click="emit('run', 'uninstall')"
          >
            <AppIcon name="trash" :size="13" />
            {{ t('common.uninstall') }}
          </button>
        </template>
        <button class="wb-btn sm ghost" :title="t('apps.fileDetail')" @click="detailsOpen = !detailsOpen">
          <AppIcon name="terminal" :size="13" />
        </button>
      </div>
    </div>

    <!-- 落点路径：装着的才显示，没装的不给一个不存在的路径 -->
    <div v-if="row.installed || detailsOpen" class="app-facts">
      <span class="fact wide">
        <span class="fact-label">PATH</span>
        <code class="fact-value mono selectable">{{ row.app_path }}</code>
      </span>
    </div>

    <!-- 「为什么这条按钮不能点」比一个灰按钮有用得多 -->
    <p v-if="row.blocker" class="blocker">
      <AppIcon name="alert" :size="12" />
      {{ row.blocker }}
    </p>

    <AppProgress v-if="showProgress" :progress="progress" :busy="props.busy" />
  </article>
</template>

<style scoped>
/* 一行一个运行时：标识 / 说明 / 动作横向排开，竖向空间留给进度。 */
.app-card {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3) var(--wb-sp-4);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-lg);
  background: var(--wb-surface);
}

.app-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-4);
  flex-wrap: wrap;
}

.app-ident {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
  min-width: 0;
}

.app-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 9px;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-full);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--wb-mute);
}

.app-badge.on {
  color: var(--wb-brand);
  border-color: var(--wb-brand-3);
  background: var(--wb-brand-soft);
}

.app-ver {
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-5);
}

.app-channel {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
  flex: 1;
}

.ch-value {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
}

.app-actions {
  display: flex;
  gap: var(--wb-sp-2);
  flex: none;
}

.app-facts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-4);
}

.fact {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
}

.fact.wide {
  flex: 1 1 320px;
}

.fact-label {
  font-size: var(--wb-fs-3xs);
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--wb-faint);
}

.fact-value {
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-5);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mono {
  font-family: var(--wb-font-mono);
}

.blocker {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-warn);
}
</style>
