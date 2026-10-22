<script setup lang="ts">
// 提交面板：勾选要入这次提交的文件 → 填提交信息（可让助手起草）→ 确认后提交。
// 提交前一定列出将入的文件与增删行数；助手只起草、不代填也不代提。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import type { GitFileVO, GitStatusVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{ status: GitStatusVO | null; sessionId: string }>()
const emit = defineEmits<{ (e: 'done', status: GitStatusVO): void }>()

const { t } = useI18n()
const toast = useToastStore()

const picked = ref<Set<string>>(new Set())
const message = ref('')
const busy = ref('')
const drafting = ref(false)
const amend = ref(false)

const files = computed<GitFileVO[]>(() => props.status?.files || [])
const pickedFiles = computed(() => files.value.filter((f) => picked.value.has(f.path)))
const stagedFiles = computed(() => files.value.filter((f) => f.staged))

const sum = computed(() =>
  pickedFiles.value.reduce(
    (n, f) => {
      n.add += f.additions
      n.del += f.deletions
      return n
    },
    { add: 0, del: 0 },
  ),
)

function shortPath(p: string) {
  const parts = p.split('/').filter(Boolean)
  return parts.length > 3 ? '…/' + parts.slice(-3).join('/') : p
}

function toggle(path: string) {
  const next = new Set(picked.value)
  if (next.has(path)) next.delete(path)
  else next.add(path)
  picked.value = next
}

// 全选 / 取消全选：改动几十个文件时逐个点选是不现实的
const allPicked = computed(() => files.value.length > 0 && pickedFiles.value.length === files.value.length)
const somePicked = computed(() => pickedFiles.value.length > 0 && !allPicked.value)

function toggleAll() {
  if (busy.value) return
  const next = new Set<string>()
  if (!allPicked.value) for (const f of files.value) next.add(f.path)
  picked.value = next
}

// 换了一批文件（提交完 / 刷新后）就按新状态重算勾选：留着上一次的勾会提交错东西
function syncPicked() {
  const next = new Set<string>()
  for (const f of files.value) if (f.staged) next.add(f.path)
  picked.value = next
}
watch(() => props.status, syncPicked)
syncPicked()

async function stage() {
  const paths = pickedFiles.value.filter((f) => !f.staged).map((f) => f.path)
  if (!paths.length) return
  busy.value = 'stage'
  try {
    emit('done', await api.git.stage(paths, props.sessionId))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function unstageAll() {
  if (!stagedFiles.value.length) return
  busy.value = 'unstage'
  try {
    emit('done', await api.git.unstage(stagedFiles.value.map((f) => f.path), props.sessionId))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

// 撤销单个文件的改动：不可逆，因此走二次确认而不是直接执行
async function discardOne(path: string) {
  busy.value = 'discard:' + path
  try {
    emit('done', await api.git.discard([path], props.sessionId))
    toast.ok(t('workspace.discardOk', { n: 1 }))
  } catch (e) {
    toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
  } finally {
    busy.value = ''
  }
}

async function draft() {
  if (drafting.value) return
  drafting.value = true
  try {
    const res = await api.git.draftMessage(pickedFiles.value.map((f) => f.path), props.sessionId)
    if (res.err) {
      toast.bad(t('common.saveFailed', { msg: res.err }))
      return
    }
    message.value = res.message
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    drafting.value = false
  }
}

async function commit() {
  if (!message.value.trim()) {
    toast.bad(t('workspace.commitEmpty'))
    return
  }
  busy.value = 'commit'
  try {
    const res = await api.git.commit_(message.value.trim(), props.sessionId, amend.value)
    message.value = ''
    const wasAmend = amend.value
    amend.value = false
    emit('done', res.status)
    toast.ok(
      wasAmend
        ? t('workspace.amendOk', { hash: res.hash.slice(0, 7) })
        : t('workspace.commitOk', { hash: res.hash.slice(0, 7) }),
    )
  } catch (e) {
    toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
  } finally {
    busy.value = ''
  }
}

const canCommit = computed(() => pickedFiles.value.length > 0 && !!message.value.trim() && !busy.value)
const needStage = computed(() => pickedFiles.value.some((f) => !f.staged))
// 有冲突时 git 不接受提交，这时要先把冲突文件标记已解决
const hasConflict = computed(() => files.value.some((f) => f.conflict))
</script>

<template>
  <div class="cmt">
    <div class="cmt-files">
      <header class="cmt-head">
        <el-tooltip
          :content="allPicked ? t('workspace.unselectAll') : t('workspace.selectAll')"
          placement="top"
          :show-after="400"
        >
          <el-checkbox
            :model-value="allPicked"
            :indeterminate="somePicked"
            :disabled="!!busy || !files.length"
            @change="toggleAll"
          />
        </el-tooltip>
        <span class="wb-label">{{ t('workspace.files') }}</span>
        <span class="cmt-count">{{ pickedFiles.length }} / {{ files.length }}</span>
        <span class="sp" />
        <el-button
          size="small"
          text
          :disabled="!!busy || !pickedFiles.length || !needStage"
          @click="stage"
        >
          {{ t('workspace.stage') }}
        </el-button>
        <el-button size="small" text :disabled="!!busy || !stagedFiles.length" @click="unstageAll">
          {{ t('workspace.unstageAll') }}
        </el-button>
      </header>

      <div v-if="!files.length" class="cmt-empty">{{ t('workspace.noChanges') }}</div>
      <ul v-else class="cmt-list">
        <li v-for="f in files" :key="f.path">
          <div class="cmt-row" :class="{ 'is-on': picked.has(f.path), 'is-conflict': f.conflict }">
            <el-checkbox
              :model-value="picked.has(f.path)"
              :disabled="!!busy"
              @change="toggle(f.path)"
            />
            <AppIcon :name="f.conflict ? 'alert' : 'doc'" size="ic-xs" />
            <span class="cmt-path" :title="f.path">{{ shortPath(f.path) }}</span>
            <el-tag v-if="f.conflict" size="small" type="warning">{{ t('workspace.conflict') }}</el-tag>
            <el-tag v-else-if="f.staged" size="small" type="primary">{{ t('workspace.staged') }}</el-tag>
            <span class="cmt-stat">
              <b class="is-add">+{{ f.additions }}</b>
              <b class="is-del">−{{ f.deletions }}</b>
            </span>
            <el-popconfirm
              :title="t('workspace.discardConfirm')"
              :confirm-button-text="t('workspace.discard')"
              :cancel-button-text="t('common.cancel')"
              width="250"
              @confirm="discardOne(f.path)"
            >
              <template #reference>
                <button
                  class="cmt-drop"
                  type="button"
                  :disabled="!!busy"
                  :title="t('workspace.discardHint')"
                >
                  <AppIcon name="trash" size="ic-2xs" />
                </button>
              </template>
            </el-popconfirm>
          </div>
        </li>
      </ul>
    </div>

    <div class="cmt-form">
      <div class="cmt-sum">
        <span class="wb-label">{{ t('workspace.commitMessage') }}</span>
        <span class="sp" />
        <span class="cmt-stat">
          <b class="is-add">+{{ sum.add }}</b>
          <b class="is-del">−{{ sum.del }}</b>
        </span>
      </div>
      <el-input
        v-model="message"
        type="textarea"
        :rows="4"
        :placeholder="t('workspace.commitPlaceholder')"
        :disabled="!!busy"
        maxlength="400"
        show-word-limit
      />

      <!-- amend 会改写已有提交：放着不动最安全，打开时把代价写在旁边 -->
      <el-checkbox v-model="amend" :disabled="!!busy" class="cmt-amend">
        {{ t('workspace.amend') }}
      </el-checkbox>
      <p v-if="amend" class="cmt-warn">
        <AppIcon name="alert" size="ic-2xs" />
        {{ t('workspace.amendHint') }}
      </p>

      <div class="cmt-acts">
        <el-button size="small" :loading="drafting" :disabled="!!busy || !pickedFiles.length" @click="draft">
          {{ drafting ? t('workspace.committing') : t('workspace.commitDraft') }}
        </el-button>
        <span class="sp" />
        <el-button
          size="small"
          type="primary"
          :loading="busy === 'commit'"
          :disabled="!canCommit || hasConflict"
          @click="commit"
        >
          {{ t('workspace.commitAction') }}
        </el-button>
      </div>

      <p v-if="hasConflict" class="cmt-hint">
        <AppIcon name="info" size="ic-2xs" />
        {{ t('workspace.conflictResolveHint') }}
      </p>
      <p v-else-if="needStage && pickedFiles.length" class="cmt-hint">
        <AppIcon name="info" size="ic-2xs" />
        {{ t('workspace.stage') }} · {{ t('workspace.commitNothing') }}
      </p>
    </div>
  </div>
</template>

<style scoped>
.cmt {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 380px);
  gap: var(--wb-sp-4);
  align-items: start;
}
.cmt-files {
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-lg);
  overflow: hidden;
}
.cmt-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-bottom: 1px solid var(--wb-border);
  background: var(--wb-elev-hover);
}
.cmt-count {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.cmt-list {
  max-height: calc(420px * var(--wb-vw-scale));
  overflow-y: auto;
}
.cmt-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  cursor: default;
  transition: background var(--wb-dur-fast) var(--wb-ease);
}
.cmt-row:hover {
  background: var(--wb-elev-hover);
}
.cmt-row.is-on {
  background: var(--wb-accent-tint);
}
.cmt-row.is-conflict {
  box-shadow: inset 2px 0 0 var(--wb-warning);
}
.cmt-drop {
  display: grid;
  place-items: center;
  width: var(--wb-ctl-icon);
  height: var(--wb-ctl-icon);
  border-radius: var(--wb-radius-sm);
  color: var(--wb-muted);
  opacity: 0;
  transition:
    opacity var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    background var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.cmt-row:hover .cmt-drop {
  opacity: 1;
}
.cmt-drop:hover:not(:disabled) {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
  transform: scale(1.06);
}
.cmt-drop:active:not(:disabled) {
  transform: scale(0.92);
}
.cmt-drop:disabled {
  opacity: 0;
  cursor: not-allowed;
}
.cmt-amend {
  align-self: flex-start;
}
.cmt-warn {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-warning);
  line-height: var(--wb-lh-base);
}
.cmt-path {
  flex: 1;
  min-width: 0;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cmt-stat {
  display: flex;
  gap: var(--wb-sp-2);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  flex: none;
}
.cmt-stat .is-add {
  color: var(--wb-success);
}
.cmt-stat .is-del {
  color: var(--wb-danger);
}
.cmt-empty {
  padding: var(--wb-sp-6);
  text-align: center;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.cmt-form {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.cmt-sum {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.cmt-acts {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.cmt-hint {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.sp {
  flex: 1;
}
@media (max-width: 980px) {
  .cmt {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
