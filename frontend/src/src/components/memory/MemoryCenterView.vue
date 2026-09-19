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

      <div v-for="g in grouped" v-else :key="g.section" style="margin-bottom: 12px">
        <div class="muted" style="margin-bottom: 4px">{{ sectionLabel(g.section) }}</div>
        <div v-for="it in g.list" :key="it.section + it.text" class="flex-r">
          <span style="flex: 1">{{ it.text }}</span>
          <button class="btn btn-sm" :title="t('ui.action.delete')" @click="remove(it)">
            <Trash2 class="ic-sm" />
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
