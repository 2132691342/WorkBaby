<script setup lang="ts">
// 环境：每个引擎一张卡——探测结果、安装命令、显式路径、落盘布局，加 profile。
// 只探测不安装：缺引擎给「命令原文 + 复制 + 去应用页」，
// 我们只代跑引擎自己的安装器，不自建分发源。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'
import type { EngineInfo } from '@/types/api'

const props = defineProps<{
  /** 探测到的引擎清单（含没装的，界面要显示「缺什么」） */
  engines: EngineInfo[]
  detecting: boolean
  /** 正在重启的引擎名（空表示没有） */
  restarting: string
  /** 正在保存的引擎路径键（空表示没有） */
  pathSaving: string
}>()

const emit = defineEmits<{
  redetect: []
  restart: [kind: string]
  savePath: [kind: string, value: string]
  saveProfile: [name: string]
  reveal: [path: string]
}>()

const { t } = useI18n()
const app = useAppStore()
const toasts = useToasts()

const profileDraft = ref(app.profile)
const pathDrafts = ref<Record<string, string>>({})
const copiedCmd = ref('')

/** 已装的引擎数，显示在标题上。 */
const installedCount = computed(() => props.engines.filter((e) => e.installed).length)

/**
 * 布局里要显示的七项，键必须与 EngineInfo.layout 的字段同名。
 *
 * 用 as const 让 key 收窄成字面量联合，item.layout[key] 才能取到具体字段——
 * 否则它是 string 索引，TS 会拒绝在一个已知结构上做宽索引。
 */
const pathFields = computed(() => [
  { key: 'root', label: t('settings.layoutRoot') },
  { key: 'agent', label: t('settings.layoutAgent') },
  { key: 'sessions', label: t('settings.layoutSessions') },
  { key: 'skills', label: t('settings.layoutSkills') },
  { key: 'plugins', label: t('settings.layoutPlugins') },
  { key: 'models', label: t('settings.layoutModels') },
  { key: 'settings', label: t('settings.layoutSettings') },
] as const)

function exeDraft(kind: string): string {
  return pathDrafts.value[kind] ?? ''
}

function setExeDraft(kind: string, value: string): void {
  pathDrafts.value = { ...pathDrafts.value, [kind]: value }
}

/**
 * 复制命令原文。
 *
 * 成功要给一次反馈：命令已经很长了，用户点了复制后如果界面毫无变化，
 * 他没法确认到底复制上没有。失败不弹——命令原文就在旁边，看得见。
 */
async function copyCommand(cmd: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(cmd)
    copiedCmd.value = cmd
    toasts.push('ok', t('settings.copied'))
    window.setTimeout(() => {
      if (copiedCmd.value === cmd) copiedCmd.value = ''
    }, 2400)
  } catch {
    copiedCmd.value = ''
  }
}
</script>

<template>
  <section class="card-pad">
    <div class="wb-section-title">
      {{ t('settings.groups.engine') }}
      <span class="tag" :class="installedCount ? 'ok' : 'warn'">
        {{ installedCount }}/{{ props.engines.length }}
      </span>
      <button
        class="wb-btn text sm push"
        :class="{ 'is-loading': props.detecting }"
        :disabled="props.detecting"
        @click="emit('redetect')"
      >
        {{ t('settings.redetect') }}
      </button>
    </div>

    <article v-for="item in props.engines" :key="item.kind" class="engine-card">
      <div class="engine-head">
        <span class="dot" :class="item.installed ? 'ok' : 'warn'" />
        <span class="engine-name">{{ item.kind }}</span>
        <span class="tag" :class="item.installed ? 'ok' : 'warn'">
          {{ item.installed ? item.version : t('settings.engineSource.none') }}
        </span>
        <span v-if="item.installed" class="tag">{{ t(`settings.engineSource.${item.source}`) }}</span>
        <span v-if="item.installed && item.channel" class="tag">
          {{ t(`settings.channel.${item.channel}`) }}
        </span>
        <span class="push" />
        <button
          class="wb-btn sm"
          :class="{ 'is-loading': props.restarting === item.kind }"
          :disabled="!item.installed || Boolean(props.restarting)"
          @click="emit('restart', item.kind)"
        >
          {{ t('settings.restartEngine') }}
        </button>
      </div>

      <!-- 缺引擎：给命令原文与两个出口，不代跑 -->
      <div v-if="!item.installed" class="cmd-row">
        <code class="cmd-text selectable">{{ app.installHint(item.kind) }}</code>
        <button class="wb-btn sm ghost" @click="copyCommand(app.installHint(item.kind))">
          {{ copiedCmd === app.installHint(item.kind) ? t('settings.copied') : t('settings.copyCommand') }}
        </button>
        <button class="wb-btn sm" @click="app.openApps()">{{ t('settings.goApps') }}</button>
      </div>

      <div v-else class="engine-facts">
        <div class="fact wide">
          <span class="fact-label">{{ t('settings.engineExe') }}</span>
          <span class="fact-value mono selectable">{{ item.exe }}</span>
        </div>
      </div>

      <div class="set-row top compact">
        <div>
          <div class="set-label">{{ t('settings.engineExe') }}</div>
          <div class="set-hint">{{ t('settings.engineExeHint') }}</div>
        </div>
        <div class="inline-input">
          <input
            class="wb-input"
            :value="exeDraft(item.kind)"
            :placeholder="item.exe || t('settings.engineExePlaceholder')"
            @input="setExeDraft(item.kind, ($event.target as HTMLInputElement).value)"
          />
          <button
            class="wb-btn"
            :class="{ 'is-loading': props.pathSaving === `exe:${item.kind}` }"
            :disabled="props.pathSaving === `exe:${item.kind}`"
            @click="emit('savePath', item.kind, exeDraft(item.kind))"
          >
            {{ t('settings.apply') }}
          </button>
        </div>
      </div>

      <div v-if="item.layout" class="layout-grid">
        <div v-for="field in pathFields" :key="field.key" class="layout-row">
          <span class="layout-label">{{ field.label }}</span>
          <button
            class="layout-path mono selectable"
            :title="t('settings.reveal')"
            @click="emit('reveal', item.layout[field.key] || '')"
          >
            {{ item.layout[field.key] || '—' }}
          </button>
        </div>
      </div>
    </article>

    <!-- profile 只有 omp 认：pi 没有 profile 概念 -->
    <div class="set-row top">
      <div>
        <div class="set-label">{{ t('settings.profile') }}</div>
        <div class="set-hint">{{ t('settings.profileHint') }}</div>
      </div>
      <div class="inline-input">
        <input v-model="profileDraft" class="wb-input" placeholder="default" />
        <button class="wb-btn" @click="emit('saveProfile', profileDraft)">
          {{ t('common.save') }}
        </button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.engine-card {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r);
  margin-bottom: var(--wb-sp-3);
}

.engine-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
}

.push {
  margin-left: auto;
}

.engine-name {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
}

.engine-facts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--wb-sp-2);
}

.fact {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.fact.wide {
  grid-column: 1 / -1;
}

.fact-label {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.fact-value {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  overflow-wrap: anywhere;
}

.cmd-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
}

.cmd-text {
  flex: 1;
  min-width: 0;
  padding: var(--wb-sp-2);
  border-radius: var(--wb-r-sm);
  background: var(--wb-code-bg);
  border: 1px solid var(--wb-line);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-ink-2);
  overflow-wrap: anywhere;
}

.set-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-2) 0;
}

.set-row.top {
  align-items: flex-start;
}

.set-row.compact {
  padding-top: 0;
  border-top: 0;
}

.set-label {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}

.set-hint {
  margin-top: 2px;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  max-width: var(--wb-w-desc);
}

.inline-input {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: none;
}

.inline-input .wb-input {
  width: 300px;
}

.layout-grid {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.layout-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: 2px 0;
}

.layout-label {
  width: 96px;
  flex: none;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.layout-path {
  flex: 1;
  min-width: 0;
  border: 0;
  background: transparent;
  text-align: left;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}

.layout-path:hover {
  color: var(--wb-brand);
}
</style>
