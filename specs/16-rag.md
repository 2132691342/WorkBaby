# 16 · 知识库与检索

知识库用于「把一批文档变成可检索的上下文」，**全部在本机完成，不引入向量数据库**。

## 1. 数据

| 表 | 字段 |
|---|---|
| `knowledge_docs`（软删） | `Name / Source / SourceType(file\|url\|text) / Status / ChunkCount / SizeBytes` |
| `knowledge_chunks` | `DocID / ChunkIdx / Content / Meta(title)` |
| `knowledge_chunks_fts`（虚拟表） | `id, doc_id, chunk_idx, title, content`，trigram 分词 |

文档状态机：`pending → parsing → indexed | failed`。

## 2. 索引

### 分块

`Chunker{ChunkSize: 500, Overlap: 50}`：

1. 按空行切段
2. 段内超长按句子边界切（找不到断点才硬切）
3. 累积到 `ChunkSize` 出块并带 `Overlap` 尾巴
4. Markdown 单行标题（≤120 rune）记录为该块的 `title`

### 加载

`PickLoader(mime)`：`TextLoader / PDFLoader / DocxLoader / HTMLLoader`。

| 限制 | 值 |
|---|---|
| 文件上限 | 60MB |
| URL 抓取超时 | 30s |
| URL 响应上限 | 5MB |

### 索引事务

删旧分块 → 删旧 FTS 行 → 批量插新分块（100/批）→ 逐条插 FTS → 回写状态与分块数。

**重建式（先删后插）**：不会残留过期内容被检索到。

## 3. 检索

`FTS5Retriever.Search(ctx, query, topK)`（topK 默认 5，上限 50）：

| 步 | 行为 |
|---|---|
| 1 | `BuildMatchQuery(query)` 生成 MATCH 串；若所有 token 都短于 3 字（trigram 窗口）则直接走兜底 |
| 2 | 主检索：`fts MATCH ? JOIN chunks JOIN docs(未删) ORDER BY bm25`；**Score = −bm25**（翻转为越大越相关） |
| 3 | 零命中 → 兜底 `searchLike`：整串 LIKE OR 各 token LIKE，候选放大到 4 倍上限 200，Go 侧用 `ScoreSubstring` 重排后截 topK |

**中文检索的关键**：FTS5 默认分词器不按字切分 CJK，整句中文会退化成单个 token，因此必须用 trigram。
代价是 2 字查询在 MATCH 上必然零命中——由上面的兜底承担。

## 4. 输出与入口

`FormatHits` 渲染成带出处的片段：

```
[1] 文档名 · 标题 · 段3
正文……
```

| 入口 | 说明 |
|---|---|
| 模型侧 | `knowledge_search` 工具（**唯一入口**），返回带编号出处的片段；`Data["hits"]` 供前端渲染来源卡 |
| 前端侧 | 知识库管理页（`kdocs` 增删改 + 重建索引）与全文搜索 |

## 5. 注入

能力注册表在 run 前按用户输入检索，作为 `Section` 注入（`Order=50`）；
也可由 `@知识` 显式触发（相关性门槛三档：`opt_in > keyword > off`）。

## 6. 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| FTS5 关键词检索（不引入 embedding） | 零额外依赖、离线可用、延迟低 | 语义相近但用词不同的内容召回不到 |
| 固定分块（500 字，重叠 50） | 实现简单、可预测 | 跨块语义被切断；表格与代码块可能被拆开 |
| 相关性门槛三档 | 避免无关知识污染上下文 | 门槛语义需要用户理解（设置页给出说明） |
| 重建式重索引 | 不会残留过期内容 | 重索引期间该文档短暂不可检索 |
| trigram 分词 + 子串兜底 | 中文可检索；短词不漏 | 短查询退化为扫描（候选上限 200） |
| 文档上限 60MB | 防大文件拖垮解析 | 超大资料需用户先拆分 |
