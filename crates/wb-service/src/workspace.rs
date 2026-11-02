//! 工作区热切换：换目录即重扫技能、重建待办存储与工具注册表，并广播给界面。

use std::path::PathBuf;
use std::sync::Arc;

use super::Services;

/// 工作区切换结果：旧路径与新路径。
#[derive(Debug)]
pub struct Switched {
    /// 切换前的工作区
    pub from: PathBuf,
    /// 切换后的工作区
    pub to: PathBuf,
}

impl Services {
    /// 当前工作区路径。
    pub fn workspace_dir(&self) -> PathBuf {
        PathBuf::from(self.workspace.lock().unwrap().clone())
    }

    /// 当前工作区字符串形态（提示词 / 事件载荷用）。
    pub fn workspace_text(&self) -> String {
        self.workspace.lock().unwrap().clone()
    }

    /// 按当前工作区重扫技能与待办：三区路径里只有工作区那一段会变。
    pub fn reload_workspace_scoped(self: &Arc<Self>) {
        let dir = self.workspace_dir();
        let skills = wb_skills::SkillRegistry::scan(
            &self.data_dir.join("skills").join("builtin"),
            &self.data_dir.join("skills").join("user"),
            &dir.join(".workbaby").join("skills"),
        );
        *self.skills.write().unwrap() = skills;
        let ctx = super::features::feature_ctx_public(self);
        *self.todos.lock().unwrap() = Some(Arc::new(wb_features::todo::TodoStore::new(&ctx)));
    }

    /// 当前待办存储（未初始化时为 None：功能关闭或刚切换工作区）。
    pub fn todos(&self) -> Option<Arc<wb_features::todo::TodoStore>> {
        self.todos.lock().unwrap().clone()
    }

    /// 切换工作区：校验目录、重扫派生状态、重建注册表、写回设置并广播。
    /// 有会话正在跑时拒绝——运行中的工具 cwd 与索引都对着旧目录。
    pub fn switch_workspace(self: &Arc<Self>, next: &str) -> wb_pkg::Result<Switched> {
        let target = next.trim();
        if target.is_empty() {
            return Err(wb_pkg::AppError::new(
                wb_domain::error::common::PARAM_INVALID,
                "工作区路径不能为空",
            ));
        }
        let canonical = wb_pkg::realpath::real_path(&PathBuf::from(target)).map_err(|_| {
            wb_pkg::AppError::new(
                wb_domain::error::common::PATH_MISSING,
                format!("工作区目录不存在：{target}"),
            )
        })?;
        if !canonical.is_dir() {
            return Err(wb_pkg::AppError::new(
                wb_domain::error::common::PATH_MISSING,
                format!("工作区不是目录：{target}"),
            ));
        }
        let running = self.chat.lock().unwrap().running_ids();
        if !running.is_empty() {
            return Err(wb_pkg::AppError::new(
                wb_domain::error::session::BUSY,
                "有对话正在运行，先停止它再切换工作区",
            ));
        }
        let from = self.workspace_dir();
        if from == canonical {
            return Ok(Switched {
                from,
                to: canonical,
            });
        }
        let text = canonical.display().to_string();
        *self.workspace.lock().unwrap() = text.clone();
        self.reload_workspace_scoped();
        // 设置 KV 是重启后的真相：写失败不该让已经生效的切换半途而废
        if let Err(e) = self.repo.settings_set("workspace", &text) {
            tracing::warn!("工作区写回设置失败：{}", e.message);
        }
        super::features::refresh_registry(self);
        self.emit_event(
            "",
            "app:workspace",
            serde_json::json!({ "workspace": text }),
        );
        // MCP 有两份配置（全局 + 工作区级），切了目录就要重连：否则工具表还是旧目录的。
        // 无 tokio 运行时（单测直调）时同步重载：不能靠 spawn 把错误吞掉。
        let sv = Arc::clone(self);
        match tokio::runtime::Handle::try_current() {
            Ok(_) => {
                tokio::spawn(async move {
                    if let Err(e) = super::mcp::reload(&sv).await {
                        tracing::warn!("工作区切换后 MCP 重载失败：{}", e.message);
                    }
                });
            }
            Err(_) => {
                self.mcp.shutdown_all();
            }
        }
        Ok(Switched {
            from,
            to: canonical,
        })
    }
}
