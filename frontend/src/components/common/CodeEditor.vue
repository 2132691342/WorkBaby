<script setup lang="ts">
// 一个受控的 CodeMirror 6 编辑器：只读 / 可编辑两种模式，语法高亮按语言给。
// 自己包而不是用现成包装：只要「文本 + 语言 + 只读开关」，
// 现成包装会把主题、工具栏、状态栏一起带进来，而这里那些都得听令牌的。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { EditorState, Compartment, type Extension } from '@codemirror/state'
import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { bracketMatching, foldGutter, syntaxHighlighting, defaultHighlightStyle } from '@codemirror/language'
import { markdown } from '@codemirror/lang-markdown'
import { json } from '@codemirror/lang-json'
import { yaml } from '@codemirror/lang-yaml'

const props = withDefaults(
  defineProps<{
    /** 文本内容。外部变更会写进编辑器（内容相同则不动，避免光标跳走） */
    modelValue: string
    /** 语言：markdown / json / yaml / text */
    lang?: 'markdown' | 'json' | 'yaml' | 'text'
    /** 只读。只读时仍可选中复制 */
    readonly?: boolean
    /** 最小高度（CSS 值）。编辑器至少占这么高，内容更长时自己滚 */
    minHeight?: string
  }>(),
  { lang: 'markdown', readonly: false, minHeight: '220px' },
)

const emit = defineEmits<{ 'update:modelValue': [string] }>()

const host = ref<HTMLElement | null>(null)
let view: EditorView | null = null
// 语言与只读都需要「运行时换掉」，所以各占一个 compartment。
const langSlot = new Compartment()
const readonlySlot = new Compartment()

function langExtension(): Extension {
  if (props.lang === 'json') return json()
  if (props.lang === 'yaml') return yaml()
  if (props.lang === 'markdown') return markdown()
  return []
}

/** 主题接到令牌上：编辑器不引入自己的色板，深色模式跟着数据集走。 */
const theme = EditorView.theme({
  '&': {
    fontSize: 'var(--wb-fs-sm)',
    color: 'var(--wb-ink)',
    backgroundColor: 'var(--wb-sunken)',
    height: '100%',
  },
  '.cm-scroller': {
    fontFamily: 'var(--wb-font-mono)',
    lineHeight: '1.65',
  },
  '.cm-content': { caretColor: 'var(--wb-brand)' },
  '.cm-gutters': {
    backgroundColor: 'var(--wb-sunken)',
    color: 'var(--wb-faint)',
    border: 'none',
  },
  '.cm-activeLine': { backgroundColor: 'var(--wb-hover)' },
  '.cm-activeLineGutter': { backgroundColor: 'var(--wb-hover)' },
  '&.cm-focused': { outline: 'none' },
  '.cm-selectionBackground, ::selection': { backgroundColor: 'var(--wb-selected) !important' },
})

function buildState(doc: string): EditorState {
  return EditorState.create({
    doc,
    extensions: [
      lineNumbers(),
      highlightActiveLine(),
      highlightActiveLineGutter(),
      history(),
      bracketMatching(),
      foldGutter(),
      syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
      keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
      EditorView.lineWrapping,
      theme,
      langSlot.of(langExtension()),
      readonlySlot.of(EditorState.readOnly.of(props.readonly)),
      EditorView.updateListener.of((update) => {
        if (!update.docChanged) return
        emit('update:modelValue', update.state.doc.toString())
      }),
    ],
  })
}

onMounted(() => {
  if (!host.value) return
  view = new EditorView({ state: buildState(props.modelValue), parent: host.value })
})

onBeforeUnmount(() => {
  view?.destroy()
  view = null
})

// 外部改了内容（切文件 / 保存后后端回填）：只有真的不同才重建，
// 否则每敲一个字都会因为父组件回传而同值重建一次状态，光标会跳到开头。
watch(
  () => props.modelValue,
  (next) => {
    if (!view) return
    const current = view.state.doc.toString()
    if (current === next) return
    view.dispatch({ changes: { from: 0, to: current.length, insert: next } })
  },
)

watch(
  () => props.lang,
  () => {
    view?.dispatch({ effects: langSlot.reconfigure(langExtension()) })
  },
)

watch(
  () => props.readonly,
  (next) => {
    view?.dispatch({ effects: readonlySlot.reconfigure(EditorState.readOnly.of(next)) })
  },
)

/** 把光标放到开头并滚到顶（切文件时用）。 */
function scrollTop() {
  view?.dispatch({ selection: { anchor: 0 }, scrollIntoView: true })
}

defineExpose({ scrollTop })
</script>

<template>
  <div ref="host" class="code-editor" :style="{ minHeight: props.minHeight }" />
</template>

<style scoped>
.code-editor {
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r);
  overflow: hidden;
  background: var(--wb-sunken);
}

.code-editor :deep(.cm-editor) {
  height: 100%;
}

.code-editor :deep(.cm-scroller) {
  overflow: auto;
}
</style>
