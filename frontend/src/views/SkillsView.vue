<script setup lang="ts">
// 技能页：一条清单，每条一行——来源分组、开关、查看、重置、删除。
// 只有一个引擎，所以不再做「矩阵」：一份技能对应一个目录，开关就是把目录搬进/搬出禁用区。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import AppIcon from '@/components/common/AppIcon.vue'
import SkillRowItem from '@/components/skills/SkillRow.vue'
import SkillDrawer from '@/components/skills/SkillDrawer.vue'
import SkillCreateDialog from '@/components/skills/SkillCreateDialog.vue'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import type { SkillRow, SkillSourceKind } from '@/types/api'

// 分组顺序与引擎的加载优先级一致：项目 → 内置 → 随包附带 → 托管
const SOURCES: SkillSourceKind[] = ['project', 'builtin', 'bundled', 'managed']

const { t } = useI18n()
const toasts = useToasts()

const rows = ref<SkillRow[]>([])
const builtinTotal = ref(0)
const builtinInstalled = ref(0)
const loading = ref(false)
const filter = ref('')
const toggling = ref('')
const deleting = ref('')
const resetting = ref('')

const contentOpen = ref(false)
const contentRow = ref<SkillRow | null>(null)
const content = ref('')
const contentLoading = ref(false)
const saving = ref(false)

const createOpen = ref(false)
const creating = ref(false)

const filtered = computed(() => {
  const query = filter.value.trim().toLowerCase()
  if (!query) return rows.value
  return rows.value.filter((row) =>
    [row.skill_name, row.skill_desc, row.skill_group].some((field) => (field ?? '').toLowerCase().includes(query)),
  )
})

// 空分组不渲染：四个来源里通常只有两三个有内容，空标题只是噪音。
const groups = computed(() =>
  SOURCES.map((source) => ({ source, rows: filtered.value.filter((row) => row.skill_source === source) })).filter(
    (group) => group.rows.length > 0,
  ),
)

async function load() {
  loading.value = true
  try {
    const data = await api.skills()
    rows.value = data.skills ?? []
    builtinTotal.value = data.builtin_total
    builtinInstalled.value = data.builtin_installed
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

async function toggle(row: SkillRow, enabled: boolean) {
  const before = row.skill_enabled
  row.skill_enabled = enabled
  toggling.value = row.skill_dir_name
  try {
    await api.toggleSkill(row.skill_path, enabled)
  } catch (err) {
    row.skill_enabled = before
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    toggling.value = ''
  }
}

async function reset(row: SkillRow) {
  resetting.value = row.skill_dir_name
  try {
    await api.resetSkill(row.skill_path)
    toasts.push('ok', t('skills.resetDone'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    resetting.value = ''
  }
}

async function openContent(row: SkillRow) {
  contentRow.value = row
  content.value = ''
  contentOpen.value = true
  contentLoading.value = true
  try {
    const data = await api.skillContent(row.skill_path)
    content.value = data.file_text ?? ''
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
    contentOpen.value = false
  } finally {
    contentLoading.value = false
  }
}

// 抽屉交上来的新正文：它已经在自己的草稿里改完了，这里只负责写盘与重载清单。
async function saveContent(text: string) {
  const row = contentRow.value
  if (!row || saving.value) return
  saving.value = true
  try {
    const data = await api.saveSkillContent(row.skill_path, text)
    content.value = data.file_text ?? text
    toasts.push('ok', t('skills.saved'))
    // 正文里可能改了 frontmatter（名字 / 描述），清单得跟着重读。
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    saving.value = false
  }
}

async function remove(row: SkillRow) {
  try {
    await ElMessageBox.confirm(t('skills.deleteConfirm', { name: row.skill_name }), t('skills.deleteTitle'), {
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
    })
  } catch {
    return
  }
  deleting.value = row.skill_dir_name
  try {
    await api.deleteSkill(row.skill_path)
    toasts.push('ok', t('common.saved'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    deleting.value = ''
  }
}

async function create(input: { name: string; description: string; body: string }) {
  if (creating.value) return
  creating.value = true
  try {
    await api.createSkill(input.name, input.description, input.body)
    toasts.push('ok', t('common.saved'))
    createOpen.value = false
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    creating.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('skills.title') }}</h1>
        <p class="page-sub">{{ t('skills.subtitle') }}</p>
      </div>
      <div class="head-actions">
        <span class="count">
          {{ t('skills.builtinProgress', { done: builtinInstalled, total: builtinTotal }) }}
        </span>
        <button
          class="wb-btn icon ghost"
          :title="t('common.refresh')"
          :class="{ 'is-loading': loading }"
          :disabled="loading"
          @click="load"
        >
          <AppIcon name="refresh" :size="15" />
        </button>
        <button class="wb-btn primary" @click="createOpen = true">
          <AppIcon name="plus" :size="14" />
          {{ t('skills.create') }}
        </button>
      </div>
    </header>

    <div class="page-body">
      <div class="filter-row">
        <input v-model="filter" class="wb-input" :placeholder="t('common.searchPlaceholder')" />
      </div>

      <section v-if="!rows.length && !loading" class="wb-empty">
        <span class="wb-tile wb-tile--lg wb-tile--neutral"><AppIcon name="book" :size="22" /></span>
        <p class="wb-empty-title">{{ t('skills.emptyTitle') }}</p>
        <p class="wb-empty-hint">{{ t('skills.emptyHint') }}</p>
        <button class="wb-btn sm primary" @click="createOpen = true">{{ t('skills.create') }}</button>
      </section>

      <p v-else-if="!filtered.length" class="no-match">{{ t('skills.noMatch') }}</p>

      <template v-else>
        <section v-for="group in groups" :key="group.source" class="source-group">
          <div class="wb-sec-head">
            <span class="wb-sec-title">
              {{ t(`skills.sources.${group.source}`) }} <span class="count">{{ group.rows.length }}</span>
            </span>
          </div>

          <div class="skill-list">
            <SkillRowItem
              v-for="row in group.rows"
              :key="row.skill_path"
              :row="row"
              :toggling="toggling"
              :deleting="deleting"
              :resetting="resetting"
              @toggle="toggle"
              @view="openContent"
              @remove="remove"
              @reset="reset"
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

    <SkillCreateDialog v-model:open="createOpen" :creating="creating" @create="create" />
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

.no-match {
  margin: 0;
  padding: var(--wb-sp-6) 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
  text-align: center;
}
</style>
