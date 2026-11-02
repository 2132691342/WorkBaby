//! 内置运行时状态视图：界面据此显示 bash / git / python 是否就位。

use serde::{Deserialize, Serialize};

/// 单个运行时可执行文件的状态。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct RuntimeBinaryVO {
    /// 可执行名（bash / git / python）
    pub binary_name: String,
    /// 是否可用
    pub binary_ready: bool,
    /// 命中的绝对路径（不可用时为空串）
    pub binary_path: String,
}

/// 运行时整体状态。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct RuntimeStatusRESP {
    /// 运行时根目录（`{data_dir}/runtime`）
    pub resp_root: String,
    /// 三个可执行文件的状态
    pub resp_binaries: Vec<RuntimeBinaryVO>,
    /// 全部就位
    pub resp_ready: bool,
    /// 解压或校验失败原因（成功时为空串）
    pub resp_failure: String,
}
