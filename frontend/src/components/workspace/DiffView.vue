<script setup lang="ts">
// 差异视图：折叠未改动区段 + 内联 / 并排切换，行渲染交给 DiffRows。
// 折叠省掉「改一行看完整个文件」；并排是真配对——第 i 条删除与
// 第 i 条新增同一行，只切四列不配对比内联更难读。只做展示，不做不可逆操作。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import DiffRows from '@/components/common/DiffRows.vue'
import { pairRuns } from '@/utils/diff'
import type { DiffLine, GitDiff } from '@/types/api'

const props = defineProps<{
  diff: GitDiff | null
  path?: string
  loading?: boolean
}>()

const { t } = useI18n()

/** 内联 / 并排。默认内联：窄栏才是这个页面的常态。 */
const sideBySide = ref(false)

/** 被展开的折叠段编号集合。键是段起始下标。 */
const opened = ref<Set<number>>(new Set())

// 换了文件或换了差异，之前展开的段落编号就没有意义了，全部收起。
watch(
  () => [props.path, props.diff],
  () => {
    opened.value = new Set()
  },
)

/** 上下文保留行数：hunk 前后各留这么多行，多的折起来。 */
const CONTEXT_LINES = 3

/** 一段可折叠的未改动区间。 */
interface Fold {
  /** 这一段有多少行 */
  count: number
  /** 这一段的第一行在 lines 里的下标 */
  at: number
}

const lines = computed<DiffLine[]>(() => props.diff?.diff_lines ?? [])

/** 增删行数：页头那个 +N −M 读数。 */
const adds = computed(() => lines.value.filter((l) => l.line_kind === 'add').length)
const dels = computed(() => lines.value.filter((l) => l.line_kind === 'del').length)

/** 并排的行对：同一改动里第 i 条删除配第 i 条新增（配对规则在 utils/diff.ts）。 */
const sidePairs = computed(() => pairRuns(lines.value))

// CSS 选择器转义：hunk 文本含空格括号加号，直接拼进 querySelector 会抛语法错；
// 浏览器的 CSS.escape 就够，这里包一层兜底，缺了的表现是「点跳转钮没反应」。
function cssEscape(text: string): string {
  if (typeof CSS !== 'undefined' && typeof CSS.escape === 'function') return CSS.escape(text)
  return text.replace(/["\\]/g, '\\$&')
}

// 改动段的跳转锚点用 hunk 的行文本而不是行号：hunk 自己没有行号，
// 而 @@ 文本在一份差异里唯一，折叠前后都稳定，跳转不能按下标算。
const hunkKeys = computed<string[]>(() => {
  const out: string[] = []
  for (const line of lines.value) {
    if (line.line_kind === 'hunk' && line.line_text) out.push(line.line_text)
  }
  return out
})

/** 当前停在第几个 hunk。-1 表示还没跳过。 */
const hunkAt = ref(-1)

const body = ref<HTMLElement | null>(null)

// 换了文件或换了差异，跳转位置作废：旧行号在新文件里指向别的内容。
watch(
  () => [props.path, props.diff],
  () => {
    hunkAt.value = -1
  },
)

// 跳到第 index 个 hunk 并带进可视区：折叠段里的行不在 DOM 里，
// 先把所在段展开，等下一帧再查选择器。
function gotoHunk(index: number): void {
  const total = hunkKeys.value.length
  if (!total) return
  const next = ((index % total) + total) % total
  hunkAt.value = next
  const key = hunkKeys.value[next]
  // 目标行落在某个折叠段里：展开它，否则滚过去什么也看不到
  for (const fold of folds.value) {
    const line = lines.value[fold.at]
    if (line && line.line_text === key) {
      const expanded = new Set(opened.value)
      expanded.add(fold.at)
      opened.value = expanded
    }
  }
  void Promise.resolve().then(() => {
    const el = body.value?.querySelector<HTMLElement>(`[data-row="${cssEscape(key)}"]`)
    el?.scrollIntoView({ block: 'center', behavior: 'smooth' })
  })
}

// 可折叠的连续上下文段：只折离最近增删行超过 CONTEXT_LINES 行的那段——
// 紧贴改动的几行是读差异最需要的参照，折掉等于逼用户自己展开。
const folds = computed<Fold[]>(() => {
  const out: Fold[] = []
  const rows = lines.value
  let runStart = -1
  let runLen = 0
  const flushRun = (end: number) => {
    if (runStart < 0) return
    // 一段上下文前后都要留 CONTEXT_LINES 行
    const head = runStart + CONTEXT_LINES
    const tail = end - CONTEXT_LINES
    if (tail - head > 0) out.push({ at: head, count: tail - head })
    runStart = -1
    runLen = 0
  }
  for (let i = 0; i < rows.length; i++) {
    if (rows[i].line_kind === 'ctx') {
      if (runStart < 0) runStart = i
      runLen++
      continue
    }
    flushRun(i)
  }
  flushRun(rows.length)
  void runLen
  return out
})

/** 哪一段被展开了。 */
function isOpen(at: number): boolean {
  return opened.value.has(at)
}

/** 展开 / 收起一段。 */
function toggle(at: number): void {
  const next = new Set(opened.value)
  if (next.has(at)) next.delete(at)
  else next.add(at)
  opened.value = next
}

/** 全部展开 / 全部收起。 */
function toggleAll(): void {
  opened.value = opened.value.size ? new Set() : new Set(folds.value.map((f) => f.at))
}

const allOpen = computed(() => folds.value.length > 0 && opened.value.size === folds.value.length)

// 真正渲染的行：展开的段给完整行，其余段只留一行入口按钮——
// 不用 v-if 逐行判断（几千行时平方级），一次算完。
const visibleLines = computed<DiffLine[]>(() => {
  if (!folds.value.length) return lines.value
  const hidden = new Map<number, Fold>()
  for (const fold of folds.value) {
    if (isOpen(fold.at)) continue
    hidden.set(fold.at, fold)
  }
  const out: DiffLine[] = []
  const rows = lines.value
  for (let i = 0; i < rows.length; ) {
    const fold = hidden.get(i)
    if (fold) {
      i += fold.count
      continue
    }
    out.push(rows[i])
    i++
  }
  return out
})

/** 没有任何差异时的说明。 */
const blank = computed(() => !props.loading && lines.value.length === 0)

const blankText = computed(() => (props.path ? t('workspace.noDiffForFile') : t('workspace.noDiff')))
</script>

<template>
  <section class="diff-card">
    <header class="dc-head">
      <span class="dc-path" :title="path">{{ path || t('workspace.changes') }}</span>
      <span v-if="adds || dels" class="dc-stat">
        <b class="is-add">+{{ adds }}</b>
        <b class="is-del">−{{ dels }}</b>
      </span>
      <button
        v-if="!sideBySide && folds.length"
        class="wb-btn text sm"
        :title="allOpen ? t('workspace.foldAll') : t('workspace.unfoldAll')"
        @click="toggleAll"
      >
        {{ allOpen ? t('workspace.foldAll') : t('workspace.unfoldAll') }}
      </button>
      <!-- 改动段跳转：多个 hunk 时才出现。一个文件一处改动时它是噪声。 -->
      <span v-if="hunkKeys.length > 1" class="dc-nav">
        <button class="wb-btn icon ghost sm" :title="t('workspace.prevChange')" @click="gotoHunk(hunkAt - 1)">
          <AppIcon name="chevron" :size="11" class="nav-up" />
        </button>
        <span class="dc-nav-count">{{ hunkAt < 0 ? hunkKeys.length : hunkAt + 1 }}/{{ hunkKeys.length }}</span>
        <button class="wb-btn icon ghost sm" :title="t('workspace.nextChange')" @click="gotoHunk(hunkAt + 1)">
          <AppIcon name="chevron" :size="11" class="nav-down" />
        </button>
      </span>
      <button class="wb-btn text sm" :title="t('workspace.diffMode')" @click="sideBySide = !sideBySide">
        {{ sideBySide ? t('workspace.diffInline') : t('workspace.diffSide') }}
      </button>
    </header>

    <div v-if="loading" class="dc-state">
      <span class="dot busy" />
      {{ t('common.loading') }}
    </div>

    <div v-else-if="blank" class="dc-state">{{ blankText }}</div>

    <div ref="body" class="dc-body">
      <DiffRows v-if="sideBySide" :pairs="sidePairs" />
      <template v-else>
        <DiffRows :lines="visibleLines" />
        <!-- 折叠段：一条可点的横条，点一下把中间那段展回来 -->
        <button
          v-for="fold in folds.filter((f) => !isOpen(f.at))"
          :key="fold.at"
          class="fold-row"
          @click="toggle(fold.at)"
        >
          <AppIcon name="chevron" :size="11" class="fold-ic" />
          {{ t('workspace.foldedLines', { n: fold.count }) }}
        </button>
      </template>
    </div>

    <footer v-if="diff?.truncated" class="dc-trunc">
      {{ t('workspace.diffTruncated', { n: diff.dropped }) }}
    </footer>
  </section>
</template>

<style scoped>
.diff-card {
  display: flex;
  flex-direction: column;
  min-height: 0;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r);
  background: var(--wb-surface);
  overflow: hidden;
}

.dc-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-bottom: 1px solid var(--wb-line);
  background: var(--wb-window);
  flex: none;
}

.dc-path {
  flex: 1;
  min-width: 0;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-ink-5);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dc-stat {
  display: inline-flex;
  gap: var(--wb-sp-2);
  flex: none;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
}

.is-add {
  color: var(--wb-ok);
  font-weight: 500;
}

.is-del {
  color: var(--wb-bad);
  font-weight: 500;
}

.dc-state {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-6);
  justify-content: center;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
}

/* 改动段跳转：一组小三件，读数在中间。读数不是控件，不做悬停反馈。 */
.dc-nav {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  flex: none;
}
.dc-nav-count {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  font-variant-numeric: tabular-nums;
  min-width: 2.6em;
  text-align: center;
}

/* 上 / 下两个方向：图标旋转 90°，与全站 chevron 语义一致 */
.nav-up {
  transform: rotate(-90deg);
}
.nav-down {
  transform: rotate(90deg);
}

.dc-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.fold-row {
  display: flex;
  align-items: center;
  gap: 5px;
  width: 100%;
  padding: 2px var(--wb-sp-3);
  border: 0;
  border-top: 1px dashed var(--wb-line);
  border-bottom: 1px dashed var(--wb-line);
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  text-align: left;
  cursor: pointer;
  transition: background-color var(--wb-dur-fast) var(--wb-ease);
}

.fold-row:hover {
  background: var(--wb-brand-3);
}

.fold-ic {
  opacity: 0.7;
}

.dc-trunc {
  padding: var(--wb-sp-1) var(--wb-sp-3);
  border-top: 1px solid var(--wb-line);
  background: var(--wb-warn-soft);
  color: var(--wb-warn);
  font-size: var(--wb-fs-3xs);
  flex: none;
}
</style>
