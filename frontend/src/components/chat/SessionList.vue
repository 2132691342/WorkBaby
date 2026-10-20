<script setup lang="ts">
// 侧栏会话列表：按时间分组 + 搜索 + 悬停动作（重命名 / 导出 / 删除）。
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSessionStore } from '../../stores/session'
import { useToastStore } from '../../stores/toast'
import { fmtRel } from '../../utils/time'
import { SaveFileDialog } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'

const emit = defineEmits<{ newSession: [] }>()

const { t } = useI18n()
const session = useSessionStore()
const toast = useToastStore()

const query = ref('')
const editingId = ref<string | null>(null)
const draft = ref('')
const busyId = ref('')
const exportingId = ref('')

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return session.list
  return session.list.filter((s) => (s.title || '').toLowerCase().includes(q))
})

// 分页：默认只渲染最新 10 条，「更多」每次追加 20 条
const PAGE = 10
const STEP = 20
const shown = ref(PAGE)

const groups = computed(() => {
  const today: typeof session.list = []
  const week: typeof session.list = []
  const earlier: typeof session.list = []
  const dayStart = new Date()
  dayStart.setHours(0, 0, 0, 0)
  const weekAgo = dayStart.getTime() - 6 * 86400_000
  for (const s of filtered.value.slice(0, query.value ? Number.MAX_SAFE_INTEGER : shown.value)) {
    if (s.updated_at >= dayStart.getTime()) today.push(s)
    else if (s.updated_at >= weekAgo) week.push(s)
    else earlier.push(s)
  }
  const out: Array<{ key: string; items: typeof session.list }> = []
  if (today.length) out.push({ key: 'today', items: today })
  if (week.length) out.push({ key: 'thisWeek', items: week })
  if (earlier.length) out.push({ key: 'earlier', items: earlier })
  return out
})
const hiddenCount = computed(() =>
  query.value ? 0 : Math.max(0, session.list.length - shown.value),
)

async function openSession(id: string) {
  try {
    await session.open(id)
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
  }
}

function startEdit(id: string, title: string) {
  editingId.value = id
  draft.value = title
}

async function commitEdit() {
  const id = editingId.value
  const title = draft.value.trim()
  editingId.value = null
  if (!id || !title) return
  try {
    await session.rename(id, title)
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  }
}

async function doRemove(id: string) {
  if (busyId.value) return
  busyId.value = id
  try {
    await session.remove(id)
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busyId.value = ''
  }
}

// 导出：先选保存位置再发请求（导出在 PI 里做，路径必须由用户定）
async function exportSession(s: { id: string; title: string }) {
  if (exportingId.value) return
  const name = (s.title || t('chat.newSession')).replace(/[\\/:*?"<>|]/g, '-').slice(0, 60)
  const target = await SaveFileDialog(t('chat.exportTitle'), `${name}.html`, '*.html')
  if (!target) return
  exportingId.value = s.id
  try {
    const path = await api.sessions.exportHTML(s.id, target)
    toast.ok(t('chat.noticeExport', { path }))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    exportingId.value = ''
  }
}

onBeforeUnmount(() => {
  editingId.value = null
})
</script>

<template>
  <div class="sess">
    <el-input
      v-model="query"
      class="sess-search"
      size="small"
      :placeholder="t('chat.searchSession')"
      clearable
    >
      <template #prefix><AppIcon name="search" size="ic-xs" /></template>
    </el-input>

    <div v-for="g in groups" :key="g.key" class="sess-group">
      <div class="wb-label sess-grp">{{ t(`chat.${g.key}`) }}</div>
      <TransitionGroup name="slist">
        <div
          v-for="s in g.items"
          :key="s.id"
          class="sess-row"
          :class="{ 'is-on': s.id === session.currentId, 'is-busy': session.openingId === s.id }"
        >
          <template v-if="editingId === s.id">
            <input
              v-model="draft"
              class="sess-rename"
              autofocus
              @keydown.enter="commitEdit"
              @keydown.esc="editingId = null"
              @blur="commitEdit"
            />
          </template>
          <template v-else>
            <button
              class="sess-hit"
              type="button"
              :disabled="!!session.openingId && session.openingId !== s.id"
              @click="openSession(s.id)"
            >
              <span class="sess-dot" :class="{ 'is-live': s.running }" />
              <span class="sess-name" :title="s.title">{{ s.title || t('chat.newSession') }}</span>
              <span class="sess-time">{{ fmtRel(s.updated_at) }}</span>
            </button>
            <span class="sess-acts">
              <el-tooltip :content="t('common.rename')" placement="top" :show-after="400">
                <button class="sess-act" type="button" @click.stop="startEdit(s.id, s.title)">
                  <AppIcon name="pencil" size="ic-2xs" />
                </button>
              </el-tooltip>
              <el-tooltip :content="t('chat.exportTitle')" placement="top" :show-after="400">
                <button
                  class="sess-act"
                  type="button"
                  :disabled="!!exportingId"
                  @click.stop="exportSession(s)"
                >
                  <AppIcon name="download" size="ic-2xs" />
                </button>
              </el-tooltip>
              <el-popconfirm
                :title="t('chat.deleteTitle')"
                :confirm-button-text="t('common.delete')"
                :cancel-button-text="t('common.cancel')"
                width="220"
                @confirm="doRemove(s.id)"
              >
                <template #reference>
                  <button class="sess-act is-danger" type="button" :disabled="!!busyId" @click.stop>
                    <AppIcon name="trash" size="ic-2xs" />
                  </button>
                </template>
              </el-popconfirm>
            </span>
          </template>
        </div>
      </TransitionGroup>
    </div>

    <button v-if="hiddenCount > 0" class="sess-more" type="button" @click="shown += STEP">
      {{ t('common.more') }} · {{ hiddenCount }}
    </button>

    <div v-if="!filtered.length" class="sess-empty">
      <span class="wb-tile"><AppIcon name="chat" size="ic-lg" /></span>
      <b>{{ query ? t('common.empty') : t('chat.noSessions') }}</b>
      <p>{{ t('chat.noSessionsHint') }}</p>
      <el-button v-if="!query" size="small" @click="emit('newSession')">{{ t('chat.newSession') }}</el-button>
    </div>
  </div>
</template>

<style scoped>
.sess {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}
.sess-search {
  margin-bottom: var(--wb-sp-2);
}
.sess-grp {
  padding: var(--wb-sp-3) var(--wb-sp-2) var(--wb-sp-1);
}
.sess-group {
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.sess-row {
  position: relative;
  display: flex;
  align-items: center;
  border-radius: var(--wb-radius-sm);
  transition:
    background var(--wb-dur) var(--wb-ease),
    box-shadow var(--wb-dur) var(--wb-ease);
}
.sess-row:hover {
  background: var(--wb-nav-hover-bg);
}
.sess-row.is-on {
  background: var(--wb-nav-active-bg);
  box-shadow: inset 2px 0 0 var(--wb-primary);
}
.sess-row.is-busy {
  opacity: 0.65;
}
.sess-hit {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2);
  text-align: left;
  border-radius: var(--wb-radius-sm);
  transition: transform var(--wb-dur-fast) var(--wb-ease);
}
.sess-hit:active:not(:disabled) {
  transform: scale(0.985);
}
.sess-hit:disabled {
  cursor: progress;
}
.sess-dot {
  width: 6px;
  height: 6px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-border-strong);
  flex: none;
}
.sess-dot.is-live {
  background: var(--wb-live);
  animation: wb-pulse 1.6s var(--wb-ease) infinite;
}
.sess-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
}
.sess-row.is-on .sess-name {
  color: var(--wb-nav-active-ink);
  font-weight: 700;
}
.sess-time {
  flex: none;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
  transition: opacity var(--wb-dur-fast) var(--wb-ease);
}
.sess-acts {
  position: absolute;
  right: var(--wb-sp-1);
  display: flex;
  align-items: center;
  gap: 1px;
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--wb-dur-fast) var(--wb-ease);
}
.sess-row:hover .sess-acts {
  opacity: 1;
  pointer-events: auto;
}
.sess-row:hover .sess-time {
  opacity: 0;
}
.sess-act {
  display: grid;
  place-items: center;
  width: calc(22px * var(--wb-vw-scale));
  height: calc(22px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  color: var(--wb-muted);
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.sess-act:hover:not(:disabled) {
  background: var(--wb-elev-raised);
  color: var(--wb-ink);
  transform: scale(1.06);
}
.sess-act:active:not(:disabled) {
  transform: scale(0.92);
}
.sess-act.is-danger:hover:not(:disabled) {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
.sess-act:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.sess-rename {
  width: 100%;
  height: var(--wb-ctl-h-sm);
  margin: var(--wb-sp-1) var(--wb-sp-2);
  padding: 0 var(--wb-sp-2);
  border: 1px solid var(--wb-primary);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-elev-raised);
  color: var(--wb-ink);
  font-size: var(--wb-fs-sm);
  outline: none;
}
.sess-more {
  margin-top: var(--wb-sp-2);
  width: 100%;
  height: var(--wb-ctl-h);
  border-radius: var(--wb-radius);
  border: 1px dashed var(--wb-border);
  color: var(--wb-muted);
  font-size: var(--wb-fs-xs);
  font-weight: 600;
  transition:
    color var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease),
    background var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.sess-more:hover {
  color: var(--wb-primary);
  border-color: var(--wb-primary-line);
  background: var(--wb-accent-tint);
  transform: scale(1.01);
}
.sess-more:active {
  transform: scale(0.98);
}
.sess-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8) var(--wb-sp-3);
  text-align: center;
}
.sess-empty b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.sess-empty p {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
}
.slist-enter-active,
.slist-leave-active {
  transition:
    opacity var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur) var(--wb-ease);
}
.slist-enter-from {
  opacity: 0;
  transform: translateX(-6px);
}
.slist-leave-to {
  opacity: 0;
  transform: scale(0.96);
}
</style>
