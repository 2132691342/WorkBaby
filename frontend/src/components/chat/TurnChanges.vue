<script setup lang="ts">
// 一轮回答改了哪些文件：从工具块就地聚合（file_path + 已归一的 diff），纯展示。
// 同一文件改两次并成一行——用户关心「一共动了多少」不是「动了几次」；
// 放轮尾不放大卡片：它只是补充，答案在 git 页。
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { ChatBlock } from '@/types/api'

const props = defineProps<{
  /** 这一轮的块（工具块带 file_path 与 diff） */
  blocks: ChatBlock[]
}>()

const { t } = useI18n()
const router = useRouter()

interface FileChange {
  path: string
  add: number
  del: number
}

/** 就地聚合：同一文件多块合并，增删行数累加。 */
const changes = computed<FileChange[]>(() => {
  const map = new Map<string, FileChange>()
  for (const block of props.blocks) {
    if (block.kind !== 'tool' || !block.file_path) continue
    let add = 0
    let del = 0
    for (const line of block.diff ?? []) {
      if (line.line_kind === 'add') add++
      else if (line.line_kind === 'del') del++
    }
    const known = map.get(block.file_path)
    if (known) {
      known.add += add
      known.del += del
    } else {
      map.set(block.file_path, { path: block.file_path, add, del })
    }
  }
  return [...map.values()]
})

const totals = computed(() => {
  let add = 0
  let del = 0
  for (const change of changes.value) {
    add += change.add
    del += change.del
  }
  return { add, del }
})

/** chip 上只显示文件名，完整路径在悬停提示里。 */
function fileName(path: string): string {
  const index = Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'))
  return index >= 0 ? path.slice(index + 1) : path
}

function openWorkspace(): void {
  void router.push({ name: 'workspace' })
}
</script>

<template>
  <div v-if="changes.length" class="turn-changes">
    <AppIcon name="git" :size="12" />
    <span class="changes-title">{{ t('chat.changesCount', { n: changes.length }) }}</span>
    <button
      v-for="change in changes"
      :key="change.path"
      type="button"
      class="change-chip"
      :title="`${change.path}  +${change.add} −${change.del}`"
      @click="openWorkspace"
    >
      <span class="change-name">{{ fileName(change.path) }}</span>
      <span v-if="change.add" class="num add">+{{ change.add }}</span>
      <span v-if="change.del" class="num del">−{{ change.del }}</span>
    </button>
    <span class="changes-total">
      <span v-if="totals.add" class="num add">+{{ totals.add }}</span>
      <span v-if="totals.del" class="num del">−{{ totals.del }}</span>
    </span>
  </div>
</template>

<style scoped>
/* 一行小字 + 一排文件 chip：它是补充读数，不做大卡片 */
.turn-changes {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px var(--wb-sp-2);
  margin-top: var(--wb-sp-1);
  color: var(--wb-faint);
  -webkit-user-select: none;
  user-select: none;
}

.changes-title {
  font-size: var(--wb-fs-3xs);
  white-space: nowrap;
}

.change-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: 220px;
  padding: 1px 8px;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-full);
  background: var(--wb-surface);
  color: var(--wb-mute);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  overflow: hidden;
  white-space: nowrap;
  transition:
    color var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.change-chip:hover {
  color: var(--wb-brand);
  border-color: var(--wb-brand-line);
  transform: scale(var(--wb-press-hover));
}

.change-chip:active {
  transform: scale(var(--wb-press-active));
}

.change-name {
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 增删行数：绿加红减，全站 git 读数同一套颜色 */
.num {
  font-variant-numeric: tabular-nums;
}

.num.add {
  color: var(--wb-ok);
}

.num.del {
  color: var(--wb-bad);
}

.changes-total {
  display: inline-flex;
  gap: var(--wb-sp-2);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
}
</style>
