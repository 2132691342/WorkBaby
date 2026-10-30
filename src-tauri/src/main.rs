// Tauri 壳入口：装配全在 workbaby_lib::run。
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    workbaby_lib::run();
}
