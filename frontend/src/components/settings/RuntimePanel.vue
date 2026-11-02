<script setup lang="ts">
// 内置运行时面板：bash / git / python 是否就位、命中的是哪个路径。
// 这一屏存在的理由：运行时解压是首次启动的隐形动作（78MB），
// 失败时只写日志，用户完全不知道「为什么 bash 跑不了」。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import type { RuntimeStatusRESP } from '../../types/api'
import { errorText } from '../../utils/errors'
import SettingsSection from './SettingsSection.vue'
import AppIcon from '../common/AppIcon.vue'

const { t } = useI18n()

const status = ref<RuntimeStatusRESP | null>(null)
const loading = ref(true)
const failed = ref('')

const ready = computed(() => status.value?.resp_ready ?? false)
const binaries = computed(() => status.value?.resp_binaries ?? [])

async function reload() {
  loading.value = true
  failed.value = ''
  try {
    status.value = await api.runtimeStatus()
  } catch (e) {
    failed.value = errorText(e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void reload()
})
</script>

<template>
  <SettingsSection
    icon="terminal"
    :title="t('settings.runtime.title')"
    :desc="t('settings.runtime.desc')"
    divided
  >
    <template #actions>
      <span v-if="status" class="rt-flag" :class="ready ? 'is-ok' : 'is-bad'">
        {{ ready ? t('settings.runtime.ready') : t('settings.runtime.incomplete') }}
      </span>
      <el-button size="small" :loading="loading" :disabled="loading" @click="reload">
        {{ t('common.refresh') }}
      </el-button>
    </template>

    <div v-if="loading && !status" class="rt-skel">
      <el-skeleton animated :rows="3" />
    </div>

    <p v-else-if="failed" class="rt-err">{{ t('common.loadFailed', { msg: failed }) }}</p>

    <template v-else>
      <div v-for="b in binaries" :key="b.binary_name" class="rt-row">
        <span class="rt-dot" :class="b.binary_ready ? 'is-ok' : 'is-bad'" aria-hidden="true" />
        <span class="rt-name">{{ b.binary_name }}</span>
        <span class="rt-path" :title="b.binary_path">
          {{ b.binary_ready ? b.binary_path : t('settings.runtime.missing') }}
        </span>
        <AppIcon :name="b.binary_ready ? 'check' : 'alert'" size="ic-xs" class="rt-ico" :class="b.binary_ready ? 'is-ok' : 'is-bad'" />
      </div>

      <p v-if="status?.resp_failure" class="rt-why">{{ status.resp_failure }}</p>

      <p class="rt-root" :title="status?.resp_root">{{ t('settings.runtime.root', { path: status?.resp_root || '—' }) }}</p>
    </template>
  </SettingsSection>
</template>

<style scoped>
.rt-flag {
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  padding: 1px var(--wb-sp-2);
  border-radius: var(--wb-radius-full);
  border: 1px solid transparent;
}
.rt-flag.is-ok {
  color: var(--wb-success);
  border-color: var(--wb-success);
  background: var(--wb-accent-tint);
}
.rt-flag.is-bad {
  color: var(--wb-danger);
  border-color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
.rt-skel {
  padding: var(--wb-sp-2) 0;
}
.rt-err {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-danger);
}
.rt-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  margin-bottom: var(--wb-sp-1);
  transition: border-color var(--wb-dur) var(--wb-ease);
}
.rt-row:hover {
  border-color: var(--wb-border-strong);
}
.rt-dot {
  width: 6px;
  height: 6px;
  border-radius: var(--wb-radius-full);
  flex: none;
}
.rt-dot.is-ok {
  background: var(--wb-success);
}
.rt-dot.is-bad {
  background: var(--wb-danger);
}
.rt-name {
  flex: none;
  width: calc(58px * var(--wb-vw-scale));
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-ink);
}
.rt-path {
  flex: 1;
  min-width: 0;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rt-ico.is-ok {
  color: var(--wb-success);
}
.rt-ico.is-bad {
  color: var(--wb-danger);
}
.rt-why {
  margin: var(--wb-sp-1) 0 0;
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-warning);
  overflow-wrap: anywhere;
}
.rt-root {
  margin: var(--wb-sp-2) 0 0;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow-wrap: anywhere;
}
</style>
