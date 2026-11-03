<script setup lang="ts">
// 计划的驳回意见与标注编辑区：两块独立表单，提交动作由父组件执行。
// 输入文本留在这里而不是父组件——它们是「这次编辑」的草稿，与计划本体是两套状态。
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  /** 正在写的动作名（空串 = 空闲），用于按钮 loading 与禁用 */
  busy: string
  /** 标注针对哪一步：0 = 整个计划 */
  annotateStep: number
}>()
const emit = defineEmits<{ reject: [note: string]; annotate: [text: string, step: number] }>()

const rejecting = defineModel<boolean>('rejecting', { required: true })
const annotating = defineModel<boolean>('annotating', { required: true })

const { t } = useI18n()
const rejectNote = ref('')
const annotateText = ref('')

/** 编辑区关掉就清草稿：否则下次打开会看到上一次没提交的文字。 */
watch(rejecting, (on) => {
  if (!on) rejectNote.value = ''
})
watch(annotating, (on) => {
  if (!on) annotateText.value = ''
})

function submitReject() {
  emit('reject', rejectNote.value)
}

function submitAnnotate() {
  if (!annotateText.value.trim()) return
  emit('annotate', annotateText.value, props.annotateStep)
}
</script>

<template>
  <div v-if="rejecting" class="note-edit">
    <el-input
      v-model="rejectNote"
      size="small"
      :placeholder="t('featurePanel.planRejectPlaceholder')"
      @keydown.enter="submitReject"
    />
    <div class="create-acts">
      <el-button size="small" type="primary" :loading="busy === 'reject'" :disabled="!!busy" @click="submitReject">
        {{ t('common.confirm') }}
      </el-button>
      <el-button size="small" :disabled="!!busy" @click="rejecting = false">
        {{ t('common.cancel') }}
      </el-button>
    </div>
  </div>

  <div v-if="annotating" class="note-edit">
    <span class="wb-label">
      {{ annotateStep > 0 ? t('featurePanel.planAnnotateStep', { n: annotateStep }) : t('featurePanel.planAnnotatePlan') }}
    </span>
    <el-input
      v-model="annotateText"
      size="small"
      :placeholder="t('featurePanel.planAnnotatePlaceholder')"
      @keydown.enter="submitAnnotate"
    />
    <div class="create-acts">
      <el-button
        size="small"
        type="primary"
        :loading="busy === 'annotate'"
        :disabled="!annotateText.trim() || !!busy"
        @click="submitAnnotate"
      >
        {{ t('common.confirm') }}
      </el-button>
      <el-button size="small" :disabled="!!busy" @click="annotating = false">
        {{ t('common.cancel') }}
      </el-button>
    </div>
  </div>
</template>
