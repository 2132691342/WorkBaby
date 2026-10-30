//! 系统能力命令：与 generate_handler 分模块，避开宏命名冲突。

use tauri::{AppHandle, Manager, State};
use tauri_plugin_autostart::ManagerExt;

use crate::ShellState;

/// server_port：前端 bootstrap 的第一条 invoke。
#[tauri::command]
pub async fn server_port(state: State<'_, ShellState>) -> Result<u16, String> {
    match state.startup.get() {
        Some(s) => Ok(s.port),
        None => Err("后端还在启动中".into()),
    }
}

/// 文件选择（取消 = null）。
#[tauri::command]
pub async fn pick_file(title: String, pattern: String) -> Result<Option<String>, String> {
    match tauri::async_runtime::spawn_blocking(move || {
        let mut dlg = rfd::FileDialog::new().set_title(&title);
        let filters: Vec<&str> = pattern.split(';').filter(|s| !s.is_empty()).collect();
        if !filters.is_empty() {
            dlg = dlg.add_filter("文件", &filters);
        }
        Ok::<_, std::convert::Infallible>(dlg.pick_file().map(|p| p.display().to_string()))
    })
    .await
    {
        Ok(Ok(v)) => Ok(v),
        Ok(Err(i)) => match i {},
        Err(e) => Err(format!("{e}")),
    }
}

/// 另存为。
#[tauri::command]
pub async fn pick_file_save(
    title: String,
    default_name: String,
    pattern: String,
) -> Result<Option<String>, String> {
    match tauri::async_runtime::spawn_blocking(move || {
        let mut dlg = rfd::FileDialog::new()
            .set_title(&title)
            .set_file_name(&default_name);
        let filters: Vec<&str> = pattern.split(';').filter(|s| !s.is_empty()).collect();
        if !filters.is_empty() {
            dlg = dlg.add_filter("文件", &filters);
        }
        Ok::<_, std::convert::Infallible>(dlg.save_file().map(|p| p.display().to_string()))
    })
    .await
    {
        Ok(Ok(v)) => Ok(v),
        Ok(Err(i)) => match i {},
        Err(e) => Err(format!("{e}")),
    }
}

/// 目录选择。
#[tauri::command]
pub async fn pick_folder(title: String) -> Result<Option<String>, String> {
    match tauri::async_runtime::spawn_blocking(move || {
        Ok::<_, std::convert::Infallible>(
            rfd::FileDialog::new()
                .set_title(&title)
                .pick_folder()
                .map(|p| p.display().to_string()),
        )
    })
    .await
    {
        Ok(Ok(v)) => Ok(v),
        Ok(Err(i)) => match i {},
        Err(e) => Err(format!("{e}")),
    }
}

/// 开机自启状态。
#[tauri::command]
pub fn autostart_get(app: AppHandle) -> bool {
    app.autolaunch().inner().is_enabled().unwrap_or(false)
}

/// 开关开机自启。
#[tauri::command]
pub fn autostart_set(app: AppHandle, on: bool) -> Result<(), String> {
    let al = app.autolaunch().inner();
    if on {
        al.enable().map_err(|e| format!("{e}"))
    } else {
        al.disable().map_err(|e| format!("{e}"))
    }
}

/// 真退出：绕过关窗到托盘。
#[tauri::command]
pub fn quit_app(app: AppHandle, state: State<'_, ShellState>) {
    state.quit.cancel();
    app.exit(0);
}

/// 窗控三键；close 由关窗策略决定（收托盘或退出）。
#[tauri::command]
pub fn window_ctl(app: AppHandle, action: String) {
    let Some(w) = app.get_webview_window("main") else {
        return;
    };
    match action.as_str() {
        "min" => w.minimize().ok(),
        "max" => {
            if w.is_maximized().unwrap_or(false) {
                w.unmaximize().ok()
            } else {
                w.maximize().ok()
            }
        }
        _ => w.close().ok(),
    };
}
