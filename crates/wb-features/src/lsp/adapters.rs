//! LSP 服务器目录：按扩展名路由到已安装的语言服务器；不下载 server。

use std::path::Path;

/// 语言服务器描述。
pub struct ServerSpec {
    /// 服务器名（模型可见，如 rust-analyzer）
    pub server_name: &'static str,
    /// 启动程序
    pub server_command: &'static str,
    /// 启动参数
    pub server_args: &'static [&'static str],
    /// 认领的扩展名（含点，小写）
    pub server_extensions: &'static [&'static str],
    /// 遍历时跳过的目录名
    pub server_skip_dirs: &'static [&'static str],
}

/// 内置服务器目录表：常用语言的常用服务器，按扩展名路由。
pub const SERVERS: &[ServerSpec] = &[
    ServerSpec {
        server_name: "rust-analyzer",
        server_command: "rust-analyzer",
        server_args: &[],
        server_extensions: &[".rs"],
        server_skip_dirs: &["target", ".git"],
    },
    ServerSpec {
        server_name: "gopls",
        server_command: "gopls",
        server_args: &[],
        server_extensions: &[".go"],
        server_skip_dirs: &["vendor", ".git"],
    },
    ServerSpec {
        server_name: "ruff",
        server_command: "ruff",
        server_args: &["server"],
        server_extensions: &[".py", ".pyi"],
        server_skip_dirs: &[".venv", "venv", "__pycache__"],
    },
    ServerSpec {
        server_name: "ty",
        server_command: "ty",
        server_args: &["server"],
        server_extensions: &[".py", ".pyi"],
        server_skip_dirs: &[".venv", "venv"],
    },
    ServerSpec {
        server_name: "biome",
        server_command: "biome",
        server_args: &["lsp-proxy"],
        server_extensions: &[
            ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts", ".json", ".jsonc",
            ".css", ".graphql", ".gql", ".html", ".vue", ".astro", ".svelte",
        ],
        server_skip_dirs: &["node_modules", "dist", ".git"],
    },
    ServerSpec {
        server_name: "typescript-language-server",
        server_command: "typescript-language-server",
        server_args: &["--stdio"],
        server_extensions: &[".ts", ".tsx", ".js", ".jsx"],
        server_skip_dirs: &["node_modules", "dist", ".git"],
    },
    ServerSpec {
        server_name: "clangd",
        server_command: "clangd",
        server_args: &["--background-index"],
        server_extensions: &[".c", ".cpp", ".cc", ".cxx", ".h", ".hpp", ".hh"],
        server_skip_dirs: &["build", ".git"],
    },
    ServerSpec {
        server_name: "jdtls",
        server_command: "jdtls",
        server_args: &[],
        server_extensions: &[".java"],
        server_skip_dirs: &[".gradle", "build"],
    },
    ServerSpec {
        server_name: "yaml-language-server",
        server_command: "yaml-language-server",
        server_args: &["--stdio"],
        server_extensions: &[".yaml", ".yml"],
        server_skip_dirs: &[".git"],
    },
    ServerSpec {
        server_name: "lua-language-server",
        server_command: "lua-language-server",
        server_args: &[],
        server_extensions: &[".lua"],
        server_skip_dirs: &[".git"],
    },
    ServerSpec {
        server_name: "intelephense",
        server_command: "intelephense",
        server_args: &["--stdio"],
        server_extensions: &[".php"],
        server_skip_dirs: &["vendor", ".git"],
    },
    ServerSpec {
        server_name: "zls",
        server_command: "zls",
        server_args: &[],
        server_extensions: &[".zig", ".zon"],
        server_skip_dirs: &[".zig-cache", "zig-out"],
    },
    ServerSpec {
        server_name: "bash-language-server",
        server_command: "bash-language-server",
        server_args: &["start"],
        server_extensions: &[".sh", ".bash"],
        server_skip_dirs: &[".git"],
    },
    ServerSpec {
        server_name: "terraform-ls",
        server_command: "terraform-ls",
        server_args: &["serve"],
        server_extensions: &[".tf", ".tfvars"],
        server_skip_dirs: &[".git"],
    },
    ServerSpec {
        server_name: "rubocop",
        server_command: "rubocop",
        server_args: &["--lsp"],
        server_extensions: &[".rb", ".rake", ".gemspec"],
        server_skip_dirs: &[".git"],
    },
];

/// 按扩展名找服务器；扩展名不在表内返回 None。
pub fn server_for_extension(ext: &str) -> Option<&'static ServerSpec> {
    let want = normalize_ext(ext);
    SERVERS
        .iter()
        .find(|s| s.server_extensions.contains(&want.as_str()))
}

/// 按名取服务器。
pub fn server_by_name(name: &str) -> Option<&'static ServerSpec> {
    SERVERS.iter().find(|s| s.server_name == name)
}

/// 扩展名归一：补前导点并转小写（表内统一带点）。
fn normalize_ext(ext: &str) -> String {
    let lower = ext.trim().to_ascii_lowercase();
    if lower.starts_with('.') {
        lower
    } else {
        format!(".{lower}")
    }
}

/// 文件是否被某服务器认领。
pub fn accepts(spec: &ServerSpec, path: &Path) -> bool {
    path.extension()
        .and_then(|e| e.to_str())
        .map(|e| spec.server_extensions.contains(&normalize_ext(e).as_str()))
        .unwrap_or(false)
}

/// 遍历工作区时跳过的目录名集合。
pub fn skip_dirs(spec: &ServerSpec) -> &[&str] {
    spec.server_skip_dirs
}

/// 语言 id（textDocument/didOpen 用）。
pub fn language_id(path: &Path) -> &'static str {
    match path
        .extension()
        .and_then(|e| e.to_str())
        .unwrap_or("")
        .to_ascii_lowercase()
        .as_str()
    {
        "rs" => "rust",
        "go" => "go",
        "py" | "pyi" => "python",
        "ts" => "typescript",
        "tsx" => "typescriptreact",
        "js" => "javascript",
        "jsx" => "javascriptreact",
        "c" | "h" => "c",
        "cpp" | "cc" | "cxx" | "hpp" | "hh" => "cpp",
        "java" => "java",
        "yaml" | "yml" => "yaml",
        "json" | "jsonc" => "json",
        "lua" => "lua",
        "php" => "php",
        "zig" | "zon" => "zig",
        "sh" | "bash" => "shellscript",
        "rb" => "ruby",
        "tf" | "tfvars" => "terraform",
        "vue" => "vue",
        "svelte" => "svelte",
        "astro" => "astro",
        "html" => "html",
        "css" => "css",
        "graphql" | "gql" => "graphql",
        _ => "plaintext",
    }
}

/// 文件路径转 file URI（Windows 盘符转 /C:/ 形式，特殊字符百分号编码）。
pub fn file_uri(path: &Path) -> String {
    let text = path.to_string_lossy().replace('\\', "/");
    let text = if text.starts_with('/') {
        text
    } else {
        format!("/{text}")
    };
    let mut out = String::from("file://");
    for ch in text.chars() {
        match ch {
            'A'..='Z' | 'a'..='z' | '0'..='9' | '-' | '.' | '_' | '~' | '/' | ':' => out.push(ch),
            ' ' => out.push_str("%20"),
            '#' => out.push_str("%23"),
            '?' => out.push_str("%3F"),
            _ => {
                let mut buf = [0u8; 4];
                for b in ch.encode_utf8(&mut buf).as_bytes() {
                    out.push_str(&format!("%{b:02X}"));
                }
            }
        }
    }
    out
}
