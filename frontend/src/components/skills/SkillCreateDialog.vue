<script setup lang="ts">
// 新建技能：名字 / 说明 / 初始正文。
// 名字在这里校验：它要变成目录名，含路径分隔符就写到别人的目录里去了。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  open: boolean
  creating: boolean
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  create: [draft: { name: string; description: string; body: string }]
}>()

const { t } = useI18n()

const draft = ref({ name: '', description: '', body: '' })

/** 名字要变成一个目录名：空或含路径分隔符都不行。 */
const nameInvalid = computed(() => !draft.value.name.trim() || /[\\/:*?"<>|]/.test(draft.value.name.trim()))

// 每次打开都是一份新的空草稿：留着上一次的内容会让人以为已经建过了。
watch(
  () => props.open,
  (open) => {
    if (open) draft.value = { name: '', description: '', body: '' }
  },
)

function submit(): void {
  if (nameInvalid.value || props.creating) return
  emit('create', {
    name: draft.value.name.trim(),
    description: draft.value.description.trim(),
    body: draft.value.body,
  })
}
</script>

<template>
  <el-dialog
    :model-value="props.open"
    :title="t('skills.createTitle')"
    width="560px"
    :close-on-click-modal="false"
    @update:model-value="(v: boolean) => emit('update:open', v)"
  >
    <el-form label-position="top" size="small">
      <el-form-item :label="t('skills.nameLabel')" :error="nameInvalid && draft.name ? t('skills.nameHint') : ''">
        <el-input v-model="draft.name" :placeholder="t('skills.nameHint')" @keyup.enter="submit" />
      </el-form-item>
      <el-form-item :label="t('skills.descLabel')">
        <el-input v-model="draft.description" :placeholder="t('skills.descHint')" />
      </el-form-item>
      <el-form-item :label="t('skills.bodyLabel')">
        <el-input v-model="draft.body" type="textarea" :rows="10" :placeholder="t('skills.bodyHint')" />
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-foot">
        <button type="button" class="wb-btn" :disabled="props.creating" @click="emit('update:open', false)">
          {{ t('common.cancel') }}
        </button>
        <button type="button"
          class="wb-btn primary"
          :class="{ 'is-loading': props.creating }"
          :disabled="props.creating || nameInvalid"
          @click="submit"
        >
          {{ t('common.create') }}
        </button>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.dialog-foot {
  display: flex;
  justify-content: flex-end;
  gap: var(--wb-sp-2);
}
</style>
