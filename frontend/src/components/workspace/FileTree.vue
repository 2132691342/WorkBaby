<script setup lang="ts">
// 文件树：工作区左栏，写靠助手、界面只读。
// 点目录逐级进入，点文件就地预览（文本抽屉或文档预览），不离开工作区。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import type { FileEntryVO } from '../../types/api'
import { fmtBytes } from '../../utils/num'
import AppIcon from '../common/AppIcon.vue'
import DocPreviewDrawer from '../doc/DocPreviewDrawer.vue'
import { errorText } from '../../utils/errors'

const { t } = useI18n()

const cwd = ref('')
const entries = ref<FileEntryVO[]>([])
const loading = ref(true)
const error = ref('')

const docPath = ref('')

const dirs = computed(() => entries.value.filter((e) => e.entry_is_dir))
const files = computed(() => entries.value.filter((e) => !e.entry_is_dir))

/** 面包屑：第一项是根（不带路径），后面逐级拼接。 */
const crumbs = computed(() => {
  const parts = cwd.value.split('/').filter(Boolean)
  const out = [{ label: t('files.root'), path: '' }]
  let acc = ''
  for (const part of parts) {
    acc = acc ? `${acc}/${part}` : part
    out.push({ label: part, path: acc })
  }
  return out
})

async function load(path: string) {
  loading.value = true
  error.value = ''
  try {
    const r = await api.files.list(path)
    cwd.value = r.resp_path
    entries.value = r.resp_entries ?? []
  } catch (e) {
    error.value = errorText(e) || t('common.loadFailed', { msg: '' })
    entries.value = []
  } finally {
    loading.value = false
  }
}

/** 返回上一级；已在根目录时不动作。 */
function up() {
  const parts = cwd.value.split('/').filter(Boolean)
  parts.pop()
  void load(parts.join('/'))
}

function openEntry(entry: FileEntryVO) {
  if (entry.entry_is_dir) {
    void load(entry.entry_path)
    return
  }
  // 文本 / 代码 / 未知扩展名都交给 DocPreview：它自己读得出就读，读不出给一句「不支持」
  docPath.value = entry.entry_path
}

onMounted(() => {
  void load('')
})

defineExpose({ load })
</script>

<template>
  <section class="tree wb-card">
    <header class="tree-head">
      <button
        class="tree-up"
        type="button"
        :disabled="!cwd"
        :title="t('files.up')"
        @click="up"
      >
        <AppIcon name="arrow-left" size="ic-xs" />
      </button>
      <nav class="crumbs">
        <button
          v-for="(c, i) in crumbs"
          :key="c.path"
          type="button"
          class="crumb"
          :class="{ 'is-last': i === crumbs.length - 1 }"
          @click="load(c.path)"
        >
          {{ c.label }}
        </button>
      </nav>
      <button class="tree-up" type="button" :title="t('common.refresh')" @click="load(cwd)">
        <AppIcon name="refresh" size="ic-xs" />
      </button>
    </header>

    <div class="tree-body">
      <div v-if="loading" class="tree-state"><el-skeleton animated :rows="4" /></div>
      <div v-else-if="error" class="tree-state is-error">{{ error }}</div>
      <div v-else-if="!entries.length" class="tree-state">{{ t('files.empty') }}</div>
      <template v-else>
        <button
          v-for="d in dirs"
          :key="d.entry_path"
          type="button"
          class="tree-row"
          @click="openEntry(d)"
        >
          <AppIcon name="folder" size="ic-xs" class="is-dir" />
          <span class="tree-name">{{ d.entry_name }}</span>
          <span class="tree-meta">›</span>
        </button>
        <button
          v-for="f in files"
          :key="f.entry_path"
          type="button"
          class="tree-row"
          @click="openEntry(f)"
        >
          <AppIcon name="doc" size="ic-xs" />
          <span class="tree-name">{{ f.entry_name }}</span>
          <span class="tree-meta">{{ fmtBytes(f.entry_size) }}</span>
        </button>
      </template>
    </div>

    <DocPreviewDrawer v-model="docPath" />
  </section>
</template>

<style scoped>
.tree {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}
.tree-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  padding: var(--wb-sp-2);
  border-bottom: 1px solid var(--wb-border);
  flex: none;
}
.tree-up {
  display: grid;
  place-items: center;
  width: var(--wb-ctl-h-sm);
  height: var(--wb-ctl-h-sm);
  border-radius: var(--wb-radius-sm);
  color: var(--wb-muted);
  flex: none;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.tree-up:hover:not(:disabled) {
  background: var(--wb-elev-hover);
  color: var(--wb-ink);
}
.tree-up:active:not(:disabled) {
  transform: scale(var(--wb-btn-scale-press));
}
.tree-up:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.crumbs {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 2px;
  overflow: hidden;
}
.crumb {
  padding: 1px var(--wb-sp-1);
  border-radius: var(--wb-radius-xs);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  white-space: nowrap;
  transition: color var(--wb-dur-fast) var(--wb-ease);
}
.crumb:hover {
  color: var(--wb-primary);
}
.crumb.is-last {
  color: var(--wb-ink);
}
.crumb::after {
  content: '/';
  color: var(--wb-faint);
  margin-left: var(--wb-sp-1);
}
.crumb.is-last::after {
  content: '';
}
.tree-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-1);
}
.tree-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: 3px var(--wb-sp-2);
  border-radius: var(--wb-radius-xs);
  color: var(--wb-ink-2);
  text-align: left;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
/* 整行元素：只缩不放（行宽 100%，放大会被裁） */
.tree-row:hover {
  background: var(--wb-elev-hover);
  transform: translateX(2px);
}
.tree-row:active {
  transform: translateX(2px) scale(var(--wb-row-scale-press));
}
.tree-row :deep(svg) {
  flex: none;
  color: var(--wb-muted);
}
.tree-row .is-dir {
  color: var(--wb-primary);
}
.tree-name {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-2xs);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tree-meta {
  flex: none;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
}
.tree-state {
  padding: var(--wb-sp-4);
  color: var(--wb-faint);
  font-size: var(--wb-fs-2xs);
  text-align: center;
}
.tree-state.is-error {
  color: var(--wb-danger);
}
.tree-note {
  padding: var(--wb-sp-2) var(--wb-sp-3);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-warning);
}
.tree-pre {
  margin: 0;
  padding: var(--wb-sp-3);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  line-height: var(--wb-lh-base);
  color: var(--wb-ink-2);
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
