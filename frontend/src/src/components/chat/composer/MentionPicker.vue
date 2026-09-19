<script setup lang="ts">
/**
 * `@` 提及选择器：ChatInput 解析光标前的 `@query` 后传入候选列表，这里只负责渲染与派发 pick。
 *
 * <p>高亮索引由父组件持有（键盘 ↑↓ 改 activeIndex），本组件只上报 hover。
 */
import { nextTick, ref, watch, type Component } from 'vue'
import { AtSign, Zap, BookOpen, Folder, Bot, FileText } from '@/components/common/icons'
import { t } from '@/i18n'

/** 可提及的对象：技能 / 知识库 / 文件夹 / 文件 / 命令。 */
export type MentionItem =
  | { type: 'skill'; id: string; name: string; insertText: string; description?: string }
  | { type: 'knowledge'; id: string; name: string; insertText: string; description?: string }
  | { type: 'folder'; id: string; name: string; insertText: string; path?: string }
  | { type: 'file'; id: string; name: string; insertText: string; description?: string }
  | { type: 'command'; id: string; name: string; insertText: string; description?: string }

const props = defineProps<{
  visible: boolean
  /** 当前 caret 之前 @ 触发的 query 字符串（如 "sk"）。 */
  query: string
  /** 候选 MentionItem 列表（已按 query 过滤好）。 */
  items: MentionItem[]
  /** 当前键盘高亮项索引（-1 表示无）。 */
  activeIndex: number
  /** 弹窗位置。 */
  position?: { top?: number; left?: number; bottom?: number; right?: number }
}>()

const emit = defineEmits<{
  /** 用户点击或键盘 Enter 选中某项 */
  pick: [item: MentionItem]
  /** 用户 Escape / 点击外部关闭 */
  close: []
  /** activeIndex 改变（↑↓ 键） */
  'update-active': [index: number]
}>()

function pickItem(item: MentionItem): void {
  emit('pick', item)
}

/** 键盘 ↑↓ 导航时高亮项自动滚入视野（列表可滚动却看不见选中项 = 经典体验 bug）。 */
const listEl = ref<HTMLElement | null>(null)
watch(
  () => props.activeIndex,
  async () => {
    await nextTick()
    const el = listEl.value?.querySelector<HTMLElement>('[data-active="true"]')
    el?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  }
)

function iconFor(item: MentionItem): Component {
  if (item.type === 'skill') return Zap
  if (item.type === 'knowledge') return BookOpen
  if (item.type === 'folder') return Folder
  if (item.type === 'file') return FileText
  return Bot
}

/** 分组标题左侧图标（与行内图标同源）。 */
function iconForGroup(type: MentionItem['type']): Component {
  return iconFor({ type, id: '', name: '', insertText: '' } as MentionItem)
}

/** 类型 → 文案（分组标题与行内标签共用）。 */
function labelFor(type: MentionItem['type']): string {
  if (type === 'skill') return t('mention.type.skill')
  if (type === 'knowledge') return t('mention.type.knowledge')
  if (type === 'folder') return t('mention.type.folder')
  if (type === 'file') return t('mention.type.file')
  return t('mention.type.command')
}

/** 分组顺序：与候选生成顺序一致（技能 → 知识 → 文件夹 → 文件 → 命令）。 */
const GROUP_ORDER: MentionItem['type'][] = ['skill', 'knowledge', 'folder', 'file', 'command']

/**
 * 按类型分组；index 保留在原始 items 中的下标——
 * 父组件的键盘高亮走扁平索引，分组只是渲染层的事，重排会错位。
 */
const groups = computed(() =>
  GROUP_ORDER.map((type) => ({
    type,
    title: labelFor(type),
    items: props.items
      .map((it, index) => ({ it, index }))
      .filter((x) => x.it.type === type)
  })).filter((g) => g.items.length > 0)
)
</script>

<template>
  <transition
    enter-active-class="transition-all duration-150 ease-out origin-bottom"
    enter-from-class="scale-95 opacity-0"
    enter-to-class="scale-100 opacity-100"
    leave-active-class="transition-all duration-100 ease-in origin-bottom"
    leave-from-class="scale-100 opacity-100"
    leave-to-class="scale-95 opacity-0"
  >
    <div
      v-if="props.visible"
      class="wb-glass absolute z-50 w-72 rounded-2xl border border-wb-border bg-wb-surface/95 p-2 shadow-[var(--wb-shadow-lg)] backdrop-blur"
      :style="{
        /* 默认朝上展开：composer 位于页面底部，向下弹会被视口截断
           （与 SlashCommandPalette / ModelSelector 的 bottom-full 对齐） */
        bottom: props.position?.bottom ?? '100%',
        top: props.position?.top ?? 'auto',
        left: props.position?.left ?? '0',
        right: props.position?.right ?? 'auto',
        marginBottom: '8px'
      }"
      @click.stop
    >
      <p class="mb-1 flex items-center gap-1 px-2 py-1 text-[10px] font-medium uppercase tracking-wider text-wb-muted">
        <AtSign class="h-3 w-3" />
        {{ t('mention.title') }}
        <span v-if="props.query" class="text-wb-primary">"{{ props.query }}"</span>
      </p>
      <p
        v-if="props.items.length === 0"
        class="rounded-lg bg-wb-primary/[0.06] px-3 py-2 text-xs text-wb-muted"
      >
        {{ t('mention.noResults') }}
      </p>
      <ul ref="listEl" class="wb-scroll max-h-64 overflow-y-auto overscroll-contain">
        <li v-for="g in groups" :key="g.type">
          <!-- 分组标题：加重 + 主色 + 小图标，避免「只看得到一行行候选却没意识到分组」的盲区 -->
          <p class="flex items-center gap-1.5 border-b border-wb-border/40 px-2 pb-1 pt-2 text-[10.5px] font-semibold uppercase tracking-wider text-wb-primary-strong">
            <component :is="iconForGroup(g.type)" class="h-3 w-3" />
            {{ g.title }}
            <span class="ml-auto rounded bg-wb-primary/10 px-1 text-[9.5px] tabular-nums text-wb-primary">{{ g.items.length }}</span>
          </p>
          <button
            v-for="row in g.items"
            :key="row.it.id"
            type="button"
            :data-active="row.index === props.activeIndex || undefined"
            class="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-xs transition-colors"
            :class="row.index === props.activeIndex ? 'bg-wb-primary/15 text-wb-primary-strong' : 'text-wb-ink hover:bg-wb-primary/5'"
            @click="pickItem(row.it)"
            @mouseenter="emit('update-active', row.index)"
          >
            <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-wb-primary/10 text-wb-primary-strong">
              <component :is="iconFor(row.it)" class="h-3.5 w-3.5" />
            </span>
            <span class="min-w-0 flex-1">
              <span class="block truncate font-medium">{{ row.it.name }}</span>
              <span v-if="row.it.type !== 'folder' && 'description' in row.it && row.it.description" class="block truncate text-[10px] text-wb-muted">{{ row.it.description }}</span>
              <span v-else-if="row.it.type === 'folder' && row.it.path" class="block truncate text-[10px] text-wb-muted">{{ row.it.path }}</span>
            </span>
          </button>
        </li>
      </ul>
    </div>
  </transition>
</template>