<script setup lang="ts">
// 一条技能（矩阵行）：名字 + 两列引擎开关 + 查看 / 删除。
// 借鉴 cc-switch 的管理方式：一份清单，哪个引擎装了、开着没有一眼看全。
// 启停语义两家不同（omp 认 enabled、pi 认 disable-model-invocation），判据在父组件。
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { EngineKind, SkillMatrixRow, SkillRow } from '@/types/api'

const props = defineProps<{
  row: SkillMatrixRow
  /** 每个引擎有没有「启停」能力（omp 认 caps.skill_toggle，pi 恒有） */
  toggleable: Partial<Record<EngineKind, boolean>>
  /** 每个引擎是否探测到：没装的引擎那列不渲染开关 */
  installed: EngineKind[]
  /** 正在切启停的那一条（`kind:name`，空表示空闲） */
  toggling: string
  /** 正在删的那一条（`kind:name`，空表示空闲） */
  deleting: string
  /** 正在往某个引擎装的那一条（`name:kind`，空表示空闲） */
  copying: string
}>()

const emit = defineEmits<{
  toggle: [cell: SkillRow, enabled: boolean]
  view: [row: SkillMatrixRow]
  remove: [row: SkillMatrixRow]
  copy: [row: SkillMatrixRow, kind: EngineKind]
}>()

const { t } = useI18n()

const ENGINES: EngineKind[] = ['omp', 'pi']

function cellOf(kind: EngineKind) {
  return props.row.cells[kind]
}

function hasEngine(kind: EngineKind): boolean {
  return props.installed.includes(kind)
}

/** 缺定义的那一格能不能「装过去」：源是用户目录的技能才能复制（Editable 语义）。 */
function canCopy(row: SkillMatrixRow, kind: EngineKind): boolean {
  const src = Object.values(row.cells).find(Boolean)
  return Boolean(src?.editable && src.source === 'user' && hasEngine(kind))
}

function busyKey(kind: EngineKind): string {
  return `${kind}:${props.row.name}`
}

/** 分组标签取两格里优先级更高的那份来源。 */
function sourceLabel(): string {
  const cell = cellOf('omp') ?? cellOf('pi')
  return cell ? t(`skills.sources.${cell.source}`) : ''
}
</script>

<template>
  <article class="skill-row card-pad">
    <div class="skill-main">
      <div class="skill-head">
        <span class="skill-name">{{ props.row.name }}</span>
        <span class="tag">{{ sourceLabel() }}</span>
        <span v-if="!Object.values(props.row.cells).some((c) => c?.editable)" class="tag readonly-tag">
          {{ t('skills.readonly') }}
        </span>
      </div>
      <p v-if="props.row.description" class="skill-desc">{{ props.row.description }}</p>
    </div>

    <div class="engine-cells">
      <template v-for="kind in ENGINES" :key="kind">
        <div v-if="hasEngine(kind)" class="engine-cell" :title="cellOf(kind)?.dir">
          <el-switch
            v-if="cellOf(kind) && props.toggleable[kind]"
            :model-value="cellOf(kind)!.enabled"
            :loading="props.toggling === busyKey(kind)"
            :disabled="Boolean(props.toggling) && props.toggling !== busyKey(kind)"
            :title="cellOf(kind)!.enabled ? t('common.enabled') : t('common.disabled')"
            @change="(value: string | number | boolean) => cellOf(kind) && emit('toggle', cellOf(kind)!, Boolean(value))"
          />
          <span
            v-else-if="cellOf(kind)"
            class="tag muted"
            :title="t('skills.noToggle')"
          >
            {{ cellOf(kind)!.enabled ? t('common.enabled') : t('common.disabled') }}
          </span>
          <!-- 缺定义：把另一边的用户级技能装过来 -->
          <button
            v-else-if="canCopy(props.row, kind)"
            class="wb-btn text sm install"
            :class="{ 'is-loading': props.copying === `${props.row.name}:${kind}` }"
            :disabled="Boolean(props.copying)"
            :title="t('skills.copyHint', { engine: kind })"
            @click="emit('copy', props.row, kind)"
          >
            <AppIcon name="plus" :size="12" />
            {{ t('skills.installTo', { engine: kind }) }}
          </button>
          <span class="engine-tag">{{ kind }}</span>
        </div>
      </template>
    </div>

    <div class="skill-actions">
      <!-- 行级动作指向整行：这一行就是「这个技能」。查看看主格（omp 优先），
           删除清掉所有用户目录里的副本（工作区 / 插件来的那份不归这里管）。 -->
      <button class="wb-btn text sm" @click="emit('view', props.row)">{{ t('skills.view') }}</button>
      <button
        v-if="Object.values(props.row.cells).some((c) => c?.editable)"
        class="wb-btn text sm danger"
        :class="{ 'is-loading': Boolean(props.deleting) }"
        :disabled="Boolean(props.deleting)"
        @click="emit('remove', props.row)"
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

/* 只读标签压暗：它是状态说明，不是可点的分类 */
.readonly-tag {
  opacity: 0.65;
}

.skill-desc {
  margin: 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
  line-height: 1.6;
  overflow-wrap: anywhere;
}

/* 引擎开关两列：每格带引擎小标签——只有一格有开关时也认得出是哪边 */
.engine-cells {
  display: flex;
  gap: var(--wb-sp-8);
  flex: none;
}

.engine-cell {
  display: flex;
  align-items: center;
  gap: 6px;
  justify-content: center;
  min-width: 64px;
  min-height: 22px;
}

.engine-tag {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.tag.muted {
  opacity: 0.6;
}

.install {
  color: var(--wb-brand);
}

.skill-actions {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: none;
}
</style>
