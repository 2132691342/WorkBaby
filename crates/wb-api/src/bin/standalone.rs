//! 独立 HTTP 服务：浏览器调试前端用（无需桌面壳）。

#[tokio::main]
async fn main() {
    let workspace = std::env::args().nth(1).unwrap_or_else(|| {
        std::env::current_dir()
            .unwrap_or_default()
            .display()
            .to_string()
    });
    let dir = wb_api::data_dir();
    match wb_api::startup(dir, workspace).await {
        Ok(s) => {
            println!(
                "wb-api listening on 127.0.0.1:{}（localStorage.wb_dev_port 设为此端口）",
                s.port
            );
            tokio::signal::ctrl_c().await.ok();
        }
        Err(e) => {
            eprintln!("启动失败：{e}");
            std::process::exit(1);
        }
    }
}
