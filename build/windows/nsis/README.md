# NSIS 安装器脚本

`project.nsi` 是**手维护文件**，不是生成物。

## 安装器的职责边界

安装包必须带上完整的 `assets/`（内置运行时 zip、内置技能与帮助文档）：
少了它们，装完的机器上没有任何运行时，首启必然装配失败，沙箱就失去了存在意义。
所以打包段里手工写了 `File /r "...\assets\*.*"`——这一段是本文件的核心，
任何重新生成安装器脚本的动作都可能把它丢掉。**执行过 `wails3 update build-assets`
之后必须回头检查这一段还在不在**，不在就补回去。

## 两个必须遵守的约束

1. **文件必须是 UTF-8 with BOM**。卸载询问有中文文案，NSIS 靠 BOM 识别编码；
   无 BOM 的 UTF-8 会按 ANSI 代码页读，中文直接报 `Bad text encoding` 并中止。
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

卸载开始时弹一次询问：**是否连 `%LOCALAPPDATA%\WorkBaby` 一起删**。
默认答案是「否」——会话、插件、技能与配置是用户的工作成果，删不删必须
由用户自己决定。WebView2 缓存（数据目录下的 `webview\`）无论选什么都清：
它可再生，且绑定的是这一次安装。
