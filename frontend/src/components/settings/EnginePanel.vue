<!-- 助手引擎：PI 与本机运行时的状态、版本与升级。
     版本对账是唯一联网的读操作，与本地快照分开：网络断了面板仍要把本地状态说清楚。 -->
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSetupLog } from '../../composables/useSetupLog'
import { useToastStore } from '../../stores/toast'
import type { EngineLatestVO, EngineStatusVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import SettingsSection from './SettingsSection.vue'

const { t } = useI18n()
const toast = useToastStore()

const status = ref<EngineStatusVO | null>(null)
const latest = ref<EngineLatestVO | null>(null)
const loading = ref(true)
const checking = ref(false)
const updating = ref(false)
const fixingShell = ref(false)
const enablingTerminal = ref(false)

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

function runtimeSource(src: string) {
  return src === 'bundled' ? t('settings.engine.sourceBundled') : t('settings.engine.sourceSys')
}

/** 命令执行环境的几种形态：认不出的一律按「没有」说，别把内部名字丢给用户看 */
function shellKindLabel(kind: string) {
  switch (kind) {
    case 'git-bash':
      return t('settings.engine.shellKindGitBash')
    case 'msys':
      return t('settings.engine.shellKindMSYS')
    case 'cygwin':
      return t('settings.engine.shellKindCygwin')
    case 'powershell':
      return t('settings.engine.shellKindPowerShell')
    default:
      return t('settings.engine.shellKindNone')
  }
}

/** 重新探测并写回 PI：用户装了 Git 或改过 PATH 之后点这里 */
async function fixShell() {
  if (fixingShell.value) return
  fixingShell.value = true
  try {
    const vo = await api.env.fixShell()
    await loadStatus()
    if (!vo.shell_usable) {
      toast.bad(t('settings.engine.shellFailed', { msg: vo.shell_error || '' }))
      return
    }
    toast.ok(
      vo.shell_kind === 'powershell'
        ? t('settings.engine.shellFixedPS')
        : t('settings.engine.shellFixedBash', { kind: shellKindLabel(vo.shell_kind) }),
    )
  } catch (e) {
    toast.bad(t('settings.engine.shellFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    fixingShell.value = false
  }
}

/** 生成终端入口并挂 PATH：界面里能用不等于终端里能用，两者要各自说清楚 */
async function enableTerminal() {
  if (enablingTerminal.value) return
  enablingTerminal.value = true
  try {
    await api.env.enableTerminal()
    await loadStatus()
    if (status.value?.terminal_ready) {
      toast.ok(t('settings.engine.terminalDone'))
    } else {
      toast.info(
        t('settings.engine.terminalStill', {
          msg: status.value?.terminal_error || status.value?.terminal_shadowed_by || '',
        }),
      )
    }
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    enablingTerminal.value = false
  }
}

onMounted(() => {
  void loadStatus()
})
</script>

<template>
  <div class="panel-wrap">
    <SettingsSection
      icon="cpu"
      :title="t('settings.engine.title')"
      :desc="t('settings.engine.desc')"
    >
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

        <div class="eng-grid">
          <div class="eng-kv">
            <span class="wb-label">{{ t('settings.engine.runtime') }}</span>
            <p class="eng-line">
              <AppIcon name="check" size="ic-2xs" :class="status.node_ready ? 'ok' : 'bad'" />
              {{
                status.node_ready
                  ? t('settings.engine.nodeOk', {
                      version: status.node_version,
                      source: runtimeSource(status.node_source),
                    })
                  : t('settings.engine.nodeMissing', { msg: status.node_error })
              }}
            </p>
            <p class="eng-line">
              <AppIcon :name="status.python_ready ? 'check' : 'info'" size="ic-2xs" :class="status.python_ready ? 'ok' : 'bad'" />
              {{
                status.python_ready
                  ? t('settings.engine.pythonOk', {
                      version: status.python_version,
                      source: runtimeSource(status.python_source),
                    })
                  : t('settings.engine.pythonMissing')
              }}
            </p>
          </div>

          <div class="eng-kv">
            <span class="wb-label">{{ t('settings.engine.agentDir') }}</span>
            <p class="eng-mono">{{ status.agent_dir || '—' }}</p>
            <p v-if="status.install_root" class="eng-mono muted">{{ status.install_root }}</p>
          </div>

          <div class="eng-kv">
            <span class="wb-label">{{ t('settings.engine.shell') }}</span>
            <p class="eng-line">
              <AppIcon :name="status.shell_usable ? 'check' : 'alert'" size="ic-2xs" :class="status.shell_usable ? 'ok' : 'bad'" />
              {{ shellKindLabel(status.shell_kind) }}
            </p>
            <p class="eng-mono muted">{{ status.shell_path || status.shell_error || '—' }}</p>
            <p class="eng-meta">{{ t('settings.engine.shellDesc') }}</p>
            <div class="eng-row">
              <el-button size="small" :loading="fixingShell" :disabled="fixingShell" @click="fixShell">
                {{ fixingShell ? t('settings.engine.shellFixing') : t('settings.engine.shellFix') }}
              </el-button>
            </div>

            <span class="wb-label eng-gap">{{ t('settings.engine.terminal') }}</span>
            <p class="eng-line">
              <AppIcon :name="status.terminal_ready ? 'check' : 'info'" size="ic-2xs" :class="status.terminal_ready ? 'ok' : 'bad'" />
              {{ status.terminal_ready ? t('settings.engine.terminalReady') : t('settings.engine.terminalMissing') }}
            </p>
            <p class="eng-mono muted">{{ status.terminal_path || status.terminal_error || '—' }}</p>
            <p v-if="status.other_pis?.length" class="eng-warn">
              <AppIcon name="alert" size="ic-2xs" />
              {{ t('settings.engine.terminalConflict', { paths: status.other_pis.join('；') }) }}
            </p>
            <div class="eng-row">
              <el-button
                v-if="!status.terminal_ready && !status.terminal_shadowed_by"
                size="small"
                :loading="enablingTerminal"
                :disabled="enablingTerminal || !status.pi_ready"
                @click="enableTerminal"
              >
                {{ enablingTerminal ? t('settings.engine.terminalEnabling') : t('settings.engine.terminalEnable') }}
              </el-button>
              <p v-else-if="status.terminal_shadowed_by" class="eng-meta">
                {{ t('settings.engine.terminalShadowHint') }}
              </p>
              <p v-else class="eng-meta">{{ t('settings.engine.terminalHint') }}</p>
            </div>
          </div>
        </div>

        <div class="eng-pkgs">
          <header class="eng-pkg-head">
            <span class="wb-label">{{ t('settings.engine.packages') }}</span>
            <span class="sp" />
            <el-tag size="small" type="info">{{ status.packages }}</el-tag>
          </header>
          <p class="eng-meta">{{ t('settings.engine.packagesDesc') }}</p>
        </div>

        <div v-if="log.length" class="eng-log">
          <div class="wb-label">{{ t('setup.installLog') }}</div>
          <pre class="log-body">{{ log.join('\n') }}</pre>
        </div>
      </template>
    </SettingsSection>
  </div>
</template>

<style scoped>
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
.eng-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(calc(300px * var(--wb-vw-scale)), 1fr));
  gap: var(--wb-sp-4);
  padding: var(--wb-sp-3) 0;
}
.eng-kv {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}
.eng-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin-top: var(--wb-sp-1);
}
.eng-gap {
  margin-top: var(--wb-sp-3);
}
.eng-line {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
}
.ok {
  color: var(--wb-success);
}
.bad {
  color: var(--wb-warning);
}
.eng-mono {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  overflow-wrap: anywhere;
}
.eng-mono.muted {
  color: var(--wb-muted);
}
.eng-pkgs {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  padding-top: var(--wb-sp-3);
  border-top: 1px solid var(--wb-border);
}
.eng-pkg-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.eng-log {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
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
.sp {
  flex: 1;
}
</style>
