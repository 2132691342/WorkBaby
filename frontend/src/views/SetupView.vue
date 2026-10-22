<!-- 环境向导：运行时 / 助手引擎 / 模型服务三段式。
     pi_env.ready=false 时强制进这里——没有 PI 就没有对话。 -->
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import * as api from '../api'
import { useSetupLog } from '../composables/useSetupLog'
import { useSettingsStore } from '../stores/settings'
import { useToastStore } from '../stores/toast'
import type { PiEnvVO } from '../types/api'
import AppIcon from '../components/common/AppIcon.vue'
import ProviderPanel from '../components/settings/ProviderPanel.vue'

const { t } = useI18n()
const router = useRouter()
const settings = useSettingsStore()
const toast = useToastStore()

const env = ref<PiEnvVO | null>(null)
const loading = ref(true)
const loadErr = ref('')
const installing = ref(false)
const rechecking = ref(false)
// 上一次安装失败的原因：闸门快照里带回来，比只看日志尾巴靠谱
const installErr = ref('')

// 安装日志：npm 可能跑几十秒，没有实时输出用户会以为卡死
const { lines: installLines, begin: beginInstallLog, end: endInstallLog } = useSetupLog()

const steps = computed(() => {
  const e = env.value
  return [
    {
      key: 'node',
      icon: 'cpu',
      title: t('setup.node'),
      desc: t('setup.nodeDesc'),
      done: !!e?.node_ready,
      meta: e?.node_ready
        ? `${e.node_source === 'bundled' ? t('setup.nodeBundled') : t('setup.nodeSystem')} · Node ${e.node_version}`
        : e?.node_error || t('setup.nodeMissing'),
    },
    {
      key: 'pi',
      icon: 'sparkles',
      title: t('setup.pi'),
      desc: t('setup.piDesc'),
      done: !!e?.pi_ready,
      meta: e?.pi_ready
        ? `${e.pi_source === 'bundled' ? t('setup.piBundled') : t('setup.piSystem')} · ${e.pi_version || '—'}`
        : e?.pi_error || t('setup.piMissing'),
    },
    {
      key: 'model',
      icon: 'brain',
      title: t('setup.model'),
      desc: t('setup.modelDesc'),
      done: !!e?.model_ready,
      meta: e?.model_ready
        ? t('setup.modelReady', { provider: settings.boot?.default_provider || '—', model: settings.boot?.default_model || '—' })
        : t('setup.modelMissing'),
    },
  ]
})

const blocking = computed(() => env.value?.blocking || '')
const allDone = computed(() => !!env.value?.ready)

async function load() {
  loading.value = true
  loadErr.value = ''
  try {
    env.value = await api.env.status()
    installErr.value = env.value?.install_error || ''
  } catch (e) {
    loadErr.value = (e as Error)?.message || t('errors.generic')
  } finally {
    loading.value = false
  }
}

async function recheck() {
  if (rechecking.value) return
  rechecking.value = true
  try {
    env.value = await api.env.recheck()
    installErr.value = env.value?.install_error || ''
    void settings.loadBoot()
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    rechecking.value = false
  }
}

/**
 * 安装是后台动作：接口立刻返回，结果靠轮询。
 * 失败要看得见原因——只报「还没装好」等于把用户丢在那里。
 */
async function install() {
  if (installing.value) return
  installing.value = true
  installErr.value = ''
  beginInstallLog()
  try {
    await api.env.install()
    const deadline = Date.now() + 6 * 60 * 1000
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 1500))
      env.value = await api.env.recheck()
      if (env.value?.pi_ready) break
      if (env.value?.install_error) {
        installErr.value = env.value.install_error
        break
      }
    }
    if (env.value?.pi_ready) {
      toast.ok(t('setup.piInstalled', { version: env.value.pi_version || '' }))
      void settings.loadBoot()
    } else if (installErr.value) {
      toast.bad(t('setup.piInstallFailed', { msg: installErr.value }))
    } else {
      toast.bad(t('setup.piTimeout'))
    }
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    installing.value = false
    endInstallLog()
  }
}

async function afterProviderSaved() {
  await recheck()
}

function enter() {
  if (!allDone.value) return
  void router.replace('/')
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="setup">
    <div class="setup-col">
      <header class="setup-head">
        <span class="wb-tile wb-tile--lg"><AppIcon name="sparkles" size="ic-xl" /></span>
        <div>
          <h1 class="setup-title">{{ t('setup.title') }}</h1>
          <p class="setup-sub">{{ t('setup.subtitle') }}</p>
        </div>
      </header>

      <div v-if="loading" class="wb-card setup-card"><el-skeleton animated :rows="6" /></div>

      <div v-else-if="loadErr" class="notice-bar notice-bar--bad">
        <AppIcon name="alert" size="ic-xs" />
        <span class="grow">{{ loadErr }}</span>
        <el-button size="small" @click="load">{{ t('common.retry') }}</el-button>
      </div>

      <template v-else>
        <ol class="steps">
          <li
            v-for="(s, i) in steps"
            :key="s.key"
            class="step"
            :class="{ 'is-done': s.done, 'is-active': blocking === s.key }"
          >
            <span class="step-num">
              <AppIcon v-if="s.done" name="check" size="ic-sm" />
              <span v-else>{{ i + 1 }}</span>
            </span>
            <div class="step-body">
              <div class="step-row">
                <b class="step-title">{{ s.title }}</b>
                <el-tag size="small" :type="s.done ? 'success' : blocking === s.key ? 'warning' : 'info'">
                  {{ s.done ? t('common.ok') : blocking === s.key ? t('setup.blocking') : t('setup.pending') }}
                </el-tag>
              </div>
              <p class="step-desc">{{ s.desc }}</p>
              <p class="step-meta wb-mono">{{ s.meta }}</p>
              <el-button
                v-if="s.key === 'pi' && !s.done"
                size="small"
                type="primary"
                :loading="installing"
                :disabled="installing"
                @click="install"
              >
                {{ installing ? t('setup.piInstalling') : installErr ? t('setup.piRetry') : t('setup.piInstall') }}
              </el-button>

              <!-- 失败要说清楚下一步：原因 + 能自己动手的那条路 -->
              <div v-if="s.key === 'pi' && !s.done && installErr" class="step-fail">
                <p class="step-fail-line">
                  <AppIcon name="alert" size="ic-2xs" />
                  {{ t('setup.piInstallFailed', { msg: installErr }) }}
                </p>
                <p class="step-fail-hint">{{ t('setup.piInstallHint') }}</p>
              </div>
              <p v-else-if="s.key === 'pi' && !s.done" class="step-hint">{{ t('setup.piManual') }}</p>
            </div>
          </li>
        </ol>

        <div v-if="installLines.length || installing" class="wb-card setup-card">
          <div class="wb-label">{{ t('setup.installLog') }}</div>
          <pre v-if="installLines.length" class="setup-log">{{ installLines.join('\n') }}</pre>
          <p v-else class="step-hint">{{ t('setup.installLogEmpty') }}</p>
        </div>

        <section v-if="!allDone" class="wb-card setup-card">
          <div class="wb-label">{{ t('setup.model') }}</div>
          <ProviderPanel @changed="afterProviderSaved" />
        </section>

        <footer class="setup-foot">
          <el-button :loading="rechecking" @click="recheck">
            {{ rechecking ? t('setup.rechecking') : t('setup.recheck') }}
          </el-button>
          <el-button type="primary" :disabled="!allDone" @click="enter">
            {{ allDone ? t('setup.goChat') : t('setup.readyHint') }}
          </el-button>
        </footer>
      </template>
    </div>
  </div>
</template>

<style scoped>
.setup {
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  background: var(--wb-bg);
}
.setup-col {
  max-width: calc(720px * var(--wb-vw-scale));
  margin: 0 auto;
  padding: var(--wb-sp-8) var(--wb-sp-5) var(--wb-sp-12);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}
.setup-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-4);
}
.setup-title {
  font-family: var(--font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 700;
  letter-spacing: -0.015em;
  color: var(--wb-ink);
}
.setup-sub {
  margin-top: 2px;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.setup-card {
  padding: var(--wb-sp-4);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.steps {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.step {
  display: flex;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-4);
  border-radius: var(--wb-radius-xl);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  box-shadow: var(--wb-card-shadow);
  transition:
    border-color var(--wb-dur) var(--wb-ease),
    background var(--wb-dur) var(--wb-ease);
}
.step.is-done {
  border-color: color-mix(in srgb, var(--wb-success) 34%, var(--wb-border));
}
.step.is-active {
  border-color: var(--wb-primary);
}
.step-num {
  width: var(--wb-ctl-icon);
  height: var(--wb-ctl-icon);
  border-radius: var(--wb-radius-full);
  display: grid;
  place-items: center;
  flex: none;
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  background: var(--wb-elev-hover);
  color: var(--wb-muted);
}
.step.is-done .step-num {
  background: var(--wb-primary);
  color: var(--wb-primary-ink);
}
.step-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  align-items: flex-start;
}
.step-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
}
.step-title {
  font-size: var(--wb-fs-md);
  font-weight: 700;
  color: var(--wb-ink);
  flex: 1;
}
.step-desc {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
}
.step-meta {
  overflow-wrap: anywhere;
}
.step-fail {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  background: var(--wb-danger-soft);
  width: 100%;
}
.step-fail-line {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-danger);
  line-height: var(--wb-lh-base);
  overflow-wrap: anywhere;
}
.step-fail-hint {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  line-height: var(--wb-lh-base);
}
.step-hint {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
}
.setup-log {
  margin: 0;
  max-height: calc(180px * var(--wb-vw-scale));
  overflow: auto;
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius);
  background: var(--wb-ink);
  color: var(--wb-bg);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  white-space: pre-wrap;
}
.setup-foot {
  display: flex;
  justify-content: flex-end;
  gap: var(--wb-sp-2);
}
.grow {
  flex: 1;
}
</style>
