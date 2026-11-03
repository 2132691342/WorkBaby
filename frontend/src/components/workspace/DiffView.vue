<script setup lang="ts">
// 差异视图：折叠未改动区段 + 内联 / 并排切换，行渲染交给 DiffRows。
// 折叠省掉「改一行看完整个文件」；并排是真配对——第 i 条删除与
// 第 i 条新增同一行，只切四列不配对比内联更难读。只做展示，不做不可逆操作。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import DiffRows from '@/components/common/DiffRows.vue'
import type { DiffLine, GitDiff } from '@/types/api'

const props = defineProps<{
  diff: GitDiff | null
  path?: string
  loading?: boolean
}>()

const { t } = useI18n()

/** 内联 / 并排。默认内联：窄栏才是这个 pane 的常态。 */
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
const contextLines = 3

interface Fold {
  count: number
  at: number
}

/**
 * 把上下文行折成「可见行 / 折叠段」。
 *
 * 只折连续的上下文行：增删行永远要显示——那正是用户要看的东西。
 * 头尾各留 contextLines 行作为锚点，中间的折起来。
 * 返回的是「显示序列」，元素要么是一行原文，要么是一个折叠段。
 */
const blocks = computed<Array<DiffLine | Fold>>(() => {
  const src = props.diff?.diff_lines ?? []
  const out: Array<DiffLine | Fold> = []
  let run: { start: number; items: DiffLine[] } | null = null

  const flush = (): void => {
    if (!run) return
    const n = run.items.length
    // 短到不值得折：折起来只省一两行，还多一次点击。
    if (n <= contextLines * 2 + 1) {
      for (const item of run.items) out.push(item)
    } else {
      for (const item of run.items.slice(0, contextLines)) out.push(item)
      out.push({ count: n - contextLines * 2, at: run.start + contextLines })
      for (const item of run.items.slice(n - contextLines)) out.push(item)
    }
    run = null
  }

  src.forEach((line, i) => {
    if (line.line_kind === 'ctx') {
      if (!run) run = { start: i, items: [] }
      run.items.push(line)
      return
    }
    flush()
    out.push(line)
  })
  flush()
  return out
})

/**
 * 并排的行对：一段删除 + 紧跟的一段新增配成对。
 *
 * 配对的规则与所有并排差异视图一致：连续的 - 块与紧随其后的 + 块逐行对齐，
 * 行数不等时短的一侧留空。上下文行与文件头两侧都画同一行。
 */
const pairs = computed<DiffLine[][]>(() => {
  const src = props.diff?.diff_lines ?? []
  const out: DiffLine[][] = []
  let i = 0
  while (i < src.length) {
    const line = src[i]
    if (line.line_kind === 'del') {
      const dels: DiffLine[] = []
      while (i < src.length && src[i].line_kind === 'del') dels.push(src[i++])
      const adds: DiffLine[] = []
      while (i < src.length && src[i].line_kind === 'add') adds.push(src[i++])
      for (let k = 0; k < Math.max(dels.length, adds.length); k++) {
        out.push([dels[k], adds[k]].filter((x): x is DiffLine => Boolean(x)))
      }
      continue
    }
    out.push([line])
    i++
  }
  return out
})

const adds = computed(
  () => (props.diff?.diff_lines ?? []).filter((l) => l.line_kind === 'add').length,
)
const dels = computed(
  () => (props.diff?.diff_lines ?? []).filter((l) => l.line_kind === 'del').length,
)
const blank = computed(() => !props.loading && (props.diff?.diff_lines?.length ?? 0) === 0)
const blankText = computed(() => (props.path ? t('workspace.diffEmpty') : t('workspace.noChanges')))

function isFold(row: DiffLine | Fold): row is Fold {
  return (row as Fold).count !== undefined
}

/** 折叠段是不是展开的。 */
function isOpen(at: number): boolean {
  return opened.value.has(at)
}

function toggle(at: number): void {
  const next = new Set(opened.value)
  if (next.has(at)) next.delete(at)
  else next.add(at)
  opened.value = next
}

const folds = computed(() => blocks.value.filter(isFold))
const allOpen = computed(() => folds.value.every((f) => isOpen(f.at)))

function toggleAll(): void {
  opened.value = allOpen.value ? new Set() : new Set(folds.value.map((f) => f.at))
}

/**
 * 内联可见行：把「显示序列」摊平成纯行，展开的折叠段按 (at, count)
 * 从原文切回来——折叠段只记这两个数，不再存一份副本。
 */
const visibleLines = computed<DiffLine[]>(() => {
  const src = props.diff?.diff_lines ?? []
  const out: DiffLine[] = []
  for (const row of blocks.value) {
    if (!isFold(row)) {
      out.push(row)
      continue
    }
    if (!isOpen(row.at)) continue
    out.push(...src.slice(row.at, row.at + row.count))
  }
  return out
})
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
      <button class="wb-btn text sm" :title="t('workspace.diffMode')" @click="sideBySide = !sideBySide">
        {{ sideBySide ? t('workspace.diffInline') : t('workspace.diffSide') }}
      </button>
    </header>

    <div v-if="loading" class="dc-state">
      <span class="dot busy" />
      {{ t('common.loading') }}
    </div>

    <div v-else-if="blank" class="dc-state">{{ blankText }}</div>

    <div v-else class="dc-body">
      <DiffRows v-if="sideBySide" :pairs="pairs" />
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
  overflow: hidden;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-lg);
  background: var(--wb-surface);
}
.dc-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-bottom: 1px solid var(--wb-line);
  flex: none;
}
.dc-path {
  flex: 1;
  min-width: 0;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-mute);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dc-stat {
  display: flex;
  gap: var(--wb-sp-2);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  flex: none;
}
.dc-stat .is-add {
  color: var(--wb-ok);
}
.dc-stat .is-del {
  color: var(--wb-bad);
}
.dc-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
}
.dc-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-10) var(--wb-sp-5);
  color: var(--wb-faint);
  font-size: var(--wb-fs-sm);
}

/* 折叠段：一条可点的横条，说明藏了多少行 */
.fold-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: 2px var(--wb-sp-3);
  border: 0;
  border-top: 1px solid var(--wb-line);
  border-bottom: 1px solid var(--wb-line);
  background: var(--wb-hover);
  color: var(--wb-mute);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  text-align: left;
  cursor: pointer;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}
.fold-row:hover {
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
}
.fold-ic {
  transform: rotate(90deg);
  flex: none;
}

.dc-trunc {
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-top: 1px solid var(--wb-line);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-warn);
  flex: none;
}
</style>
