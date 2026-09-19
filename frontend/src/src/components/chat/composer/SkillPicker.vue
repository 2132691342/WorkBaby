<script setup lang="ts">
/**
 * 技能选择器：工具行「技能」按钮唤起，列出全部已启用技能供点选。
 *
 * <p>与 `@` 提及选择器分工不同：这里不依赖输入框里的触发字符，
 * 点按钮就一定弹得出来（技能列表先加载完再展开，杜绝「空列表一闪而过」）。
 * 选中后把 `@技能名` 插入光标处，落点仍是输入框，后端 Skill 匹配照常生效。
 */
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { Search, Zap } from '@/components/common/icons'
import { t } from '@/i18n'
import { useSkillsStore } from '@/stores/skills'
import type { Skill } from '@/types/api'

const props = defineProps<{ visible: boolean }>()

const emit = defineEmits<{
  pick: [skill: Skill]
  close: []
}>()

const skills = useSkillsStore()
const query = ref('')
const activeIndex = ref(0)
const inputRef = ref<HTMLInputElement | null>(null)
const listRef = ref<HTMLElement | null>(null)

/** 名称 / 描述 / 使用场景模糊匹配；未启用技能不进列表（选了也不会生效）。 */
const filtered = computed<Skill[]>(() => {
  const q = query.value.trim().toLowerCase()
  const all = skills.skills.filter((s) => s.enabled !== false)
  if (!q) return all
  return all.filter((s) =>
    [s.name, s.description ?? '', s.when_to_use ?? ''].join(' ').toLowerCase().includes(q)
  )
})

/** 展开时：确保技能已加载 + 搜索框自动聚焦 + 高亮复位。 */
watch(
  () => props.visible,
  async (v) => {
    if (!v) return
    if (skills.skills.length === 0) await skills.load().catch(() => undefined)
    query.value = ''
    activeIndex.value = 0
    await nextTick()
    inputRef.value?.focus()
  },
  { immediate: true }
)

watch([activeIndex, filtered], async () => {
  await nextTick()
  listRef.value
    ?.querySelector<HTMLElement>('[data-active="true"]')
    ?.scrollIntoView({ block: 'nearest' })
})

function move(delta: number): void {
  const len = filtered.value.length
  if (len === 0) return
  activeIndex.value = (activeIndex.value + delta + len) % len
}

function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    move(1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    move(-1)
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const s = filtered.value[activeIndex.value]
    if (s) emit('pick', s)
  } else if (e.key === 'Escape') {
    e.preventDefault()
    emit('close')
  }
}

onMounted(() => {
  if (skills.skills.length === 0) void skills.load().catch(() => undefined)
})
</script>

<template>
  <transition
    enter-active-class="transition-all duration-150 ease-out origin-bottom"
    enter-from-class="scale-95 opacity-0"
    enter-to-class="scale-100 opacity-100"
    leave-active-class="transition-all duration-100 ease-in origin-bottom"
    leave-from-class="scale-100 opacity-100"
    leave-to-class="scale-95 opacity-0"
  >
    <div
      v-if="props.visible"
      class="wb-glass absolute bottom-full left-0 z-50 mb-2 w-80 overflow-hidden rounded-2xl border border-wb-border bg-wb-surface/95 shadow-[var(--wb-shadow-lg)] backdrop-blur"
      @keydown="onKeydown"
    >
      <div class="flex items-center gap-2 border-b border-wb-border px-3 py-2">
        <Search class="h-3.5 w-3.5 shrink-0 text-wb-muted" />
        <input
          ref="inputRef"
          v-model="query"
          type="text"
          class="min-w-0 flex-1 bg-transparent text-xs text-wb-ink outline-none placeholder:text-wb-muted"
          :placeholder="t('mention.searchSkill')"
          @keydown="onKeydown"
        />
        <span class="shrink-0 rounded bg-wb-primary/10 px-1.5 text-[10px] text-wb-primary">
          {{ filtered.length }}
        </span>
      </div>

      <p
        v-if="filtered.length === 0"
        class="px-3 py-4 text-center text-xs text-wb-muted"
      >
        {{ skills.loading ? t('common.loading') : t('mention.noResults') }}
      </p>

      <ul v-else ref="listRef" class="wb-scroll max-h-72 overflow-y-auto overscroll-contain p-1.5">
        <li v-for="(s, idx) in filtered" :key="s.id ?? s.name">
          <button
            type="button"
            :data-active="idx === activeIndex || undefined"
            class="flex w-full items-start gap-2.5 rounded-xl px-2.5 py-2 text-left transition-colors"
            :class="idx === activeIndex ? 'bg-wb-primary/10' : 'hover:bg-wb-primary/5'"
            @mouseenter="activeIndex = idx"
            @click="emit('pick', s)"
          >
            <span
              class="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-wb-lavender/15 text-wb-lavender"
            >
              <Zap class="h-3 w-3" />
            </span>
            <span class="min-w-0 flex-1">
              <span
                class="block truncate text-xs font-medium"
                :class="idx === activeIndex ? 'text-wb-primary-strong' : 'text-wb-ink'"
              >{{ s.name }}</span>
              <span v-if="s.description" class="block truncate text-[10px] text-wb-muted">
                {{ s.description }}
              </span>
              <span v-else-if="s.when_to_use" class="block truncate text-[10px] text-wb-muted">
                {{ s.when_to_use }}
              </span>
            </span>
          </button>
        </li>
      </ul>

      <p class="border-t border-wb-border px-3 py-1.5 text-[10px] text-wb-muted">
        {{ t('mention.skillHint') }}
      </p>
    </div>
  </transition>
</template>
