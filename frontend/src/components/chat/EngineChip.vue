<script setup lang="ts">
// 引擎状态芯片：一句话说清「引擎在不在、忙不忙」。
// 引擎在进程内，所以没有「装没装」与「重启」这两件事：
// 该看的是它此刻有没有在干活、有没有出错。
import { computed, ref } from 'vue'
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
  if (!runtime.value?.engine_ready) return 'missing'
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

/** 引擎出错了：最近一次失败原因还在，点一下重新拉一次会话状态。 */
const crashed = computed(() => Boolean(chat.bindErr))

// 重拉期间锁住芯片：不锁的话用户会连点，两次重拉比一次更慢。
const restartBusy = ref(false)

async function restart() {
  if (restartBusy.value) return
  restartBusy.value = true
  try {
    await app.refreshEngines()
    await chat.refresh(chat.sessionFile)
    toasts.push('ok', t('chat.engineRestarted'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    restartBusy.value = false
  }
}

function goApps() {
  void router.push({ name: 'apps' })
}
</script>

<template>
  <button
    class="chip"
    :class="[state, { crashed, 'is-loading': restartBusy }]"
    type="button"
    :disabled="restartBusy"
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
  position: relative;
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
  transform: scale(var(--wb-press-hover));
}

.chip.missing:active,
.chip.crashed:active {
  transform: scale(var(--wb-press-active));
  transition-duration: 60ms;
}

.chip:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

/* 重拉中：转圈 + 锁交互 + 锁尺寸。转圈照抄 base-buttons .wb-btn.is-loading
   的伪元素实现，led 与图标让位（visibility 占位不塌，文字保留）。 */
.chip.is-loading {
  cursor: progress;
  opacity: 0.85;
  pointer-events: none;
}

.chip.is-loading .wb-led,
.chip.is-loading .ic {
  visibility: hidden;
}

.chip.is-loading::before {
  content: '';
  position: absolute;
  left: var(--wb-sp-2);
  top: 50%;
  margin-top: -6px;
  width: 12px;
  height: 12px;
  border: 1.6px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: wb-spin 640ms linear infinite;
}

.ic {
  flex: none;
  opacity: 0.8;
}

.txt {
  white-space: nowrap;
}
</style>
