//! Tauri 壳库：系统命令（7 项）、托盘、端口握手事件。业务全在 wb-api。

use std::sync::Arc;

use tauri::{AppHandle, Emitter, Manager, WindowEvent};
use tokio_util::sync::CancellationToken;

/// 壳持有的全局态：启动结果（端口 + 服务容器）。
pub mod cmds;

pub struct ShellState {
    /// HTTP 启动结果
    pub startup: Arc<tokio::sync::OnceCell<Arc<wb_api::Startup>>>,
    /// 装配失败原因（app:startup-error 载荷）
    pub failure: Arc<tokio::sync::OnceCell<String>>,
    /// 真退出信号
    pub quit: CancellationToken,
}

impl ShellState {
    /// 空态（启动前）。
    pub fn new() -> Self {
        Self {
            startup: Arc::new(tokio::sync::OnceCell::new()),
            failure: Arc::new(tokio::sync::OnceCell::new()),
            quit: CancellationToken::new(),
        }
    }
}

impl Default for ShellState {
    /// 空态（new 的别名）。
    fn default() -> Self {
        Self::new()
    }
}

pub fn setup_tray(app: &AppHandle) -> tauri::Result<()> {
    use tauri::menu::{Menu, MenuItem};
    let show = MenuItem::with_id(app, "show", "显示 WorkBaby", true, None::<&str>)?;
    let new_session = MenuItem::with_id(app, "new-session", "新建对话", true, None::<&str>)?;
    let quit = MenuItem::with_id(app, "quit", "退出", true, None::<&str>)?;
    let menu = Menu::with_items(app, &[&show, &new_session, &quit])?;
    let mut tray = tauri::tray::TrayIconBuilder::with_id("main-tray")
        .menu(&menu)
        .show_menu_on_left_click(false)
        .tooltip("WorkBaby");
    if let Some(icon) = app.default_window_icon() {
        tray = tray.icon(icon.clone());
    }
    tray.build(app)?;
    Ok(())
}

/// 托盘菜单分发。
pub fn on_tray_event(app: &AppHandle, id: &str) {
    match id {
        "show" | "new-session" => {
            if let Some(w) = app.get_webview_window("main") {
                w.show().ok();
                w.set_focus().ok();
            }
            if id == "new-session" {
                app.emit("app:new-session", ()).ok();
            }
        }
        "quit" => {
            if let Some(st) = app.try_state::<ShellState>() {
                st.quit.cancel();
            }
            app.exit(0);
        }
        _ => {}
    }
}

/// 关窗策略：close_action=tray（默认）时隐藏窗口而不是关闭。
pub fn on_close_requested(app: &AppHandle) -> bool {
    let to_tray = app
        .try_state::<ShellState>()
        .and_then(|st| {
            st.startup.get().map(|s| {
                s.sv.repo
                    .settings_get("close_action")
                    .ok()
                    .flatten()
                    .as_deref()
                    != Some("quit")
            })
        })
        .unwrap_or(true);
    if to_tray {
        if let Some(w) = app.get_webview_window("main") {
            w.hide().ok();
            app.emit("app:to-tray", ()).ok();
        }
        true // 拦截关闭
    } else {
        false
    }
}

/// 日志初始化：分文件落盘（数据目录\logs）。
pub fn init_logs(data_dir: &std::path::Path) {
    let dir = data_dir.join("logs");
    std::fs::create_dir_all(&dir).ok();
    let file = tracing_appender::rolling::daily(&dir, "app.log");
    tracing_subscriber::fmt()
        .with_writer(file)
        .with_ansi(false)
        .with_max_level(tracing::Level::INFO)
        .init();
}

/// 应用装配与运行（官方模板形态：main.rs 只调这里，命令宏不跨 crate 展开）。
pub fn run() {
    let state = ShellState::new();
    let startup_cell = state.startup.clone();
    let failure_cell = state.failure.clone();
    tauri::Builder::default()
        .manage(state)
        .plugin(tauri_plugin_single_instance::init(|app, _argv, _cwd| {
            if let Some(w) = app.get_webview_window("main") {
                w.show().ok();
                w.set_focus().ok();
            }
        }))
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_autostart::init(
            tauri_plugin_autostart::MacosLauncher::LaunchAgent,
            None,
        ))
        .plugin(tauri_plugin_clipboard_manager::init())
        .plugin(tauri_plugin_opener::init())
        .invoke_handler(tauri::generate_handler![
            cmds::server_port,
            cmds::pick_file,
            cmds::pick_file_save,
            cmds::pick_folder,
            cmds::autostart_get,
            cmds::autostart_set,
            cmds::quit_app,
            cmds::window_ctl,
        ])
        .setup(|app| {
            let handle = app.handle().clone();
            let spawn_handle = handle.clone();
            init_logs(&wb_api::data_dir());
            tauri::async_runtime::spawn(async move {
                let handle = spawn_handle;
                let workspace = std::env::current_dir()
                    .unwrap_or_else(|_| std::path::PathBuf::from("."))
                    .display()
                    .to_string();
                match wb_api::startup(wb_api::data_dir(), workspace).await {
                    Ok(s) => {
                        let port = s.port;
                        let _ = startup_cell.set(Arc::new(s));
                        handle
                            .emit("app:ready", serde_json::json!({ "server_port": port }))
                            .ok();
                    }
                    Err(e) => {
                        let _ = failure_cell.set(e.message.clone());
                        handle
                            .emit(
                                "app:startup-error",
                                serde_json::json!({ "message": e.message }),
                            )
                            .ok();
                    }
                }
            });
            setup_tray(&handle)?;
            if let Some(w) = handle.get_webview_window("main") {
                let h = handle.clone();
                w.on_window_event(move |e| {
                    if let WindowEvent::CloseRequested { api, .. } = e {
                        if on_close_requested(&h) {
                            api.prevent_close();
                        }
                    }
                });
            }
            Ok(())
        })
        .on_tray_icon_event(|tray, event| {
            if matches!(
                event,
                tauri::tray::TrayIconEvent::Click {
                    button: tauri::tray::MouseButton::Left,
                    button_state: tauri::tray::MouseButtonState::Up,
                    ..
                }
            ) {
                let app = tray.app_handle();
                if let Some(w) = app.get_webview_window("main") {
                    w.show().ok();
                    w.set_focus().ok();
                }
            }
        })
        .on_menu_event(|app, ev| on_tray_event(app, ev.id().as_ref()))
        .run(tauri::generate_context!())
        .expect("WorkBaby 启动失败");
}
