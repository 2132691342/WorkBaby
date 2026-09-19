# 快速上手

WorkBaby 是一个本地优先的个人 AI 助手桌面客户端。本文带你 3 分钟跑通第一条对话。

## 1. 配置模型

打开左侧「设置」→「模型」标签，点击添加 Provider：

- **OpenAI 兼容**：填 API Key、Base URL、模型名（如 DeepSeek / Moonshot / 智谱 / Ollama）
- **Anthropic**：填 API Key、模型名（如 claude-sonnet）
- **Ollama**：本地推理，Base URL 填 `http://localhost:11434`

保存后点「测试连通」，出现绿色提示即表示可用。

## 2. 发第一条消息

回到「聊天」页，输入框直接打字，回车发送。回复以流式渲染，思考过程（若模型支持）会单独折叠展示。

## 3. 常用快捷键

| 快捷键 | 作用 |
|---|---|
| `Ctrl+N` | 新建会话 |
| `Ctrl+/` 或 `Ctrl+Shift+P` | 聚焦输入框 |
| `Enter` | 发送 |
| `Shift+Enter` | 换行 |

## 4. 下一步

- 上传知识库文档（「知识库」页）让 AI 回答私有资料
- 启用联网搜索（「设置」→「搜索」）
- 绑定工作区目录，让 AI 读写你的项目文件
