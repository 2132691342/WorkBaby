/**
 * markdown-it 插件的本地类型声明。
 *
 * <p>这两个插件体积小、维护活跃，但官方与 DefinitelyTyped 都没提供类型，
 * 因此在项目内补最小可用声明（只声明我们实际用到的签名，不做全量建模）。
 */
declare module 'markdown-it-task-lists' {
  import type MarkdownIt from 'markdown-it'

  interface TaskListsOptions {
    /** 是否启用。默认 true。 */
    enabled?: boolean
    /** 是否给复选框加 label。默认 false。 */
    label?: boolean
    /** label 是否放在复选框之后。默认 false。 */
    labelAfter?: boolean
  }

  const taskLists: MarkdownIt.PluginWithOptions<TaskListsOptions>
  export default taskLists
}

declare module 'markdown-it-footnote' {
  import type MarkdownIt from 'markdown-it'

  const footnote: MarkdownIt.PluginSimple
  export default footnote
}
