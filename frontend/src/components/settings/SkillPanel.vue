<script setup lang="ts">
// 技能面板：列的就是 PI 会加载的那些技能（用户目录 / 工作区 / 插件包自带）。
// 启停靠把目录移出 PI 的扫描范围，不是在自己的库里记一个开关。
// 长清单走分页（默认 10 行，5/10/20/50 可选）；来源用行内标签区分。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { SkillVO } from '../../types/api'
import { OpenFileDialog } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'
import CodeEditor from '../common/CodeEditor.vue'
import SettingsSection from './SettingsSection.vue'

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

const SOURCES = ['global', 'workspace', 'package'] as const

// 组内保持扫描原序，组间按全局 → 工作区 → 包排
const filtered = computed(() => {
  const q = filter.value.trim().toLowerCase()
  const items = (store.skills ?? []).filter(
    (s) => !q || s.name.toLowerCase().includes(q) || (s.description || '').toLowerCase().includes(q),
  )
  const bySource = SOURCES.map((key) => items.filter((s) => s.source === key)).flat()
  return source.value === 'all' ? bySource : bySource.filter((s) => s.source === source.value)
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

const sourceLabel = (s: string) =>
  t((SOURCES as readonly string[]).includes(s) ? `settings.skills.${s}` : 'common.unknown')

const counts = computed(() => ({
  total: store.skills?.length ?? 0,
  enabled: (store.skills ?? []).filter((s) => s.enabled).length,
}))

async function reload() {
  await store.loadSkills()
}

async function toggle(s: SkillVO) {
  if (busy.value) return
  busy.value = s.id
  try {
    await api.skills.toggle(s.id, !s.enabled)
    await reload()
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function open(s: SkillVO) {
  opened.value = s
  content.value = ''
  drawer.value = true
  try {
    const r = await api.skills.content(s.id)
    content.value = r.content
  } catch (e) {
    content.value = t('common.loadFailed', { msg: (e as Error)?.message || '' })
  }
}

async function remove(s: SkillVO) {
  if (busy.value) return
  busy.value = `del:${s.id}`
  try {
    await api.skills.remove(s.id)
    await reload()
    toast.ok(`${t('common.delete')} ${s.name}`)
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function importSkill() {
  if (busy.value) return
  busy.value = 'import'
  try {
    const path = await OpenFileDialog(t('settings.skills.importHint'), 'SKILL.md')
    if (!path) return
    const r = await api.skills.import([path])
    await reload()
    if (r.imported) toast.ok(`${r.imported}`)
    if (r.skipped?.length) toast.info(r.skipped.slice(0, 3).join('; '))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
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
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

onMounted(() => {
  void reload()
})
</script>

<template>
  <div class="panel-wrap">
    <SettingsSection icon="book" :title="t('settings.skills.title')" :desc="t('settings.skills.desc')">
      <template #actions>
        <el-button size="small" :loading="busy === 'import'" :disabled="!!busy" @click="importSkill">
          {{ t('settings.skills.import') }}
        </el-button>
        <el-button size="small" type="primary" :disabled="!!busy" @click="creating = true">
          <AppIcon name="plus" size="ic-xs" />
          <span class="ml-1">{{ t('settings.skills.create') }}</span>
        </el-button>
      </template>

      <div class="skill-bar">
        <el-input v-model="filter" size="small" :placeholder="t('common.search')" clearable class="skill-search">
          <template #prefix><AppIcon name="search" size="ic-xs" /></template>
        </el-input>
        <el-radio-group v-model="source" size="small">
          <el-radio-button value="all">{{ t('common.all') }}</el-radio-button>
          <el-radio-button v-for="s in SOURCES" :key="s" :value="s">
            {{ t(`settings.skills.${s}`) }}
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
          <article v-for="s in paged" :key="s.id" class="skill" :class="{ 'is-off': !s.enabled }">
            <button class="skill-main" type="button" @click="open(s)">
              <span class="skill-name">
                {{ s.name }}
                <el-tag size="small" :class="`is-src-${s.source}`">{{ sourceLabel(s.source) }}</el-tag>
              </span>
              <span class="skill-desc">{{ s.description || '—' }}</span>
            </button>
            <div class="skill-acts">
              <el-tag size="small" :type="s.enabled ? 'primary' : 'info'">
                {{ s.enabled ? t('settings.skills.enabled') : t('settings.skills.disabled') }}
              </el-tag>
              <el-switch
                :model-value="s.enabled"
                :loading="busy === s.id"
                :disabled="s.source === 'package' || !!busy"
                @change="toggle(s)"
              />
              <el-popconfirm
                v-if="s.source !== 'package'"
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
    </SettingsSection>

    <el-drawer v-model="drawer" :title="opened?.name || t('settings.skills.content')" size="640px">
      <p v-if="opened?.description" class="skill-drawer-desc">{{ opened.description }}</p>
      <CodeEditor :model-value="content" lang="markdown" readonly :min-height="420" />
    </el-drawer>

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
.skill-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--wb-sp-3);
}
.skill-search {
  max-width: calc(220px * var(--wb-vw-scale));
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
    box-shadow var(--wb-dur) var(--wb-ease);
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
  gap: 1px;
  text-align: left;
}
.skill-name {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
}
.skill-name :deep(.el-tag) {
  flex: none;
}
.skill-name :deep(.el-tag.is-src-package) {
  background: var(--wb-accent-tint);
  border-color: transparent;
  color: var(--wb-primary);
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
.skill-del,
.skill-lock {
  display: grid;
  place-items: center;
  width: calc(22px * var(--wb-vw-scale));
  height: calc(22px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  color: var(--wb-muted);
}
.skill-del:hover:not(:disabled) {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
.skill-drawer-desc {
  margin-bottom: var(--wb-sp-3);
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.pager {
  display: flex;
  justify-content: flex-end;
  padding-top: var(--wb-sp-2);
}
.no-match {
  padding: var(--wb-sp-6) 0;
  text-align: center;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
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
.pad {
  padding: var(--wb-sp-4);
}
.ml-1 {
  margin-left: var(--wb-sp-1);
}
.sp {
  flex: 1;
}
</style>
