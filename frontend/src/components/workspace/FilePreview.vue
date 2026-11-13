<script setup lang="ts">
// 二进制文件的可视化预览：PDF / Office / 图片按类型分流渲染。
// 只吃后端给的 base64，不解包、不落临时文件；表格用模板迭代渲染而非 sheet_to_html——
// 后者吐 HTML 字符串要靠 v-html 注入，自己文件里的内容不开这个口子。
import { computed, defineAsyncComponent, onMounted, ref, watch } from 'vue'
import type { FileRead } from '@/types/api-workspace'

// 三个预览库都不轻（pdf/xlsx 各几百 KB），静态引会把首屏包拽大。
// 按需异步拆包：只有真预览到对应类型才去拉，首屏不替用不到的东西买单。
const VuePdfEmbed = defineAsyncComponent(() => import('vue-pdf-embed'))

const props = defineProps<{ file: FileRead }>()

// 由扩展名推 MIME，只用于图片 data URI 与 docx 的 Blob 类型。
const mime = computed(() => {
  const ext = (props.file.file_path.split('.').pop() || '').toLowerCase()
  const map: Record<string, string> = {
    png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', gif: 'image/gif',
    webp: 'image/webp', bmp: 'image/bmp', svg: 'image/svg+xml',
  }
  return map[ext] || 'application/octet-stream'
})

const dataUri = computed(() =>
  props.file.file_base64 ? `data:${mime.value};base64,${props.file.file_base64}` : '',
)

/** base64 解成字节，给 docx / xlsx 这类要 ArrayBuffer 的解析器用。 */
function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

const docHost = ref<HTMLElement | null>(null)
const fail = ref('')

// 表格预览：每张表的表名 + 二维单元格数组。上限挡住超大表，避免几万行把 DOM 撑爆。
const sheets = ref<{ name: string; rows: string[][] }[]>([])
const sheetCap = 200

async function buildSheet() {
  try {
    const XLSX = await import('xlsx')
    const wb = XLSX.read(b64ToBytes(props.file.file_base64 || ''), { type: 'array' })
    sheets.value = wb.SheetNames.slice(0, 5).map((name) => {
      const aoa = XLSX.utils.sheet_to_json<string[]>(wb.Sheets[name], {
        header: 1, raw: false, defval: '',
      })
      return { name, rows: aoa.slice(0, sheetCap) as string[][] }
    })
  } catch (err) {
    fail.value = '表格解析失败：' + (err instanceof Error ? err.message : String(err))
  }
}

async function buildDoc() {
  if (!docHost.value) return
  try {
    const { renderAsync } = await import('docx-preview')
    const blob = new Blob([b64ToBytes(props.file.file_base64 || '')], { type: mime.value })
    await renderAsync(blob, docHost.value)
  } catch (err) {
    fail.value = '文档渲染失败：' + (err instanceof Error ? err.message : String(err))
  }
}

// 类型或内容变了就重渲染；docx-preview 是命令式渲染，得手动重跑。
function render() {
  fail.value = ''
  sheets.value = []
  if (props.file.file_kind === 'sheet') void buildSheet()
  else if (props.file.file_kind === 'doc') void buildDoc()
}

onMounted(render)
watch(() => [props.file.file_kind, props.file.file_base64], render)
</script>

<template>
  <!-- 图片：原生 <img> 足够，交给 CSS 自适应宽度 -->
  <img v-if="file.file_kind === 'image'" :src="dataUri" class="img-preview" alt="图片预览" />

  <!-- PDF：vue-pdf-embed 自己分页渲染 -->
  <div v-else-if="file.file_kind === 'pdf'" class="pdf-preview">
    <VuePdfEmbed :source="dataUri" />
  </div>

  <!-- Word：docx-preview 往挂载点里塞样式化的 DOM -->
  <div v-else-if="file.file_kind === 'doc'" class="doc-preview">
    <p v-if="fail" class="preview-fail">{{ fail }}</p>
    <div v-show="!fail" ref="docHost" class="doc-host" />
  </div>

  <!-- 表格：逐格渲染，多表用表名分段 -->
  <div v-else-if="file.file_kind === 'sheet'">
    <p v-if="fail" class="preview-fail">{{ fail }}</p>
    <template v-else>
      <section v-for="sheet in sheets" :key="sheet.name">
        <h4 class="sheet-name">{{ sheet.name }}</h4>
        <div class="sheet-scroll">
          <table class="sheet-table">
            <tr v-for="(row, ri) in sheet.rows" :key="ri">
              <td v-for="(cell, ci) in row" :key="ci">{{ cell }}</td>
            </tr>
          </table>
        </div>
      </section>
    </template>
  </div>

  <!-- 认不出的二进制：只给占位 -->
  <p v-else class="preview-fail">这类文件暂不支持预览</p>
</template>

<style scoped>
.img-preview {
  max-width: 100%;
  height: auto;
  border-radius: var(--wb-r);
  background: var(--wb-sunken);
}
.pdf-preview :deep(.vue-pdf-embed) {
  max-width: 100%;
}
.doc-preview :deep(p) {
  margin: 0 0 var(--wb-sp-2);
}
.doc-host {
  overflow-x: auto;
}
.sheet-name {
  margin: var(--wb-sp-3) 0 var(--wb-sp-2);
  font-size: var(--wb-fs-sm);
  color: var(--wb-faint);
}
.sheet-scroll {
  overflow: auto;
  max-height: 60vh;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r);
}
.sheet-table {
  border-collapse: collapse;
  font-size: var(--wb-fs-sm);
}
.sheet-table td {
  padding: var(--wb-sp-1) var(--wb-sp-2);
  border: 1px solid var(--wb-line);
  white-space: nowrap;
}
.preview-fail {
  margin: 0;
  padding: var(--wb-sp-3);
  color: var(--wb-warn);
}
</style>
