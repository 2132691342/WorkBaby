<script setup lang="ts">
// 一轮回答改了哪些文件：从工具块就地聚合（file_path + 差异行），纯展示。
// 同一文件改两次并成一行——用户关心「一共动了多少」不是「动了几次」；
// 放在轮尾不放大卡片：它只是补充，答案在工作区页。
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { DiffLine } from '../../types/chatEvents'
import AppIcon from '../common/AppIcon.vue'

/** 参与聚合的一份工具结果。 */
export interface ChangeSource {
  /** 这次调用改动的文件（相对工作区） */
  file_path: string
  /** 差异行 */
  diff: DiffLine[]
}

const props = defineProps<{ sources: ChangeSource[] }>()

const { t } = useI18n()
const router = useRouter()

interface FileChange {
  /** 相对工作区路径 */
  path: string
  /** 新增行数 */
  add: number
  /** 删除行数 */
  del: number
}

/** 就地聚合：同一文件多块合并，增删行数累加。 */
const changes = computed<FileChange[]>(() => {
  const map = new Map<string, FileChange>()
  for (const src of props.sources) {
    if (!src.file_path) continue
    let add = 0
    let del = 0
    for (const line of src.diff ?? []) {
      if (line.line_kind === 'add') add++
      else if (line.line_kind === 'del') del++
    }
    const known = map.get(src.file_path)
    if (known) {
      known.add += add
      known.del += del
    } else {
      map.set(src.file_path, { path: src.file_path, add, del })
    }
  }
  return [...map.values()]
})

const totals = computed(() => {
  let add = 0
  let del = 0
  for (const c of changes.value) {
    add += c.add
    del += c.del
  }
  return { add, del }
})

/** chip 上只显示文件名，完整路径在悬停提示里。 */
function fileName(path: string): string {
  const i = Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'))
  return i >= 0 ? path.slice(i + 1) : path
}
</script>

<template>
  <div v-if="changes.length" class="turn-changes">
    <AppIcon name="git-diff" size="ic-2xs" />
    <span class="changes-title">{{ t('chat.changesCount', { n: changes.length }) }}</span>
    <button
      v-for="c in changes"
      :key="c.path"
      type="button"
      class="change-chip"
      :title="`${c.path}  +${c.add} −${c.del}`"
      @click="router.push({ name: 'workspace' })"
    >
      <span class="change-name">{{ fileName(c.path) }}</span>
      <span v-if="c.add" class="num add">+{{ c.add }}</span>
      <span v-if="c.del" class="num del">−{{ c.del }}</span>
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
  gap: var(--wb-sp-1) var(--wb-sp-2);
  margin-top: var(--wb-sp-1);
  color: var(--wb-muted);
  user-select: none;
}
.changes-title {
  font-size: var(--wb-fs-2xs);
  white-space: nowrap;
}
.change-chip {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  max-width: calc(220px * var(--wb-vw-scale));
  padding: 1px var(--wb-sp-2);
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-full);
  background: var(--wb-elev-raised);
  color: var(--wb-muted);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  overflow: hidden;
  white-space: nowrap;
  transition:
    color var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.change-chip:hover {
  color: var(--wb-primary);
  border-color: var(--wb-primary);
  transform: scale(1.06);
}
.change-chip:active {
  transform: scale(0.92);
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
  color: var(--wb-success);
}
.num.del {
  color: var(--wb-danger);
}
.changes-total {
  display: inline-flex;
  gap: var(--wb-sp-2);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
}
</style>
