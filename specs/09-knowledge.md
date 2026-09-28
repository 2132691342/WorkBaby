# 09 · 知识库

## 定位

让助手能引用用户自己的资料。无向量库、无嵌入模型：
**SQLite FTS5 trigram + 子串兜底**，本机即可用，中文友好。

## 流程

```
选择文件 → loader 提取纯文本 → chunker 切块 → 入库 → FTS 触发器同步
检索：FTS MATCH → bm25 排序 → 短查询走 LIKE 子串兜底
```

## 支持格式

| 格式 | 提取方式 |
|---|---|
| .md / .txt | 原文 |
| .pdf | 逐页取文本 |
| .docx | document.xml 去标签 |
| .xlsx | sharedStrings + sheet 单元格逐行 |
| .html | 去标签转纯文本 |
| .csv | 按行 |

## 切分

- 目标 600 字符 / 块，硬上限 1200
- 逐行累加到目标长度；块与块之间重复上一块的末段作为重叠，避免答案正好落在切口上
- 每块记录 `seq`（在文档内的顺序），检索结果可按序还原

## 检索

1. `MATCH` + `bm25` 排序，取 top N（默认 5）
2. 查询串短于 3 字符（trigram 最小 token）→ FTS 必然零命中 → 走 `LIKE` 子串兜底
3. 命中带 `doc_id / title / path / seq / content / score`

**注入方式：模型经 `knowledge_search` 工具自主调用，不自动塞 prompt。**
自动召回会让每轮对话都背着全部资料，既贵又稀释注意力。

## 状态机

`pending → indexed → failed`；失败原因落 `error` 字段，前端直接展示，
不让用户对着一个「添加失败」干瞪眼。

## 取舍

不做向量检索：个人文档规模（百 MB 级）下关键词检索足够准，
省掉嵌入模型与向量库两个重依赖。
