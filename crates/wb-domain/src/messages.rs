//! 错误码的默认人话文案：后端抛错时只有 details 缺 message，或边界处补一句通用说明。
//!
//! 界面语言的最终裁决在前端（`utils/errors.ts` 按 code 查两套语言包），
//! 这里只保证「不经过界面也要能读懂」：日志、CLI 调试、SSE 兜底。

/// 错误码 → (中文, English)。未收录的码走空串，由调用方用自己的细节描述。
fn texts(code: &str) -> (&'static str, &'static str) {
    match code {
        // 通用 / 文件 / 路径
        "1001" => ("参数缺失或非法", "Missing or invalid parameter"),
        "1002" => ("文件读写失败", "File read or write failed"),
        "1003" => ("路径越界", "Path escapes the allowed root"),
        "1004" => ("路径不存在", "Path does not exist"),
        "1017" => ("git 命令失败", "git command failed"),
        "1020" => ("git 提交失败", "git commit failed"),
        "1021" => ("还有冲突没解决", "Unresolved conflicts remain"),
        "1024" => (
            "远端有新的提交，先拉取",
            "The remote has new commits; pull first",
        ),
        "1030" => ("压缩包解析失败", "Could not parse the archive"),
        // 配置 / 持久化
        "2001" => ("配置格式不对", "Malformed configuration"),
        "2002" => ("本地数据库出错", "Local database error"),
        "2004" => ("数据目录不可写", "The data directory is not writable"),
        // Provider
        "3001" => ("找不到这个模型服务", "No such model service"),
        "3002" => ("找不到这个模型", "No such model"),
        "3003" => ("还没填密钥", "No API key configured"),
        "3005" => ("接口地址不合法", "Invalid base URL"),
        "3109" => (
            "上游返回的内容看不懂",
            "Could not parse the upstream response",
        ),
        "3111" => ("名字重复了", "That name is already taken"),
        "3200" => ("连不通这个服务", "Could not reach the service"),
        // 会话运行
        "5001" => (
            "这个工作目录下还有对话在跑",
            "Another conversation is running here",
        ),
        "5002" => ("会话不存在", "No such session"),
        "5003" => ("会话文件被占用", "The session file is busy"),
        "5004" => ("排队已满", "The queue is full"),
        "5005" => ("这个提问已经过期", "That question has expired"),
        "5006" => ("上下文压缩失败", "Context compaction failed"),
        "5100" => ("内核还没准备好", "The kernel is not ready yet"),
        // 工具与外部进程
        "7001" => ("超时了", "Timed out"),
        "7002" => ("已取消", "Cancelled"),
        "7003" => ("这个工具现在用不了", "That tool is unavailable right now"),
        "7004" => ("工具参数不对", "Invalid tool arguments"),
        "7100" => ("语言服务器启动失败", "Could not start the language server"),
        "7101" => ("语言服务器报错", "The language server returned an error"),
        "7102" => ("没装这个语言服务器", "The language server is not installed"),
        "7103" => (
            "这种文件没有对应的语言服务器",
            "No language server handles this file type",
        ),
        "7301" => ("记忆条目不存在", "No such memory entry"),
        // Skill
        "8001" => ("SKILL.md 解析失败", "Could not parse SKILL.md"),
        "8002" => (
            "description 缺失或太长",
            "Description is missing or too long",
        ),
        "8003" => ("技能名不合法", "Invalid skill name"),
        "8004" => ("技能名重复了", "That skill name already exists"),
        "8005" => (
            "内置技能不能改也不能删",
            "Built-in skills cannot be edited or deleted",
        ),
        // MCP
        "9101" => ("MCP 配置不对", "Invalid MCP configuration"),
        "9102" => ("MCP 服务起不来", "Could not start the MCP server"),
        "9103" => ("MCP 工具调用失败", "MCP tool call failed"),
        // 内核
        "9701" => ("会话文件损坏", "The session file is corrupt"),
        "9703" => ("模型流中断了", "The model stream broke"),
        "9704" => ("子代理启动失败", "Could not start the subagent"),
        "9705" => (
            "模型反复请求工具仍未收尾，已停下",
            "The model kept calling tools without finishing; stopped",
        ),
        _ => ("", ""),
    }
}

/// 按语言取默认文案；`lang` 传 `"en"` 取英文，其余取中文。
pub fn default_message(code: &str, lang: &str) -> &'static str {
    let (zh, en) = texts(code);
    if lang.eq_ignore_ascii_case("en") {
        en
    } else {
        zh
    }
}

/// 错误码是否有默认文案（诊断与测试用）。
pub fn is_known(code: &str) -> bool {
    !texts(code).0.is_empty()
}
