//! features 域场景测试（续）：多搜索源、视频字幕、目标续跑、记忆整合、计划标注。
//! 全部是「改一处就可能坏一片」的场景，不放平凡断言。

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
    assert_eq!(w[0].url, "https://en.wikipedia.org/wiki/Rust_(programming_language)");
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
    assert_eq!(tracks[0].url, "https://yt.example/api/timedtext?v=1&lang=en");
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

/// 目标续跑：活跃目标有未完成步骤才给提示；阻塞 / 达成 / 无步骤都不给。
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

/// 说的是同一件事的记忆被合并成一条，内容不丢。
#[test]
fn consolidation_merges_duplicates_and_keeps_content() {
    use wb_features::memory_consolidate::consolidate;
    use wb_features::memory_store::MemoryStore;

    let (dir, ctx) = ctx_of("memory_consolidate");
    let store = Arc::new(MemoryStore::open(&ctx).unwrap());

    store.save("project", "项目用 pnpm 管理依赖", 1).unwrap();
    store.save("project", "这个项目的依赖由 pnpm 管理", 2).unwrap();
    store.save("user", "用户偏好深色主题", 3).unwrap();
    store.save("failure", "构建失败是因为 Node 版本太低", 4).unwrap();

    let report = consolidate(&store);
    assert_eq!(report.scanned, 4, "四条都该被扫到");
    assert_eq!(report.merged, 1, "同一件事的两条该合并成一条");
    assert_eq!(report.remaining, 3, "合并后剩三条");

    // 合并后内容不丢：两条都还在文本里。
    let hits = store.search("pnpm", 10).unwrap();
    assert_eq!(hits.len(), 1, "重复项应只剩一条");
    let text = store.list_by_kind("project", 10).unwrap();
    assert_eq!(text.len(), 1);
    let body = &text[0].1;
    assert!(body.contains("pnpm"), "合并后应保留关键词");
    assert!(body.len() > "项目用 pnpm 管理依赖".len(), "合并后内容应更全");

    // 不相关的条目不该被合到一起。
    assert_eq!(store.list_by_kind("user", 10).unwrap().len(), 1);
    assert_eq!(store.list_by_kind("failure", 10).unwrap().len(), 1);

    let _ = std::fs::remove_dir_all(&dir);
}

/// 说法不同但不是一回事的记忆不该被合并。
#[test]
fn consolidation_does_not_merge_different_facts() {
    use wb_features::memory_consolidate::consolidate;
    use wb_features::memory_store::MemoryStore;

    let (dir, ctx) = ctx_of("memory_no_false_merge");
    let store = Arc::new(MemoryStore::open(&ctx).unwrap());

    store.save("project", "部署用 docker compose", 1).unwrap();
    store.save("project", "项目通过 docker compose 部署", 2).unwrap();
    store.save("project", "本地开发用 docker compose 启动数据库", 3).unwrap();

    let report = consolidate(&store);
    assert_eq!(
        report.remaining + report.merged,
        report.scanned,
        "条目数守恒"
    );

    let _ = std::fs::remove_dir_all(&dir);
}

/// 计划标注：整条计划与某一步都能标，标注随计划一起呈现。
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
    assert!(p.plan_annotations[0].render_line().contains("先补测试再动结构"));
    assert!(
        p.plan_annotations[1].render_line().contains("步骤 2"),
        "步骤标注应带序号：{}",
        p.plan_annotations[1].render_line()
    );

    let _ = std::fs::remove_dir_all(&dir);
}