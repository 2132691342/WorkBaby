<script setup lang="ts">
// 工作区芯片：在输入框底栏直接切工作目录——它决定这句话在哪个工程里执行，
// 属于输入的一部分。显示完整路径（窄屏才截断）：
// 同时开几个工程时「…\my-project」认不出是哪个。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'
import { pickFolder } from '@/api/wails'

const { t } = useI18n()
const app = useAppStore()
const toasts = useToasts()

const open = ref(false)
const busy = ref(false)
const root = ref<HTMLElement | null>(null)

/** 当前工作目录的完整路径。 */
const current = computed(() => app.workspace)

// 芯片上的显示文本：整条路径放得下就显示整条（短路径截断反而看不出是哪个）；
// 真正长时才截，保留盘符与最后两段。
const shown = computed(() => {
  const path = current.value
  if (!path) return t('workspace.pickWorkspace')
  if (path.length <= 28) return path
  const parts = path.split(/[\\/]/).filter(Boolean)
  if (parts.length <= 2) return path
  return `${parts[0]}\\…\\${parts.slice(-2).join('\\')}`
})

/** 最近目录列表：当前目录排在最前，其余按时间倒序。 */
const recents = computed(() => {
  const rows = app.workspaces.filter((w) => w.workspace_path !== current.value)
  return rows.slice(0, 8)
})

async function switchTo(path: string): Promise<void> {
  if (busy.value || path === current.value) {
    open.value = false
    return
  }
  busy.value = true
  try {
    await app.setWorkspace(path)
    toasts.push('ok', t('workspace.switched', { path }))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busy.value = false
    open.value = false
  }
}

async function browse(): Promise<void> {
  open.value = false
  const dir = await pickFolder(current.value)
  if (!dir) return
  await switchTo(dir)
}

function onDocPointerDown(event: PointerEvent): void {
  if (root.value?.contains(event.target as Node)) return
  open.value = false
}

function onEsc(event: KeyboardEvent): void {
  if (event.key === 'Escape') open.value = false
}

onMounted(() => {
  document.addEventListener('pointerdown', onDocPointerDown)
  document.addEventListener('keydown', onEsc)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocPointerDown)
  document.removeEventListener('keydown', onEsc)
})
</script>

<template>
  <div ref="root" class="ws">
    <button
      class="ws-chip"
      :class="{ 'is-on': open, 'is-busy': busy }"
      type="button"
      :title="current || t('workspace.pickWorkspace')"
      @click="open = !open"
    >
      <AppIcon name="folder" :size="13" />
      <span class="ws-t mono">{{ shown }}</span>
      <AppIcon v-if="busy" name="refresh" :size="12" class="spin" />
      <AppIcon v-else name="chevron" :size="12" class="caret" :class="{ up: open }" />
    </button>

    <Transition name="pop">
      <div v-if="open" class="ws-panel">
        <div class="ws-head">
          <span class="ws-label">{{ t('workspace.workspace') }}</span>
          <span class="ws-full mono selectable">{{ current || '—' }}</span>
        </div>
        <button class="ws-row ws-browse" type="button" @click="browse">
          <AppIcon name="folder" :size="13" />
          <span class="ws-grow">{{ t('workspace.pickWorkspace') }}</span>
        </button>
        <template v-if="recents.length">
          <div class="ws-label ws-sub">{{ t('workspace.recentDirs') }}</div>
          <button
            v-for="row in recents"
            :key="row.workspace_path"
            class="ws-row"
            type="button"
            :disabled="busy"
            :title="row.workspace_path"
            @click="switchTo(row.workspace_path)"
          >
            <AppIcon name="clock" :size="12" />
            <span class="ws-grow mono">{{ row.workspace_path }}</span>
          </button>
        </template>
        <p v-else class="ws-empty">{{ t('workspace.noRecentDirs') }}</p>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.ws {
  position: relative;
  min-width: 0;
}

.ws-chip {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  height: var(--wb-ctl-sm);
  /* 路径可能很长：芯片本身不限宽，文本自己截。
     限宽 320px 会把「C:\Users\LIKX」这种短路径也截成看不出的一团。 */
  max-width: 46vw;
  padding: 0 var(--wb-sp-2);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-sm);
  background: transparent;
  color: var(--wb-mute);
  font-family: inherit;
  font-size: var(--wb-fs-3xs);
  font-weight: 600;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.ws-chip:hover:not(:disabled) {
  background: var(--wb-hover);
  border-color: var(--wb-line-strong);
  color: var(--wb-ink-2);
  transform: scale(var(--wb-press-hover));
}

.ws-chip:active:not(:disabled) {
  transform: scale(var(--wb-press-active));
  transition-duration: 60ms;
}

.ws-chip.is-on {
  background: var(--wb-brand-soft);
  border-color: var(--wb-brand-3);
  color: var(--wb-brand);
}

.ws-chip:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.ws-t {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.caret {
  flex: none;
  opacity: 0.55;
  transform: rotate(90deg);
  transition: transform var(--wb-dur-fast) var(--wb-ease);
}

.caret.up {
  transform: rotate(-90deg);
}

.spin {
  flex: none;
  animation: wb-spin 900ms linear infinite;
}

/* 面板贴着芯片往上弹：工作区切换是「接着输入」的动作，
   弹到屏幕外会让人找不到。 */
.ws-panel {
  position: absolute;
  bottom: calc(100% + var(--wb-sp-2));
  left: 0;
  z-index: var(--wb-z-pop);
  width: 380px;
  max-width: 70vw;
  padding: var(--wb-sp-2);
  border: 1px solid var(--wb-line-strong);
  border-radius: var(--wb-r-lg);
  background: var(--wb-raised);
  box-shadow: var(--wb-shadow-lift);
}

.ws-head {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 0 var(--wb-sp-2) var(--wb-sp-2);
  border-bottom: 1px solid var(--wb-line);
  margin-bottom: var(--wb-sp-1);
}

.ws-label {
  font-size: var(--wb-fs-3xs);
  font-weight: 700;
  letter-spacing: 0.07em;
  text-transform: uppercase;
  color: var(--wb-faint);
}

.ws-sub {
  padding: var(--wb-sp-2) var(--wb-sp-2) 2px;
}

/* 完整路径单独一行：芯片上被截掉的那些段在这里能看全 */
.ws-full {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-ink-2);
  overflow-wrap: anywhere;
  user-select: text;
}

.ws-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: var(--wb-sp-2);
  border: 0;
  border-radius: var(--wb-r-sm);
  background: transparent;
  color: var(--wb-ink-2);
  font-family: inherit;
  font-size: var(--wb-fs-2xs);
  text-align: left;
  cursor: pointer;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}

.ws-row:hover:not(:disabled) {
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
}

.ws-row:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.ws-browse {
  color: var(--wb-brand);
}

.ws-grow {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ws-empty {
  margin: 0;
  padding: var(--wb-sp-2);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.pop-enter-active,
.pop-leave-active {
  transition:
    opacity var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.pop-enter-from,
.pop-leave-to {
  opacity: 0;
  transform: translateY(4px);
}
</style>
