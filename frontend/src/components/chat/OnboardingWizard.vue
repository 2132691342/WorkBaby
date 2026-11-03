<script setup lang="ts">
// 首启引导：没配好模型时，把「选服务商 → 填密钥 → 测连通」这条链摆在眼前。
// 原来只给一个「去设置」按钮：用户得自己知道要配什么、填在哪、怎么确认配好了。
// 这三步在这里一次做完，配完立刻能发第一条消息。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { ProviderPresetVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import { errorText } from '../../utils/errors'

const emit = defineEmits<{ done: [] }>()
const { t } = useI18n()
const settings = useSettingsStore()
const toast = useToastStore()

/** 引导的三个阶段：选一个 → 填密钥 → 确认能用 */
type Step = 'pick' | 'key' | 'ready'
const step = ref<Step>('pick')
const presets = ref<ProviderPresetVO[]>([])
const picked = ref<ProviderPresetVO | null>(null)
const apiKey = ref('')
const busy = ref(false)
const connected = ref(false)

/** 还没填密钥的预设也要能选：本地模型（Ollama）本来就不需要密钥。 */
const needKey = computed(() => picked.value?.preset_id !== 'ollama')

async function load() {
  try {
    presets.value = await api.providers.presets()
  } catch {
    presets.value = []
  }
}

/** 选一个预设：进下一步填密钥。 */
function choose(p: ProviderPresetVO) {
  picked.value = p
  apiKey.value = ''
  connected.value = false
  step.value = 'key'
}

/** 保存并测连通：一次做完，配完立刻知道成没成。 */
async function save() {
  const p = picked.value
  if (!p || busy.value) return
  busy.value = true
  try {
    await api.providers.applyPreset(p.preset_id, apiKey.value.trim())
    await settings.refreshBoot()
    await settings.loadProviders()
    // 测一下连通：密钥填错时用户能立刻看到，而不是发第一条消息才发现
    const list = await api.providers.list()
    const added = list.resp_providers.find((x) => x.provider_id === p.preset_id)
    if (added && added.provider_has_key) {
      const t = await api.providers.test(p.preset_id)
      connected.value = !!t.resp_ok
    }
    if (connected.value) {
      step.value = 'ready'
    } else {
      toast.info(t('settings.providers.presetNoTest'))
    }
    await load()
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  } finally {
    busy.value = false
  }
}

/** 回到选服务商那一步重新挑。 */
function back() {
  step.value = 'pick'
  picked.value = null
  apiKey.value = ''
  connected.value = false
}

/** 配完了：把界面切回可发消息的状态。 */
function finish() {
  emit('done')
}

load()
</script>

<template>
  <div class="onb rise rise-4">
    <!-- 步骤条：让用户知道自己在第几步 -->
    <ol class="onb-steps">
      <li class="onb-step" :class="{ 'is-on': step === 'pick', 'is-done': step !== 'pick' }">
        <span class="onb-no">1</span>
        <span>{{ t('onboarding.stepPick') }}</span>
      </li>
      <li class="onb-step" :class="{ 'is-on': step === 'key', 'is-done': step === 'ready' }">
        <span class="onb-no">2</span>
        <span>{{ t('onboarding.stepKey') }}</span>
      </li>
      <li class="onb-step" :class="{ 'is-on': step === 'ready' }">
        <span class="onb-no">3</span>
        <span>{{ t('onboarding.stepReady') }}</span>
      </li>
    </ol>

    <!-- 第 1 步：挑服务商 -->
    <div v-if="step === 'pick'" class="onb-panel">
      <b class="onb-title">{{ t('onboarding.pickTitle') }}</b>
      <p class="onb-sub">{{ t('onboarding.pickSub') }}</p>
      <div class="onb-grid">
        <button
          v-for="p in presets.filter((x) => !x.preset_installed)"
          :key="p.preset_id"
          type="button"
          class="onb-card wb-press"
          @click="choose(p)"
        >
          <span class="onb-card-name">{{ p.preset_name }}</span>
          <span class="onb-card-hint">{{ p.preset_hint }}</span>
        </button>
      </div>
      <p v-if="!presets.filter((x) => !x.preset_installed).length" class="onb-sub">
        {{ t('onboarding.pickAllAdded') }}
      </p>
    </div>

    <!-- 第 2 步：填密钥 -->
    <div v-else-if="step === 'key'" class="onb-panel">
      <b class="onb-title">{{ t('onboarding.keyTitle', { name: picked?.preset_name || '' }) }}</b>
      <p class="onb-sub">{{ t('onboarding.keySub') }}</p>
      <el-input
        v-model="apiKey"
        type="password"
        show-password
        size="small"
        :placeholder="needKey ? t('onboarding.keyPlaceholder') : t('onboarding.keyNotNeeded')"
        :disabled="!needKey"
        @keydown.enter="save"
      />
      <div class="onb-acts">
        <el-button size="small" @click="back">{{ t('onboarding.back') }}</el-button>
        <el-button size="small" type="primary" :loading="busy" :disabled="busy" @click="save">
          {{ t('onboarding.save') }}
        </el-button>
      </div>
    </div>

    <!-- 第 3 步：确认能用 -->
    <div v-else class="onb-panel">
      <span class="wb-tile onb-ok"><AppIcon name="check" size="ic-lg" /></span>
      <b class="onb-title">{{ t('onboarding.readyTitle') }}</b>
      <p class="onb-sub">{{ t('onboarding.readySub') }}</p>
      <div class="onb-acts">
        <el-button size="small" type="primary" @click="finish">{{ t('onboarding.start') }}</el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.onb {
  width: 100%;
  max-width: calc(620px * var(--wb-vw-scale));
  padding: var(--wb-sp-4);
  border-radius: var(--wb-radius-xl);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  box-shadow: var(--wb-card-shadow);
  text-align: left;
}
.onb-steps {
  display: flex;
  gap: var(--wb-sp-3);
  margin: 0 0 var(--wb-sp-3);
  padding: 0;
  list-style: none;
}
.onb-step {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  transition: color var(--wb-dur) var(--wb-ease);
}
.onb-step.is-on {
  color: var(--wb-ink);
  font-weight: 700;
}
.onb-step.is-done {
  color: var(--wb-success);
}
.onb-no {
  display: grid;
  place-items: center;
  width: calc(18px * var(--wb-vw-scale));
  height: calc(18px * var(--wb-vw-scale));
  border-radius: 50%;
  border: 1px solid currentColor;
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
}
.onb-panel {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.onb-title {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.onb-sub {
  margin: 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  line-height: 1.45;
}
.onb-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(calc(180px * var(--wb-vw-scale)), 1fr));
  gap: var(--wb-sp-2);
  margin-top: var(--wb-sp-1);
}
.onb-card {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--wb-sp-2);
  text-align: left;
  border-radius: var(--wb-radius-md);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-surface);
  cursor: pointer;
}
.onb-card:hover {
  border-color: var(--wb-brand-2);
  background: var(--wb-brand-3);
}
.onb-card-name {
  font-size: var(--wb-fs-xs);
  font-weight: 700;
  color: var(--wb-ink);
}
.onb-card-hint {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  line-height: 1.35;
}
.onb-acts {
  display: flex;
  gap: var(--wb-sp-2);
  justify-content: flex-end;
  margin-top: var(--wb-sp-1);
}
.onb-ok {
  color: var(--wb-success);
}
</style>
