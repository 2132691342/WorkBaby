# 06 · 内置 Python 运行时

## 定位

只内置 Python 一种运行时：办公场景（表格 / 文档 / 图片 / 爬虫）覆盖面最大，
embeddable 包体可控。其他语言一律走系统 PATH。

## 查找顺序

```go
runtime.PythonExe(paths) :=
  EnsurePython(paths)     // ① 内置：首次调用自动解压到数据目录
| lookPath("python.exe")  // ② 系统 PATH
| lookPath("python")      // ③ 无扩展名（兼容异常环境）
```

返回空串表示不可用：`python` 工具在执行时给出明确错误码 8002，
`/bootstrap` 的 `python_ready` 让前端能提前在设置页显示状态灯。

## 解压

内置运行时以 `python-<版本>-win-x64.tar.gz` 随产物分发在 exe 同级的 `runtimes/`：

1. 目标目录里没有 `.version` 或版本不一致 → 解压
2. 解压逐条目校验路径，拒绝绝对路径与 `..`（8004）
3. 总体积上限 512MB，防压缩包炸弹
4. 写 `.version` 标记；命中版本就跳过解压，秒开

## 目录布局

```
<exeDir>/runtimes/python-3.12.13-win-x64.tar.gz   # 随产物分发
%APPDATA%/WorkBaby/runtime/python/                 # 解压后
├── python.exe
├── python312._pth
└── Lib/site-packages/
```

## python 工具

| 参数 | 说明 |
|---|---|
| code | 要执行的 Python 源码 |
| timeout_ms | 超时毫秒，默认 120000，上限 600000 |

- 脚本写入会话临时目录执行，工作目录 = 会话工作目录
- `PYTHONIOENCODING=utf-8`：不加的话中文 print 会抛 UnicodeEncodeError
- `PYTHONDONTWRITEBYTECODE=1`：不在用户工作目录里撒 `__pycache__`
- `pip install` 不走此工具——需要装包时用 `powershell`（需审批）

## 取舍

不做多版本管理、不做虚拟环境隔离：个人助手要的是「开箱能跑」，
确定性来自内置解释器本身，而不是环境管理。
