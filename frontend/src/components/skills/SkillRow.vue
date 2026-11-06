<script setup lang="ts">
// 一条技能：名字 + 分组 + 开关 + 查看 / 删除 / 重置。
// 只有一条技能对应一个目录，所以启停就是「把目录搬进/搬出禁用区」。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SkillRow } from '@/types/api'

const props = defineProps<{
  row: SkillRow
  /** 正在切启停的那一条目录名（空表示空闲） */
  toggling: string
  /** 正在删的那一条目录名（空表示空闲） */
  deleting: string
  /** 正在重置的那一条目录名（空表示空闲） */
  resetting: string
}>()

const emit = defineEmits<{
  toggle: [row: SkillRow, enabled: boolean]
  view: [row: SkillRow]
  remove: [row: SkillRow]
  reset: [row: SkillRow]
}>()

const { t } = useI18n()

const busy = computed(() => Boolean(props.toggling || props.deleting || props.resetting))
const isToggling = computed(() => props.toggling === props.row.skill_dir_name)
const isDeleting = computed(() => props.deleting === props.row.skill_dir_name)
const isResetting = computed(() => props.resetting === props.row.skill_dir_name)

/** 项目技能属于那个仓库，不归这里删；随包附带与内置可以删（删了不回来）。 */
const removable = computed(() => props.row.skill_editable)
/** 只有内置技能能从随包原件恢复。 */
const resettable = computed(() => props.row.skill_source === 'builtin' && props.row.skill_modified)
/** 项目技能由所在仓库管理，壳里的开关对它无效。 */
const toggleable = computed(() => props.row.skill_source !== 'project')
</script>

<template>
  <article class="skill-row card-pad">
    <div class="skill-main">
      <div class="skill-head">
        <span class="skill-name">{{ row.skill_name }}</span>
        <span v-if="row.skill_group" class="tag">{{ row.skill_group }}</span>
        <span v-if="row.skill_modified" class="tag modified-tag">{{ t('skills.modified') }}</span>
        <span v-if="row.skill_requires" class="tag muted" :title="row.skill_requires">
          {{ t('skills.requires', { dep: row.skill_requires }) }}
        </span>
      </div>
      <p v-if="row.skill_desc" class="skill-desc">{{ row.skill_desc }}</p>
    </div>

    <div class="skill-switch" :title="row.skill_dir">
      <el-switch
        :model-value="row.skill_enabled"
        :loading="isToggling"
        :disabled="!toggleable || (busy && !isToggling)"
        :title="row.skill_enabled ? t('common.enabled') : t('common.disabled')"
        @change="(value: string | number | boolean) => emit('toggle', row, Boolean(value))"
      />
    </div>

    <div class="skill-actions">
      <button class="wb-btn text sm" @click="emit('view', row)">{{ t('skills.view') }}</button>
      <button
        v-if="resettable"
        class="wb-btn text sm"
        :class="{ 'is-loading': isResetting }"
        :disabled="busy && !isResetting"
        :title="t('skills.resetHint')"
        @click="emit('reset', row)"
      >
        {{ t('skills.reset') }}
      </button>
      <button
        v-if="removable"
        class="wb-btn text sm danger"
        :class="{ 'is-loading': isDeleting }"
        :disabled="busy && !isDeleting"
        @click="emit('remove', row)"
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

/* 改过的标签压暗：它是状态说明，不是可点的分类 */
.modified-tag {
  opacity: 0.65;
}

.skill-desc {
  margin: 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
  line-height: 1.6;
  overflow-wrap: anywhere;
}

.skill-switch {
  flex: none;
  display: flex;
  align-items: center;
  min-width: 44px;
  justify-content: center;
}

.tag.muted {
  opacity: 0.6;
}

.skill-actions {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: none;
}
</style>
