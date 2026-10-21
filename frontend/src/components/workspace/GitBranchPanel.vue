<script setup lang="ts">
// 分支面板：列分支、切分支、新建、合并 / 变基、删分支、拉取、推送（含分级的强制推送）。
// 强制推送不在白名单之外——它默认就没有，必须用户显式选级别；纯 --force 还要输分支名确认。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import type { GitBranchVO, GitForce, GitStatusVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{ sessionId: string; status: GitStatusVO | null }>()
const emit = defineEmits<{ (e: 'done', status: GitStatusVO): void }>()

const { t } = useI18n()
const toast = useToastStore()

const branches = ref<GitBranchVO[]>([])
const loading = ref(true)
const busy = ref('')
const creating = ref(false)
const newName = ref('')
// 强制推送的确认态：级别 + 要输入的分支名，两者都对上才执行
const forceOpen = ref(false)
const forceLevel = ref<GitForce>('lease')
const forceConfirm = ref('')

const locals = computed(() => branches.value.filter((b) => !b.remote))
const remotes = computed(() => branches.value.filter((b) => !!b.remote))
const current = computed(() => branches.value.find((b) => b.current))
const hasUpstream = computed(() => !!props.status?.upstream)
const dirty = computed(() => (props.status?.files || []).length > 0)

async function load() {
  loading.value = true
  try {
    branches.value = await api.git.branches(props.sessionId)
  } catch (e) {
    branches.value = []
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    loading.value = false
  }
}
onMounted(load)

function fail(e: unknown) {
  toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
}

async function run(key: string, fn: () => Promise<GitStatusVO>, ok?: string) {
  if (busy.value) return
  busy.value = key
  try {
    emit('done', await fn())
    if (ok) toast.ok(ok)
    await load()
  } catch (e) {
    fail(e)
  } finally {
    busy.value = ''
  }
}

function switchTo(name: string) {
  void run('switch:' + name, () => api.git.switchBranch(name, props.sessionId))
}

function checkoutRemote(name: string) {
  // 切远端分支要让 git 建一条本地跟踪分支，否则会落到 detached HEAD
  void run('switch:' + name, () => api.git.switchBranch(name, props.sessionId))
}

function create() {
  const name = newName.value.trim()
  if (!name) {
    toast.bad(t('workspace.branchName'))
    return
  }
  void run('create', () => api.git.createBranch(name, props.sessionId), '').then(() => {
    newName.value = ''
    creating.value = false
  })
}

function merge(name: string, rebase: boolean) {
  const key = (rebase ? 'rebase:' : 'merge:') + name
  void run(key, () => api.git.merge(name, rebase, props.sessionId))
}

function del(name: string) {
  void run('del:' + name, () => api.git.deleteBranch(name, props.sessionId))
}

function pull() {
  void run('pull', () => api.git.pull({}, props.sessionId), t('workspace.pulled'))
}

function fetch() {
  void run('fetch', () => api.git.fetch('', props.sessionId), t('workspace.fetched'))
}

function push() {
  void run('push', () => api.git.push({}, props.sessionId), t('workspace.pushed'))
}

function pushForce() {
  const branch = current.value?.name || props.status?.branch || ''
  if (forceLevel.value === 'force' && forceConfirm.value.trim() !== branch) {
    toast.bad(t('workspace.pushForceMismatch'))
    return
  }
  void run(
    'push-force',
    () =>
      api.git.push(
        {
          force: forceLevel.value,
          // 只有纯 --force 需要确认串；lease 靠远端租约自证安全
          confirm: forceLevel.value === 'force' ? forceConfirm.value.trim() : undefined,
        },
        props.sessionId,
      ),
    t('workspace.pushed'),
  ).then(() => {
    forceOpen.value = false
    forceConfirm.value = ''
  })
}
</script>

<template>
  <div class="br">
    <header class="br-head">
      <span class="br-cur">
        <AppIcon name="git-branch" size="ic-sm" />
        {{ current?.name || status?.branch || t('workspace.branchEmpty') }}
      </span>
      <el-tag v-if="!hasUpstream" size="small" type="info">{{ t('workspace.upstreamNone') }}</el-tag>
      <span class="sp" />
      <el-button size="small" :loading="busy === 'fetch'" :disabled="!!busy" @click="fetch">
        {{ busy === 'fetch' ? t('workspace.fetching') : t('workspace.fetch') }}
      </el-button>
      <el-button size="small" :loading="busy === 'pull'" :disabled="!!busy" @click="pull">
        {{ busy === 'pull' ? t('workspace.pulling') : t('workspace.pull') }}
      </el-button>
      <el-button size="small" :loading="busy === 'push'" :disabled="!!busy" @click="push">
        {{ busy === 'push' ? t('workspace.pushing') : t('workspace.push') }}
      </el-button>
      <el-button size="small" type="primary" :disabled="!!busy" @click="creating = !creating">
        {{ t('workspace.branchNew') }}
      </el-button>
    </header>

    <div v-if="creating" class="br-new">
      <el-input
        v-model="newName"
        size="small"
        :placeholder="t('workspace.branchName')"
        @keydown.enter="create"
      />
      <el-button size="small" type="primary" :loading="busy === 'create'" @click="create">
        {{ t('common.create') }}
      </el-button>
      <el-button size="small" @click="creating = false">{{ t('common.cancel') }}</el-button>
    </div>

    <div v-if="loading" class="br-state"><span class="spin" />{{ t('common.loading') }}</div>

    <template v-else>
      <ul v-if="locals.length" class="br-list">
        <li v-for="b in locals" :key="'l:' + b.name">
          <div class="br-row" :class="{ 'is-on': b.current }">
            <span class="br-dot" :class="{ 'is-on': b.current }" />
            <span class="br-name" :title="b.name">{{ b.name }}</span>
            <el-tag v-if="b.current" size="small" type="primary">{{ t('workspace.branchCurrent') }}</el-tag>
            <span v-if="b.ahead" class="br-num">{{ t('workspace.ahead', { n: b.ahead }) }}</span>
            <span v-if="b.behind" class="br-num">{{ t('workspace.behind', { n: b.behind }) }}</span>
            <span class="sp" />
            <template v-if="!b.current">
              <el-button
                size="small"
                text
                :loading="busy === 'switch:' + b.name"
                :disabled="!!busy || dirty"
                :title="dirty ? t('workspace.discardHint') : ''"
                @click="switchTo(b.name)"
              >
                {{ t('workspace.branchSwitch') }}
              </el-button>
              <el-button
                size="small"
                text
                :loading="busy === 'merge:' + b.name"
                :disabled="!!busy || dirty || !!status?.state"
                @click="merge(b.name, false)"
              >
                {{ t('workspace.merge') }}
              </el-button>
              <el-button
                size="small"
                text
                :loading="busy === 'rebase:' + b.name"
                :disabled="!!busy || dirty || !!status?.state"
                @click="merge(b.name, true)"
              >
                {{ t('workspace.rebase') }}
              </el-button>
              <el-popconfirm
                :title="t('workspace.branchDeleteConfirm', { name: b.name })"
                :confirm-button-text="t('workspace.branchDelete')"
                :cancel-button-text="t('common.cancel')"
                width="260"
                @confirm="del(b.name)"
              >
                <template #reference>
                  <button class="br-drop" type="button" :disabled="!!busy" :title="t('workspace.branchDeleteHint')">
                    <AppIcon name="trash" size="ic-2xs" />
                  </button>
                </template>
              </el-popconfirm>
            </template>
          </div>
        </li>
      </ul>
      <div v-else class="br-state">{{ t('workspace.branchEmpty') }}</div>

      <template v-if="remotes.length">
        <div class="br-div">{{ t('workspace.branchRemote') }}</div>
        <ul class="br-list">
          <li v-for="b in remotes" :key="'r:' + b.name">
            <div class="br-row">
              <span class="br-dot" />
              <span class="br-name is-remote" :title="b.name">{{ b.name }}</span>
              <span class="sp" />
              <el-button
                size="small"
                text
                :loading="busy === 'switch:' + b.name"
                :disabled="!!busy || dirty"
                @click="checkoutRemote(b.name)"
              >
                {{ t('workspace.branchSwitch') }}
              </el-button>
              <el-button
                size="small"
                text
                :loading="busy === 'merge:' + b.name"
                :disabled="!!busy || dirty || !!status?.state"
                @click="merge(b.name, false)"
              >
                {{ t('workspace.merge') }}
              </el-button>
            </div>
          </li>
        </ul>
      </template>
    </template>

    <!-- 危险区：默认折叠，展开后必须先选级别，纯 force 还要输分支名 -->
    <div class="br-danger">
      <button class="br-danger-head" type="button" @click="forceOpen = !forceOpen">
        <AppIcon name="chevron" size="ic-2xs" :class="{ 'is-open': forceOpen }" />
        {{ t('workspace.pushForce') }}
      </button>
      <div v-if="forceOpen" class="br-danger-body">
        <label class="br-opt">
          <el-radio v-model="forceLevel" value="lease" :disabled="!!busy">
            {{ t('workspace.pushForce') }}
          </el-radio>
          <span class="br-opt-hint">{{ t('workspace.pushForceHint') }}</span>
        </label>
        <label class="br-opt">
          <el-radio v-model="forceLevel" value="force" :disabled="!!busy">
            {{ t('workspace.pushForceTrue') }}
          </el-radio>
          <span class="br-opt-hint is-warn">{{ t('workspace.pushForceTrueHint') }}</span>
        </label>
        <el-input
          v-if="forceLevel === 'force'"
          v-model="forceConfirm"
          size="small"
          class="wb-mono"
          :placeholder="t('workspace.pushForceConfirm', { branch: current?.name || status?.branch || '' })"
        />
        <div class="br-danger-acts">
          <span class="sp" />
          <el-button size="small" :disabled="!!busy" @click="forceOpen = false">
            {{ t('common.cancel') }}
          </el-button>
          <el-button
            size="small"
            type="danger"
            :loading="busy === 'push-force'"
            :disabled="!!busy"
            @click="pushForce"
          >
            {{ t('workspace.pushForce') }}
          </el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.br {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.br-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.br-cur {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
}
.br-new {
  display: flex;
  gap: var(--wb-sp-2);
  animation: wb-sink var(--wb-dur) var(--wb-ease);
}
.br-list {
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.br-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  transition: background var(--wb-dur-fast) var(--wb-ease);
}
.br-row:hover {
  background: var(--wb-elev-hover);
}
.br-row.is-on {
  background: var(--wb-accent-tint);
}
.br-dot {
  width: calc(7px * var(--wb-vw-scale));
  height: calc(7px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  background: var(--wb-border-strong);
  flex: none;
}
.br-dot.is-on {
  background: var(--wb-primary);
}
.br-name {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.br-name.is-remote {
  color: var(--wb-ink-2);
  font-weight: 500;
}
.br-num {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.br-div {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  padding: var(--wb-sp-1) 0;
}
.br-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8) 0;
  color: var(--wb-muted);
  font-size: var(--wb-fs-sm);
}
.br-drop {
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
.br-row:hover .br-drop {
  opacity: 1;
}
.br-drop:hover:not(:disabled) {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
  transform: scale(1.06);
}
.br-drop:active:not(:disabled) {
  transform: scale(0.92);
}
/* 危险区默认收起：这一整块平时不该出现在视线里 */
.br-danger {
  border-top: 1px solid var(--wb-border);
  padding-top: var(--wb-sp-2);
}
.br-danger-head {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  transition: color var(--wb-dur-fast) var(--wb-ease);
}
.br-danger-head:hover {
  color: var(--wb-danger);
}
.br-danger-head :deep(svg) {
  transition: transform var(--wb-dur-fast) var(--wb-ease);
}
.br-danger-head :deep(svg.is-open) {
  transform: rotate(90deg);
}
.br-danger-body {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding-top: var(--wb-sp-2);
  animation: wb-sink var(--wb-dur) var(--wb-ease);
}
.br-opt {
  display: flex;
  flex-direction: column;
  gap: calc(var(--wb-sp-1) * 0.5);
  cursor: pointer;
}
.br-opt-hint {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
  padding-left: var(--wb-sp-5);
}
.br-opt-hint.is-warn {
  color: var(--wb-danger);
}
.br-danger-acts {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.spin {
  width: calc(13px * var(--wb-vw-scale));
  height: calc(13px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  border: 1.8px solid var(--wb-border-strong);
  border-top-color: var(--wb-primary);
  animation: wb-spin 0.8s linear infinite;
}
.sp {
  flex: 1;
}
@keyframes wb-sink {
  from {
    opacity: 0;
    transform: translateY(calc(-4px * var(--wb-vw-scale)));
  }
  to {
    opacity: 1;
    transform: none;
  }
}
</style>
