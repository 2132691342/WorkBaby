<script setup lang="ts">
// 改动清单里的一组文件（暂存 / 未暂存各一组）：一行一个文件，
// 状态码 / 文件名 / 增删 / 悬停动作。两组行结构一致才抽出来共用——
// 写两遍必有第二处忘了「冲突行要高亮」这类判断。只渲染与派发。
import { useI18n } from 'vue-i18n'
import type { GitFile } from '@/types/api'

defineProps<{
  files: GitFile[]
  /** 当前选中的文件路径 */
  selected: string
  /** 进行中的动作名（空表示空闲） */
  pending: string
  /** 这一组的文件是否已进暂存区：决定行上有哪些动作 */
  staged: boolean
}>()

const emit = defineEmits<{
  pick: [file: GitFile]
  stage: [file: GitFile]
  unstage: [file: GitFile]
  discard: [file: GitFile]
  resolve: [file: GitFile]
  /** 按口径解决冲突：ours / theirs / 空串（我改完了） */
  resolveAs: [file: GitFile, side: 'ours' | 'theirs' | '']
  history: [file: GitFile]
}>()

const { t } = useI18n()

function changeTag(file: GitFile): string {
  if (file.has_conflict) return 'U'
  return file.change_code || '?'
}

/** 文件名：行里只显示末段，完整路径在 title 上。 */
function baseName(file: GitFile): string {
  const parts = file.file_path.split(/[\\/]/).filter(Boolean)
  return parts.length ? parts[parts.length - 1] : file.file_path
}
</script>

<template>
  <div
    v-for="file in files"
    :key="file.file_path"
    class="file-line"
    :class="{ 'is-active': selected === file.file_path, conflict: file.has_conflict }"
    :title="file.file_path"
    @click="emit('pick', file)"
  >
    <span class="chg" :class="`chg-${changeTag(file)}`">{{ changeTag(file) }}</span>
    <span class="path">{{ baseName(file) }}</span>
    <span class="num">
      <b class="add">+{{ file.insertions }}</b><b class="del">−{{ file.deletions }}</b>
    </span>
    <span class="acts">
      <!-- 冲突的三种真实结局都要给到：只给「标记已解决」等于让用户去外部编辑器手改 -->
      <template v-if="file.has_conflict && !staged">
        <button
          class="act warn"
          :title="t('workspace.keepOursHint')"
          :disabled="Boolean(pending)"
          @click.stop="emit('resolveAs', file, 'ours')"
        >
          {{ t('workspace.keepOurs') }}
        </button>
        <button
          class="act warn"
          :title="t('workspace.keepTheirsHint')"
          :disabled="Boolean(pending)"
          @click.stop="emit('resolveAs', file, 'theirs')"
        >
          {{ t('workspace.keepTheirs') }}
        </button>
        <button
          class="act"
          :title="t('workspace.markResolvedHint')"
          :disabled="Boolean(pending)"
          @click.stop="emit('resolveAs', file, '')"
        >
          {{ t('workspace.markResolved') }}
        </button>
      </template>
      <button class="act" :title="t('workspace.history')" @click.stop="emit('history', file)">
        {{ t('workspace.history') }}
      </button>
      <button v-if="staged" class="act" :disabled="Boolean(pending)" @click.stop="emit('unstage', file)">
        {{ t('workspace.unstage') }}
      </button>
      <template v-else>
        <button class="act" :disabled="Boolean(pending)" @click.stop="emit('stage', file)">
          {{ t('workspace.stage') }}
        </button>
        <button class="act danger" :disabled="Boolean(pending)" @click.stop="emit('discard', file)">
          {{ t('workspace.discard') }}
        </button>
      </template>
    </span>
  </div>
</template>

<style scoped>
/* 一行文件：状态码 / 文件名 / 增删 / 悬停才出现的动作。
   整行可点，所以有悬停底色与按下缩放；选中项给淡底 + 左缘竖条。 */
.file-line {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  height: calc(26px * var(--wb-vw-scale));
  padding: 0 var(--wb-sp-2) 0 var(--wb-sp-3);
  border: 0;
  background: transparent;
  color: var(--wb-ink);
  font-size: var(--wb-fs-xs);
  text-align: left;
  border-radius: var(--wb-r-sm);
  cursor: pointer;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.file-line:hover {
  background: var(--wb-hover);
}

.file-line:active {
  transform: scale(var(--wb-press-row-active));
  transition-duration: 60ms;
}

/* 选中态：淡底 + 左缘竖条。竖条而不是描边——
   描边会把这一行框成一个盒子，一列文件就变成一列盒子。 */
.file-line.is-active {
  background: var(--wb-selected);
  box-shadow: inset 2px 0 0 var(--wb-brand);
}

/* 冲突：左缘换警告色，并给一条淡警告底——
   冲突是要用户动手处理的那一行，不该和普通改动长得一样。 */
.file-line.conflict {
  background: var(--wb-warn-soft);
  box-shadow: inset 2px 0 0 var(--wb-warn);
}

.path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
}

.num {
  display: flex;
  gap: 6px;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  flex: none;
  font-variant-numeric: tabular-nums;
}

.num .add {
  color: var(--wb-ok);
}
.num .del {
  color: var(--wb-bad);
}

.chg {
  width: 14px;
  text-align: center;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  font-weight: 700;
  flex: none;
}
.chg-M {
  color: var(--wb-warn);
}
.chg-A {
  color: var(--wb-ok);
}
.chg-D {
  color: var(--wb-bad);
}
.chg-R,
.chg-C {
  color: var(--wb-brand);
}
.chg-U {
  color: var(--wb-bad);
}
.chg-\? {
  color: var(--wb-faint);
}

/* 行内动作：平时不占位置，悬停或键盘聚焦时才出现。
   一列文件每行都挂三个文字按钮时，扫读路径这件事会被按钮墙打断。 */
.acts {
  display: flex;
  align-items: center;
  gap: 2px;
  flex: none;
  opacity: 0;
  transition: opacity var(--wb-dur-fast) var(--wb-ease);
}

.file-line:hover .acts,
.file-line:focus-within .acts {
  opacity: 1;
}

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
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.act:hover:not(:disabled) {
  background: var(--wb-surface);
  color: var(--wb-brand);
  transform: scale(var(--wb-press-hover));
}

.act:active:not(:disabled) {
  transform: scale(var(--wb-press-active));
}

.act.danger:hover:not(:disabled) {
  color: var(--wb-bad);
}

.act.warn {
  color: var(--wb-warn);
}

.act:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
</style>
