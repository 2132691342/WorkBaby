<!-- 助手引擎：PI 内核的版本、升级与它读的运行参数（思考档、压缩预算）。
     运行时（Node / Git / bash / 终端）在运行时环境面板——这里不重复。 -->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSetupLog } from '../../composables/useSetupLog'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { EngineLatestVO, EngineStatusVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import SettingsSection from './SettingsSection.vue'

const { t } = useI18n()
const toast = useToastStore()
const settings = useSettingsStore()

const LEVELS = ['off', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'] as const

const status = ref<EngineStatusVO | null>(null)
const latest = ref<EngineLatestVO | null>(null)
const loading = ref(true)
const checking = ref(false)
const updating = ref(false)

// 运行参数：思考档与压缩预算，写入 PI 的 settings.json
const level = ref('')
const reserve = ref(0)
const keep = ref(0)
const busy = ref('')

// 安装 / 升级的输出也挂在全局 SSE 通道上
const { lines: log, begin: beginLog, end: endLog, whenDone } = useSetupLog()

const updateAvailable = computed(() => !!status.value?.pi_ready && !!latest.value?.update_available)

async function loadStatus() {
  try {
    status.value = await api.engine.status()
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    loading.value = false
  }
}

/** 联网查最新版本：慢就慢，失败只说明查不到，不影响本地读数 */
async function checkLatest() {
  if (checking.value) return
  checking.value = true
  try {
    latest.value = await api.engine.latest()
  } catch (e) {
    latest.value = {
      pi_version: '',
      registry_error: (e as Error)?.message || t('errors.generic'),
      update_available: false,
      packages: [],
    }
  } finally {
    checking.value = false
  }
}

async function upgrade() {
  if (updating.value) return
  updating.value = true
  beginLog('update')
  // 后端在动作真结束时发一帧 done：等它，比“每两秒拉一次版本”又快又准。
  const waiter = whenDone(6 * 60 * 1000)
  try {
    await api.engine.update()
    const result = await waiter
    await loadStatus()
    if (!result.done) {
      toast.info(t('setup.piTimeout'))
    } else if (!result.ok) {
      toast.bad(t('settings.engine.installError', { msg: status.value?.install_error || t('errors.generic') }))
    } else {
      toast.ok(t('settings.engine.updated', { version: status.value?.pi_version || '' }))
      await checkLatest()
    }
  } catch (e) {
    toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
  } finally {
    updating.value = false
    endLog()
  }
}

function sourceLabel(src: string) {
  switch (src) {
    case 'system':
      return t('settings.engine.sourceSystem')
    case 'npm-global':
      // npm 全局前缀里那份可能就是我们装的，也可能是用户自己装的
      return status.value?.managed ? t('settings.engine.sourceManaged') : t('settings.engine.sourceSystem')
    case 'override':
      return t('settings.engine.sourceOverride')
    case 'env':
      return t('settings.engine.sourceEnv')
    default:
      return src || '—'
  }
}

async function saveThinking() {
  busy.value = 'thinking'
  try {
    await api.settings.setThinking(level.value)
    await settings.loadPISettings()
    toast.ok(t('common.save'))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function saveCompaction() {
  busy.value = 'compaction'
  try {
    await api.settings.setCompaction(Number(reserve.value) || 0, Number(keep.value) || 0)
    await settings.loadPISettings()
    toast.ok(t('common.save'))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

// 数字框每点一下就存一次会连着写盘、连着弹提示：按一下步进按钮五次就是五条「已保存」。
// 合流成一次写，提示也只说一句。
let compactionTimer = 0
function queueCompaction() {
  if (compactionTimer) clearTimeout(compactionTimer)
  compactionTimer = window.setTimeout(() => {
    compactionTimer = 0
    void saveCompaction()
  }, 400)
}
onBeforeUnmount(() => {
  if (compactionTimer) clearTimeout(compactionTimer)
})

onMounted(async () => {
  void loadStatus()
  if (!settings.piSettings) await settings.loadPISettings()
  level.value = settings.piSettings?.default_thinking_level || ''
  reserve.value = settings.piSettings?.compaction_reserve || 0
  keep.value = settings.piSettings?.compaction_keep || 0
})
</script>

<template>
  <div class="panel-wrap">
    <SettingsSection icon="cpu" :title="t('settings.engine.kernel')" :desc="t('settings.engine.desc')">
      <template #actions>
        <el-button size="small" :loading="checking" :disabled="checking || loading" @click="checkLatest">
          {{ checking ? t('settings.engine.checking') : t('settings.engine.check') }}
        </el-button>
      </template>

      <div v-if="loading" class="wb-card eng-card">
        <el-skeleton animated :rows="4" />
      </div>

      <template v-else-if="status">
        <article class="wb-card eng-hero">
          <span class="wb-tile wb-tile--lg"><AppIcon name="cpu" size="ic-xl" /></span>
          <div class="eng-hero-tx">
            <div class="eng-hero-row">
              <b>{{ t('settings.engine.piTitle') }}</b>
              <el-tag v-if="status.pi_ready" size="small" :type="updateAvailable ? 'warning' : 'success'">
                {{
                  updateAvailable
                    ? t('settings.engine.updateAvailable', { version: latest?.pi_version })
                    : t('settings.engine.upToDate')
                }}
              </el-tag>
              <el-tag v-else size="small" type="danger">{{ t('settings.engine.notInstalled') }}</el-tag>
            </div>
            <p class="eng-ver wb-mono">
              {{ t('settings.engine.version') }} · {{ status.pi_version || '—' }}
              <template v-if="latest?.pi_version">
                &nbsp;→ {{ t('settings.engine.latest') }} · {{ latest.pi_version }}
              </template>
            </p>
            <p class="eng-meta">
              {{ t('settings.engine.path') }} · <span class="wb-mono">{{ status.pi_path || '—' }}</span>
              &nbsp;· {{ sourceLabel(status.pi_source) }}
            </p>
            <p class="eng-meta">
              {{ status.running > 0 ? t('settings.engine.running', { n: status.running }) : t('settings.engine.runningNone') }}
            </p>
          </div>
          <div class="eng-hero-acts">
            <el-button
              v-if="status.pi_ready"
              size="small"
              type="primary"
              :loading="updating"
              :disabled="updating"
              @click="upgrade"
            >
              {{ updating ? t('settings.engine.updating') : t('settings.engine.update') }}
            </el-button>
            <p v-else class="eng-hint">{{ t('settings.engine.installFirst') }}</p>
          </div>
        </article>

        <p v-if="latest?.registry_error" class="eng-warn">
          <AppIcon name="alert" size="ic-2xs" />
          {{ t('settings.engine.registryFail', { msg: latest.registry_error }) }}
        </p>
        <p v-if="status.install_error" class="eng-warn">
          <AppIcon name="alert" size="ic-2xs" />
          {{ t('settings.engine.installError', { msg: status.install_error }) }}
        </p>

        <div v-if="log.length" class="eng-log">
          <div class="wb-label">{{ t('setup.installLog') }}</div>
          <pre class="log-body">{{ log.join('\n') }}</pre>
        </div>
      </template>
    </SettingsSection>

    <SettingsSection icon="sliders" :title="t('settings.engine.params')" :desc="t('settings.engine.paramsHint')" divided>
      <div class="wb-set-row">
        <div class="wb-set-row-tx">
          <b>{{ t('settings.engine.thinkingLevel') }}</b>
          <p>{{ t('settings.engine.thinkingHint') }}</p>
        </div>
        <el-select v-model="level" class="sel" :disabled="!!busy" @change="saveThinking">
          <el-option value="" :label="t('settings.engine.thinkingDefault')" />
          <el-option v-for="l in LEVELS" :key="l" :value="l" :label="t(`thinking.${l}`)" />
        </el-select>
      </div>

      <div class="wb-set-row">
        <div class="wb-set-row-tx">
          <b>{{ t('settings.engine.compactionReserve') }}</b>
          <p>{{ t('settings.engine.compactionReserveHint') }}</p>
        </div>
        <el-input-number v-model="reserve" :min="0" :step="1000" controls-position="right" :disabled="!!busy" @change="queueCompaction" />
      </div>

      <div class="wb-set-row">
        <div class="wb-set-row-tx">
          <b>{{ t('settings.engine.compactionKeep') }}</b>
          <p>{{ t('settings.engine.compactionKeepHint') }}</p>
        </div>
        <el-input-number v-model="keep" :min="0" :step="1000" controls-position="right" :disabled="!!busy" @change="queueCompaction" />
      </div>
    </SettingsSection>
  </div>
</template>

<style scoped>
/* 表单类面板收窄：控件列与标签列都靠左，面板宽 920px 时右侧会拖出一片空地 */
.panel-wrap {
  --wb-panel-w: calc(720px * var(--wb-vw-scale));
}
.sel {
  max-width: calc(220px * var(--wb-vw-scale));
}
.eng-card {
  padding: var(--wb-sp-4);
}
.eng-hero {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-4);
  padding: var(--wb-sp-4);
}
.eng-hero-tx {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}
.eng-hero-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.eng-ver {
  color: var(--wb-ink-2);
}
.eng-meta {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow-wrap: anywhere;
}
.eng-hero-acts {
  flex: none;
  display: flex;
  align-items: center;
}
.eng-hint {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  max-width: calc(220px * var(--wb-vw-scale));
  line-height: var(--wb-lh-base);
}
.eng-warn {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-warning);
  line-height: var(--wb-lh-base);
}
.eng-log {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  padding-top: var(--wb-sp-3);
}
.log-body {
  margin: 0;
  max-height: calc(220px * var(--wb-vw-scale));
  overflow: auto;
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius);
  background: var(--wb-code-bg);
  border: 1px solid var(--wb-border);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-ink-2);
  white-space: pre-wrap;
}
</style>
