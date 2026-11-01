<script setup lang="ts">
// 改动清单：助手到底改了哪些文件，点一行看差异。
// 冲突文件在这里给「标记已解决」——不做这一步 git 不接受提交，用户会被卡死。
import { useI18n } from 'vue-i18n'
import type { GitFileVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{
  /** 当前仓库的文件改动清单 */
  files: GitFileVO[]
  /** 选中的文件路径（空串 = 看全量差异） */
  selected: string
  /** 正在标记解决的路径 */
  resolving: string
}>()

const emit = defineEmits<{
  (e: 'pick', path: string): void
  (e: 'resolve', path: string): void
}>()

const { t } = useI18n()

/** 状态字母与配色：git 的 porcelain 码只保留首字母语义。 */
const TAGS: Record<string, { text: string; tone: string }> = {
  M: { text: 'M', tone: 'is-mod' },
  A: { text: 'A', tone: 'is-add' },
  D: { text: 'D', tone: 'is-del' },
  R: { text: 'R', tone: 'is-mod' },
  C: { text: 'C', tone: 'is-mod' },
  T: { text: 'T', tone: 'is-mod' },
  U: { text: '!', tone: 'is-conflict' },
  '?': { text: 'U', tone: 'is-new' },
}

/** 路径只留末两段：改动清单是「哪个文件」，不是「完整路径」。 */
function shortPath(p: string) {
  const parts = p.split('/').filter(Boolean)
  return parts.length > 2 ? '…/' + parts.slice(-2).join('/') : p
}

function tagOf(status: string) {
  return TAGS[status] || TAGS.M
}
</script>

<template>
  <section class="wb-card file-card">
    <header class="fc-head">
      <span class="wb-label">{{ t('workspace.files') }}</span>
      <span class="fc-count">{{ props.files.length }}</span>
    </header>

    <div v-if="!props.files.length" class="fc-empty">
      <AppIcon name="check" size="ic-lg" />
      <b>{{ t('workspace.noChanges') }}</b>
      <p>{{ t('workspace.noChangesHint') }}</p>
    </div>

    <ul v-else class="fc-list">
      <li v-for="f in props.files" :key="f.git_path">
        <button
          class="fc-row"
          type="button"
          :class="{ 'is-on': props.selected === f.git_path }"
          :title="f.git_path"
          @click="emit('pick', f.git_path)"
        >
          <span class="fc-tag" :class="tagOf(f.git_status).tone">
            {{ tagOf(f.git_status).text }}
          </span>
          <span class="fc-path" :title="f.git_path">{{ shortPath(f.git_path) }}</span>
          <el-button
            v-if="f.git_conflict"
            size="small"
            text
            class="fc-resolve"
            :loading="props.resolving === f.git_path"
            :title="t('workspace.conflictResolveHint')"
            @click.stop="emit('resolve', f.git_path)"
          >
            {{ t('workspace.conflictResolve') }}
          </el-button>
          <span v-else class="fc-stat">
            <b class="is-add">+{{ f.git_additions }}</b>
            <b class="is-del">−{{ f.git_deletions }}</b>
          </span>
        </button>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.file-card {
  overflow: hidden;
}
.fc-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3) var(--wb-sp-3) var(--wb-sp-2);
  border-bottom: 1px solid var(--wb-border);
}
.fc-count {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.fc-list {
  max-height: calc(520px * var(--wb-vw-scale));
  overflow-y: auto;
  padding: var(--wb-sp-1);
}
/* 整行元素：只缩不放（行宽 100%，放大会被裁） */
.fc-row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  text-align: left;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    box-shadow var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.fc-row:hover {
  background: var(--wb-elev-hover);
}
.fc-row:active {
  transform: scale(var(--wb-row-scale-press));
}
.fc-row.is-on {
  background: var(--wb-accent-tint);
  box-shadow: inset 2px 0 0 var(--wb-primary);
}
.fc-tag {
  width: calc(16px * var(--wb-vw-scale));
  height: calc(16px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  display: grid;
  place-items: center;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  flex: none;
}
.fc-tag.is-add {
  background: var(--wb-success-soft);
  color: var(--wb-success);
}
.fc-tag.is-del {
  background: var(--wb-danger-soft);
  color: var(--wb-danger);
}
.fc-tag.is-mod {
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
}
.fc-tag.is-new {
  background: var(--wb-live-soft);
  color: var(--wb-live);
}
.fc-tag.is-conflict {
  background: var(--wb-warning-soft);
  color: var(--wb-warning);
}
.fc-path {
  flex: 1;
  min-width: 0;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.fc-stat {
  display: flex;
  gap: var(--wb-sp-1);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  flex: none;
}
.fc-stat .is-add {
  color: var(--wb-success);
}
.fc-stat .is-del {
  color: var(--wb-danger);
}
.fc-resolve {
  flex: none;
}
.fc-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8) var(--wb-sp-4);
  text-align: center;
  color: var(--wb-muted);
}
.fc-empty b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.fc-empty p {
  font-size: var(--wb-fs-2xs);
  line-height: var(--wb-lh-base);
}
</style>
