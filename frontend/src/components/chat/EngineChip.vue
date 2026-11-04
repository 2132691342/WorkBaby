<script setup lang="ts">
// 引擎状态芯片：一句话说清「引擎装没装、在不在跑、忙不忙」。
// 只有一个引擎，所以它不再是切换器——切换已经没有意义，
// 该看的是它对不对得起来：没装就点去应用页，崩了就点重启。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppIcon from '@/components/common/AppIcon.vue'
import { useAppStore } from '@/stores/app'
import { useChatStore } from '@/stores/chat'
import { useToasts } from '@/composables/useToasts'

const { t } = useI18n()
const app = useAppStore()
const chat = useChatStore()
const router = useRouter()
const toasts = useToasts()

const runtime = computed(() => app.runtime)

/** 三态：就绪 / 未就绪 / 运行中。 */
const state = computed<'ready' | 'missing' | 'running'>(() => {
  if (!runtime.value?.pi_ready) return 'missing'
  if (chat.streaming || chat.compacting) return 'running'
  return 'ready'
})

const label = computed(() => {
  switch (state.value) {
    case 'missing':
      return t('chat.engineMissing')
    case 'running':
      return t('chat.engineRunning')
    default:
      return t('chat.engineReady')
  }
})

/** 引擎崩了：最近一次失败原因还在，给出重启入口。 */
const crashed = computed(() => Boolean(chat.bindErr))

async function restart() {
  try {
    await app.refreshEngines()
    await chat.refresh(chat.sessionFile)
    toasts.push('ok', t('common.saved'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

function goApps() {
  void router.push({ name: 'apps' })
}
</script>

<template>
  <button
    class="chip"
    :class="[state, { crashed }]"
    type="button"
    :title="crashed ? chat.bindErr : label"
    @click="state === 'missing' ? goApps() : crashed ? restart() : undefined"
  >
    <span class="wb-led" :class="state === 'running' ? 'wb-led--ok' : state === 'missing' ? 'wb-led--warn' : 'wb-led--ok'" />
    <AppIcon name="box" :size="12" class="ic" />
    <span class="txt">{{ crashed ? t('chat.engineCrashed') : label }}</span>
  </button>
</template>

<style scoped>
.chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 22px;
  padding: 0 var(--wb-sp-2);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-full);
  background: var(--wb-surface);
  color: var(--wb-mute);
  font-family: inherit;
  font-size: var(--wb-fs-3xs);
  cursor: default;
  transition:
    border-color var(--wb-dur-fast) var(--wb-ease),
    background-color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.chip.missing,
.chip.crashed {
  cursor: pointer;
  border-color: var(--wb-warn);
  color: var(--wb-warn);
}

.chip.missing:hover,
.chip.crashed:hover {
  background: var(--wb-warn-soft);
  transform: scale(1.04);
}

.chip.missing:active,
.chip.crashed:active {
  transform: scale(0.96);
}

.ic {
  flex: none;
  opacity: 0.8;
}

.txt {
  white-space: nowrap;
}
</style>
