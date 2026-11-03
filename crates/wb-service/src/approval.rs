//! 工具审批闸门：破坏性操作要经过用户点头才真执行。
//!
//! 面向「第一次用 AI 的办公人员」——这类用户不会预判 `rm -rf` 的后果，
//! 没有闸门就等于把机器直接交出去。闸门不是万能的：它只拦「一眼就能看出危险」的那类操作，
//! 复杂脚本仍要靠模型自己的判断与 Git 写白名单兜底。

use std::sync::Arc;
use std::time::Duration;

use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

/// 审批模式。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ApprovalMode {
    /// 全放行（用户明确表示「我信任它」）
    Off,
    /// 只读操作自动放行，破坏性操作要点头
    Ask,
    /// 所有工具调用都要点头
    All,
}

impl ApprovalMode {
    /// 解析模式码；未知值按 `Ask` 处理（宁可多问一次）。
    pub fn parse(code: &str) -> Self {
        match code.trim() {
            "off" => Self::Off,
            "all" => Self::All,
            _ => Self::Ask,
        }
    }

    /// 模式码（写回设置用）。
    pub fn code(self) -> &'static str {
        match self {
            Self::Off => "off",
            Self::Ask => "ask",
            Self::All => "all",
        }
    }
}

/// 审批等待上限：一直等下去等于把会话挂死。
const APPROVAL_TIMEOUT: Duration = Duration::from_secs(300);

/// 判定一次工具调用是否需要审批；返回原因（`None` = 直接放行）。
/// 判据只看「一眼能看出危险」的信号：删除命令、改系统、装包、动 git 历史。
/// 复杂脚本不在此列——逐行分析脚本既不可靠又会拖慢每一次调用。
pub fn needs_approval(tool_name: &str, args: &Value) -> Option<String> {
    match tool_name {
        "bash" | "powershell" => {
            let cmd = args["command"].as_str().unwrap_or("");
            let lower = cmd.to_lowercase();
            if let Some(why) = destructive_in_shell(&lower) {
                return Some(why);
            }
            None
        }
        "write" | "edit" => {
            let path = args["path"].as_str().unwrap_or("");
            if is_system_path(path) {
                Some(format!("要改系统路径：{path}"))
            } else {
                None
            }
        }
        _ => None,
    }
}

/// shell 命令里的破坏性信号。
fn destructive_in_shell(lower: &str) -> Option<String> {
    // 递归删除：这是最致命的一类
    for pat in ["rm -rf", "rm -fr", "rm -r ", "rd /s", "rmdir /s"] {
        if lower.contains(pat) {
            return Some(format!("命令里有递归删除（{pat}）"));
        }
    }
    // 磁盘级操作
    for pat in ["format ", "mkfs", "diskpart", "dd if=", "cipher /w"] {
        if lower.contains(pat) {
            return Some(format!("命令里有磁盘级操作（{pat}）"));
        }
    }
    // 权限与所有权
    for pat in [
        "chmod 777",
        "icacls",
        "takeown",
        "net user",
        "net localgroup",
    ] {
        if lower.contains(pat) {
            return Some(format!("命令里有权限变更（{pat}）"));
        }
    }
    // 装包 / 改环境：会动用户机器的依赖状态
    for pat in [
        "pip install",
        "npm install -g",
        "npm i -g",
        "yarn global add",
        "cargo install",
        "choco install",
        "winget install",
        "scoop install",
    ] {
        if lower.contains(pat) {
            return Some(format!("命令里有全局安装（{pat}）"));
        }
    }
    // 改 git 历史：会毁掉已有提交
    for pat in [
        "git push --force",
        "git push -f",
        "git reset --hard",
        "git clean -fd",
        "git checkout --",
        "git branch -D",
    ] {
        if lower.contains(pat) {
            return Some(format!("命令里有不可逆的 git 操作（{pat}）"));
        }
    }
    // 改系统服务与注册表
    for pat in [
        "reg add",
        "reg delete",
        "sc config",
        "sc delete",
        "new-service",
        "set-service",
        "schtasks /create",
    ] {
        if lower.contains(pat) {
            return Some(format!("命令里有系统级变更（{pat}）"));
        }
    }
    None
}

/// 路径是否在系统目录里。
fn is_system_path(path: &str) -> bool {
    let p = path.to_lowercase().replace('/', "\\");
    p.starts_with("c:\\windows")
        || p.starts_with("c:\\program files")
        || p.starts_with("c:\\program files (x86)")
        || p.starts_with("c:\\users\\all users")
}

/// 一次审批请求的入参。
pub struct ApprovalRequest<'a> {
    /// 回答板：审批与内核提问共用一条通道
    pub board: &'a Arc<wb_features::ask_user::AnswerBoard>,
    /// 事件出口
    pub emit: &'a (dyn Fn(&str, &str, Value) + Send + Sync),
    /// 所属会话
    pub session_id: &'a str,
    /// 被审的工具名
    pub tool_name: &'a str,
    /// 工具参数
    pub args: &'a Value,
    /// 危险原因（给用户看的那句）
    pub reason: &'a str,
    /// 取消令牌
    pub cancel: &'a tokio_util::sync::CancellationToken,
    /// 会话级信任名单
    pub trusted: &'a Arc<std::sync::Mutex<std::collections::HashMap<String, Vec<String>>>>,
}

/// 一次审批请求的结果。
pub struct ApprovalOutcome {
    /// 是否放行
    pub approved: bool,
    /// 用户选择的是「本会话都批」
    pub trusted: bool,
}

/// 发一张审批卡并等用户决定；返回是否放行。
pub async fn request(req: ApprovalRequest<'_>) -> ApprovalOutcome {
    let ApprovalRequest {
        board,
        emit,
        session_id,
        tool_name,
        args,
        reason,
        cancel,
        trusted,
    } = req;
    let approval_id = wb_pkg::ulid::new_id(wb_pkg::Prefix::Ask);
    let summary = summarize(tool_name, args);
    let payload = json!({
        "id": approval_id,
        "method": "confirm",
        "title": format!("需要你确认：{tool_name}"),
        "message": reason,
        "options": ["批准", "拒绝", "本会话都批"],
        "placeholder": "",
        "prefill": "",
        "timeout_ms": APPROVAL_TIMEOUT.as_millis() as u64,
        "session_id": session_id,
        "run_id": "",
        "approval_tool": tool_name,
        "approval_summary": summary,
    });
    // 先登记再发事件：否则用户手快点下批准时，监听者还没挂上
    let rx = board.register_with_card(&approval_id, payload.clone());
    emit(session_id, "chat:ask", payload);
    let outcome = tokio::select! {
        r = rx => Some(r),
        _ = tokio::time::sleep(APPROVAL_TIMEOUT) => None,
        _ = cancel.cancelled() => None,
    };
    board.forget(&approval_id);
    let answer = match outcome {
        Some(Ok(v)) => v,
        _ => {
            emit(
                session_id,
                "chat:ask_closed",
                json!({ "id": approval_id, "reason": "expired" }),
            );
            return ApprovalOutcome {
                approved: false,
                trusted: false,
            };
        }
    };
    emit(
        session_id,
        "chat:ask_closed",
        json!({ "id": approval_id, "reason": "answered" }),
    );
    let choice = answer
        .get("value")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    match choice.as_str() {
        "批准" | "approve" | "yes" | "y" | "允许" => ApprovalOutcome {
            approved: true,
            trusted: false,
        },
        "本会话都批" | "trust" | "always" => {
            trusted
                .lock()
                .unwrap()
                .entry(session_id.to_string())
                .or_default()
                .push(tool_name.to_string());
            ApprovalOutcome {
                approved: true,
                trusted: true,
            }
        }
        _ => ApprovalOutcome {
            approved: false,
            trusted: false,
        },
    }
}

/// 参数摘要（给界面看的那一行）。
fn summarize(tool_name: &str, args: &Value) -> String {
    match tool_name {
        "bash" | "powershell" => args["command"].as_str().unwrap_or("").to_string(),
        "write" | "edit" | "read" => args["path"].as_str().unwrap_or("").to_string(),
        _ => {
            let s = args.to_string();
            if s.len() > 160 {
                format!("{}…", &s[..160])
            } else {
                s
            }
        }
    }
}

/// 实现内核的 `Gate` 契约：按当前模式判定一次工具调用。
struct ServiceGate {
    /// 回答板：审批与内核提问共用一条通道
    board: Arc<wb_features::ask_user::AnswerBoard>,
    /// 审批模式
    mode: ApprovalMode,
    /// 所属会话
    session_id: String,
    /// 事件出口
    emitter: Option<Arc<dyn crate::Emitter>>,
    /// 会话级信任名单：本会话都批过的工具不再问
    trusted: Arc<std::sync::Mutex<std::collections::HashMap<String, Vec<String>>>>,
}

#[async_trait::async_trait]
impl wb_kernel::blocks::Gate for ServiceGate {
    async fn check(
        &self,
        tool_name: &str,
        args: &serde_json::Value,
        cancel: &tokio_util::sync::CancellationToken,
    ) -> (bool, String) {
        match self.mode {
            ApprovalMode::Off => return (true, String::new()),
            ApprovalMode::Ask => {
                // 本会话已信任这类工具：直接放行
                if self
                    .trusted
                    .lock()
                    .unwrap()
                    .get(&self.session_id)
                    .is_some_and(|l| l.iter().any(|t| t == tool_name))
                {
                    return (true, String::new());
                }
            }
            ApprovalMode::All => {}
        }
        let reason = match needs_approval(tool_name, args) {
            Some(r) => r,
            None => {
                // 只读操作不需要点头：拦住它只会让助手变得难用
                if self.mode == ApprovalMode::Ask {
                    return (true, String::new());
                }
                // 全问模式下连只读工具也要点头
                "这个操作需要你确认".to_string()
            }
        };
        let emit = move |session_id: &str, event: &str, data: serde_json::Value| {
            if let Some(em) = &self.emitter {
                em.emit(session_id, event, data);
            }
        };
        let outcome = request(ApprovalRequest {
            board: &self.board,
            emit: &emit,
            session_id: &self.session_id,
            tool_name,
            args,
            reason: &reason,
            cancel,
            trusted: &self.trusted,
        })
        .await;
        if outcome.approved {
            (true, String::new())
        } else {
            (false, format!("用户没有批准这个操作：{reason}"))
        }
    }
}

/// 造一个闸门；模式为 `Off` 时返回 None（内核不拦任何调用）。
pub fn gate_for(
    sv: &Arc<crate::Services>,
    session_id: &str,
) -> Option<Arc<dyn wb_kernel::blocks::Gate>> {
    let mode = current_mode(sv);
    if mode == ApprovalMode::Off {
        return None;
    }
    Some(Arc::new(ServiceGate {
        board: sv.asks.clone(),
        mode,
        session_id: session_id.to_string(),
        emitter: sv.emitter.lock().unwrap().clone(),
        trusted: sv.trusted_tools.clone(),
    }))
}

/// 当前审批模式：设置项 `approval_mode`，缺省 `ask`（默认要问一次）。
pub fn current_mode(sv: &Arc<crate::Services>) -> ApprovalMode {
    sv.repo
        .settings_get("approval_mode")
        .ok()
        .flatten()
        .map(|v| ApprovalMode::parse(&v))
        .unwrap_or(ApprovalMode::Ask)
}

/// 设置审批模式。
pub fn set_mode(sv: &Arc<crate::Services>, code: &str) -> wb_pkg::Result<()> {
    let mode = ApprovalMode::parse(code);
    sv.repo.settings_set("approval_mode", mode.code())
}
