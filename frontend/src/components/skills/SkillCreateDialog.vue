<script setup lang="ts">
// 新建技能：名字 / 说明 / 初始正文 / 写进哪些引擎。
// 名字在这里校验：它要变成目录名，含路径分隔符就写到别人的目录里去了。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { EngineKind } from '@/types/api'

const props = defineProps<{
  open: boolean
  creating: boolean
  /** 探测到的引擎：写到哪个引擎是勾选的，没装的不给勾 */
  installed: EngineKind[]
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  create: [draft: { name: string; description: string; body: string; targets: EngineKind[] }]
}>()

const { t } = useI18n()

const draft = ref({ name: '', description: '', body: '' })
const targets = ref<{ omp: boolean; pi: boolean }>({ omp: true, pi: true })

/** 名字要变成一个目录名：空或含路径分隔符都不行。 */
const nameInvalid = computed(
  () => !draft.value.name.trim() || /[\\/:*?"<>|]/.test(draft.value.name.trim()),
)

// 每次打开都是一份新的空草稿：留着上一次的内容会让人以为已经建过了。
watch(
  () => props.open,
  (open) => {
    if (open) {
      draft.value = { name: '', description: '', body: '' }
      targets.value = { omp: props.installed.includes('omp'), pi: props.installed.includes('pi') }
    }
  },
)

function submit(): void {
  if (nameInvalid.value || props.creating) return
  const picked = (['omp', 'pi'] as EngineKind[]).filter((k) => targets.value[k])
  if (!picked.length) return
  emit('create', {
    name: draft.value.name.trim(),
    description: draft.value.description.trim(),
    body: draft.value.body,
    targets: picked,
  })
}
</script>

<template>
  <el-dialog
    :model-value="props.open"
    :title="t('skills.createTitle')"
    width="560px"
    @update:model-value="(v: boolean) => emit('update:open', v)"
  >
    <div class="form">
      <label class="field">
        <span class="field-label">{{ t('skills.nameLabel') }}</span>
        <input v-model="draft.name" class="wb-input" :placeholder="t('skills.nameHint')" />
        <span v-if="nameInvalid && draft.name" class="field-hint bad">{{ t('skills.nameHint') }}</span>
      </label>
      <label class="field">
        <span class="field-label">{{ t('skills.descLabel') }}</span>
        <input v-model="draft.description" class="wb-input" :placeholder="t('skills.descHint')" />
      </label>
      <label class="field">
        <span class="field-label">{{ t('skills.bodyLabel') }}</span>
        <textarea v-model="draft.body" class="wb-input" rows="10" :placeholder="t('skills.bodyHint')" />
      </label>
      <div class="field">
        <span class="field-label">{{ t('skills.writeTargets') }}</span>
        <div class="target-row">
          <label class="target-check"><input v-model="targets.omp" type="checkbox" :disabled="!props.installed.includes('omp')" /> omp</label>
          <label class="target-check"><input v-model="targets.pi" type="checkbox" :disabled="!props.installed.includes('pi')" /> pi</label>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="dialog-foot">
        <button class="wb-btn" :disabled="props.creating" @click="emit('update:open', false)">
          {{ t('common.cancel') }}
        </button>
        <button
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
.form {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}

.field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.field-label {
  font-size: var(--wb-fs-3xs);
  font-weight: 650;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--wb-faint);
}

.field-hint {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.field-hint.bad {
  color: var(--wb-bad);
}

.target-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
}

.target-check {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink);
  cursor: pointer;
}

.target-check:has(input:disabled) {
  opacity: 0.45;
  cursor: not-allowed;
}

.dialog-foot {
  display: flex;
  justify-content: flex-end;
  gap: var(--wb-sp-2);
}
</style>
