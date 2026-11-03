<script setup lang="ts">
// 一个引擎在「应用」页里的一行：版本对比、三个动作、安装进度、手动命令。
// 不认识后端：能点什么由 AppRow 的 can_* 决定，动作由父组件发。
// 动手前把命令原文给用户看；没装给两种拿法；「为什么点不动」一句话说清。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import AppProgress from '@/components/apps/AppProgress.vue'
import AppManualCommands from '@/components/apps/AppManualCommands.vue'
import AppConfirm from '@/components/apps/AppConfirm.vue'
import type { AppRow } from '@/types/api'
import type { AppProgressLog } from '@/stores/app'

const props = defineProps<{
  row: AppRow
  /** 这个引擎当前的长任务现场；没有就是 undefined */
  progress?: AppProgressLog
  /** 这个引擎正在跑（父组件发起的请求还没回来） */
  busy: boolean
  copied: string
}>()

const emit = defineEmits<{
  run: [action: 'install' | 'update' | 'uninstall']
  copy: [text: string]
}>()

const { t } = useI18n()
// 折叠态属于这一条进度框自己：它刷新时不该把用户的展开状态重置。
const manualOpen = ref(false)

/** 有现场或正在跑才展开；跑完自动收起。 */
const showProgress = computed(() => Boolean(props.progress || props.busy))

/** 未知版本、落后、最新 —— 三种状态各有各的说法，绝不混用。 */
const versionState = computed<'outdated' | 'current' | 'unknown' | 'none'>(() => {
  if (!props.row.installed) return 'none'
  if (props.row.outdated) return 'outdated'
  if (props.row.latest_version) return 'current'
  return 'unknown'
})

/** 升级 / 卸载要先把命令给用户看。 */
const pendingAction = ref<'update' | 'uninstall' | ''>('')

/** 确认面板里要显示的那行命令。 */
const previewCommand = computed(() => {
  if (pendingAction.value === 'update') return props.row.update_preview ?? ''
  if (pendingAction.value === 'uninstall') return props.row.uninstall_preview ?? ''
  return ''
})

/** 卸载是一次不可逆操作，所以它要过一道确认；升级不用。 */
function askUninstall() {
  pendingAction.value = 'uninstall'
}

function askUpdate() {
  pendingAction.value = 'update'
}

function cancel() {
  pendingAction.value = ''
}

function confirm() {
  const action = pendingAction.value
  pendingAction.value = ''
  if (action) emit('run', action)
}
</script>

<template>
  <article class="app-card">
    <!-- 第一行：名字 / 版本 / 动作。四个部分共用一行，
         装了什么、能做什么一眼看完，不用展开。 -->
    <div class="app-row">
      <div class="app-ident">
        <span class="app-badge" :class="{ on: row.installed }">{{ row.app_name }}</span>
        <span class="app-ver mono">{{ row.installed ? row.version : t('apps.notInstalled') }}</span>
        <span v-if="versionState === 'outdated'" class="tag brand">{{ t('apps.updateAvailable') }}</span>
        <span v-else-if="versionState === 'unknown'" class="tag" :title="row.latest_note">
          {{ t('apps.latestUnknown') }}
        </span>
        <span v-else-if="versionState === 'current'" class="tag ok">{{ t('apps.upToDate') }}</span>
      </div>

      <div class="app-channel">
        <template v-if="row.installed">
          <span class="ch-label">{{ t('apps.channel') }}</span>
          <span class="ch-value">{{ row.channel_label }}</span>
        </template>
        <template v-else>
          <span class="ch-value ch-missing">{{ t('apps.notInstalledHint') }}</span>
        </template>
      </div>

      <div class="app-actions">
        <button
          class="wb-btn primary sm"
          :class="{ 'is-loading': busy }"
          :disabled="!row.can_install || busy"
          :title="row.install_command"
          @click="emit('run', 'install')"
        >
          <AppIcon name="download" :size="13" />
          {{ row.installed ? t('apps.reinstall') : t('common.install') }}
        </button>
        <button
          v-if="row.installed && row.update_command"
          class="wb-btn sm"
          :class="{ 'is-loading': busy }"
          :disabled="!row.can_update || busy"
          @title="row.update_preview"
          @click="askUpdate"
        >
          <AppIcon name="refresh" :size="13" />
          {{ t('common.update') }}
        </button>
        <button
          v-if="row.can_uninstall"
          class="wb-btn sm danger"
          :class="{ 'is-loading': busy }"
          :disabled="busy"
          @click="askUninstall"
        >
          <AppIcon name="trash" :size="13" />
          {{ t('common.uninstall') }}
        </button>
        <button
          class="wb-btn sm ghost"
          :title="t('apps.manualCommands')"
          @click="manualOpen = !manualOpen"
        >
          <AppIcon name="terminal" :size="13" />
        </button>
      </div>
    </div>

    <!-- 装着的：给出落点；没装着的：给出要跑的命令 -->
    <div v-if="row.installed" class="app-facts">
      <span class="fact wide">
        <span class="fact-label">{{ t('apps.location') }}</span>
        <code class="fact-value mono selectable">{{ row.exe }}</code>
      </span>
    </div>
    <div v-else class="cmd-row">
      <code class="cmd-text mono selectable">{{ row.install_command }}</code>
      <button class="wb-btn sm ghost" @click="emit('copy', row.install_command)">
        {{ copied === row.install_command ? t('settings.copied') : t('settings.copyCommand') }}
      </button>
    </div>

    <!-- 「为什么这条按钮不能点」比一个灰按钮有用得多 -->
    <p v-if="row.blocker" class="blocker">
      <AppIcon name="alert" :size="12" />
      {{ row.blocker }}
    </p>

    <!-- 动手之前的确认：把那行命令摆出来。
         用户点的是「装 / 升级 / 卸载」，但真正发生的是这条命令。 -->
    <AppConfirm
      v-if="pendingAction"
      :action="pendingAction"
      :command="previewCommand"
      @cancel="cancel"
      @confirm="confirm"
    />

    <!-- 手动命令：给不想（或不能）由界面代跑的人 -->
    <AppManualCommands v-if="manualOpen" :row="row" :copied="copied" @copy="(text) => emit('copy', text)" />

    <AppProgress v-if="showProgress" :progress="progress ?? null" :busy="props.busy" />

    <p class="app-foot">
      <a class="docs" :href="row.docs_url" target="_blank" rel="noreferrer">{{ t('apps.docs') }}</a>
    </p>
  </article>
</template>

<style scoped>
/* 一行一个引擎：四个部分（标识 / 渠道 / 动作 / 事实）横向排开，
   竖向空间留给命令、进度这些真正需要展开的内容。 */
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
.ch-label {
  font-size: var(--wb-fs-3xs);
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--wb-faint);
}
.ch-value {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-5);
}
.ch-missing {
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

.cmd-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-2) var(--wb-sp-2) var(--wb-sp-3);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r);
  background: var(--wb-code-bg);
}

.cmd-text {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-5);
  overflow-x: auto;
  white-space: nowrap;
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

/* 确认面板的样式随 AppConfirm 搬走了；
   手动命令清单与进度框也一样。 */
.app-foot {
  margin: 0;
}

.docs {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
  text-decoration: none;
}

.docs:hover {
  color: var(--wb-brand);
  text-decoration: underline;
}
</style>
