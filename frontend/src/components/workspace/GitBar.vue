<script setup lang="ts">
// 版本栏：当前分支、上游、领先 / 落后，以及取回 / 拉取 / 推送。
// 每个控件有自己的前置条件（远端 / 上游 / 强推），只负责按状态决定显示什么；
// loading 按动作名区分——共用一个「忙」标志，用户不知道哪一步在跑。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { GitStatus } from '@/types/api'

const props = defineProps<{ status: GitStatus | null; pending: string }>()

const emit = defineEmits<{
  fetch: []
  pull: [isRebase: boolean]
  push: [force: string, confirm?: string]
  addRemote: []
}>()

const { t } = useI18n()

/** 有远端才能同步。没有远端时按钮不渲染（不渲染成灰按钮）。 */
const hasRemote = computed(() => (props.status?.remotes?.length ?? 0) > 0)

/** 有上游才有「推送到上游」这个确定动作；没有就退到「推送并建立跟踪」。 */
const hasUpstream = computed(() => Boolean(props.status?.upstream_ref))

/** 强推只有带租约一档（裸 --force 会静默抹掉别人的提交，后端不提供）；确认要输分支名。 */
const forceOpen = ref(false)
const confirmText = ref('')
/** 拉取的变基选项藏在拆分按钮里：普通拉取仍是单击直达。 */
const pullOpen = ref(false)

/** 确认文本必须与分支名完全一致，否则强推按钮不给点。 */
const canForce = computed(() => confirmText.value === (props.status?.branch_name ?? ''))

/** 某个动作是否在跑：pending 是动作名（见 useGitActions）。 */
function busy(name: string): boolean {
  return props.pending === name
}

/** 同步类动作是否在跑（拉取 / 取回 / 推送）：它们共用远端这一把锁。 */
function syncing(): boolean {
  return props.pending.startsWith('pull') || props.pending.startsWith('fetch') || props.pending.startsWith('push')
}

function submitForce() {
  if (!canForce.value) return
  emit('push', 'with-lease', confirmText.value)
  forceOpen.value = false
  confirmText.value = ''
}
</script>

<template>
  <div class="git-bar">
    <span class="git-chip">
      <AppIcon name="branch" :size="13" />
      <b class="git-branch">{{ status?.branch_name || t('workspace.noCommit') }}</b>
    </span>

    <span v-if="status?.upstream_ref" class="git-chip git-upstream">
      <span class="git-upstream-name">{{ status.upstream_ref }}</span>
    </span>

    <span v-if="status && (status.ahead || status.behind)" class="git-chip git-count">
      <b v-if="status.ahead" class="is-ahead">↑{{ status.ahead }}</b>
      <b v-if="status.behind" class="is-behind">↓{{ status.behind }}</b>
    </span>

    <span v-if="!hasRemote" class="git-hint">{{ t('workspace.noRemote') }}</span>

    <span class="git-sp" />

    <!-- 没有远端时唯一的出口：先能加一个远端，相步类按钮才有意义 -->
    <button v-if="!hasRemote" class="wb-btn sm primary" @click="emit('addRemote')">
      <AppIcon name="plus" :size="13" />
      {{ t('workspace.addRemote') }}
    </button>

    <button
      v-if="hasRemote"
      class="wb-btn sm ghost"
      :class="{ 'is-loading': busy('fetch') }"
      :disabled="syncing()"
      :title="t('workspace.fetch')"
      @click="emit('fetch')"
    >
      <AppIcon name="refresh" :size="13" />
      {{ t('workspace.fetch') }}
    </button>

    <!-- 拉取是拆分按钮：单击普通拉取；变基这种换历史的变体收进菜单。 -->
    <span v-if="hasRemote" class="pull-split">
      <button
        class="wb-btn sm ghost"
        :class="{ 'is-loading': busy('pull') }"
        :disabled="syncing()"
        :title="t('workspace.pull')"
        @click="pullOpen = false; emit('pull', false)"
      >
        <AppIcon name="download" :size="13" />
        {{ t('workspace.pull') }}
      </button>
      <button
        class="wb-btn sm ghost pull-caret"
        :disabled="syncing()"
        :title="t('workspace.pullRebase')"
        @click="pullOpen = !pullOpen"
      >
        <AppIcon name="chevron" :size="10" />
      </button>
      <button
        v-if="pullOpen"
        class="wb-btn sm ghost"
        :disabled="syncing()"
        :title="t('workspace.pullRebaseHint')"
        @click="pullOpen = false; emit('pull', true)"
      >
        {{ t('workspace.pullRebase') }}
      </button>
    </span>

    <button
      v-if="hasRemote"
      class="wb-btn sm primary"
      :class="{ 'is-loading': busy('push:none') }"
      :disabled="syncing()"
      :title="hasUpstream ? t('workspace.push') : t('workspace.pushFirst')"
      @click="emit('push', 'none')"
    >
      <AppIcon name="send" :size="13" />
      {{ hasUpstream ? t('workspace.push') : t('workspace.pushFirst') }}
    </button>

    <!-- 强推：单独一个入口，且要二次确认。
         强推会覆盖别人的提交，所以它不能是普通推送的「高级选项」，
         得让人显式点出来。 -->
    <button
      v-if="hasRemote"
      class="wb-btn sm ghost"
      :disabled="syncing()"
      :title="t('workspace.forcePush')"
      @click="forceOpen = !forceOpen"
    >
      {{ t('workspace.forcePush') }}
    </button>

    <div v-if="forceOpen" class="force-panel">
      <div class="force-head">{{ t('workspace.forcePush') }}</div>
      <label class="force-opt">
        <span>
          <b>{{ t('workspace.forceLease') }}</b>
          <em>{{ t('workspace.forceLeaseHint') }}</em>
        </span>
      </label>
      <div class="force-confirm">
        <input
          v-model="confirmText"
          class="wb-input"
          type="text"
          :placeholder="t('workspace.forceConfirmPlaceholder', { name: status?.branch_name ?? '' })"
        />
        <span class="force-confirm-hint">{{ t('workspace.forceConfirmHint') }}</span>
      </div>
      <div class="force-actions">
        <button class="wb-btn sm ghost" @click="forceOpen = false">{{ t('common.cancel') }}</button>
        <button class="wb-btn sm danger" :disabled="!canForce" @click="submitForce">
          {{ t('workspace.forcePush') }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 版本栏：一条横杠，把「现在在哪个分支、与远端差多少、怎么同步」说清。
   它不是卡片：描边 + 圆角会让它跟下面的内容抢「这是一个盒子」的信号，
   而这里只是工具条。 */
.git-bar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-r);
  background: var(--wb-surface);
  border: 1px solid var(--wb-line);
  flex: none;
}

.git-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
  min-width: 0;
}

.git-branch {
  font-family: var(--wb-font-mono);
  color: var(--wb-brand);
  font-weight: 650;
}

.git-upstream-name {
  font-family: var(--wb-font-mono);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 200px;
}

.git-count {
  font-family: var(--wb-font-mono);
  gap: 6px;
  font-variant-numeric: tabular-nums;
}

.is-ahead {
  color: var(--wb-ok);
}

.is-behind {
  color: var(--wb-warn);
}

.git-hint {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.git-sp {
  flex: 1;
}

/* 拆分按钮：主键与菜单键贴成一块，菜单键窄一点只放箭头。 */
.pull-split {
  display: inline-flex;
  align-items: stretch;
}

.pull-split > * + * {
  margin-left: 1px;
}

.pull-caret {
  padding-left: 5px;
  padding-right: 5px;
}

/* 强推面板：就在版本栏下方展开，不弹窗——
   弹窗会挡住版本栏，而强推的前提正是「看着版本栏决定要不要推」。 */
.force-panel {
  flex-basis: 100%;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  margin-top: var(--wb-sp-1);
  padding: var(--wb-sp-3);
  border-radius: var(--wb-r-sm);
  background: var(--wb-hover);
}

.force-head {
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--wb-faint);
}

.force-opt {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-2);
  cursor: pointer;
}

.force-opt > span {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.force-opt b {
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink);
}

.force-opt b.is-danger {
  color: var(--wb-bad);
}

.force-opt em {
  font-style: normal;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  line-height: 1.5;
}

.force-confirm {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.force-confirm-hint {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.force-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--wb-sp-2);
}
</style>
