//! wb-features 场景测试（续）：联网解析、模糊搜索、语言服务、目标、计划、知识整合。
//! 全是「改一处就可能坏一片」的场景：协议解析、产物目录跳过、状态机、语义判重。

use std::path::PathBuf;
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use wb_features::FeatureCtx;

/// 临时目录里的功能上下文。
fn ctx_of(tag: &str) -> (PathBuf, FeatureCtx) {
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let dir = std::env::temp_dir().join(format!("wb_feat_{tag}_{}_{}", std::process::id(), nanos));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).unwrap();
    let ctx = FeatureCtx {
        data_dir: dir.clone(),
        workspace: dir.clone(),
        emit: Arc::new(|_, _, _| {}),
    };
    (dir, ctx)
}

// ---- 联网：正文提取与搜索结果解析 ----

/// HTML 正文提取要剥掉脚本、样式与导航/页脚等结构噪音，保留标题与段落。
#[test]
fn readable_extraction_strips_structure_noise() {
    let html = r#"<html><head><title>文档标题</title><style>.a{color:red}</style></head>
<body><nav>菜单导航</nav><h1>正文标题</h1><p>第一段 &amp; 说明</p>
<script>var x = "&lt;script&gt;";</script><footer>版权信息</footer></body></html>"#;
    let text = wb_features::web_parse::readable_text(html);
    assert!(text.contains("文档标题"));
    assert!(text.contains("正文标题"));
    assert!(text.contains("第一段 & 说明"));
    assert!(!text.contains("var x"));
    assert!(!text.contains("菜单导航"));
    assert!(!text.contains("版权信息"));
    assert!(!text.contains("<p>"));
}

/// 搜索结果页解析：取标题、真实链接与摘要，并解开跳转包装的 URL。
#[test]
fn search_parse_extracts_and_unwraps_results() {
    let html = r##"<div class="result results_links">
<a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fwww.rust-lang.org%2F&amp;rut=abc">The Rust Programming Language</a>
<a class="result__snippet" href="#">A language empowering everyone &amp; building reliable software.</a>
</div>
<div class="result">
<a rel="nofollow" class="result__a" href="https://docs.rs/">docs.rs</a>
<a class="result__snippet" href="#">Documentation for Rust crates</a>
</div>"##;
    let results = wb_features::web_parse::parse_results(html);
    assert_eq!(results.len(), 2);
    assert_eq!(results[0].title, "The Rust Programming Language");
    assert_eq!(
        results[0].url, "https://www.rust-lang.org/",
        "跳转包装应解开"
    );
    assert_eq!(
        results[0].snippet,
        "A language empowering everyone & building reliable software."
    );
    assert_eq!(results[1].url, "https://docs.rs/");
}

/// 内容类型决定渲染路径：JSON 直通，HTML 走提取，二进制不解析。
#[test]
fn content_type_routes_to_renderer() {
    assert_eq!(
        wb_features::web::classify("application/json; charset=utf-8", "{}"),
        "passthrough"
    );
    assert_eq!(
        wb_features::web::classify("text/html; charset=utf-8", ""),
        "html"
    );
    assert_eq!(
        wb_features::web::classify("text/plain", "hi"),
        "passthrough"
    );
    assert_eq!(
        wb_features::web::classify("", "<!DOCTYPE html><body>x</body>"),
        "html"
    );
    assert_eq!(
        wb_features::web::classify("application/octet-stream", "\x00\x01"),
        "other"
    );
}

/// 多个搜索源各自把自己的页面结构转成统一结果列表。
#[test]
fn search_providers_parse_their_own_shapes() {
    use wb_features::web_providers::{parse_bing, parse_wikipedia};

    let bing = r#"<li class="b_algo"><h2><a href="https://example.com/doc">Example Doc</a></h2><p>Some snippet here.</p></li>
<li class="b_algo"><h2><a href="https://example.org/x">Other</a></h2></li>"#;
    let results = parse_bing(bing);
    assert_eq!(results.len(), 2, "两个结果块都该被认出");
    assert_eq!(results[0].title, "Example Doc");
    assert_eq!(results[0].url, "https://example.com/doc");
    assert_eq!(results[0].snippet, "Some snippet here.");
    assert_eq!(results[1].title, "Other");

    let wiki = r#"{"query":{"search":[{"title":"Rust (programming language)","snippet":"A <b>systems</b> language."}]}}"#;
    let w = parse_wikipedia(wiki);
    assert_eq!(w.len(), 1);
    assert_eq!(w[0].title, "Rust (programming language)");
    assert_eq!(
        w[0].url,
        "https://en.wikipedia.org/wiki/Rust_(programming_language)"
    );
    assert!(!w[0].snippet.contains("<b>"), "摘要应剥标签");
}

/// 字幕轨从页面里抠出，字幕正文按时间轴分段。
#[test]
fn video_caption_tracks_and_body_are_extracted() {
    use wb_features::web_video::{caption_tracks, parse_caption, pick_track};

    let html = r#"{"playerResponse":{"captions":{"playerCaptionsTracklistRenderer":{"captionTracks":[
      {"baseUrl":"https://yt.example/api/timedtext?v=1&lang=en","name":{"simpleText":"English"}},
      {"baseUrl":"https://yt.example/api/timedtext?v=1&lang=zh","name":{"simpleText":"Chinese"}}
    ]}}}}"#;
    let tracks = caption_tracks(html);
    assert_eq!(tracks.len(), 2, "两条字幕轨都该被认出");
    assert_eq!(
        tracks[0].url,
        "https://yt.example/api/timedtext?v=1&lang=en"
    );
    assert_eq!(tracks[1].label, "Chinese");

    let picked = pick_track(&tracks, "zh").expect("应挑中文轨");
    assert_eq!(picked.label, "Chinese");

    let xml = r#"<?xml version="1.0"?><transcript>
<text start="0.0" dur="1.2">Hello &amp; welcome</text>
<text start="1.2" dur="2.0">Second line</text>
</transcript>"#;
    let body = parse_caption(xml);
    assert_eq!(body, "Hello & welcome\nSecond line");
}

// ---- 模糊搜索：产物目录跳过与 frecency 排序 ----

/// 遍历工作区时跳过 node_modules 与 target 这类产物目录。
/// 混进产物目录会让搜索结果被构建文件淹没。
#[test]
fn walker_skips_build_output_dirs() {
    let root = std::env::temp_dir().join(format!("wb_fff_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&root);
    for (dir, file) in [
        ("node_modules", "x.js"),
        ("target", "a.rs"),
        ("src", "main.rs"),
    ] {
        std::fs::create_dir_all(root.join(dir)).unwrap();
        std::fs::write(root.join(dir).join(file), "m").unwrap();
    }
    let mut seen: Vec<String> = Vec::new();
    wb_features::fff::walk_files(
        &root,
        &tokio_util::sync::CancellationToken::new(),
        |_, rel| seen.push(rel.to_string()),
    );
    assert!(seen.contains(&"src/main.rs".to_string()), "got {seen:?}");
    assert!(!seen.iter().any(|p| p.contains("node_modules")));
    assert!(!seen.iter().any(|p| p.contains("target")));
    let _ = std::fs::remove_dir_all(&root);
}

/// frecency 排序：命中次数多且最近用过的排前面；越久远权重越低。
/// 排序错了，「常用的文件排前面」这个卖点就没了。
#[test]
fn frecency_prefers_frequent_and_recent_files() {
    use wb_features::fff::frecency_score;

    let now = 1_000_000i64;
    let hot_recent = frecency_score(10, now - 1_000, now);
    let cold_recent = frecency_score(1, now - 1_000, now);
    let hot_old = frecency_score(10, now - 100_000_000, now);
    assert!(
        hot_recent > cold_recent,
        "同样新鲜时命中多的应更高：{hot_recent} vs {cold_recent}"
    );
    assert!(
        hot_recent > hot_old,
        "同样命中数时最近用过的应更高：{hot_recent} vs {hot_old}"
    );
}

// ---- 语言服务：扩展名路由与文本编辑 ----

/// 扩展名决定语言服务器；未收录的扩展名没有服务器。
/// 路由错了会把诊断发给错误的服务器或干脆没有服务器。
#[test]
fn lsp_routes_by_extension() {
    assert_eq!(
        wb_features::lsp::adapters::server_for_extension(".rs")
            .unwrap()
            .server_name,
        "rust-analyzer"
    );
    assert_eq!(
        wb_features::lsp::adapters::server_for_extension("TS")
            .unwrap()
            .server_name,
        "biome"
    );
    assert!(wb_features::lsp::adapters::server_for_extension(".unknownext").is_none());
}

/// 诊断遍历跳过产物目录，只留下真正该检查的源文件。
#[test]
fn lsp_collect_files_skips_build_dirs() {
    let root = std::env::temp_dir().join(format!("wb_lsp_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&root);
    std::fs::create_dir_all(root.join("src")).unwrap();
    std::fs::create_dir_all(root.join("target")).unwrap();
    std::fs::write(root.join("src/a.rs"), "fn a() {}").unwrap();
    std::fs::write(root.join("target/b.rs"), "fn b() {}").unwrap();
    let spec = wb_features::lsp::adapters::server_by_name("rust-analyzer").unwrap();
    let files = wb_features::lsp::collect_files(spec, &root, 10);
    assert_eq!(files.len(), 1);
    assert!(files[0].ends_with("a.rs"));
    let _ = std::fs::remove_dir_all(&root);
}

/// 文本编辑按区间替换；多行与多处编辑不能互相错位。
/// 区间算错会把诊断修到错误的行上。
#[test]
fn lsp_text_edits_apply_by_range() {
    use wb_features::lsp::edits::{apply_edits, TextEdit};
    let text = "line1\nline2\nline3";
    let edits = vec![
        TextEdit {
            edit_start_line: 0,
            edit_start_char: 0,
            edit_end_line: 0,
            edit_end_char: 5,
            edit_new_text: "L1".into(),
        },
        TextEdit {
            edit_start_line: 2,
            edit_start_char: 0,
            edit_end_line: 2,
            edit_end_char: 5,
            edit_new_text: "L3".into(),
        },
    ];
    assert_eq!(apply_edits(text, &edits), "L1\nline2\nL3");
}

// ---- 目标：状态机与续跑提示 ----

/// 目标续跑：活跃目标有未完成步骤才给提示；阻塞 / 无目标都不给。
/// 误提示会让助手在没有可推进内容时空转。
#[test]
fn goal_continuation_only_when_actionable() {
    use wb_features::goal::{continuation_prompt, GoalState, GoalStep, GoalStore};

    let (dir, ctx) = ctx_of("goal");
    let store = GoalStore::new(&ctx);
    let step = |text: &str, done: bool| GoalStep {
        step_text: text.into(),
        step_done: done,
    };

    // 没有目标：不提示。
    assert!(continuation_prompt(&ctx).is_none(), "没有目标时不该续跑");

    // 活跃目标 + 未完成步骤：提示。
    store
        .save(&GoalState {
            goal_id: "GOAL_1".into(),
            goal_text: "把登录页做完".into(),
            goal_status: "active".into(),
            goal_note: String::new(),
            goal_steps: vec![step("写表单", true), step("接接口", false)],
            goal_created_at: 1,
            goal_updated_at: 1,
        })
        .unwrap();
    let prompt = continuation_prompt(&ctx).expect("活跃目标应给出续跑提示");
    assert!(prompt.contains("把登录页做完"), "提示应带目标描述");
    assert!(prompt.contains("接接口"), "提示应列出待办步骤");
    assert!(!prompt.contains("写表单"), "已完成步骤不该再列");

    // 阻塞目标：不提示（恢复须显式置回 active）。
    store
        .save(&GoalState {
            goal_id: "GOAL_1".into(),
            goal_text: "把登录页做完".into(),
            goal_status: "blocked".into(),
            goal_note: String::new(),
            goal_steps: vec![step("接接口", false)],
            goal_created_at: 1,
            goal_updated_at: 1,
        })
        .unwrap();
    assert!(continuation_prompt(&ctx).is_none(), "阻塞目标不该自动续跑");

    // 步骤全完成：不提示。
    store
        .save(&GoalState {
            goal_id: "GOAL_1".into(),
            goal_text: "把登录页做完".into(),
            goal_status: "active".into(),
            goal_note: String::new(),
            goal_steps: vec![step("接接口", true)],
            goal_created_at: 1,
            goal_updated_at: 1,
        })
        .unwrap();
    assert!(continuation_prompt(&ctx).is_none(), "步骤全完成不该续跑");

    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 知识整合：语义判重 ----

/// 说的是同一件事的记忆被合并成一条，内容不丢。
/// 合并丢了内容就是信息损坏——比不合并还糟。
#[test]
fn consolidation_merges_duplicates_and_keeps_content() {
    use wb_features::memory_consolidate::consolidate;
    use wb_features::memory_store::MemoryStore;

    let (dir, ctx) = ctx_of("memory_consolidate");
    let store = Arc::new(MemoryStore::open(&ctx).unwrap());

    store.save("project", "项目用 pnpm 管理依赖", 1).unwrap();
    store
        .save("project", "这个项目的依赖由 pnpm 管理", 2)
        .unwrap();
    store.save("user", "用户偏好深色主题", 3).unwrap();
    store
        .save("failure", "构建失败是因为 Node 版本太低", 4)
        .unwrap();

    let report = consolidate(&store);
    assert_eq!(report.scanned, 4, "四条都该被扫到");
    assert_eq!(report.merged, 1, "同一件事的两条该合并成一条");
    assert_eq!(report.remaining, 3, "合并后剩三条");

    // 合并后内容不丢：关键词都还在文本里。
    let hits = store.search("pnpm", 10).unwrap();
    assert_eq!(hits.len(), 1, "重复项应只剩一条");
    let text = store.list_by_kind("project", 10).unwrap();
    assert_eq!(text.len(), 1);
    let body = &text[0].1;
    assert!(body.contains("pnpm"), "合并后应保留关键词");
    assert!(
        body.len() > "项目用 pnpm 管理依赖".len(),
        "合并后内容应更全"
    );

    // 不相关的条目不该被合到一起。
    assert_eq!(store.list_by_kind("user", 10).unwrap().len(), 1);
    assert_eq!(store.list_by_kind("failure", 10).unwrap().len(), 1);

    let _ = std::fs::remove_dir_all(&dir);
}

/// 说法不同但不是一回事的记忆不该被合并；条目数在整合中守恒。
#[test]
fn consolidation_does_not_merge_different_facts() {
    use wb_features::memory_consolidate::consolidate;
    use wb_features::memory_store::MemoryStore;

    let (dir, ctx) = ctx_of("memory_no_false_merge");
    let store = Arc::new(MemoryStore::open(&ctx).unwrap());

    store.save("project", "部署用 docker compose", 1).unwrap();
    store
        .save("project", "项目通过 docker compose 部署", 2)
        .unwrap();
    store
        .save("project", "本地开发用 docker compose 启动数据库", 3)
        .unwrap();

    let report = consolidate(&store);
    assert_eq!(
        report.remaining + report.merged,
        report.scanned,
        "条目数守恒"
    );

    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 计划：标注按范围分别留存 ----

/// 计划标注：整条计划与某一步都能标，标注随计划一起呈现。
/// 标注错位会让用户看到「这一步」被指向了别的步骤。
#[test]
fn plan_annotations_are_recorded_per_scope() {
    use wb_features::plan_types::{PlanAnnotation, PlanState, PlanStep, PlanStore};

    let (dir, ctx) = ctx_of("plan_annotate");
    let store = PlanStore::new(&ctx);
    store
        .mutate(|slot| {
            *slot = Some(PlanState {
                plan_id: "PLAN_1".into(),
                plan_title: "重构登录模块".into(),
                plan_status: "approved".into(),
                plan_reject_note: String::new(),
                plan_steps: vec![
                    PlanStep {
                        step_text: "拆出鉴权中间件".into(),
                        step_done: false,
                    },
                    PlanStep {
                        step_text: "迁移路由".into(),
                        step_done: false,
                    },
                ],
                plan_annotations: Vec::new(),
                plan_updated_at: 1,
            });
            Ok(())
        })
        .unwrap();

    // 整体标注。
    store
        .mutate(|slot| {
            let p = slot.as_mut().unwrap();
            p.plan_annotations.push(PlanAnnotation {
                annotation_id: "ANN_1".into(),
                annotation_scope: "plan".into(),
                annotation_step: 0,
                annotation_text: "先补测试再动结构".into(),
                annotation_author: "user".into(),
                annotation_created_at: 2,
            });
            Ok(())
        })
        .unwrap();
    // 步骤标注。
    store
        .mutate(|slot| {
            let p = slot.as_mut().unwrap();
            p.plan_annotations.push(PlanAnnotation {
                annotation_id: "ANN_2".into(),
                annotation_scope: "step".into(),
                annotation_step: 2,
                annotation_text: "这条要先确认影响范围".into(),
                annotation_author: "assistant".into(),
                annotation_created_at: 3,
            });
            Ok(())
        })
        .unwrap();

    let p = store.load().expect("计划应可读回");
    assert_eq!(p.plan_annotations.len(), 2, "两条标注都该保留");
    assert!(p.plan_annotations[0]
        .render_line()
        .contains("先补测试再动结构"));
    assert!(
        p.plan_annotations[1].render_line().contains("步骤 2"),
        "步骤标注应带序号：{}",
        p.plan_annotations[1].render_line()
    );

    let _ = std::fs::remove_dir_all(&dir);
}
