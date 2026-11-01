<script setup lang="ts">
// 文档预览：按文件类型分派到 PDF（vue-pdf-embed）、Word（docx-preview）、
// Excel（XLSX 读成表格）三条渲染路径。统一由父级传入 ArrayBuffer，
// 组件不碰文件系统。
import { computed, defineAsyncComponent, onBeforeUnmount, ref, watch } from 'vue'
import { errorText } from '../../utils/errors'

// PDF 渲染器体积很大（pdfjs-dist 约 3MB）：只在真正要预览时才拉，
// 不能让它进首屏包。Excel 同理。
const VuePdfEmbed = defineAsyncComponent(() => import('vue-pdf-embed'))

const props = withDefaults(
  defineProps<{
    /** 文件内容 */
    data?: ArrayBuffer
    /** 文件名（用来判类型与取 sheet 名） */
    fileName?: string
    height?: number
  }>(),
  { height: 520 },
)

type Kind = 'pdf' | 'word' | 'excel' | 'text' | 'unknown'

const kind = computed<Kind>(() => {
  const n = (props.fileName || '').toLowerCase()
  if (n.endsWith('.pdf')) return 'pdf'
  if (n.endsWith('.docx') || n.endsWith('.doc')) return 'word'
  if (n.endsWith('.xlsx') || n.endsWith('.xls') || n.endsWith('.csv')) return 'excel'
  if (n.endsWith('.md') || n.endsWith('.txt') || n.endsWith('.json')) return 'text'
  return 'unknown'
})

const wordHost = ref<HTMLDivElement | null>(null)
const excelRows = ref<string[][]>([])
const excelSheets = ref<string[]>([])
const currentSheet = ref('')
const textBody = ref('')
const failed = ref('')

/** PDF 渲染宽度（px）：窄栏也不能窄到看不清。 */
const pageWidth = computed(() => Math.min(880, Math.max(360, props.height)))

/** 文件头四字节判 PDF：扩展名会骗人，magic 不会。 */
function isPdf(data: ArrayBuffer): boolean {
  const b = new Uint8Array(data.slice(0, 4))
  return b.length === 4 && b[0] === 0x25 && b[1] === 0x50 && b[2] === 0x44 && b[3] === 0x46
}

async function renderWord() {
  if (!wordHost.value || !props.data) return
  failed.value = ''
  try {
    const { renderAsync } = await import('docx-preview')
    wordHost.value.innerHTML = ''
    await renderAsync(new Uint8Array(props.data), wordHost.value, undefined, {
      className: 'wb-docx',
      inWrapper: true,
    })
  } catch (e) {
    failed.value = String(errorText(e))
  }
}

function renderExcel() {
  if (!props.data) return
  failed.value = ''
  void import('xlsx').then((XLSX) => {
    try {
      const wb = XLSX.read(new Uint8Array(props.data!), { type: 'array' })
      excelSheets.value = wb.SheetNames
      currentSheet.value = wb.SheetNames[0] ?? ''
      renderSheet(XLSX)
    } catch (e) {
      failed.value = String(errorText(e))
    }
  })
}

function renderSheet(XLSX: typeof import('xlsx')) {
  if (!props.data || !currentSheet.value) return
  try {
    const wb = XLSX.read(new Uint8Array(props.data), { type: 'array' })
    const sheet = wb.Sheets[currentSheet.value]
    if (!sheet) return
    excelRows.value = XLSX.utils.sheet_to_json<string[]>(sheet, { header: 1, blankrows: false })
  } catch (e) {
    failed.value = String(errorText(e))
  }
}

function renderText() {
  if (!props.data) return
  try {
    textBody.value = new TextDecoder('utf-8').decode(new Uint8Array(props.data))
  } catch (e) {
    failed.value = String(errorText(e))
  }
}

function renderAll() {
  failed.value = ''
  if (!props.data) {
    excelRows.value = []
    textBody.value = ''
    return
  }
  if (kind.value === 'pdf' || isPdf(props.data)) return
  if (kind.value === 'word') {
    void renderWord()
    return
  }
  if (kind.value === 'excel') {
    renderExcel()
    return
  }
  // 类型不明：先当文本试，读不出就报「不支持的格式」
  renderText()
}

watch(() => props.data, renderAll, { immediate: true })
watch(currentSheet, () => {
  if (props.data && currentSheet.value) {
    void import('xlsx').then((XLSX) => renderSheet(XLSX))
  }
})

onBeforeUnmount(() => {
  if (wordHost.value) wordHost.value.innerHTML = ''
})
</script>

<template>
  <div class="doc" :style="{ height: `${height}px` }">
    <div v-if="failed" class="doc-mask">
      <span>{{ failed }}</span>
    </div>

    <div v-else-if="kind === 'pdf' || (data && isPdf(data))" class="doc-pdf">
      <VuePdfEmbed
        v-if="data"
        :source="data"
        :width="pageWidth"
        text-layer
        annotation-layer
      />
    </div>

    <div v-else-if="kind === 'word'" ref="wordHost" class="doc-word" />

    <div v-else-if="kind === 'excel'" class="doc-excel">
      <div v-if="excelSheets.length > 1" class="sheet-bar">
        <el-radio-group v-model="currentSheet" size="small">
          <el-radio-button v-for="s in excelSheets" :key="s" :value="s">
            {{ s }}
          </el-radio-button>
        </el-radio-group>
      </div>
      <div class="sheet-scroll">
        <table v-if="excelRows.length" class="sheet-table">
          <tbody>
            <tr v-for="(row, ri) in excelRows" :key="ri">
              <td v-for="(cell, ci) in row" :key="ci" :class="{ 'is-head': ri === 0 }">
                {{ cell ?? '' }}
              </td>
            </tr>
          </tbody>
        </table>
        <p v-else class="doc-empty">{{ $t('common.docEmpty') }}</p>
      </div>
    </div>

    <pre v-else-if="kind === 'text'" class="doc-text">{{ textBody }}</pre>

    <div v-else class="doc-mask">
      <span>{{ $t('common.docUnsupported') }}</span>
    </div>
  </div>
</template>

<style scoped>
.doc {
  position: relative;
  border-radius: var(--wb-radius-lg);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  overflow: hidden;
}
.doc-pdf,
.doc-word,
.doc-excel,
.doc-text {
  height: 100%;
  overflow: auto;
}
.doc-pdf {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-4);
  background: var(--wb-elev-hover);
}
.doc-word {
  padding: var(--wb-sp-4);
}
.sheet-bar {
  position: sticky;
  top: 0;
  z-index: 1;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  background: var(--wb-elev-workspace);
  border-bottom: 1px solid var(--wb-border);
}
.sheet-scroll {
  padding: var(--wb-sp-3);
}
.sheet-table {
  border-collapse: collapse;
  width: 100%;
  font-size: var(--wb-fs-2xs);
}
.sheet-table td {
  border: 1px solid var(--wb-border);
  padding: 3px var(--wb-sp-2);
  white-space: nowrap;
  color: var(--wb-ink);
}
.sheet-table td.is-head {
  background: var(--wb-elev-hover);
  font-weight: 700;
}
.doc-text {
  margin: 0;
  padding: var(--wb-sp-4);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-ink);
  white-space: pre-wrap;
  word-break: break-word;
}
.doc-mask {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--wb-muted);
  font-size: var(--wb-fs-sm);
}
.doc-empty {
  padding: var(--wb-sp-6);
  text-align: center;
  color: var(--wb-muted);
  font-size: var(--wb-fs-sm);
}
</style>
