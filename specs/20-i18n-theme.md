# 19 · 国际化与主题

## 1. 国际化（i18n）

### 结构

| 文件 | 作用 |
|---|---|
| `i18n/dict-zh.ts` | 中文词条 |
| `i18n/dict-en.ts` | 英文词条 |
| `i18n/index.ts` | `t(key, ...args)` 实现与语言切换 |
| `i18n/types.ts` | key 类型约束（新增键必须在两本字典都存在） |

### 约定

| 项 | 规范 |
|---|---|
| 文案来源 | 所有面向用户文案走 `t()`，禁止硬编码中文/英文字符串 |
| 键命名 | 域前缀 + 语义（`chat.tool.guardBadge` / `settings.model.title`） |
| 插值 | `t('key', v1, v2)`，占位符在字典里写 `{0}` / `{1}` |
| 字典对齐 | zh / en 的**键集合与顺序必须一致** |
| 新增键 | 两本字典同时加，跑 `node scripts/i18n-sync.mjs check` 校验 |

### 门禁

```bash
node scripts/i18n-sync.mjs check    # 键集合与顺序一致
node scripts/i18n-sync.mjs dead     # 找出未被引用的死键
```

未对齐会导致「切到英文时部分文案仍是中文」（表现为 UI 混语），因此设为门禁而非约定。

### 覆盖范围

窗口按钮、导出模板、帮助正文、错误提示、工具活动描述（后端 `ActivityDesc` 也走 i18n key）。

## 2. 主题

### 双主题

| 主题 | 令牌块 | 底色 | 面 | 强调色 |
|---|---|---|---|---|
| 浅色 · 晴空 | `:root` / `[data-theme='light']` | `#f2f6fc` 冷白 | `#ffffff` | `#2f80ed` 亮蓝 |
| 暗色 · 紫夜 | `[data-theme='dark']` | 近黑 | 深灰紫卡 | `#7c5cf8` 紫罗兰 |

两套主题**共用同一组语义令牌**（`--wb-*`），组件不写死色值——这是「暗色漏白」的根治手段。

### 切换与持久化

| 项 | 实现 |
|---|---|
| 切换 | `useTheme` 设置 `document.documentElement.dataset.theme` |
| 持久化 | 写入 `system_settings` |
| 预览 | `useTheme.ts` 的 `THEMES` 表（设置页主题预览卡片），取值必须与 CSS 同步 |

### 用户背景图

用户可设置背景图（`--wb-user-bg-url` / `-opacity` / `-blur`）。

| 机制 | 说明 |
|---|---|
| 主色提取 | 从背景图提取主色 |
| 统一应用 | 经 `applyExtractedPrimary()` 应用 / 移除（含强调色族派生） |
| 禁止 | 别处不得直接 `style.setProperty('--wb-primary')`（会出现「部分控件跟色、部分不跟」） |

### 令牌纪律

色值只在 `themes.css` 定义；组件用 `wb-*` token 类；控件高度用四档令牌；
字号用刻度类；禁止裸像素与任意值。完整规则见 [`DESIGN.md`](../../../DESIGN.md) 与
[`docs/COMPONENT-GUIDELINES.md`](../../../docs/COMPONENT-GUIDELINES.md)。

## 3. 字体的跨语言处理

| 族 | 令牌 | 用途 |
|---|---|---|
| 显示字体 | `--font-disp`（Bahnschrift） | 标题与读数（DIN 风） |
| 中文 | `--font-cn`（Microsoft YaHei UI） | 中文正文回退 |
| 等宽 | JetBrains Mono | 路径、ID、token、代码 |

字体栈按「UI 系统无衬线 → 中文族 → 通用」排列，保证中英混排时基线一致。
不打包 Inter（避免非必要体积）。

## 4. 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 双字典 + 门禁校验 | 不会出现 UI 混语；死键可清理 | 每次加文案要动两个文件 |
| 键名带域前缀 | 定位快（`chat.*` 与后端事件同域） | 键名较长 |
| 语义令牌（非直接色值） | 换主题只改一处；暗色不漏白 | 需要维护令牌表并同步预览视图 |
| 背景图主色统一派生 | 全应用配色一致 | 提取失败时需回退（有默认主色） |
| 三族字体（不打包 Web 字体） | 体积小、无加载闪烁 | 依赖系统字体可用性（Windows 下均有） |
| 主题持久化走设置表 | 与其他设置同源 | 首次渲染需要读取设置（避免闪白） |
