<script setup lang="ts">
// 改动清单：暂存 / 未暂存两组，组内按目录折叠——平铺一列时
// 「都在哪儿改的」这个判断改动范围的第一眼信息就被拍没了。
// 只管分组与折叠；行样式在 GitFileRows，写请求由父组件统一挂 loading。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import GitFileRows from '@/components/workspace/GitFileRows.vue'
import type { GitFile, GitStash } from '@/types/api'

const props = defineProps<{
  staged: GitFile[]
  unstaged: GitFile[]
  /** 当前选中的文件路径 */
  selected: string
  /** 进行中的动作名（空表示空闲） */
  pending: string
  /** 已存的 stash 清单，最新在前 */
  stashes: GitStash[]
}>()

const emit = defineEmits<{
  pick: [file: GitFile]
  stage: [file: GitFile]
  unstage: [file: GitFile]
  discard: [file: GitFile]
  resolve: [file: GitFile]
  stageAll: []
  history: [file: GitFile]
  stashPush: []
  stashPop: []
  stashDrop: [row: GitStash]
}>()

const { t } = useI18n()

/** 被折叠的目录集合。默认全展开：改动通常不多，先全看到。 */
const collapsed = ref<Set<string>>(new Set())

interface Group {
  dir: string
  files: GitFile[]
}

/** 按路径里的目录分段。根目录的文件归到 '.'。 */
function dirOf(file: GitFile): string {
  const path = file.file_path.replace(/\\/g, '/')
  const idx = path.lastIndexOf('/')
  return idx < 0 ? '.' : path.slice(0, idx)
}

function groupBy(files: GitFile[]): Group[] {
  const map = new Map<string, GitFile[]>()
  for (const file of files) {
    const dir = dirOf(file)
    const list = map.get(dir)
    if (list) list.push(file)
    else map.set(dir, [file])
  }
  return [...map.entries()]
    .map(([dir, files]) => ({ dir, files }))
    .sort((a, b) => a.dir.localeCompare(b.dir))
}

const stagedGroups = computed(() => groupBy(props.staged))
const unstagedGroups = computed(() => groupBy(props.unstaged))

function isCollapsed(key: string): boolean {
  return collapsed.value.has(key)
}

function toggle(key: string): void {
  const next = new Set(collapsed.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  collapsed.value = next
}

/** 目录里改动的合计，折叠时也能看到规模。 */
function groupStat(files: GitFile[]): { adds: number; dels: number } {
  let adds = 0
  let dels = 0
  for (const f of files) {
    adds += f.insertions ?? 0
    dels += f.deletions ?? 0
  }
  return { adds, dels }
}
</script>

<template>
  <div class="changes">
    <div v-if="!props.staged.length && !props.unstaged.length" class="empty-mini">
      {{ t('workspace.noChanges') }}
    </div>

    <template v-else>
      <section v-if="props.staged.length" class="group">
        <div class="group-head">
          <span>{{ t('workspace.staged') }}</span>
          <span class="count">{{ props.staged.length }}</span>
        </div>
        <div v-for="g in stagedGroups" :key="`s-${g.dir}`" class="dir">
          <button class="dir-head" @click="toggle(`s-${g.dir}`)">
            <AppIcon name="chevron" :size="10" class="dir-ic" :class="{ collapsed: isCollapsed(`s-${g.dir}`) }" />
            <span class="dir-name">{{ g.dir }}</span>
            <span class="dir-stat">
              <b class="add">+{{ groupStat(g.files).adds }}</b>
              <b class="del">−{{ groupStat(g.files).dels }}</b>
            </span>
          </button>
          <GitFileRows
            v-if="!isCollapsed(`s-${g.dir}`)"
            :files="g.files"
            :selected="props.selected"
            :pending="props.pending"
            staged
            @pick="emit('pick', $event)"
            @unstage="emit('unstage', $event)"
            @history="emit('history', $event)"
          />
        </div>
      </section>

      <section v-if="props.unstaged.length" class="group">
        <div class="group-head">
          <span>{{ t('workspace.unstaged') }}</span>
          <span class="count">{{ props.unstaged.length }}</span>
          <button class="act push" :disabled="Boolean(props.pending)" @click="emit('stageAll')">
            {{ t('workspace.stageAll') }}
          </button>
          <button
            class="act"
            :class="{ 'is-loading': props.pending === 'stash-push' }"
            :disabled="Boolean(props.pending)"
            :title="t('workspace.stashHint')"
            @click="emit('stashPush')"
          >
            {{ t('workspace.stashPush') }}
          </button>
        </div>
        <div v-for="g in unstagedGroups" :key="`u-${g.dir}`" class="dir">
          <button class="dir-head" @click="toggle(`u-${g.dir}`)">
            <AppIcon name="chevron" :size="10" class="dir-ic" :class="{ collapsed: isCollapsed(`u-${g.dir}`) }" />
            <span class="dir-name">{{ g.dir }}</span>
            <span class="dir-stat">
              <b class="add">+{{ groupStat(g.files).adds }}</b>
              <b class="del">−{{ groupStat(g.files).dels }}</b>
            </span>
          </button>
          <GitFileRows
            v-if="!isCollapsed(`u-${g.dir}`)"
            :files="g.files"
            :selected="props.selected"
            :pending="props.pending"
            :staged="false"
            @pick="emit('pick', $event)"
            @stage="emit('stage', $event)"
            @discard="emit('discard', $event)"
            @resolve="emit('resolve', $event)"
            @history="emit('history', $event)"
          />
        </div>
      </section>

      <section v-if="props.stashes.length" class="group">
        <div class="group-head">
          <span>{{ t('workspace.stashTitle') }}</span>
          <span class="count">{{ props.stashes.length }}</span>
        </div>
        <div v-for="row in props.stashes" :key="row.stash_ref" class="stash-row">
          <span class="stash-subject" :title="row.stash_subject">{{ row.stash_subject }}</span>
          <span class="row-actions">
            <button
              class="act"
              :class="{ 'is-loading': props.pending === 'stash-pop' }"
              :disabled="Boolean(props.pending)"
              @click="emit('stashPop')"
            >
              {{ t('workspace.stashPop') }}
            </button>
            <button
              class="act is-danger"
              :class="{ 'is-loading': props.pending === `stash-drop:${row.stash_ref}` }"
              :disabled="Boolean(props.pending)"
              @click="emit('stashDrop', row)"
            >
              {{ t('common.delete') }}
            </button>
          </span>
        </div>
      </section>
    </template>
  </div>
</template>

<style scoped>
.changes {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}

/* 分组标题：暂存 / 工作区。它们是标签，不画线——
   下面已经有目录组了，再加一条线就是两层框。 */
.group-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-3xs);
  font-weight: 700;
  letter-spacing: 0.07em;
  text-transform: uppercase;
  color: var(--wb-faint);
  margin-bottom: 2px;
}

.count {
  font-family: var(--wb-font-mono);
  color: var(--wb-mute);
  letter-spacing: 0;
}

/* 「全部加入」：与行内动作同一套刻度，但它常驻（组级操作不该藏起来） */
.act {
  padding: 0 5px;
  height: calc(18px * var(--wb-vw-scale));
  border: none;
  border-radius: var(--wb-r-xs);
  background: transparent;
  color: var(--wb-mute);
  font-size: var(--wb-fs-3xs);
  font-weight: 600;
  white-space: nowrap;
  transition: color var(--wb-dur-fast) var(--wb-ease);
}

.act:hover:not(:disabled) {
  color: var(--wb-brand);
}

.push {
  margin-left: auto;
}

.act.is-danger:hover:not(:disabled) {
  color: var(--wb-bad);
}

/* stash 行：一句话说明加两个动作，与目录行同一套密度。 */
.stash-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: 3px var(--wb-sp-2);
  border-radius: var(--wb-r-sm);
  font-size: var(--wb-fs-2xs);
}

.stash-row:hover {
  background: var(--wb-hover);
}

.stash-subject {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--wb-ink-2);
}

.row-actions {
  display: flex;
  flex: none;
}

/* 目录头：可点的横条，带这一组的增删合计。
   它是分组标题，不是条目：平时不加底色，只在悬停时给一点提示。 */
.dir-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: 3px var(--wb-sp-2);
  border: 0;
  background: transparent;
  color: var(--wb-ink-2);
  font: inherit;
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
  text-align: left;
  border-radius: var(--wb-r-sm);
  cursor: pointer;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}

.dir-head:hover {
  background: var(--wb-hover);
  color: var(--wb-ink);
}

.dir-ic {
  flex: none;
  transform: rotate(90deg);
  transition: transform var(--wb-dur-fast) var(--wb-ease);
}

.dir-ic.collapsed {
  transform: rotate(0deg);
}

.dir-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--wb-font-mono);
}
.dir-stat {
  display: flex;
  gap: var(--wb-sp-2);
  font-family: var(--wb-font-mono);
  flex: none;
  font-variant-numeric: tabular-nums;
}

.dir-stat .add {
  color: var(--wb-ok);
}

.dir-stat .del {
  color: var(--wb-bad);
}

.empty-mini {
  padding: var(--wb-sp-4);
  color: var(--wb-faint);
  font-size: var(--wb-fs-2xs);
  text-align: center;
}
</style>
