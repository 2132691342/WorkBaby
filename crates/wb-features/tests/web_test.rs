//! wb-features 联网与 LSP 场景测试：正文提取 / 搜索源解析 / 视频字幕 / 语言服务路由。

// ---- 联网：正文提取、搜索解析、视频字幕 ----

/// 网页解析三件套：正文提取剥结构噪音；多家搜索源转统一结果（跳转解开、标签剥掉）；字幕轨按语言偏好选轨。
#[test]
fn web_parsing_extracts_text_results_and_captions() {
    let html = r#"<html><head><title>文档标题</title><style>.a{color:red}</style></head>
<body><nav>菜单导航</nav><h1>正文标题</h1><p>第一段 &amp; 说明</p>
<script>var x = "&lt;script&gt;";</script><footer>版权信息</footer></body></html>"#;
    let text = wb_features::web_parse::readable_text(html);
    assert!(text.contains("正文标题") && text.contains("第一段 & 说明"));
    assert!(!text.contains("var x") && !text.contains("菜单导航") && !text.contains("<p>"));

    let ddg = r##"<div class="result results_links">
<a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fwww.rust-lang.org%2F&amp;rut=abc">The Rust Programming Language</a>
<a class="result__snippet" href="#">A language empowering everyone &amp; building reliable software.</a>
</div>"##;
    let dd = wb_features::web_parse::parse_results(ddg);
    assert_eq!(
        dd[0].result_url, "https://www.rust-lang.org/",
        "跳转包装应解开"
    );
    assert_eq!(
        dd[0].result_snippet,
        "A language empowering everyone & building reliable software."
    );

    let bing = r#"<li class="b_algo"><h2><a href="https://example.com/doc">Example Doc</a></h2><p>Some snippet here.</p></li>"#;
    let b = wb_features::web_providers::parse_bing(bing);
    assert_eq!(b[0].result_title, "Example Doc");
    assert_eq!(b[0].result_url, "https://example.com/doc");

    let wiki = r#"{"query":{"search":[{"title":"Rust","snippet":"A <b>systems</b> language."}]}}"#;
    let w = wb_features::web_providers::parse_wikipedia(wiki);
    assert_eq!(w[0].result_url, "https://en.wikipedia.org/wiki/Rust");
    assert!(!w[0].result_snippet.contains("<b>"), "摘要应剥标签");

    // 四家的 API 响应形状各异，各家解析器要认得自家字段名。
    let brave = r#"{"web":{"results":[{"title":"Brave A","url":"https://a.example","description":"desc a"}]}}"#;
    let br = wb_features::web_providers::parse_brave(brave);
    assert_eq!(br.len(), 1);
    assert_eq!(br[0].result_snippet, "desc a", "Brave 摘要取 description");
    let tavily = r#"{"results":[{"title":"Tav B","url":"https://b.example","content":"body b"}]}"#;
    assert_eq!(
        wb_features::web_providers::parse_tavily(tavily)[0].result_snippet,
        "body b"
    );
    let serper = r#"{"organic":[{"title":"Ser C","link":"https://c.example","snippet":"snip c"}]}"#;
    assert_eq!(
        wb_features::web_providers::parse_serper(serper)[0].result_url,
        "https://c.example",
        "Serper 的链接字段是 link"
    );
    let exa = r#"{"results":[{"title":"Exa D","url":"https://d.example","text":"text d"}]}"#;
    assert_eq!(
        wb_features::web_providers::parse_exa(exa)[0].result_title,
        "Exa D"
    );
    assert!(
        wb_features::web_providers::parse_brave(r#"{"web":{"results":[{"title":"x"}]}}"#)
            .is_empty(),
        "缺 url 不应当成一条有效结果"
    );

    // 排队：未配密钥的 API 源不进队，免密钥公开源始终兜底。
    let q = wb_features::web_providers::queue_for("");
    let codes: Vec<&str> = q.iter().map(|(p, _)| p.provider_code).collect();
    for c in &codes {
        let p = wb_features::web_providers::by_code(c).unwrap();
        assert!(
            p.provider_env_keys.is_empty() || wb_features::web_providers::key_for(p).is_some(),
            "未配密钥的 API 源不该进队：{c}"
        );
    }
    for open in ["duckduckgo", "bing", "wikipedia"] {
        assert!(codes.contains(&open), "公开源应一直在队：{open}");
    }
    assert_eq!(
        wb_features::web_providers::by_code("brave")
            .unwrap()
            .provider_env_keys,
        ["BRAVE_API_KEY"],
        "实现里的变量名要和工具描述一致"
    );
    assert!(
        wb_features::web_providers::queue_for("nope").is_empty(),
        "未知源码 → 空队"
    );

    // 界面里填的密钥优先于环境变量：用户不必先学会改环境变量才能联网。
    wb_features::config::set_web_keys(wb_features::web_providers::parse_keys(
        "brave=cfg-key\n\n tavily = t-key \nbad-line",
    ));
    let brave = wb_features::web_providers::by_code("brave").unwrap();
    assert_eq!(
        wb_features::web_providers::key_for(brave).as_deref(),
        Some("cfg-key"),
        "配置里的密钥应优先"
    );
    assert!(
        wb_features::web_providers::queue_for("")
            .iter()
            .any(|(p, k)| p.provider_code == "brave" && k.is_some()),
        "配了密钥的 API 源应进队"
    );
    wb_features::config::set_web_keys(Vec::new());
    assert_eq!(
        wb_features::web_providers::codes().len(),
        wb_features::web_providers::PROVIDERS.len(),
        "工具参数的 enum 必须跟实际源表同步"
    );

    use wb_features::web_video::{caption_tracks, parse_caption, pick_track};
    let yt = r#"{"playerResponse":{"captions":{"playerCaptionsTracklistRenderer":{"captionTracks":[
      {"baseUrl":"https://yt.example/api/timedtext?v=1&lang=en","name":{"simpleText":"English"}},
      {"baseUrl":"https://yt.example/api/timedtext?v=1&lang=zh","name":{"simpleText":"Chinese"}}
    ]}}}}"#;
    let tracks = caption_tracks(yt);
    assert_eq!(tracks.len(), 2, "两条字幕轨都该被认出");
    assert_eq!(pick_track(&tracks, "zh").unwrap().label, "Chinese");
    let xml = r#"<transcript><text start="0.0" dur="1.2">Hello &amp; welcome</text>
<text start="1.2" dur="2.0">Second line</text></transcript>"#;
    assert_eq!(parse_caption(xml), "Hello & welcome\nSecond line");
}

// ---- 语言服务：扩展名路由、产物目录跳过、文本编辑 ----

/// 扩展名决定语言服务器、诊断遍历跳过产物目录、文本编辑按区间替换不错位。
#[test]
fn lsp_routes_dirs_and_edits() {
    use wb_features::lsp::adapters::{server_by_name, server_for_extension};
    assert_eq!(
        server_for_extension(".rs").unwrap().server_name,
        "rust-analyzer"
    );
    assert_eq!(server_for_extension("TS").unwrap().server_name, "biome");
    assert!(server_for_extension(".unknownext").is_none());

    let root = std::env::temp_dir().join(format!("wb_lsp_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&root);
    std::fs::create_dir_all(root.join("src")).unwrap();
    std::fs::create_dir_all(root.join("target")).unwrap();
    std::fs::write(root.join("src/a.rs"), "fn a() {}").unwrap();
    std::fs::write(root.join("target/b.rs"), "fn b() {}").unwrap();
    let files =
        wb_features::lsp::collect_files(server_by_name("rust-analyzer").unwrap(), &root, 10);
    assert_eq!(files.len(), 1);
    assert!(files[0].ends_with("a.rs"));

    let edit = |l0: u32, c0: u32, l1: u32, c1: u32, text: &str| wb_features::lsp::edits::TextEdit {
        edit_start_line: l0,
        edit_start_char: c0,
        edit_end_line: l1,
        edit_end_char: c1,
        edit_new_text: text.into(),
    };
    let edits = vec![edit(0, 0, 0, 5, "L1"), edit(2, 0, 2, 5, "L3")];
    assert_eq!(
        wb_features::lsp::edits::apply_edits("line1\nline2\nline3", &edits),
        "L1\nline2\nL3"
    );
    let _ = std::fs::remove_dir_all(&root);
}
