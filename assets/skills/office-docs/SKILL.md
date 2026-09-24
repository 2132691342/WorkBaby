---
name: office-docs
version: 1.0.0
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
  - 生成 pdf
  - 转 pdf
  - 合并 pdf
  - 提取表格
description: 在本机用内置 Python 处理 Word / Excel / PPT / PDF：读取、改写、生成、格式转换、合并拆分、提取表格与图片。当用户提到这些文件类型或要产出这类交付物时使用。不适用于纯文本/代码的读写。
allowed_tools: ["file_read", "file_write", "file_list", "file_edit", "exec", "doc_reader"]
---

# 办公文档处理

WorkBaby 自带 Python 运行时，**没有预装第三方库**。标准做法：写脚本 → 用内置 python 跑 → 读回结果校验。

## 一、环境与依赖

先确认解释器可用，再按需装包（首次需要联网，后续复用）：

```
python -c "import sys; print(sys.version)"
pip install openpyxl python-docx python-pptx pypdf reportlab
```

| 任务 | 库 | 安装 |
|---|---|---|
| Excel 读写 | `openpyxl` | `pip install openpyxl` |
| Word 读写 | `python-docx` | `pip install python-docx` |
| PPT 处理 | `python-pptx` | `pip install python-pptx` |
| PDF 读取/合并/拆分/水印 | `pypdf` | `pip install pypdf` |
| PDF 生成 | `reportlab` | `pip install reportlab` |
| 批量/表格数据 | `pandas` | `pip install pandas` |

已有能力优先复用：`doc_reader` 工具可直接抽取 pdf/docx/txt/md/html/csv/json 的正文，**先试它，不必写脚本**。

## 二、标准流程

1. **先看**：`file_read` 或 `doc_reader` 拿到真实结构与内容，不凭用户描述猜（尤其是模板文件，要先看清占位符）。
2. **写脚本**：用 `file_write` 落到工作区内（如 `.workbaby/scripts/task_xxx.py`），路径用相对路径。
3. **跑**：`exec` 执行，参数数组里给绝对路径或明确的 cwd，不要在命令里拼 Python 代码。
4. **校验**：`file_read` 读回产物确认；Excel 要读值（不是公式），PDF 要抽文本对照。
5. **报告**：说清产物路径与关键结果；没跑通要说明失败原因，不许声称已完成。

## 三、各格式要点

**Excel**
- 读公式的结果必须 `load_workbook(..., data_only=True)`；读公式本身用默认参数，两者要两次加载。
- 写入用 `openpyxl`：`ws.cell(row, col).value = ...`；改完必须 `wb.save(path)`。
- CSV 用 `pandas.read_csv` / `to_csv(index=False)`；中文注意 `encoding='utf-8-sig'`，否则 Excel 打开乱码。
- 合并单元格的值只在左上角单元格。

**Word**
- `python-docx` 只能新建或追加，**不能编辑已有文档结构**；改现有文件用 `replace` 改文本，复杂度高时先跟用户确认。
- 中文字体要同时设置 `style.font.name` 与 `rPr` 的 `eastAsia`（不设则中文掉西文字体）。
- 用 `doc.tables` / `doc.paragraphs` 遍历，`add_heading(level)` 建层级。

**PPT**
- `python-pptx` 只能基于模板新建，不能直接改现有 pptx 的版式；做「套模板」先 `Presentation('template.pptx')`。
- 用 `slide_layouts[i]` 选版式，占位符通过 `placeholder_format.idx` 定位，不要假设索引固定。
- 文本框 `tf.word_wrap = True`，否则长文本溢出。

**PDF**
- 用 `pypdf`：`PdfReader` 读页/文本/表单；`PdfWriter` 合并 (`append`)、拆分、旋转、加水印（先生成水印 PDF 再 `merge_page`）。
- 扫描件没有文本层（抽出来是空的），直接告诉用户，不要用空结果假装成功。
- 生成 PDF：`reportlab` 适合从头排版；已有 HTML/MD 要先转成 PDF 时，说明限制或改用 Word 路线。
- 加密/解密用 `reader.decrypt(password)`；处理加密文件**必须**先向用户要密码。

## 四、硬性约束

- 所有读写限定在工作区沙箱内，路径越界会被拒绝，不要试图用绝对路径绕开。
- 产物默认写在工作区内；交付时给出相对路径。
- 一次失败改方法，不原地重复跑同一条命令超过两次。
- 没装成功依赖就明说，不要假装处理完成。
