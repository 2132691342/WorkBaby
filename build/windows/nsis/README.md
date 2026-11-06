# NSIS 安装器脚本

`project.nsi` 是**手维护文件**，不是生成物。

## 安装器的职责边界

安装包必须带上完整的 `assets/`（内置运行时 zip、内置技能与帮助文档）：
少了它们，装完的机器上没有任何运行时，首启必然装配失败，沙箱就失去了存在意义。
所以打包段里手工写了 `File /r "...\assets\*.*"`——这一段是本文件的核心，
任何重新生成安装器脚本的动作都可能把它丢掉。**执行过 `wails3 update build-assets`
之后必须回头检查这一段还在不在**，不在就补回去。

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
