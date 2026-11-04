<script setup lang="ts">
// 一条技能：名字 + 分组 + 依赖提示 + 启停 + 查看 / 重置 / 删除。
// 只服务 pi 一个引擎，所以不再做多引擎矩阵；分组直接来自后端的清单标签。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { SkillMatrixRow } from '@/types/api'

const props = defineProps<{
  row: SkillMatrixRow
  /** 正在切启停的这一条（空表示空闲） */
  toggling: string
  /** 正在删的这一条（空表示空闲） */
  deleting: string
  /** 正在重置的这一条（空表示空闲） */
  resetting: string
}>()

const emit = defineEmits<{
  toggle: [enabled: boolean]
  view: []
  remove: []
  reset: []
  copy: []
}>()

const { t } = useI18n()

/** 这一行代表的那份技能（单引擎，取唯一一格）。 */
const cell = computed(() => props.row.cells.pi ?? props.row.cells.omp)
const busy = computed(() => Boolean(props.toggling || props.deleting || props.resetting))

/** 项目技能不由本壳托管：可看不可改。 */
const readonly = computed(() => cell.value?.source === 'project')
/** 只有托管区里的技能能重置回原件。 */
const resettable = computed(() => Boolean(cell.value?.skill_modified && cell.value?.editable))
/** 只有托管区里的技能能被删。 */
const removable = computed(() => Boolean(cell.value?.editable && cell.value?.source !== 'project'))
/** 工作区技能可以抄进托管区长期化（唯一出路）。 */
const copyable = computed(() => cell.value?.source === 'project')

function sourceLabel(): string {
  const source = cell.value?.source
  return source ? t(`skills.sources.${source}`) : ''
}
</script>

<template>
  <article class="skill-row card-pad">
    <div class="skill-main">
      <div class="skill-head">
        <span class="skill-name">{{ props.row.name }}</span>
        <span class="tag">{{ sourceLabel() }}</span>
        <span v-if="cell?.skill_group" class="tag muted">{{ cell.skill_group }}</span>
        <span v-if="cell?.skill_modified" class="tag warn">{{ t('skills.modified') }}</span>
        <span v-if="readonly" class="tag readonly-tag">{{ t('skills.readonly') }}</span>
      </div>
      <p v-if="props.row.description" class="skill-desc">{{ props.row.description }}</p>
      <p v-if="cell?.skill_requires" class="skill-req">
        <AppIcon name="alert" :size="11" />
        {{ t('skills.requires', { what: cell.skill_requires }) }}
      </p>
    </div>

    <div class="skill-switch">
      <el-switch
        v-if="cell"
        :model-value="cell.enabled"
        :loading="props.toggling === props.row.name"
        :disabled="busy && props.toggling !== props.row.name"
        :title="cell.enabled ? t('common.enabled') : t('common.disabled')"
        @change="(value: string | number | boolean) => emit('toggle', Boolean(value))"
      />
      <span v-else class="tag muted">—</span>
    </div>

    <div class="skill-actions">
      <button class="wb-btn text sm" @click="emit('view')">{{ t('skills.view') }}</button>
      <button
        v-if="copyable"
        class="wb-btn text sm"
        :disabled="busy"
        :title="t('skills.copyHint')"
        @click="emit('copy')"
      >
        {{ t('skills.copy') }}
      </button>
      <button
        v-if="resettable"
        class="wb-btn text sm"
        :class="{ 'is-loading': props.resetting === cell?.path }"
        :disabled="busy"
        :title="t('skills.resetHint')"
        @click="emit('reset')"
      >
        {{ t('skills.reset') }}
      </button>
      <button
        v-if="removable"
        class="wb-btn text sm danger"
        :class="{ 'is-loading': props.deleting === props.row.name }"
        :disabled="busy"
        @click="emit('remove')"
      >
        {{ t('common.delete') }}
      </button>
    </div>
  </article>
</template>

<style scoped>
.skill-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
}

.skill-main {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
  flex: 1;
}

.skill-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
}

.skill-name {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-sm);
  font-weight: 650;
  color: var(--wb-ink);
}

.skill-desc {
  margin: 0;
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-mute);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.skill-req {
  display: flex;
  align-items: center;
  gap: 4px;
  margin: 0;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-warn);
}

.skill-switch {
  flex: none;
  display: flex;
  align-items: center;
}

.skill-actions {
  flex: none;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
}

.tag.warn {
  border-color: var(--wb-warn-soft);
  color: var(--wb-warn);
}

.readonly-tag {
  color: var(--wb-faint);
}
</style>
