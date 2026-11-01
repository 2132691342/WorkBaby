<script setup lang="ts">
// 斜杠命令菜单：内置命令（真的会执行的动作）+ 已启用技能，前缀过滤 + 键盘高亮。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '../common/AppIcon.vue'
import { COMMANDS, type SlashItem } from '../../utils/composerMenu'

/** 候选条数上限：刷屏的菜单比没有菜单更难用。 */
const MAX_MATCH = 20

const props = defineProps<{
  /** 输入框当前全文，形如 `/comp` */
  query: string
  /** 已启用技能（内置命令之外的第二类候选） */
  skills: { skill_name: string; skill_description: string }[]
  /** 技能清单是否正在加载（空表时的加载态） */
  loading: boolean
  /** 键盘高亮下标 */
  index: number
}>()

const emit = defineEmits<{
  pick: [item: SlashItem]
  hover: [index: number]
  /** 原始 pointerdown：父组件据此阻止点菜单时输入框丢焦点 */
  pointerdown: [event: PointerEvent]
}>()

const { t } = useI18n()

const items = computed<SlashItem[]>(() => {
  const q = props.query
  if (!q || /\s/.test(q)) return []
  const kw = q.slice(1).toLowerCase()
  const match = (name: string, desc: string) =>
    !kw || name.toLowerCase().includes(kw) || desc.toLowerCase().includes(kw)
  const cmds: SlashItem[] = COMMANDS.filter((c) => match(c.key, t(c.descKey))).map((c) => ({
    kind: 'cmd',
    key: c.key,
    name: t(c.nameKey),
    desc: t(c.descKey),
  }))
  const skills: SlashItem[] = props.skills
    .filter((s) => match(s.skill_name, s.skill_description))
    .slice(0, MAX_MATCH)
    .map((s) => ({
      kind: 'skill',
      key: s.skill_name,
      name: s.skill_name,
      desc: s.skill_description,
    }))
  return [...cmds, ...skills]
})

/** 回车时实际提交哪一条：高亮下标可能越界（过滤后表变短了）。 */
function itemAt(i: number): SlashItem | undefined {
  return items.value[Math.min(i, items.value.length - 1)]
}

defineExpose({ items, itemAt })
</script>

<template>
  <div class="pop menu-pop" @pointerdown="emit('pointerdown', $event)">
    <div v-if="loading && !items.length" class="pop-row is-muted">
      <span class="pop-spin" />
      {{ t('chat.slashLoading') }}
    </div>
    <div v-else-if="!items.length" class="pop-row is-muted">{{ t('chat.slashNoMatch') }}</div>
    <div v-else class="pop-list">
      <button
        v-for="(it, i) in items"
        :key="`${it.kind}-${it.key}`"
        :data-idx="i"
        class="pop-row"
        :class="{ 'is-hi': i === index }"
        type="button"
        @mouseenter="emit('hover', i)"
        @click="emit('pick', it)"
      >
        <AppIcon :name="it.kind === 'skill' ? 'sparkles' : 'terminal'" size="ic-xs" />
        <span class="pop-grow">
          <b class="pop-t">{{ it.kind === 'cmd' ? `/${it.key}` : it.name }}</b>
          <span class="pop-d">{{ it.kind === 'cmd' ? it.name + ' · ' + it.desc : it.desc }}</span>
        </span>
        <span v-if="it.kind === 'skill'" class="pop-tag">{{ t('chat.slashTagSkill') }}</span>
        <span v-else class="pop-tag is-builtin">{{ t('chat.slashTagBuiltin') }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
/* 弹层外壳（.pop / .menu-pop / .pop-row / .pop-grow 等）在 content.css：
   它不只服务这一个菜单，重复定义会让两处样式各自漂移。 */
</style>
