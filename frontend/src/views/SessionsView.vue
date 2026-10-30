<script setup lang="ts">
// 会话页：全部会话的清单台。左侧按时间分组的列表，右侧是选中会话的概要与操作。
// 与对话页共用 session store——同一份数据，不另存账本。
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useSessionStore } from '../stores/session'
import { useToastStore } from '../stores/toast'
import * as api from '../api'
import { fmtRel } from '../utils/time'
import { pickFileSave } from '../api/shell'
import AppIcon from '../components/common/AppIcon.vue'
import PageHeader from '../components/layout/PageHeader.vue'

const { t } = useI18n()
const router = useRouter()
const session = useSessionStore()
const toast = useToastStore()

const query = ref('')
const selected = ref<string | null>(null)
const busyId = ref('')

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return session.list
  return session.list.filter(
    (s) =>
      (s.session_title || '').toLowerCase().includes(q) ||
      (s.session_model || '').toLowerCase().includes(q),
  )
})

// 按时间分组：今天 / 本周 / 更早
const groups = computed(() => {
  const today: typeof session.list = []
  const week: typeof session.list = []
  const earlier: typeof session.list = []
  const dayStart = new Date()
  dayStart.setHours(0, 0, 0, 0)
  const weekAgo = dayStart.getTime() - 6 * 86_400_000
  for (const s of filtered.value) {
    if (s.session_updated_at >= dayStart.getTime()) today.push(s)
    else if (s.session_updated_at >= weekAgo) week.push(s)
    else earlier.push(s)
  }
  const out: Array<{ key: string; items: typeof session.list }> = []
  if (today.length) out.push({ key: 'today', items: today })
  if (week.length) out.push({ key: 'thisWeek', items: week })
  if (earlier.length) out.push({ key: 'earlier', items: earlier })
  return out
})

const current = computed(() =>
  selected.value ? session.list.find((s) => s.session_id === selected.value) ?? null : null,
)

async function load() {
  try {
    await session.loadList()
    // 默认选中最近一条
    if (!selected.value && session.list.length) {
      selected.value = [...session.list].sort(
        (a, b) => b.session_updated_at - a.session_updated_at,
      )[0].session_id
    }
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
  }
}

onMounted(load)
watch(() => session.list.length, load)

function openInChat() {
  if (!selected.value) return
  void router.push({ path: '/', query: { session: selected.value } })
}

async function rename(s: { session_id: string; session_title: string }) {
  const title = window.prompt(t('common.rename'), s.session_title)
  if (!title || title === s.session_title) return
  busyId.value = s.session_id
  try {
    await session.rename(s.session_id, title)
    toast.ok(t('common.save'))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busyId.value = ''
  }
}

async function remove(s: { session_id: string }) {
  if (busyId.value) return
  busyId.value = s.session_id
  try {
    await session.remove(s.session_id)
    if (selected.value === s.session_id) selected.value = null
    toast.ok(t('common.delete'))
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busyId.value = ''
  }
}

async function exportSession(s: { session_id: string; session_title: string }) {
  if (busyId.value) return
  const name = (s.session_title || t('chat.newSession')).replace(/[\\/:*?"<>|]/g, '-').slice(0, 60)
  const target = await pickFileSave(t('chat.exportTitle'), `${name}.html`, '*.html')
  if (!target) return
  busyId.value = s.session_id
  try {
    const path = await api.sessions.exportHTML(s.session_id, target)
    toast.ok(t('chat.noticeExport', { path }))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busyId.value = ''
  }
}
</script>

<template>
  <div class="view">
    <PageHeader :title="t('sessions.title')" :subtitle="t('sessions.subtitle')">
      <template #actions>
        <el-button size="small" type="primary" @click="router.push('/')">
          {{ t('chat.newSession') }}
        </el-button>
      </template>
    </PageHeader>

    <div class="body">
      <div class="split">
        <!-- 左：清单 -->
        <div class="pane">
          <el-input v-model="query" size="small" :placeholder="t('chat.searchSession')" clearable>
            <template #prefix><AppIcon name="search" size="ic-xs" /></template>
          </el-input>

          <div v-if="!session.list.length" class="empty">
            <span class="wb-tile"><AppIcon name="chat" size="ic-lg" /></span>
            <b>{{ t('chat.noSessions') }}</b>
            <p>{{ t('chat.noSessionsHint') }}</p>
          </div>

          <div v-for="g in groups" :key="g.key" class="grp">
            <div class="wb-label grp-cap">{{ t(`chat.${g.key}`) }}</div>
            <button
              v-for="s in g.items"
              :key="s.session_id"
              type="button"
              class="row"
              :class="{ 'is-on': s.session_id === selected }"
              @click="selected = s.session_id"
            >
              <span class="row-dot" :class="{ 'is-live': s.session_running }" />
              <span class="row-tx">
                <span class="row-name">{{ s.session_title || t('chat.newSession') }}</span>
                <span class="row-meta">{{ fmtRel(s.session_updated_at) }}</span>
              </span>
            </button>
          </div>
        </div>

        <!-- 右：概要与操作 -->
        <div class="pane detail">
          <template v-if="current">
            <h2 class="d-title">{{ current.session_title || t('chat.newSession') }}</h2>
            <dl class="d-grid">
              <div class="d-row">
                <dt>{{ t('sessions.model') }}</dt>
                <dd>{{ current.session_model || '—' }}</dd>
              </div>
              <div class="d-row">
                <dt>{{ t('sessions.provider') }}</dt>
                <dd>{{ current.session_provider || '—' }}</dd>
              </div>
              <div class="d-row">
                <dt>{{ t('sessions.messages') }}</dt>
                <dd>{{ current.session_message_count }}</dd>
              </div>
              <div class="d-row">
                <dt>{{ t('sessions.tokens') }}</dt>
                <dd>{{ current.session_total_tokens }}</dd>
              </div>
              <div class="d-row">
                <dt>{{ t('sessions.cost') }}</dt>
                <dd>{{ current.session_cost }}</dd>
              </div>
              <div class="d-row">
                <dt>{{ t('sessions.workspace') }}</dt>
                <dd class="d-path" :title="current.session_workspace">{{ current.session_workspace || '—' }}</dd>
              </div>
            </dl>

            <div class="d-acts">
              <el-button size="small" type="primary" @click="openInChat">
                {{ t('sessions.open') }}
              </el-button>
              <el-button size="small" :disabled="!!busyId" @click="rename(current)">
                {{ t('common.rename') }}
              </el-button>
              <el-button size="small" :disabled="!!busyId" @click="exportSession(current)">
                {{ t('chat.exportTitle') }}
              </el-button>
              <el-popconfirm
                :title="t('chat.deleteTitle')"
                :confirm-button-text="t('common.delete')"
                :cancel-button-text="t('common.cancel')"
                width="220"
                @confirm="remove(current)"
              >
                <template #reference>
                  <el-button size="small" type="danger" plain :disabled="!!busyId">
                    {{ t('common.delete') }}
                  </el-button>
                </template>
              </el-popconfirm>
            </div>
          </template>

          <div v-else class="empty">
            <span class="wb-tile wb-tile--neutral"><AppIcon name="doc" size="ic-lg" /></span>
            <b>{{ t('sessions.noSelection') }}</b>
            <p>{{ t('sessions.noSelectionHint') }}</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.view {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  min-height: 0;
}
.body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5);
}
.split {
  display: grid;
  grid-template-columns: minmax(240px, 320px) 1fr;
  gap: var(--wb-sp-4);
  align-items: start;
}
.pane {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.detail {
  padding: var(--wb-sp-5);
  border-radius: var(--wb-radius-xl);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
}
.grp {
  display: flex;
  flex-direction: column;
  gap: 1px;
  margin-top: var(--wb-sp-2);
}
.grp-cap {
  padding: var(--wb-sp-1) 0;
}
.row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  text-align: left;
  cursor: pointer;
  transition:
    background var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.row:hover {
  background: var(--wb-elev-hover);
  transform: translateX(2px);
}
.row:active {
  transform: translateX(2px) scale(0.99);
}
.row.is-on {
  background: var(--wb-accent-tint);
}
.row-dot {
  width: 6px;
  height: 6px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-border-strong);
  flex: none;
}
.row-dot.is-live {
  background: var(--wb-live);
  box-shadow: 0 0 0 3px var(--wb-accent-tint);
}
.row-tx {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.row-name {
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.row-meta {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.d-title {
  font-family: var(--font-display);
  font-size: var(--wb-fs-lg);
  font-weight: 700;
  color: var(--wb-ink);
}
.d-grid {
  margin: var(--wb-sp-4) 0;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.d-row {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-3);
}
.d-row dt {
  flex: none;
  width: calc(88px * var(--wb-vw-scale));
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--wb-muted);
}
.d-row dd {
  flex: 1;
  min-width: 0;
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.d-path {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs) !important;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.d-acts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-2);
  padding-top: var(--wb-sp-3);
  border-top: 1px solid var(--wb-border);
}
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8) var(--wb-sp-6);
  text-align: center;
}
.empty b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.empty p {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
</style>
