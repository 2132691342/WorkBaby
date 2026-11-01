<script setup lang="ts">
// @ 引用浏览：面包屑 + 目录逐级进入 + 名字过滤，选中文件即成为待发送附件。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import type { FileEntryVO } from '../../types/api'
import { useToastStore } from '../../stores/toast'
import AppIcon from '../common/AppIcon.vue'

const emit = defineEmits<{
  /** 选中一个文件：参数是相对工作区的路径 */
  pick: [entry: FileEntryVO]
  /** 转去系统文件选择框 */
  pickExternal: []
  /** 原始 pointerdown：父组件据此阻止点菜单时输入框丢焦点 */
  pointerdown: [event: PointerEvent]
}>()

const { t } = useI18n()
const toast = useToastStore()

const path = ref('')
const entries = ref<FileEntryVO[]>([])
const loading = ref(false)
const filter = ref('')

const crumbs = computed(() => {
  const parts = path.value ? path.value.split('/').filter(Boolean) : []
  return [
    { name: t('chat.mentionRoot'), path: '' },
    ...parts.map((p, i) => ({ name: p, path: parts.slice(0, i + 1).join('/') })),
  ]
})

const shown = computed(() => {
  const q = filter.value.trim().toLowerCase()
  return q ? entries.value.filter((e) => e.entry_name.toLowerCase().includes(q)) : entries.value
})

/** 键盘高亮下标：目录切换与过滤变化都要归零，否则回车会选中看不见的一行。 */
const index = ref(0)
watch([path, filter], () => (index.value = 0))

function currentIndex(): number {
  return index.value
}

function setIndex(next: number) {
  index.value = next
}

// 目录请求用序号判新旧：连点两个目录时两次请求并发返回，只看 loading 会错位
let seq = 0
async function loadDir(next: string) {
  const mine = ++seq
  loading.value = true
  path.value = next
  filter.value = ''
  try {
    const resp = await api.files.list(next)
    if (mine !== seq) return
    entries.value = resp.resp_entries ?? []
  } catch (e) {
    if (mine !== seq) return
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
    entries.value = []
  } finally {
    if (mine === seq) loading.value = false
  }
}

function enter(entry: FileEntryVO) {
  if (!entry.entry_is_dir) {
    emit('pick', entry)
    return
  }
  void loadDir(path.value ? `${path.value}/${entry.entry_name}` : entry.entry_name)
}

function up() {
  const idx = path.value.lastIndexOf('/')
  void loadDir(idx > 0 ? path.value.slice(0, idx) : '')
}

/** 回车提交：选中项可能因过滤变短而越界，取下界。 */
function pickAt(i: number) {
  const entry = shown.value[Math.min(i, shown.value.length - 1)]
  if (entry) enter(entry)
}

/** 当前路径拼出的相对路径（父组件拿它判断是否重复）。 */
function relativeOf(entry: FileEntryVO): string {
  return path.value ? `${path.value}/${entry.entry_name}` : entry.entry_name
}

defineExpose({ loadDir, shown, pickAt, up, relativeOf, currentIndex, setIndex })
</script>

<template>
  <div class="pop menu-pop is-mention" @pointerdown="emit('pointerdown', $event)">
    <div class="mb-head">
      <button class="mb-btn" type="button" :title="t('chat.mentionUp')" :disabled="!path" @click="up">
        <AppIcon name="chevron-left" size="ic-xs" />
      </button>
      <div class="crumbs">
        <template v-for="(cr, i) in crumbs" :key="cr.path || '__root__'">
          <span v-if="i" class="crumb-sep">/</span>
          <button
            class="crumb"
            :class="{ 'is-cur': i === crumbs.length - 1 }"
            type="button"
            @click="loadDir(cr.path)"
          >
            {{ cr.name }}
          </button>
        </template>
      </div>
      <input v-model="filter" class="mb-filter" :placeholder="t('chat.mentionFilter')" />
    </div>
    <div class="mb-list">
      <div v-if="loading" class="pop-row is-muted"><span class="pop-spin" />{{ t('common.loading') }}</div>
      <div v-else-if="!shown.length" class="pop-row is-muted">
        {{ filter ? t('chat.mentionNoMatch') : t('chat.mentionEmpty') }}
      </div>
      <button
        v-for="(e, i) in shown"
        :key="e.entry_name"
        class="pop-row"
        :class="{ 'is-hi': i === index }"
        type="button"
        @mouseenter="index = i"
        @click="enter(e)"
      >
        <AppIcon :name="e.entry_is_dir ? 'folder' : 'doc'" size="ic-xs" />
        <span class="pop-grow"><b class="pop-t">{{ e.entry_name }}</b></span>
        <AppIcon v-if="e.entry_is_dir" name="chevron-right" size="ic-xs" />
      </button>
    </div>
    <button class="pop-row mb-out" type="button" @click="emit('pickExternal')">
      <AppIcon name="search" size="ic-xs" />
      <span class="pop-grow">
        <b class="pop-t">{{ t('chat.attachPick') }}</b>
        <span class="pop-d">{{ t('chat.attachPickHint') }}</span>
      </span>
    </button>
  </div>
</template>

<style scoped>
/* 弹出层外壳与列表行样式在 content.css（多个菜单共用），
   这里只留目录浏览特有的头部、面包屑与过滤框。 */
.menu-pop.is-mention {
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
}
.mb-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2);
  border-bottom: 1px solid var(--wb-border);
  flex: none;
}
.mb-btn {
  display: grid;
  place-items: center;
  width: calc(22px * var(--wb-vw-scale));
  height: calc(22px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  color: var(--wb-muted);
}
.mb-btn:hover:not(:disabled) {
  background: var(--wb-elev-hover);
  color: var(--wb-ink);
}
.mb-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}
.crumbs {
  display: flex;
  align-items: center;
  gap: 2px;
  min-width: 0;
  overflow: hidden;
  flex: 1;
}
.crumb {
  padding: 2px 4px;
  border-radius: var(--wb-radius-xs);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-muted);
  white-space: nowrap;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}
.crumb:hover {
  background: var(--wb-elev-hover);
  color: var(--wb-ink);
}
.crumb.is-cur {
  color: var(--wb-ink);
}
.crumb-sep {
  color: var(--wb-muted);
  font-size: var(--wb-fs-2xs);
}
.mb-filter {
  width: calc(88px * var(--wb-vw-scale));
  height: var(--wb-ctl-h-sm);
  padding: 0 var(--wb-sp-2);
  flex: none;
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-elev-raised);
  color: var(--wb-ink);
  font-size: var(--wb-fs-2xs);
  outline: none;
}
.mb-filter:focus {
  border-color: var(--wb-primary);
}
.mb-list {
  overflow-y: auto;
  min-height: 0;
  flex: 1;
  padding: var(--wb-sp-1);
}
.mb-out {
  border-top: 1px solid var(--wb-border);
  border-radius: 0;
  flex: none;
}
</style>
