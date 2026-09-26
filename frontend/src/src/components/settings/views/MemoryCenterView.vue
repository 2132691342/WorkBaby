<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { Trash2 } from '@/components/common/icons'
import { useMemoryStore } from '@/stores/memory'
import { t } from '@/i18n'
import type { MemoryEntry } from '@/types/api'

/**
 * 长期记忆：一份 MEMORY.md 的浏览 / 检索 / 追加 / 删除。
 * 三分类（情景 / 语义 / 程序）与回写收件箱已下线——记忆只有一处，写入即生效。
 */
const memory = useMemoryStore()
const { overview, items, loading, query, error } = storeToRefs(memory)
const { load, search, append, remove } = memory

const draft = ref('')
const draftSection = ref('事实')

const sectionOptions = computed(() => [
  { key: '用户偏好', label: t('memory.section.preference') },
  { key: '事实', label: t('memory.section.fact') },
  { key: '流程', label: t('memory.section.procedure') },
  { key: '项目约定', label: t('memory.section.project') }
])

/** 按分节分组展示（保持后端固定分节序）。 */
const grouped = computed(() => {
  const map = new Map<string, MemoryEntry[]>()
  for (const it of items.value) {
    const list = map.get(it.section) ?? []
    list.push(it)
    map.set(it.section, list)
  }
  return [...map.entries()].map(([section, list]) => ({ section, list }))
})

/** 各分节计数（用于顶部条形分布可视化）。 */
const sectionCounts = computed(() =>
  sectionOptions.value.map((s) => ({
    key: s.key,
    label: s.label,
    count: items.value.filter((it) => it.section === s.key).length
  }))
)
/** 用于把分布条按最大档归一化（每条 fill% = count / maxCount）。 */
const maxCount = computed(
  () => sectionCounts.value.reduce((m, s) => (s.count > m ? s.count : m), 0)
)

function sectionLabel(section: string): string {
  return sectionOptions.value.find((s) => s.key === section)?.label ?? section
}

async function submit(): Promise<void> {
  const content = draft.value.trim()
  if (!content) return
  await append({ section: draftSection.value, content })
  draft.value = ''
}

onMounted(load)
</script>

<template>
  <div class="wb-ui" style="display: flex; flex-direction: column; gap: 12px">
    <div class="card p-sm">
      <div class="flex-r mb10">
        <strong>{{ t('memory.title') }}</strong>
        <span class="badge b-info">{{ t('memory.entries', overview.entries) }}</span>
      </div>
      <p class="muted">{{ t('memory.subtitle') }}</p>
      <p class="muted mono" style="margin-top: 6px">{{ t('memory.file') }}：{{ overview.file }}</p>
    </div>

    <div class="card p-sm">
      <div class="flex-r mb10">
        <select v-model="draftSection" class="input" style="max-width: 9rem">
          <option v-for="s in sectionOptions" :key="s.key" :value="s.key">{{ s.label }}</option>
        </select>
        <input
          v-model="draft"
          class="input"
          style="flex: 1"
          :placeholder="t('memory.addPlaceholder')"
          @keyup.enter="submit"
        />
        <button class="btn btn-sm btn-primary" :disabled="!draft.trim()" @click="submit">
          {{ t('memory.add') }}
        </button>
      </div>
    </div>

    <div class="card p-sm">
      <div class="flex-r mb10">
        <input
          v-model="query"
          class="input"
          style="flex: 1"
          :placeholder="t('memory.searchPlaceholder')"
          @keyup.enter="search"
        />
        <button class="btn btn-sm" @click="search">{{ t('ui.action.search') }}</button>
      </div>

      <p v-if="error" class="empty">{{ error }}</p>
      <p v-else-if="loading" class="empty">{{ t('ui.status.loading') }}</p>
      <p v-else-if="items.length === 0" class="empty">{{ t('memory.empty') }}</p>

      <!-- 分节分布 + 条目列表：两个 v-else 不能并列，包在一个 template v-else 里 -->
      <template v-else>
        <!-- 分节分布：4 条按比例的水平条 -->
        <div class="mem-dist">
          <div class="mem-dist__label">{{ t('memory.distribution') }}</div>
          <div class="mem-dist__rows">
            <div v-for="s in sectionCounts" :key="s.key" class="mem-dist__row">
              <span class="mem-dist__lbl">{{ s.label }}</span>
              <span class="mem-dist__track">
                <span
                  class="mem-dist__fill"
                  :class="`fill-${s.key}`"
                  :style="{ width: maxCount ? (s.count / maxCount * 100) + '%' : '0%' }"
                />
              </span>
              <strong class="mem-dist__num tnum">{{ s.count }}</strong>
            </div>
          </div>
        </div>

        <!-- 已有条目分组列表 -->
        <div v-for="g in grouped" :key="g.section" style="margin-bottom: 12px">
          <div class="muted" style="margin-bottom: 4px">{{ sectionLabel(g.section) }}</div>
          <div v-for="it in g.list" :key="it.section + it.text" class="flex-r">
            <span style="flex: 1">{{ it.text }}</span>
            <button class="btn btn-sm" :title="t('ui.action.delete')" @click="remove(it)">
              <Trash2 class="ic-sm" />
            </button>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
/* 分节分布：每行一格宽度的小型条形条，颜色与设计系统强调色对齐 */
.mem-dist {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 12px;
  padding: 10px 12px;
  border: 1px solid var(--wb-border);
  border-radius: 12px;
  background: var(--wb-bg);
}
.mem-dist__label {
  font-size: 11px;
  color: var(--wb-muted);
  font-weight: 500;
}
.mem-dist__rows {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.mem-dist__row {
  display: grid;
  grid-template-columns: 5.5rem 1fr 2.5rem;
  align-items: center;
  gap: 10px;
}
.mem-dist__lbl {
  font-size: 11.5px;
  color: var(--wb-ink);
}
.mem-dist__track {
  position: relative;
  height: 8px;
  border-radius: 999px;
  background: var(--wb-surface-hover);
  overflow: hidden;
}
.mem-dist__fill {
  position: absolute;
  inset: 0 auto 0 0;
  border-radius: 999px;
  transition: width 0.32s cubic-bezier(0.22, 1, 0.36, 1);
}
.mem-dist__num {
  font-size: 12px;
  text-align: right;
  color: var(--wb-ink);
}
/* 4 个分节各自用一道强调色（与品牌色板呼应） */
.fill-\u7528\u6237\u504f\u597d { background: linear-gradient(90deg, #63a4ff, var(--wb-primary)); }
.fill-\u4e8b\u5b9e { background: linear-gradient(90deg, #3cd69c, var(--wb-mint)); }
.fill-\u6d41\u7a0b { background: linear-gradient(90deg, #a68bff, var(--wb-lavender)); }
.fill-\u9879\u76ee\u7ea6\u5b9a { background: linear-gradient(90deg, #ffb44d, var(--wb-lemon)); }
</style>
