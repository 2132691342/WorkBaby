<script setup lang="ts">
// 文档预览抽屉：从文件树点开一个 PDF / Word / Excel 就地预览，不离开当前页。
// 字节走后端 /files/bytes（base64），组件只管解码与渲染。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import DocPreview from './DocPreview.vue'
import ResizableDrawer from '../common/ResizableDrawer.vue'
import { errorText } from '../../utils/errors'

const props = defineProps<{
  /** 相对工作区的文件路径；空串 = 关闭 */
  modelValue: string
}>()

const emit = defineEmits<{ 'update:modelValue': [string] }>()

const { t } = useI18n()
const toast = useToastStore()

const bytes = ref<ArrayBuffer | null>(null)
const fileName = ref('')
const loading = ref(false)
const error = ref('')

const open = computed({
  get: () => !!props.modelValue,
  set: (v: boolean) => {
    if (!v) emit('update:modelValue', '')
  },
})

/** base64 → ArrayBuffer。 */
function decodeBase64(b64: string): ArrayBuffer {
  const bin = atob(b64)
  const buf = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) buf[i] = bin.charCodeAt(i)
  return buf.buffer
}

async function load(path: string) {
  if (!path) {
    bytes.value = null
    fileName.value = ''
    error.value = ''
    return
  }
  loading.value = true
  error.value = ''
  bytes.value = null
  try {
    const r = await api.files.bytes(path)
    bytes.value = decodeBase64(r.resp_content_base64)
    fileName.value = r.resp_file_name
  } catch (e) {
    error.value = errorText(e) || t('common.loadFailed', { msg: '' })
    toast.bad(error.value)
  } finally {
    loading.value = false
  }
}

watch(() => props.modelValue, load, { immediate: true })
</script>

<template>
  <ResizableDrawer
    v-model="open"
    storage-key="doc-preview"
    :title="fileName || modelValue"
    class="doc-drawer"
  >
    <div v-if="loading" class="doc-loading">
      <el-skeleton animated :rows="8" />
    </div>
    <p v-else-if="error" class="doc-err">{{ error }}</p>
    <DocPreview v-else-if="bytes" :data="bytes" :file-name="fileName" :height="560" />
  </ResizableDrawer>
</template>

<style scoped>
.doc-loading {
  padding: var(--wb-sp-4);
}
.doc-err {
  padding: var(--wb-sp-4);
  font-size: var(--wb-fs-sm);
  color: var(--wb-danger);
}
.doc-drawer :deep(.el-drawer__body) {
  padding: var(--wb-sp-3);
}
</style>
