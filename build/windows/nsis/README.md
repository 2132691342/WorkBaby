# NSIS 安装器脚本

`project.nsi` 是**手维护文件**，不是生成物。

## 安装器的职责边界

安装包必须带上完整的 `assets/`（内置技能包）：
少了它们，装完的机器上没有推荐技能，首启的技能页就是空的。
（git / node / python 一律探测系统已装，运行时不随包分发。）
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
task package
# 产物 bin/WorkBaby-amd64-installer.exe 应在十几 MB 量级（exe + 技能包压缩后）
# 体积上百 MB 说明 assets 里混进了不该随包的东西（比如运行时 zip）
```

装完检查 `%LOCALAPPDATA%\Programs\WorkBaby\assets\skills\` 下有推荐技能目录。

## 数据目录

卸载开始时弹一次询问：**是否连 `%LOCALAPPDATA%\WorkBaby` 一起删**。
默认答案是「否」——会话、MCP 配置、技能与设置是用户的工作成果，
删不删必须由用户自己决定。WebView2 缓存（数据目录下的 `webview\`）
无论选什么都清：它可再生，且绑定的是这一次安装。
