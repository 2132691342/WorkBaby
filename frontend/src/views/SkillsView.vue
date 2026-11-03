<script setup lang="ts">
// 技能页：一份清单 × 每引擎一列开关的矩阵（借鉴 cc-switch）。
// 两引擎认同一套 SKILL.md，扫描各走各的根；各自的目录仍是唯一事实——
// 矩阵只是视图与同步动作：「装到另一边」= 把用户级技能目录复制过去。
// 启停语义两家不同（omp 认 enabled，pi 认 disable-model-invocation），由后端分叉。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import AppIcon from '@/components/common/AppIcon.vue'
import SkillRowItem from '@/components/skills/SkillRow.vue'
import SkillDrawer from '@/components/skills/SkillDrawer.vue'
import SkillCreateDialog from '@/components/skills/SkillCreateDialog.vue'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import { useAppStore } from '@/stores/app'
import type { EngineKind, SkillMatrixRow, SkillRow } from '@/types/api'

type SkillSource = SkillRow['source']

// 分组顺序与引擎的加载优先级一致：工作区 → 用户 → 托管 → 插件
const SOURCES: SkillSource[] = ['project', 'user', 'managed', 'plugin']

const { t } = useI18n()
const toasts = useToasts()
const app = useAppStore()

const installedKinds = computed(() => app.installed.map((e) => e.kind))
const kind = computed<EngineKind>(() => app.active)

const rows = ref<SkillRow[]>([])
const loading = ref(false)
const filter = ref('')
const toggling = ref('')
const deleting = ref('')
const copying = ref('')

const contentOpen = ref(false)
const contentRow = ref<SkillRow | null>(null)
const content = ref('')
// 编辑态与草稿都在 SkillDrawer 里：它们只在那一个抽屉存在期间有意义。
const contentLoading = ref(false)
const saving = ref(false)

const createOpen = ref(false)
const creating = ref(false)

/** 矩阵：按名合并两引擎的扫描结果，同名在两边各占一格。 */
const matrix = computed<SkillMatrixRow[]>(() => {
  const map = new Map<string, SkillMatrixRow>()
  for (const row of rows.value) {
    let entry = map.get(row.name)
    if (!entry) {
      entry = { name: row.name, description: row.description, group_source: row.source, cells: {} }
      map.set(row.name, entry)
    }
    if (!entry.description) entry.description = row.description
    entry.cells[row.kind] = row
  }
  // 分组取两格里优先级最高的来源（SOURCES 顺序即优先级：工作区最前）
  const out = [...map.values()]
  for (const row of out) {
    const sources = Object.values(row.cells).map((c) => c!.source)
    row.group_source = SOURCES.find((s) => sources.includes(s)) ?? 'user'
    if (!row.description) row.description = Object.values(row.cells).find((c) => c!.description)?.description ?? ''
  }
  return out.sort((a, b) => a.name.localeCompare(b.name))
})

/** 每个引擎的启停能力：omp 认 caps.skill_toggle，pi 恒有（disable-model-invocation）。 */
const toggleable = computed<Partial<Record<EngineKind, boolean>>>(() => ({
  omp: Boolean(app.capsOf('omp')?.skill_toggle),
  pi: true,
}))

const filtered = computed(() => {
  const query = filter.value.trim().toLowerCase()
  if (!query) return matrix.value
  return matrix.value.filter((row) =>
    [row.name, row.description].some((field) => (field ?? '').toLowerCase().includes(query)),
  )
})

// 空分组不渲染：四个来源里通常只有两个有内容，空标题只是噪音
const groups = computed(() =>
  SOURCES.map((source) => ({ source, rows: filtered.value.filter((row) => row.group_source === source) })).filter(
    (group) => group.rows.length > 0,
  ),
)

async function load() {
  loading.value = true
  try {
    const data = await api.skills(undefined)
    rows.value = data.skills ?? []
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

async function toggle(cell: SkillRow, enabled: boolean) {
  const before = cell.enabled
  cell.enabled = enabled
  toggling.value = `${cell.kind}:${cell.name}`
  try {
    await api.toggleSkill(cell.path, enabled, cell.kind)
  } catch (err) {
    cell.enabled = before
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    toggling.value = ''
  }
}

/** 缺格补装：把另一边的用户级技能复制过来。 */
async function copyTo(row: SkillMatrixRow, target: EngineKind) {
  const src = Object.values(row.cells).find((c) => c?.editable && c.source === 'user')
  if (!src) return
  copying.value = `${row.name}:${target}`
  try {
    await api.skillCopyTo(src.path, target)
    toasts.push('ok', t('skills.copied', { engine: target }))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    copying.value = ''
  }
}

async function openContent(row: SkillMatrixRow) {
  const cell = row.cells.omp ?? row.cells.pi
  if (!cell) return
  contentRow.value = cell
  content.value = ''
  contentOpen.value = true
  contentLoading.value = true
  try {
    const data = await api.skillContent(cell.path)
    content.value = data.content ?? ''
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
    contentOpen.value = false
  } finally {
    contentLoading.value = false
  }
}

// 抽屉交上来的新正文：它已经在自己的草稿里改完了，这里只负责写盘与重载清单。
async function saveContent(text: string) {
  const cell = contentRow.value
  if (!cell || saving.value) return
  saving.value = true
  try {
    const data = await api.saveSkillContent(cell.kind, cell.path, text)
    content.value = data.content ?? text
    toasts.push('ok', t('skills.saved'))
    // 正文里可能改了 frontmatter（名字 / 描述 / 启用），清单得跟着重读。
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    saving.value = false
  }
}

async function remove(row: SkillMatrixRow) {
  const editable = Object.values(row.cells).filter((c) => c?.editable)
  if (!editable.length) return
  try {
    await ElMessageBox.confirm(t('skills.deleteConfirm', { name: row.name, engines: editable.map((c) => c!.kind).join(' / ') }), t('skills.deleteTitle'), {
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
    })
  } catch {
    return
  }
  deleting.value = row.name
  try {
    for (const cell of editable) {
      await api.deleteSkill(cell!.path, cell!.kind)
    }
    toasts.push('ok', t('common.saved'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    deleting.value = ''
  }
}

function openCreate() {
  createOpen.value = true
}

async function create(input: { name: string; description: string; body: string; targets: EngineKind[] }) {
  if (creating.value) return
  creating.value = true
  try {
    for (const target of input.targets) {
      await api.createSkill(input.name, input.description, input.body, target)
    }
    toasts.push('ok', t('common.saved'))
    createOpen.value = false
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    creating.value = false
  }
}

watch(kind, () => {
  rows.value = []
  void load()
})

onMounted(load)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('skills.title') }}</h1>
        <p class="page-sub">{{ t('skills.matrixSubtitle') }}</p>
      </div>
      <div class="head-actions">
        <span class="count">{{ t('skills.count', { n: matrix.length }) }}</span>
        <button
          class="wb-btn icon ghost"
          :title="t('common.refresh')"
          :class="{ 'is-loading': loading }"
          :disabled="loading"
          @click="load"
        >
          <AppIcon name="refresh" :size="15" />
        </button>
        <button class="wb-btn primary" @click="openCreate">
          <AppIcon name="plus" :size="14" />
          {{ t('skills.create') }}
        </button>
      </div>
    </header>

    <div class="page-body">
      <div class="filter-row">
        <input v-model="filter" class="wb-input" :placeholder="t('common.searchPlaceholder')" />
      </div>

      <section v-if="!matrix.length && !loading" class="wb-empty">
        <span class="wb-tile wb-tile--lg wb-tile--neutral"><AppIcon name="book" :size="22" /></span>
        <p class="wb-empty-title">{{ t('skills.emptyTitle') }}</p>
        <p class="wb-empty-hint">{{ t('skills.emptyHint') }}</p>
        <button class="wb-btn sm primary" @click="openCreate">{{ t('skills.create') }}</button>
      </section>

      <p v-else-if="!filtered.length" class="no-match">{{ t('skills.noMatch') }}</p>

      <template v-else>
        <section v-for="group in groups" :key="group.source" class="source-group">
          <div class="wb-sec-head">
            <span class="wb-sec-title">{{ t(`skills.sources.${group.source}`) }} <span class="count">{{ group.rows.length }}</span></span>
          </div>

          <div class="skill-list">
            <SkillRowItem
              v-for="row in group.rows"
              :key="row.name"
              :row="row"
              :toggleable="toggleable"
              :installed="installedKinds"
              :toggling="toggling"
              :deleting="deleting"
              :copying="copying"
              @toggle="toggle"
              @view="openContent"
              @remove="remove"
              @copy="copyTo"
            />
          </div>
        </section>
      </template>
    </div>

    <SkillDrawer
      v-model:open="contentOpen"
      :row="contentRow"
      :content="content"
      :loading="contentLoading"
      :saving="saving"
      @save="saveContent"
    />

    <SkillCreateDialog v-model:open="createOpen" :creating="creating" :installed="installedKinds" @create="create" />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--wb-sp-4);
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-4);
  border-bottom: 1px solid var(--wb-line);
  flex: none;
}

.page-title {
  margin: 0;
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 650;
  color: var(--wb-ink);
}

.page-sub {
  margin: 4px 0 0;
  max-width: var(--wb-w-desc, 620px);
  font-size: var(--wb-fs-sm);
  line-height: 1.65;
  color: var(--wb-mute);
}

.head-actions {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}

.page-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-10);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-5);
}

.filter-row {
  flex: none;
  max-width: 360px;
}

.count {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  letter-spacing: 0.02em;
}

.source-group {
  display: flex;
  flex-direction: column;
}

.skill-list {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}

/* 技能条目的样式随 SkillRow 搬走了 */
.no-match {
  margin: 0;
  padding: var(--wb-sp-6) 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
  text-align: center;
}

/* 抽屉与新建弹窗的样式随 SkillDrawer / SkillCreateDialog 搬走了 */
</style>
