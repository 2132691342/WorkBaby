<script setup lang="ts">
// 会话页：列表复用 SessionList（与对话页左栏同一份实现），这里只补范围切换与汇总读数。
// 汇总读 app.sessions——它恒为「当前目录」的快照，所以标题行明确标出这是当前目录的合计，
// 不随上面的列表范围变化，避免把两件事读成一件。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppIcon from '@/components/common/AppIcon.vue'
import SessionList from '@/components/sessions/SessionList.vue'
import WbSeg from '@/components/common/WbSeg.vue'
import WbEmpty from '@/components/common/WbEmpty.vue'
import type { SegItem } from '@/types/ui'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'
import { fmtCost, fmtCount } from '@/utils/format'

const { t } = useI18n()
const app = useAppStore()
const router = useRouter()
const toasts = useToasts()

const scope = ref<'workspace' | 'all'>('workspace')

/** 没选工作目录时这一页没有「当前目录」可言：汇总读数全是 0，那是假结论。 */
const noWorkspace = computed(() => !app.workspace)

/** 范围分段：只看当前目录 / 看全部目录。 */
const scopeItems = computed<SegItem[]>(() => [
  { value: 'workspace', label: t('sessions.thisWorkspace') },
  { value: 'all', label: t('sessions.allWorkspace') },
])
const refreshing = ref(false)
// 列表的重新取数走它暴露的 load()：重挂会让整张列表闪一下还丢滚动位置。
const listRef = ref<InstanceType<typeof SessionList> | null>(null)
// 汇总的竞态守卫：切换范围或快速连点刷新时，晚到的旧快照不能盖回新数据。
let refreshSeq = 0

watch(scope, () => {
  void listRef.value?.load()
})

const counts = computed(() => {
  let messages = 0
  let tokens = 0
  let cost = 0
  for (const row of app.sessions) {
    messages += row.message_count ?? 0
    tokens += row.total_tokens ?? 0
    cost += row.cost_usd ?? 0
  }
  return { sessions: app.sessions.length, messages, tokens, cost }
})


async function refresh() {
  const mine = ++refreshSeq
  refreshing.value = true
  try {
    await app.refreshSessions()
    if (mine !== refreshSeq) return
    await listRef.value?.load()
  } catch (err) {
    if (mine !== refreshSeq) return
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    if (mine === refreshSeq) refreshing.value = false
  }
}

// 会话的落点是对话页：带上 path，由对话页按它载入这一条。
// 空串来自列表空态的「开始对话」：不带 session 参数，直接进对话页开新会话。
async function openSession(path: string) {
  try {
    await router.push(path ? { name: 'chat', query: { session: path } } : { name: 'chat' })
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

function onChanged() {
  void app.refreshSessions().catch((err: unknown) => {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  })
}

/** 没选工作目录时的出口：直接去选目录，选完 store 自己刷新会话清单与汇总。 */
const choosingWorkspace = ref(false)
async function pickWorkspace(): Promise<void> {
  const before = app.workspace
  choosingWorkspace.value = true
  try {
    await app.chooseWorkspace()
    // 取消选目录时工作区没变，不能报「已切到」。
    if (app.workspace !== before) toasts.push('ok', t('workspace.switched', { path: app.workspace }))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    choosingWorkspace.value = false
  }
}
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('sessions.title') }}</h1>
        <p class="page-sub">{{ t('sessions.subtitle', { engine: t('engine.name') }) }}</p>
      </div>
      <div class="head-actions">
        <WbSeg v-model="scope" :items="scopeItems" />
        <button type="button"
          class="wb-btn icon ghost"
          :class="{ 'is-loading': refreshing }"
          :title="t('common.refresh')" :aria-label="t('common.refresh')"
          :disabled="refreshing"
          @click="refresh"
        >
          <AppIcon name="refresh" :size="15" />
        </button>
      </div>
    </header>

    <div class="page-body">
      <!-- 没选目录：这一页没有「当前目录」可讲，给一句说明与一个出口 -->
      <WbEmpty
        v-if="noWorkspace"
        class="ws-empty"
        icon="folder"
        :title="t('sessions.noWorkspaceTitle')"
        :hint="t('sessions.noWorkspaceHint')"
      >
        <template #action>
          <button
            class="wb-btn primary"
            type="button"
            :class="{ 'is-loading': choosingWorkspace }"
            :disabled="choosingWorkspace"
            @click="pickWorkspace"
          >
            {{ t('workspace.pickWorkspace') }}
          </button>
        </template>
      </WbEmpty>

      <template v-else>
      <!-- 汇总读数：目录里一条会话都没有时，那四个 0 会被当成「这个目录没花过钱」的结论。
           没有会话就没有合计，这一块换成一句说明。 -->
      <section v-if="counts.sessions" class="summary">
        <div class="wb-sec-head">
          <span class="wb-sec-title">{{ t('sessions.thisWorkspace') }}</span>
          <span class="wb-mono summary-scope selectable" :title="app.workspace">{{ app.workspace }}</span>
        </div>
        <div class="stat-row">
          <div class="wb-card card-pad stat">
            <span class="wb-metric-label">{{ t('nav.sessions') }}</span>
            <span class="wb-metric">{{ counts.sessions }}</span>
          </div>
          <div class="wb-card card-pad stat">
            <span class="wb-metric-label">{{ t('sessions.messages') }}</span>
            <span class="wb-metric">{{ counts.messages }}</span>
          </div>
          <div class="wb-card card-pad stat">
            <span class="wb-metric-label">{{ t('sessions.tokens') }}</span>
            <span class="wb-metric">{{ fmtCount(counts.tokens) }}</span>
          </div>
          <div class="wb-card card-pad stat">
            <span class="wb-metric-label">{{ t('sessions.cost') }}</span>
            <span class="wb-metric">{{ fmtCost(counts.cost) }}</span>
          </div>
        </div>
      </section>

      <section v-else class="summary summary-empty">
        <div class="wb-sec-head">
          <span class="wb-sec-title">{{ t('sessions.thisWorkspace') }}</span>
          <span class="wb-mono summary-scope selectable" :title="app.workspace">{{ app.workspace }}</span>
        </div>
        <p class="summary-hint">{{ t('sessions.noSummaryHint') }}</p>
      </section>

      <section class="list-panel">
        <SessionList
          ref="listRef"
          :workspace-only="scope === 'workspace'"
          @open="openSession"
          @changed="onChanged"
        />
      </section>
      </template>
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--wb-sp-4);
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-4);
  border-bottom: 1px solid var(--wb-line);
  flex: none;
}

.page-title {
  margin: 0;
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 650;
  color: var(--wb-ink);
}

.page-sub {
  margin: var(--wb-sp-1) 0 0;
  max-width: var(--wb-w-desc);
  font-size: var(--wb-fs-sm);
  line-height: 1.65;
  color: var(--wb-mute);
}

.head-actions {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: none;
}



.page-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
  padding: var(--wb-sp-4) var(--wb-sp-6) var(--wb-sp-6);
}

.summary {
  flex: none;
}

/* 一个会话都没有：合计栏换成一句说明，不摆四个 0 */
.summary-empty {
  border: 1px dashed var(--wb-line);
  border-radius: var(--wb-r-lg);
  padding: var(--wb-sp-3);
}

.summary-hint {
  margin: 0;
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-mute);
}

.summary-scope {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stat-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: var(--wb-sp-3);
}

/* 读数是纯文本：不套描边胶囊、不做悬停反馈 */
.stat {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}

.list-panel {
  flex: 1;
  min-height: 0;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-lg);
  background: var(--wb-surface);
  overflow: hidden;
}

/* 没选目录：整页给一句说明与一个出口，不再摆一屏 0 */
.ws-empty {
  flex: 1;
  justify-content: center;
}
</style>
