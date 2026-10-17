---
name: office-docs
version: 2.0.0
when_to_use:
  - .docx
  - .xlsx
  - .pptx
  - .pdf
  - word 文档
  - excel
  - 电子表格
  - ppt
  - 幻灯片
  - 提取表格
  - 合并 pdf
description: 在本机处理 Word / Excel / PPT / PDF：读取内容、生成与批量处理表格文本、提取数据。用户提到这些文件类型或要产出这类交付物时使用。不适用于纯文本/代码的读写。
---

# 办公文档处理

先记住两条环境事实，它们决定所有做法：

- 内置 Python 是精简运行时：**没有第三方库，也没有 pip**。`pip install` 一定失败，不要试。
- 读办公文档正文优先走**知识库检索**（`knowledge_search`），写产物用 `python` 标准库或直接写文件。

## 一、读取

| 文件 | 做法 |
|---|---|
| txt / md / csv / json / log | `read` 直接读；大文件用 offset / limit 分段 |
| docx / xlsx / pptx / pdf | 让用户把文件加进知识库，再用 `knowledge_search` 检索（四类都能抽文字） |
| 需要看整体结构或抽全部表格 | `python` 标准库脚本：这三种格式本质是 zip + xml，用 `zipfile` + `re` 抽文本 |
| 用户用 @ 引用了 docx / xlsx / pdf | 读不到正文是正常的（二进制文档），改走知识库，不要让用户以为文件是坏的 |

## 二、生成与修改

| 交付物 | 做法 |
|---|---|
| .csv | 最稳：`python` 写 CSV，`encoding='utf-8-sig'`（Excel 双击打开不乱码） |
| .xlsx | `python` 标准库 `zipfile` 拼最小 xlsx；工作区有现成模板时「解压 → 改 xml → 重打包」 |
| .docx | 同上（zip + xml）；内容简单时先产出 md / txt，告知用户可在 Word 里另存 |
| .pdf | 生成 / 合并 / 拆页**做不到**（没有第三方库）：如实说明，给出替代（用户自己的 Office 里另存 PDF） |

写 xlsx 的最小套路：

1. 用 `zipfile.ZipFile(w)` 写三个部件起步：`[Content_Types].xml`、`xl/workbook.xml`、`xl/worksheets/sheet1.xml`；
2. 单元格文本直接内联 `<is><t>文字</t></is>`，省掉 sharedStrings；
3. 写完必须用 `python` 重新打开 zip、抽单元格文本验证一遍再报告。

## 三、可选：借用户本机的 Office 做转换

只在需要 Word→PDF 这类第三方程式能力、且用户机器装了对应软件时走这条路：

```
powershell -Command "try { New-Object -ComObject Word.Application | Out-Null; 'word-ok' } catch { 'no-word' }"
```

先探测再提方案；用户确认后才执行（会拉起 Office、弹窗口、占用软件）。探测不到就明说做不了，不要硬试。

## 四、硬性约束

- 所有读写限定在工作区沙箱内，路径越界会被拒绝，不要试图用绝对路径绕开。
- 产物写在工作区，交付时给相对路径；一次失败改方法，不原地重复同一条命令超过两次。
- 校验后才报告：读回产物确认内容；没跑通就说清卡在哪，不许声称已完成。
- 删除、覆盖、批量改动前先说清影响范围。
