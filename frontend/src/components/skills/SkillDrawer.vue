<script setup lang="ts">
// 技能正文抽屉：看原文，用户目录下的那份可以就地改。
// 「能不能改」只由 row.editable 决定：项目级的是别人的文件，
// 改了会在下次升级时被覆盖，用户自己都说不清「我明明改过」。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import CodeEditor from '@/components/common/CodeEditor.vue'
import type { SkillRow } from '@/types/api'

const props = defineProps<{
  open: boolean
  row: SkillRow | null
  content: string
  loading: boolean
  saving: boolean
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  save: [text: string]
}>()

const { t } = useI18n()

const editing = ref(false)
const draftContent = ref('')

// 换了一条技能就退出编辑态：留着上一次的草稿会让人以为改的是新的那条。
watch(
  () => props.row?.skill_path,
  () => {
    editing.value = false
    draftContent.value = props.content
  },
  { immediate: true },
)

// 正文从外面到货时同步进草稿（只在没在编辑时，否则会顶掉用户正在打的字）。
watch(
  () => props.content,
  (next) => {
    if (!editing.value) draftContent.value = next
  },
)

const dirty = computed(() => editing.value && draftContent.value !== props.content)

function cancel(): void {
  editing.value = false
  draftContent.value = props.content
}

function save(): void {
  if (!dirty.value) return
  emit('save', draftContent.value)
}
</script>

<template>
  <el-drawer
    :model-value="props.open"
    :title="props.row?.skill_name || t('skills.view')"
    size="720px"
    @update:model-value="(v: boolean) => emit('update:open', v)"
  >
    <div v-if="props.loading" class="drawer-loading">{{ t('common.loading') }}</div>
    <template v-else>
      <div class="drawer-bar">
        <span class="drawer-path wb-mono selectable">{{ props.row?.skill_path }}</span>
        <span class="push" />
        <template v-if="props.row?.skill_editable">
          <template v-if="editing">
            <button class="wb-btn sm" :disabled="props.saving" @click="cancel">
              {{ t('common.cancel') }}
            </button>
            <button
              class="wb-btn primary sm"
              :class="{ 'is-loading': props.saving }"
              :disabled="props.saving || !dirty"
              @click="save"
            >
              {{ t('common.save') }}
            </button>
          </template>
          <button v-else class="wb-btn sm" @click="editing = true">
            <AppIcon name="edit" :size="13" />
            {{ t('common.edit') }}
          </button>
        </template>
        <span v-else class="tag readonly-tag">{{ t('skills.readonly') }}</span>
      </div>
      <CodeEditor v-model="draftContent" lang="markdown" :readonly="!editing" min-height="70vh" />
    </template>
  </el-drawer>
</template>

<style scoped>
.drawer-loading {
  padding: var(--wb-sp-6);
  text-align: center;
  color: var(--wb-faint);
  font-size: var(--wb-fs-sm);
}

.drawer-bar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin-bottom: var(--wb-sp-3);
}

.drawer-path {
  min-width: 0;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.push {
  flex: 1;
}

/* 只读标记：项目级的那份是别人的文件 */
.readonly-tag {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}
</style>
