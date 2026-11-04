# NSIS 安装器脚本

`project.nsi` 是**手维护文件**，不是生成物。

## 为什么不跟随 `wails3 update build-assets`

上游模板只装 exe，不装 `assets/`。本项目的 `assets/` 里有内置运行时 zip、
内置技能与帮助文档——少了它们，装完的机器上没有任何运行时，首启必然装配失败，
沙箱这个存在意义就没了。所以这里手工加了一段 `File /r "...\assets\*.*"`。

`wails3 update build-assets` 会用上游模板覆盖本文件。**执行过它之后必须回头
检查 `project.nsi` 是否还带着 assets 打包段**，没有就补回去。

## 两个必须遵守的约束

1. **文件只能有 ASCII**。NSIS 以系统 ANSI 代码页读脚本，UTF-8 中文注释会直接
   报 `Bad text encoding` 并中止。注释写英文。
2. **`File /r` 的路径是相对 `build/windows/nsis/`**，即仓库根往上三级。
   改目录结构要同步改这段。

## 验证

```bash
wails3 task package
# 产物 bin/WorkBaby-amd64-installer.exe 应在 120MB 上下（= exe + assets 压缩后）
# 只有十几 MB 说明 assets 没打进去
```

装完检查 `%LOCALAPPDATA%\Programs\WorkBaby\assets\runtime\` 下三个 zip 是否齐备。

## 数据目录

卸载时**刻意不删** `%LOCALAPPDATA%\WorkBaby`：会话、插件、技能与配置是用户的
工作成果，卸载程序不该替用户决定删掉它们。要清由用户在界面上自己动手。
