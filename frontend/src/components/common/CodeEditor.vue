<script setup lang="ts">
// CodeMirror 代码编辑器：markdown / json 高亮 + one-dark 主题。
// 用于技能正文查看与 MCP / 插件配置编辑；跟随主题切换重刷背景。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { EditorView, basicSetup } from 'codemirror'
import { EditorState } from '@codemirror/state'
import { markdown } from '@codemirror/lang-markdown'
import { json } from '@codemirror/lang-json'
import { oneDark } from '@codemirror/theme-one-dark'

const props = withDefaults(
  defineProps<{
    modelValue: string
    lang?: 'markdown' | 'json'
    readonly?: boolean
    minHeight?: number
  }>(),
  { lang: 'markdown', readonly: false, minHeight: 160 },
)

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const host = ref<HTMLElement | null>(null)
let view: EditorView | null = null
// 外部赋值触发的更新不再回发，防循环
let applying = false

function extension() {
  const lang = props.lang === 'json' ? json() : markdown()
  const update = EditorView.updateListener.of((u) => {
    if (u.docChanged && !applying) {
      emit('update:modelValue', u.state.doc.toString())
    }
  })
  const theme = EditorView.theme({
    '&': { fontSize: '12px', backgroundColor: 'transparent' },
    '.cm-gutters': { backgroundColor: 'transparent', border: 'none', color: 'var(--wb-muted)' },
    '.cm-activeLine': { backgroundColor: 'var(--wb-tint)' },
    '.cm-activeLineGutter': { backgroundColor: 'var(--wb-tint)' },
    '&.cm-focused': { outline: 'none' },
  })
  return [lang, basicSetup, oneDark, theme, update, EditorView.editable.of(!props.readonly)]
}

onMounted(() => {
  if (!host.value) return
  view = new EditorView({
    state: EditorState.create({ doc: props.modelValue, extensions: extension() }),
    parent: host.value,
  })
})

// 外部值变化（载入技能正文 / 切换 MCP 作用域）要同步进编辑器
watch(
  () => props.modelValue,
  (next) => {
    if (!view) return
    const cur = view.state.doc.toString()
    if (next === cur) return
    applying = true
    view.dispatch({ changes: { from: 0, to: cur.length, insert: next } })
    applying = false
  },
)

watch(
  () => props.readonly,
  () => {
    // editable 状态在扩展里：重建视图最可靠（视图廉价，编辑器实例不共享）
    if (!host.value) return
    const doc = view?.state.doc.toString() ?? props.modelValue
    view?.destroy()
    view = new EditorView({
      state: EditorState.create({ doc, extensions: extension() }),
      parent: host.value,
    })
  },
)

onBeforeUnmount(() => {
  view?.destroy()
  view = null
})
</script>

<template>
  <div ref="host" class="cm-host" :style="{ minHeight: `${minHeight}px` }" />
</template>

<style scoped>
.cm-host {
  width: 100%;
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius);
  overflow: hidden;
  background: var(--wb-code-bg);
}
.cm-host :deep(.cm-editor) {
  min-height: inherit;
}
</style>
