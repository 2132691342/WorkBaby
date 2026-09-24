# docs/COMPONENT-GUIDELINES.md · 组件开发规范

> 视觉令牌与视觉纪律见 [`DESIGN.md`](../DESIGN.md)；页面结构见 [`PAGE-STRUCTURE.md`](PAGE-STRUCTURE.md)。

## 1. 前端目录

```
frontend/src/src/
├── api/          http（统一 unwrap）/ client（路径白名单）/ stream（SSE）/ bootstrap（端口注入）/ contract
├── chat/         纯领域模型：blocks（块序列）/ streamingBlocks / toolTrace / toolVisuals / merge（消息对账）/ tokens
├── components/   按业务域分包（chat / settings / dashboard / pet / common / knowledge / mcp …）
├── composables/  useTheme / useToast / useDialog / useThrottledContent / useShortcuts …
├── stores/       域 store（chat / sideChat / settings / pet / tasks / skills / knowledge …）
├── types/api.ts  后端契约类型（与 RESP 字段一一对应，唯一类型来源）
├── markdown/     markdown-it 双实例 + DOMPurify + hljs / mermaid / katex
├── i18n/         中英词条（dict-zh / dict-en），key 集合与顺序对齐
└── style.css / themes.css / wb-ui.css   设计令牌与组件基元
```

## 2. 组件分层

| 层 | 位置 | 职责 |
|---|---|---|
| 页面 | `components/<域>/XxxView.vue` | 路由落点：编排数据加载与布局，不含可复用 UI |
| 业务组件 | `components/<域>/*.vue` | 单一业务语义（消息卡、审批卡、工具时间线） |
| 通用组件 | `components/common/` | 无业务语义（EmptyState / Skeleton / PageState / Field / FormDialog / ImageCropper） |
| 纯模型 | `chat/models/*.ts` | 无 Vue 依赖的纯函数（块归一化、分组、工具迹线、diff 解析） |

**原则**：能放 `chat/models/` 的逻辑不放进组件——纯函数可单测、可复用、与渲染解耦。

## 3. 组件编写规范

| 维度 | 规范 |
|---|---|
| 组织 | `<script setup lang="ts">` + `<template>` + `<style scoped>`；props 用 `defineProps<{...}>()` 类型式声明 |
| 命名 | 组件文件 PascalCase（`MessageBlocksRenderer.vue`）；props / emits 用 snake_case（与后端契约一致，如 `session_id`） |
| 事件 | `defineEmits<{ retry: [toolCallId: string] }>()` 元组式声明；不用字符串数组 |
| 类型 | 禁止 `any`；后端契约类型从 `types/api.ts` 导入，不在组件内重复定义 |
| 文案 | 所有面向用户文案走 `t()`，禁止硬编码中文/英文字符串 |
| 空态 | 统一 `EmptyState`；加载态 `Skeleton`；列表页三态（加载/空/错误）经 `PageState` 包装 |

## 4. 样式规范

| 维度 | 规范 |
|---|---|
| 色值 | 只用 `themes.css` 的 `--wb-*` 令牌；组件内禁止写死颜色（暗色会漏白） |
| Tailwind 类 | 用语义 token 类（`text-wb-ink` / `bg-wb-surface-2` / `border-wb-border`），不用 `gray-*` 等默认调色板 |
| 尺寸 | 控件高度用 `--wb-ctl-h*` 四档；字号用 `text-3xs` / `text-2xs` / `text-xs2` / `text-ctl`；禁止裸像素 |
| 组件语法 | 唯一实现在 `wb-ui.css`（按钮 / 卡片 / 标签 / 状态灯 / 表格 / 时间线 / 输入区）；组件不得复制副本 |
| `<style scoped>` | 只写该组件独有的结构性样式；可复用的外观语法必须回填 `wb-ui.css` |
| 动态 class | 用对象/数组语法，禁止字符串拼接 |
| 渲染安全 | `v-html` 仅用于已转义后再着色的内容（如工具结果 JSON 高亮：先 `escapeHTML` 再插 `class` 为常量的 `<span>`） |

## 5. 依赖规范

| 依赖 | 使用边界 |
|---|---|
| Element Plus | 只作补充（表格 / 弹层 / 表单控件 / Tab）；禁止 `el-empty` / `el-skeleton`（会拖入整套 EP 到懒加载 chunk 且样式与令牌不同源） |
| 图标 | `@/components/common/icons`（Lucide 风格，24 网格，统一笔画 1.7）；不新增图标库 |
| 图表 | `components/dashboard/` 内按需引入；图表色只用 `--wb-ch-1..4` 蓝阶（整屏单色相） |
| 剪贴板 | `useClipboard({ legacy: true })`（JCEF/WebView 非安全上下文下需 `execCommand` 兜底） |
| 状态 | Pinia Setup Store；跨会话共享状态进 store，组件局部状态用 `ref` |
| 懒加载 | 路由级 `() => import(...)`；`MarkdownRenderer` / mermaid / katex 按需动态引入 |

**新增依赖前**：确认现有能力与标准库是否可替代；说明理由与体积影响。

## 6. 状态与数据流

| 主题 | 规范 |
|---|---|
| 流式状态 | 单份（`streaming` / `streamingBlocks` / `activeHandle`）+ `streamingSessionID` 标记归属；非当前会话事件直接丢弃，切回后靠权威快照补齐 |
| 事件入口 | SSE 事件经 `stream.ts` 归一为 `ChatStreamEvent`，回调携带 `StreamEventMeta`（session_id / run_id） |
| 批量落地 | `StreamEventBatcher` 按帧批量 apply；`chat:done` 等终态先 flush 再收尾 |
| 消息对账 | `chat:done` 后拉权威快照，`merge.ts` 三态判定：`dedupe`（权威已含 → 不 push）/ `fill`（空占位 → 补正文 + 本地块还原）/ `missing`（本地兜底插入） |
| 并发守卫 | 快照拉取用 `loadSeq` 序号守卫：过期响应直接丢弃 |
| 契约 | 字段一律 snake_case，与后端 RESP 完全一致 |

## 7. 无障碍

| 项 | 要求 |
|---|---|
| 可点击元素 | 一律用 `<button type="button">`，不用 `div` + `@click`（键盘与焦点语义） |
| 图标按钮 | 必须有 `:title` 或 `aria-label` |
| 状态变化 | 纯图标表达状态时补文字或 `:title`（如工具卡 `×N` 附失败计数说明） |
| 焦点 | 不主动 `outline: none`；弹层用 Element Plus 的焦点陷阱 |
| 对比度 | 正文用 `--wb-ink`、次要信息用 `--wb-ink-2`、弱提示用 `--wb-muted`；不在浅色上叠主色淡字 |
| 动效 | 三档时长只解释状态变化；不做装饰性动画与 `prefers-reduced-motion` 冲突的场景 |

## 8. 契约同步义务

| 改动 | 必须同步 |
|---|---|
| 新增前端 API 调用 | `api/client.ts` 的 `KNOWN_PREFIXES`（否则调用立即抛错，而非静默 404）；由 `check-contract.ps1` 校验 |
| 新增消费字段 | `types/api.ts` 类型 + 后端 RESP 的 json tag |
| 新增文案 | `i18n/dict-zh.ts` 与 `dict-en.ts` 同步加键，跑 `node scripts/i18n-sync.mjs check` |
| 新增令牌 | `themes.css`（含 `useTheme.ts` 预览表同步） |
| 新增组件语法 | `wb-ui.css`（不在组件内复制副本） |
