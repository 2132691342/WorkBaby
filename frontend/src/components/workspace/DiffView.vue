<script setup lang="ts">
// 差异视图：把后端折好的行结构渲染成等宽行（旧 / 新行号 + 正文）。
// 只做展示——提交与回滚都交给版本工具，界面不替用户决定不可逆操作。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GitDiffLineVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{
  lines: GitDiffLineVO[]
  path: string
  truncated: boolean
  dropped: number
  loading: boolean
}>()

const { t } = useI18n()

const adds = computed(() => props.lines.filter((l) => l.git_line_kind === 'add').length)
const dels = computed(() => props.lines.filter((l) => l.git_line_kind === 'del').length)
const blank = computed(() => !props.loading && props.lines.length === 0)
const blankText = computed(() =>
  props.path ? t('workspace.diffEmpty') : t('workspace.noChanges'),
)
</script>

<template>
  <section class="wb-card diff-card">
    <header class="dc-head">
      <span class="dc-path" :title="path">{{ path || t('workspace.changes') }}</span>
      <span v-if="lines.length" class="dc-stat">
        <b class="is-add">+{{ adds }}</b>
        <b class="is-del">−{{ dels }}</b>
      </span>
    </header>

    <div v-if="loading" class="dc-state">
      <span class="dc-spin" />
      {{ t('common.loading') }}
    </div>

    <div v-else-if="blank" class="dc-state">
      <AppIcon name="git-diff" size="ic-lg" />
      {{ blankText }}
    </div>

    <div v-else class="diff dc-body">
      <div
        v-for="(l, i) in lines"
        :key="i"
        class="diff-row"
        :class="{ 'is-add': l.git_line_kind === 'add', 'is-del': l.git_line_kind === 'del', 'is-hunk': l.git_line_kind === 'hunk' }"
      >
        <span>{{ l.git_line_old_no || '' }}</span>
        <span>{{ l.git_line_new_no || '' }}</span>
        <span>{{ l.git_line_text }}</span>
      </div>
    </div>

    <footer v-if="truncated" class="dc-trunc">
      <AppIcon name="alert" size="ic-xs" />
      {{ t('workspace.diffTruncated', { n: dropped }) }}
    </footer>
  </section>
</template>

<style scoped>
.diff-card {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}
.dc-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3) var(--wb-sp-4);
  border-bottom: 1px solid var(--wb-border);
}
.dc-path {
  flex: 1;
  min-width: 0;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dc-stat {
  display: flex;
  gap: var(--wb-sp-2);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  flex: none;
}
.dc-stat .is-add {
  color: var(--wb-success);
}
.dc-stat .is-del {
  color: var(--wb-danger);
}
.dc-body {
  border: 0;
  border-radius: 0;
  max-height: calc(560px * var(--wb-vw-scale));
  overflow: auto;
}
.dc-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-10) var(--wb-sp-5);
  color: var(--wb-muted);
  font-size: var(--wb-fs-sm);
  text-align: center;
}
.dc-spin {
  width: calc(13px * var(--wb-vw-scale));
  height: calc(13px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  border: 1.8px solid var(--wb-border-strong);
  border-top-color: var(--wb-primary);
  animation: wb-spin 0.8s linear infinite;
}
.dc-trunc {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-4);
  border-top: 1px solid var(--wb-border);
  background: var(--wb-warning-soft);
  color: var(--wb-warning);
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
}
</style>
