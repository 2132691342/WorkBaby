//! 内置运行时：PortableGit 与 Python 的 zip 编译期嵌入，后台解压到数据目录并对外报进度。

use std::io::{Cursor, Read as _};
use std::path::Path;
use std::sync::{Arc, Mutex};

use wb_pkg::{fsutil, AppError, Result};

/// PortableGit 归档（bash / git / GNU 工具，随安装包分发）。
pub static PORTABLEGIT_ZIP: &[u8] = include_bytes!(concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/../../assets/runtime/PortableGit.zip"
));

/// Python 3.13 归档（agent 跑数据脚本与文档转换的解释器）。
pub static PYTHON_ZIP: &[u8] = include_bytes!(concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/../../assets/runtime/Python313.zip"
));

/// 解压件数：PortableGit + Python。
const TOTAL_PARTS: i64 = 2;

/// 运行时装配进度（后台线程写、HTTP 读）。
#[derive(Debug, Default, Clone)]
pub struct RuntimeProgress {
    /// 已完成件数
    pub done_count: i64,
    /// 全部完成
    pub finished: bool,
    /// 当前正在解压的件名
    pub current: String,
    /// 失败原因（每件一条）
    pub failures: Vec<String>,
}

/// 运行时状态句柄：进程内单例，随 Services 一起分发。
pub type RuntimeState = Arc<Mutex<RuntimeProgress>>;

/// 空态句柄（后台解压尚未启动或测试用）。
pub fn empty_state() -> RuntimeState {
    Arc::new(Mutex::new(RuntimeProgress::default()))
}

/// 全部内置运行时就位（同步）；单件失败只记录不返回错误，失败原因汇总返回。
pub fn ensure_all(data_dir: &Path) -> Vec<String> {
    let state = empty_state();
    run_parts(data_dir, &state, |_name| {});
    let st = state.lock().unwrap();
    st.failures.clone()
}

/// 后台解压：启动立即返回，线程按件推进进度；每完成一件回调一次通知。
pub fn start_background(
    data_dir: std::path::PathBuf,
    state: RuntimeState,
    on_progress: Arc<dyn Fn(&RuntimeProgress) + Send + Sync>,
) {
    let handle = state.clone();
    let spawned = std::thread::Builder::new()
        .name("wb-runtime".into())
        .spawn(move || {
            run_parts(&data_dir, &handle, |_name| {
                let snapshot = handle.lock().unwrap().clone();
                on_progress(&snapshot);
            });
            let snapshot = handle.lock().unwrap().clone();
            on_progress(&snapshot);
        });
    if let Err(e) = spawned {
        let mut st = state.lock().unwrap();
        st.finished = true;
        st.failures.push(format!("运行时解压线程启动失败：{e}"));
    }
}

/// 按件解压：PortableGit → Python，逐件推进进度。
fn run_parts(data_dir: &Path, state: &RuntimeState, mut on_part: impl FnMut(&str)) {
    let parts: [(&str, &str, std::path::PathBuf, &'static [u8]); 2] = [
        (
            "PortableGit",
            "PortableGit",
            wb_domain::runtime::portablegit_root(data_dir),
            PORTABLEGIT_ZIP,
        ),
        (
            "Python",
            "python",
            wb_domain::runtime::python_root(data_dir),
            PYTHON_ZIP,
        ),
    ];
    for (label, entry_root, target, zip_bytes) in parts {
        {
            let mut st = state.lock().unwrap();
            st.current = label.to_string();
        }
        on_part(label);
        let outcome = ensure_archive(&target, zip_bytes, entry_root, label);
        let mut st = state.lock().unwrap();
        st.current.clear();
        st.done_count += 1;
        if let Err(e) = outcome {
            st.failures.push(format!("{label}：{e}"));
        }
    }
    let mut st = state.lock().unwrap();
    st.finished = true;
    st.current.clear();
}

/// 单个运行时解压：指纹（zip 字节数）不符即清空重解压；归档内顶层目录为 `entry_root`。
fn ensure_archive(
    target: &Path,
    zip_bytes: &'static [u8],
    entry_root: &str,
    label: &str,
) -> Result<()> {
    let stamp = zip_bytes.len().to_string();
    let marker = target.join(".pack_stamp");
    if target.is_dir() && fsutil::read_to_string(&marker).is_ok_and(|s| s == stamp) {
        return Ok(());
    }
    // 指纹不符先清空：残留与半成品目录一并清除
    let _ = std::fs::remove_dir_all(target);
    extract(target, zip_bytes, entry_root)?;
    if let Err(e) = fsutil::write_atomic(&marker, stamp.as_bytes()) {
        tracing::warn!("{label} 指纹写入失败：{e}");
    }
    Ok(())
}

/// 当前运行时状态：以磁盘上的可执行文件为准，不依赖解压进度。
pub fn status(
    data_dir: &Path,
    state: &RuntimeState,
) -> wb_domain::runtime_status::RuntimeStatusRESP {
    use wb_domain::runtime_status::{RuntimeBinaryVO, RuntimeStatusRESP};
    let progress = state.lock().unwrap().clone();
    let probe = [
        ("bash", wb_domain::runtime::find_bundled_bash(data_dir)),
        ("git", wb_domain::runtime::find_bundled_git(data_dir)),
        ("python", wb_domain::runtime::find_bundled_python(data_dir)),
    ];
    // powershell 走的是「用户装的 pwsh 7 → 系统 5.1」级联，不参与解压进度；
    // 它是「环境是否可用」的读数，与内置件是否就位分开报
    let powershell = wb_domain::runtime::find_powershell(data_dir);
    let binaries: Vec<RuntimeBinaryVO> = probe
        .into_iter()
        .map(|(binary_name, hit)| RuntimeBinaryVO {
            binary_name: binary_name.to_string(),
            binary_ready: hit.is_some(),
            binary_path: hit.map(|p| p.display().to_string()).unwrap_or_default(),
        })
        .chain(std::iter::once(RuntimeBinaryVO {
            binary_name: "powershell".to_string(),
            binary_ready: powershell.is_some(),
            binary_path: powershell
                .map(|p| p.display().to_string())
                .unwrap_or_default(),
        }))
        .collect();
    // 「就绪」只看内置三件：powershell 的级联末端是系统 5.1，它不决定内置运行时是否可用
    let all_ready = binaries
        .iter()
        .filter(|b| b.binary_name != "powershell")
        .all(|b| b.binary_ready);
    // 三件都以磁盘为准：解压线程还在跑但文件已就位时，界面不必继续显示「准备中」
    let phase = if all_ready {
        "ready"
    } else if progress.finished {
        "failed"
    } else {
        "preparing"
    };
    RuntimeStatusRESP {
        resp_root: wb_domain::runtime::runtime_root(data_dir)
            .display()
            .to_string(),
        resp_ready: all_ready,
        resp_binaries: binaries,
        resp_failure: progress.failures.join("; "),
        resp_phase: phase.to_string(),
        resp_done_count: progress.done_count,
        resp_total_count: TOTAL_PARTS,
        resp_current: progress.current.clone(),
    }
}

/// 全量解压到 target；符号链接跳过（Windows 无需，且防路径逃逸）。
fn extract(target: &Path, zip_bytes: &'static [u8], entry_root: &str) -> Result<()> {
    fsutil::ensure_dir(target)?;
    let mut archive = zip::ZipArchive::new(Cursor::new(zip_bytes)).map_err(|e| {
        AppError::new(
            wb_domain::error::common::ARCHIVE_INVALID,
            format!("{entry_root} zip 解析失败：{e}"),
        )
    })?;
    for i in 0..archive.len() {
        let mut entry = archive.by_index(i).map_err(|e| {
            AppError::new(
                wb_domain::error::common::ARCHIVE_INVALID,
                format!("{entry_root} zip 条目读取失败：{e}"),
            )
        })?;
        let Some(rel) = entry.enclosed_name() else {
            continue;
        };
        // 归档顶层目录名已被 target 取代，剥掉一层才能让根路径契约（runtime/PortableGit 等）成立。
        let Some(stripped) = rel.strip_prefix(entry_root).ok() else {
            continue;
        };
        let rel = stripped.to_path_buf();
        if rel.as_os_str().is_empty() {
            continue;
        }
        let dest = target.join(&rel);
        if entry.is_dir() {
            fsutil::ensure_dir(&dest)?;
            continue;
        }
        if entry.is_symlink() {
            continue;
        }
        let Some(parent) = dest.parent() else {
            continue;
        };
        fsutil::ensure_dir(parent)?;
        let mut bytes = Vec::with_capacity(entry.size() as usize);
        entry.read_to_end(&mut bytes).map_err(|e| {
            AppError::wrap(
                wb_domain::error::common::IO_FAILED,
                format!("{entry_root} 条目读出失败"),
                e,
            )
        })?;
        fsutil::write_atomic(&dest, &bytes)?;
    }
    Ok(())
}
