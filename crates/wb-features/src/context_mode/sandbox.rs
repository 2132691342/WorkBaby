//! 沙箱执行：多语言解释器调度；只有 stdout 进上下文，stderr 折叠为附注。

use std::path::{Path, PathBuf};

use serde_json::Value;

/// 语言运行时描述：可执行程序 + 启动参数模板。
pub struct Runtime {
    /// 语言码（小写，模型可见）
    pub lang_code: &'static str,
    /// 展示名
    pub lang_label: &'static str,
    /// 可执行程序名（在 PATH 与工作区里找）
    pub lang_program: &'static str,
    /// 启动参数（`{file}` 是脚本文件占位）
    pub lang_args: &'static [&'static str],
    /// 脚本文件扩展名
    pub lang_ext: &'static str,
}

/// 支持的语言表：覆盖常用脚本语言，解释器缺失时按可用性降级。
pub const RUNTIMES: &[Runtime] = &[
    Runtime {
        lang_code: "javascript",
        lang_label: "JavaScript",
        lang_program: "node",
        lang_args: &["{file}"],
        lang_ext: "mjs",
    },
    Runtime {
        lang_code: "typescript",
        lang_label: "TypeScript",
        lang_program: "deno",
        lang_args: &[
            "run",
            "--no-prompt",
            "--allow-read",
            "--allow-write",
            "{file}",
        ],
        lang_ext: "ts",
    },
    Runtime {
        lang_code: "python",
        lang_label: "Python",
        lang_program: "python",
        lang_args: &["{file}"],
        lang_ext: "py",
    },
    Runtime {
        lang_code: "shell",
        lang_label: "Shell",
        lang_program: "bash",
        lang_args: &["{file}"],
        lang_ext: "sh",
    },
    Runtime {
        lang_code: "powershell",
        lang_label: "PowerShell",
        lang_program: "pwsh",
        lang_args: &["-NoProfile", "-File", "{file}"],
        lang_ext: "ps1",
    },
    Runtime {
        lang_code: "ruby",
        lang_label: "Ruby",
        lang_program: "ruby",
        lang_args: &["{file}"],
        lang_ext: "rb",
    },
    Runtime {
        lang_code: "php",
        lang_label: "PHP",
        lang_program: "php",
        lang_args: &["{file}"],
        lang_ext: "php",
    },
    Runtime {
        lang_code: "perl",
        lang_label: "Perl",
        lang_program: "perl",
        lang_args: &["{file}"],
        lang_ext: "pl",
    },
    Runtime {
        lang_code: "go",
        lang_label: "Go",
        lang_program: "go",
        lang_args: &["run", "{file}"],
        lang_ext: "go",
    },
    Runtime {
        lang_code: "dotnet",
        lang_label: ".NET",
        lang_program: "dotnet",
        lang_args: &["script", "{file}"],
        lang_ext: "csx",
    },
    Runtime {
        lang_code: "java",
        lang_label: "Java",
        lang_program: "java",
        lang_args: &["{file}"],
        lang_ext: "java",
    },
    Runtime {
        lang_code: "lua",
        lang_label: "Lua",
        lang_program: "lua",
        lang_args: &["{file}"],
        lang_ext: "lua",
    },
];

/// 按语言码查运行时；未知语言返回 None。
pub fn find_runtime(language: &str) -> Option<&'static Runtime> {
    let want = language.trim().to_ascii_lowercase();
    RUNTIMES.iter().find(|r| r.lang_code == want)
}

/// 解析可执行程序：内置运行时能提供的语言一律走内置（bash / python），其余交系统 PATH。
pub fn resolve_program(runtime: &Runtime) -> String {
    if let Some(data) = wb_domain::runtime::default_data_dir() {
        let bundled = match runtime.lang_program {
            "bash" => wb_domain::runtime::find_bundled_bash(&data),
            "python" => wb_domain::runtime::find_bundled_python(&data),
            "git" => wb_domain::runtime::find_bundled_git(&data),
            _ => None,
        };
        if let Some(p) = bundled {
            return p.display().to_string();
        }
    }
    runtime.lang_program.to_string()
}

/// 沙箱执行结果。
pub struct RunOutcome {
    /// 标准输出（唯一进上下文的部分）
    pub run_stdout: String,
    /// 标准错误（折叠为附注）
    pub run_stderr: String,
    /// 退出码（被信号杀为 None）
    pub run_code: Option<i32>,
    /// 是否超时
    pub run_timed_out: bool,
    /// 实际启动的命令（供界面展示）
    pub run_command: String,
}

/// 单次执行超时上限（秒）；模型可传更小的值。
pub const MAX_TIMEOUT_SECS: u64 = 120;
/// 默认超时（秒）。
pub const DEFAULT_TIMEOUT_SECS: u64 = 30;

/// 在沙箱目录写脚本文件；返回写出的路径。
pub fn write_script(dir: &Path, ext: &str, code: &str) -> Result<PathBuf, String> {
    std::fs::create_dir_all(dir).map_err(|e| format!("建沙箱目录失败：{e}"))?;
    let path = dir.join(format!("s{}.{}", wb_pkg::ulid::raw_ulid(), ext));
    std::fs::write(&path, code).map_err(|e| format!("写脚本文件失败：{e}"))?;
    Ok(path)
}

/// 组装启动参数：把 `{file}` 占位换成脚本真实路径。
pub fn build_args(runtime: &Runtime, file: &Path) -> Vec<String> {
    let file_text = file.display().to_string();
    runtime
        .lang_args
        .iter()
        .map(|a| a.replace("{file}", &file_text))
        .collect()
}

/// 渲染可读的命令串（界面展示用，不含真实引号转义细节）。
pub fn command_text(runtime: &Runtime, file: &Path) -> String {
    let mut parts = vec![runtime.lang_program.to_string()];
    parts.extend(build_args(runtime, file));
    parts.join(" ")
}

/// 解析工具入参：语言码 / 代码 / 超时秒。
pub fn parse_args(args: &Value) -> Result<(String, String, u64), String> {
    let language = args
        .get("language")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .trim()
        .to_string();
    let code = args
        .get("code")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    if language.is_empty() {
        return Err("language 不能为空".into());
    }
    if code.trim().is_empty() {
        return Err("code 不能为空".into());
    }
    let timeout = args
        .get("timeout")
        .and_then(|v| v.as_u64())
        .unwrap_or(DEFAULT_TIMEOUT_SECS)
        .clamp(1, MAX_TIMEOUT_SECS);
    Ok((language, code, timeout))
}

/// 列出支持的语言码（模型选型用）。
pub fn supported_languages() -> Vec<&'static str> {
    RUNTIMES.iter().map(|r| r.lang_code).collect()
}
