<script setup lang="ts">
// 技能清单：独立技能页与设置页共用同一份渲染、开关与新建逻辑。
// 技能来自内置包 / 用户目录 / 工作区三处；开关由后端落库，长清单走分页。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { SkillVO } from '../../types/api'
import { pickFile } from '../../api/shell'
import AppIcon from '../common/AppIcon.vue'
import CodeEditor from '../common/CodeEditor.vue'
import ResizableDrawer from '../common/ResizableDrawer.vue'
import { errorText } from '../../utils/errors'

const { t } = useI18n()
const store = useSettingsStore()
const toast = useToastStore()

const PAGE_SIZES = [5, 10, 20, 50]

const busy = ref('')
const filter = ref('')
const source = ref<string>('all')
const page = ref(1)
const pageSize = ref(10)
const opened = ref<SkillVO | null>(null)
const content = ref('')
const drawer = ref(false)
const creating = ref(false)
const draft = ref({ name: '', description: '', body: '' })

const SOURCES = ['builtin', 'user', 'workspace'] as const

// 来源文案键只有 workspace 收录过：其余退到语义一致的既有键，避免亮出原始键名
const SOURCE_KEYS: Record<string, string> = {
  builtin: 'settings.tools.builtin',
  user: 'settings.skills.mine',
  workspace: 'settings.skills.workspace',
}

// 组内保持扫描原序，组间按内置 → 用户 → 工作区排
const filtered = computed(() => {
  const q = filter.value.trim().toLowerCase()
  const items = (store.skills ?? []).filter(
    (s) =>
      !q ||
      s.skill_name.toLowerCase().includes(q) ||
      (s.skill_description || '').toLowerCase().includes(q),
  )
  const bySource = SOURCES.map((key) => items.filter((s) => s.skill_source === key)).flat()
  return source.value === 'all' ? bySource : bySource.filter((s) => s.skill_source === source.value)
})

const paged = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filtered.value.slice(start, start + pageSize.value)
})

// 搜索词 / 来源 / 每页行数一变就回第一页；页码越界时收回最后一页
watch([filter, source, pageSize], () => {
  page.value = 1
})
watch(
  () => filtered.value.length,
  (n) => {
    const last = Math.max(1, Math.ceil(n / pageSize.value))
    if (page.value > last) page.value = last
  },
)

const sourceLabel = (s: string) => t(SOURCE_KEYS[s] ?? 'common.unknown')

const counts = computed(() => ({
  total: store.skills?.length ?? 0,
  enabled: (store.skills ?? []).filter((s) => s.skill_enabled).length,
}))

async function reload() {
  await store.loadSkills()
}

async function toggle(s: SkillVO) {
  if (busy.value) return
  busy.value = s.skill_name
  try {
    await api.skills.toggle(s.skill_name, !s.skill_enabled)
    await reload()
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

async function open(s: SkillVO) {
  opened.value = s
  content.value = ''
  drawer.value = true
  try {
    const r = await api.skills.content(s.skill_name)
    content.value = r.resp_content
  } catch (e) {
    content.value = t('common.loadFailed', { msg: errorText(e) })
  }
}

async function remove(s: SkillVO) {
  if (busy.value) return
  busy.value = `del:${s.skill_name}`
  try {
    await api.skills.remove(s.skill_name)
    await reload()
    toast.ok(`${t('common.delete')} ${s.skill_name}`)
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

async function importSkill() {
  if (busy.value) return
  busy.value = 'import'
  try {
    const path = await pickFile(t('settings.skills.importHint'), 'SKILL.md')
    if (!path) return
    const r = await api.skills.import([path])
    await reload()
    if (r.resp_imported) toast.ok(t('common.imported', { n: r.resp_imported }))
    if (r.resp_skipped?.length) toast.info(t('common.skipped', { n: r.resp_skipped.length }))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

async function createSkill() {
  if (busy.value) return
  if (!draft.value.name.trim()) return
  busy.value = 'create'
  try {
    await api.skills.create({
      name: draft.value.name.trim(),
      description: draft.value.description,
      body: draft.value.body,
    })
    await reload()
    creating.value = false
    draft.value = { name: '', description: '', body: '' }
    toast.ok(t('common.save'))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

onMounted(() => {
  void reload()
})

defineExpose({ reload, counts, importSkill })
</script>

<template>
  <div class="skill-wrap">
    <div v-if="$slots.actions" class="skill-acts-row">
      <slot name="actions" />
    </div>

    <div class="skill-bar">
      <el-input v-model="filter" size="small" :placeholder="t('common.search')" clearable class="skill-search">
        <template #prefix><AppIcon name="search" size="ic-xs" /></template>
      </el-input>
      <el-radio-group v-model="source" size="small">
        <el-radio-button value="all">{{ t('common.all') }}</el-radio-button>
        <el-radio-button v-for="s in SOURCES" :key="s" :value="s">
          {{ sourceLabel(s) }}
        </el-radio-button>
      </el-radio-group>
      <span class="sp" />
      <span class="wb-label">{{ counts.enabled }} / {{ counts.total }}</span>
    </div>

    <div v-if="store.skillsLoading && !store.skills.length" class="wb-card pad">
      <el-skeleton animated :rows="4" />
    </div>

    <div v-else-if="!store.skills.length" class="wb-card empty-box">
      <span class="wb-tile wb-tile--lg"><AppIcon name="book" size="ic-xl" /></span>
      <b>{{ t('settings.skills.empty') }}</b>
      <p>{{ t('settings.skills.emptyHint') }}</p>
    </div>

    <template v-else>
      <div class="skill-list">
        <article
          v-for="s in paged"
          :key="s.skill_name"
          class="skill"
          :class="{ 'is-off': !s.skill_enabled }"
        >
          <button class="skill-main" type="button" @click="open(s)">
            <span class="skill-name">
              {{ s.skill_name }}
              <el-tag size="small" :class="`is-src-${s.skill_source}`">{{ sourceLabel(s.skill_source) }}</el-tag>
            </span>
            <span class="skill-desc">{{ s.skill_description || '—' }}</span>
          </button>
          <div class="skill-acts">
            <el-tag size="small" :type="s.skill_enabled ? 'primary' : 'info'">
              {{ s.skill_enabled ? t('settings.skills.enabled') : t('settings.skills.disabled') }}
            </el-tag>
            <el-switch
              :model-value="s.skill_enabled"
              :loading="busy === s.skill_name"
              :disabled="s.skill_source === 'builtin' || !!busy"
              @change="toggle(s)"
            />
            <el-popconfirm
              v-if="s.skill_source !== 'builtin'"
              :title="t('settings.skills.deleteTitle')"
              width="240"
              @confirm="remove(s)"
            >
              <template #reference>
                <button class="skill-del" type="button" :disabled="!!busy">
                  <AppIcon name="trash" size="ic-xs" />
                </button>
              </template>
            </el-popconfirm>
            <el-tooltip v-else :content="t('settings.skills.readOnly')" placement="top">
              <span class="skill-lock"><AppIcon name="shield" size="ic-xs" /></span>
            </el-tooltip>
          </div>
        </article>

        <div v-if="!paged.length" class="wb-card no-match">{{ t('settings.skills.noMatch') }}</div>
      </div>

      <div class="pager">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          size="small"
          background
          layout="total, sizes, prev, pager, next"
          :total="filtered.length"
          :page-sizes="PAGE_SIZES"
        />
      </div>
    </template>

    <ResizableDrawer
      v-model="drawer"
      storage-key="skill-content"
      :title="opened?.skill_name || t('settings.skills.content')"
    >
      <p v-if="opened?.skill_description" class="skill-drawer-desc">{{ opened.skill_description }}</p>
      <CodeEditor :model-value="content" lang="markdown" readonly :min-height="420" />
    </ResizableDrawer>

    <el-dialog v-model="creating" :title="t('settings.skills.create')" width="640px" destroy-on-close>
      <el-form label-position="top">
        <el-form-item :label="t('settings.skills.createName')">
          <el-input v-model="draft.name" placeholder="my-skill" />
        </el-form-item>
        <el-form-item :label="t('settings.skills.createDesc')">
          <el-input v-model="draft.description" />
        </el-form-item>
        <el-form-item :label="t('settings.skills.createBody')">
          <el-input v-model="draft.body" type="textarea" :rows="10" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="creating = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="busy === 'create'" :disabled="!draft.name.trim()" @click="createSkill">
          {{ t('common.create') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.skill-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.skill-acts-row {
  display: flex;
  justify-content: flex-end;
  gap: var(--wb-sp-2);
}
.skill-bar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.skill-search {
  width: calc(220px * var(--wb-vw-scale));
}
.sp {
  flex: 1;
}
.skill-list {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.skill {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  transition:
    border-color var(--wb-dur) var(--wb-ease),
    box-shadow var(--wb-dur) var(--wb-ease),
    opacity var(--wb-dur) var(--wb-ease);
}
.skill:hover {
  border-color: var(--wb-border-strong);
  box-shadow: var(--wb-card-shadow);
}
.skill.is-off {
  opacity: 0.62;
}
.skill-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  text-align: left;
  cursor: pointer;
}
.skill-name {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
}
.skill-desc {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.skill-acts {
  flex: none;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.skill-del {
  display: flex;
  align-items: center;
  justify-content: center;
  width: calc(24px * var(--wb-vw-scale));
  height: calc(24px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-sm);
  color: var(--wb-muted);
  cursor: pointer;
  transition:
    background var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.skill-del:hover {
  background: var(--wb-danger-soft);
  color: var(--wb-danger);
  transform: scale(1.08);
}
.skill-del:active {
  transform: scale(var(--wb-btn-scale-press));
}
.skill-lock {
  display: flex;
  align-items: center;
  justify-content: center;
  width: calc(24px * var(--wb-vw-scale));
  height: calc(24px * var(--wb-vw-scale));
  color: var(--wb-muted);
  opacity: 0.6;
}
.pager {
  display: flex;
  justify-content: flex-end;
}
.pad {
  padding: var(--wb-sp-4);
}
.empty-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8) var(--wb-sp-6);
  text-align: center;
}
.empty-box b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.empty-box p {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.no-match {
  padding: var(--wb-sp-5);
  text-align: center;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.skill-drawer-desc {
  margin-bottom: var(--wb-sp-3);
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
</style>
