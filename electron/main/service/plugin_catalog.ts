//! 内置插件清单：随壳分发的 12 个 pi 扩展包，首启由内置 npm 自动装进沙箱。

/** 一个内置插件的静态描述；版本号即 pin 的 npm 精确版本。 */
export interface BuiltinPlugin {
  /** npm 包名 */
  plugin_name: string
  /** 精确版本；空串表示跟随 latest */
  plugin_version: string
  /** 界面上一句话说明 */
  plugin_desc: string
  /** 是否默认启用 */
  plugin_default_on: boolean
}

/** 内置插件清单：顺序即首启安装顺序，联网访问排最前（其余插件常依赖它）。 */
export const BUILTIN_PLUGINS: BuiltinPlugin[] = [
  {
    plugin_name: 'pi-web-access',
    plugin_version: '0.42.0',
    plugin_desc: '联网搜索、抓取 URL、克隆 GitHub、PDF 与视频理解',
    plugin_default_on: true,
  },
  {
    plugin_name: '@juicesharp/rpiv-todo',
    plugin_version: '2.13.9',
    plugin_desc: '实时待办清单，跨 /reload 与上下文压缩存活',
    plugin_default_on: true,
  },
  {
    plugin_name: '@juicesharp/rpiv-ask-user-question',
    plugin_version: '2.13.9',
    plugin_desc: '结构化问卷，让模型在拿不准时先问你',
    plugin_default_on: true,
  },
  {
    plugin_name: 'pi-hermes-memory',
    plugin_version: '0.9.10',
    plugin_desc: '持久记忆、会话检索、密钥扫描与自动知识整合',
    plugin_default_on: true,
  },
  {
    plugin_name: 'context-mode',
    plugin_version: '1.0.169',
    plugin_desc: '省上下文：沙箱执行 + FTS5 知识库 + 意图搜索',
    plugin_default_on: true,
  },
  {
    plugin_name: '@ff-labs/pi-fff',
    plugin_version: '0.11.0',
    plugin_desc: '模糊文件名与内容搜索',
    plugin_default_on: true,
  },
  {
    plugin_name: '@narumitw/pi-lsp',
    plugin_version: '0.49.10',
    plugin_desc: '语言无关的 LSP 工具，共享 runner',
    plugin_default_on: true,
  },
  {
    plugin_name: '@plannotator/pi-extension',
    plugin_version: '0.28.23',
    plugin_desc: '交互式计划评审、标注与 PR 审查',
    plugin_default_on: true,
  },
  {
    plugin_name: 'pi-codex-goal',
    plugin_version: '0.6.1',
    plugin_desc: 'Codex 式目标追踪与续跑',
    plugin_default_on: true,
  },
  {
    plugin_name: 'pi-loadout',
    plugin_version: '0.0.37',
    plugin_desc: '交互式工具与技能装载配置',
    plugin_default_on: false,
  },
  {
    plugin_name: 'pi-inspect',
    plugin_version: '0.5.0',
    plugin_desc: '自省面板：工具、斜杠命令、技能与注入的系统提示',
    plugin_default_on: false,
  },
  {
    plugin_name: 'pi-context-view',
    plugin_version: '0.7.2',
    plugin_desc: '可视化上下文用量，检查隐藏的基础提示与工具定义',
    plugin_default_on: false,
  },
]

/** 按包名取内置描述；非内置给 undefined。 */
export function builtinOf(plugin_name: string): BuiltinPlugin | undefined {
  return BUILTIN_PLUGINS.find((p) => p.plugin_name === plugin_name)
}

/** npm 来源写法：pi 的 packages 数组认 `npm:<name>@<version>`。 */
export function npmSourceOf(item: BuiltinPlugin): string {
  return `npm:${item.plugin_name}${item.plugin_version ? `@${item.plugin_version}` : ''}`
}
