<script setup lang="ts">
// 环境：外部工具链（git / node / python）的状态卡——就绪情况、版本、落点路径、重探。
// 引擎在进程内，没有「装没装」这件事，所以这张卡只管外部工具链。
// 只探测不安装：缺了给「去应用页」，装不装由用户决定。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'
import type { AppRow, EngineLayout, RuntimeStatus } from '@/types/api'

const props = defineProps<{
  /** 运行时状态 */
  runtime: RuntimeStatus | null
  /** 应用页的行：装没装、版本、路径都在这里 */
  apps: AppRow[]
  /** 引擎落盘布局；解不出来时为 undefined */
  layout?: EngineLayout
  detecting: boolean
}>()

const emit = defineEmits<{
  redetect: []
  reveal: [path: string]
}>()

const { t } = useI18n()
const app = useAppStore()
const toasts = useToasts()
const copiedCmd = ref('')

/** 已就绪的运行时的数量，显示在标题上。 */
const readyCount = computed(() => {
  const rt = props.runtime
  if (!rt) return 0
  return [rt.node_ready, rt.git_ready, rt.python_ready, rt.engine_ready].filter(Boolean).length
})

// 布局里要显示的项：用 as const 把 key 收窄成字面量联合，layout[key] 才能
// 取到具体字段，否则 string 索引会被 TS 拒绝。
const pathFields = computed(
  () =>
    [
      { key: 'agent_dir', label: t('settings.layoutAgent') },
      { key: 'sessions_dir', label: t('settings.layoutSessions') },
      { key: 'skills_dir', label: t('settings.layoutSkills') },
      { key: 'settings_file', label: t('settings.layoutSettings') },
    ] as const,
)

// 复制一段文本：成功要给一次反馈——路径很长，界面毫无变化就确认不了复制上没有。
async function copyText(text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text)
    copiedCmd.value = text
    toasts.push('ok', t('settings.copied'))
    window.setTimeout(() => {
      if (copiedCmd.value === text) copiedCmd.value = ''
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
      <span class="tag" :class="readyCount === 4 ? 'ok' : 'warn'">{{ readyCount }}/4</span>
      <button
        class="wb-btn text sm push"
        :class="{ 'is-loading': props.detecting }"
        :disabled="props.detecting"
        @click="emit('redetect')"
      >
        {{ t('settings.redetect') }}
      </button>
    </div>

    <article v-for="item in props.apps" :key="item.app_key" class="engine-card">
      <div class="engine-head">
        <span class="dot" :class="item.installed ? 'ok' : 'warn'" />
        <span class="engine-name">{{ item.app_name }}</span>
        <span class="tag" :class="item.installed ? 'ok' : 'warn'">
          {{ item.installed ? item.app_version || '—' : t('apps.notInstalled') }}
        </span>
      </div>

      <!-- 缺运行时：只给「去应用页」，不在设置页里代跑安装 -->
      <div v-if="!item.installed" class="cmd-row">
        <span class="cmd-text">{{ t('settings.runtimeMissing') }}</span>
        <button class="wb-btn sm" @click="app.openApps()">{{ t('settings.goApps') }}</button>
      </div>

      <div v-else class="engine-facts">
        <div class="fact wide">
          <span class="fact-label">PATH</span>
          <button class="fact-value mono selectable" @click="copyText(item.app_path)">
            {{ item.app_path }}
          </button>
        </div>
      </div>
    </article>

    <div v-if="props.layout" class="layout-grid">
      <div v-for="field in pathFields" :key="field.key" class="layout-row">
        <span class="layout-label">{{ field.label }}</span>
        <button
          class="layout-path mono selectable"
          :title="t('settings.reveal')"
          @click="emit('reveal', props.layout[field.key])"
        >
          {{ props.layout[field.key] || '—' }}
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
  border: 0;
  background: transparent;
  padding: 0;
  text-align: left;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  overflow-wrap: anywhere;
  cursor: pointer;
}

.fact-value:hover {
  color: var(--wb-brand);
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
  font-size: var(--wb-fs-2xs);
  color: var(--wb-warn);
}

.layout-grid {
  display: flex;
  flex-direction: column;
  gap: 1px;
  margin-top: var(--wb-sp-2);
  padding-top: var(--wb-sp-2);
  border-top: 1px solid var(--wb-line);
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
