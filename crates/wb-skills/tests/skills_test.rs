//! wb-skills 场景测试：SKILL.md 解析与内置技能包完整性。坏一个技能就是运行时静默缺技能，须在构建期拦住。

use wb_skills::frontmatter::{parse_skill_md, valid_description, valid_name};
use wb_skills::registry::BUILTIN_SKILLS;

#[test]
fn frontmatter_parser_accepts_valid_and_rejects_bad_input() {
    let md = "---\nname: tdd\ndescription: |\n  line one\n  line two\nversion: 1.0.0\ncustom: {a: 1}\n---\n\n# 正文\n内容";
    let (fm, body) = parse_skill_md(md).unwrap();
    assert_eq!(fm.name.as_deref(), Some("tdd"));
    assert!(fm.description.unwrap().contains("line two"));
    assert_eq!(fm.version.as_deref(), Some("1.0.0"));
    assert!(
        fm.extra.contains_key("custom"),
        "未知键应原样保留：{:?}",
        fm.extra
    );
    assert!(body.contains("# 正文"));

    assert_eq!(
        parse_skill_md("# 正文\n没有 frontmatter").unwrap_err().code,
        "8001"
    );
    assert_eq!(
        parse_skill_md("---\nname: tdd\n\n# 正文").unwrap_err().code,
        "8001"
    );

    assert!(valid_name("tdd"));
    assert!(!valid_name("-bad-"));
    assert!(!valid_name("has--double"));
    assert!(!valid_name("Has-Capitals"));
    assert!(!valid_name(&"x".repeat(65)));
    assert!(!valid_description("  "));
    assert!(!valid_description(&"x".repeat(1025)));
    assert!(valid_description(&"x".repeat(1024)));
}

#[test]
fn builtin_pack_is_complete_and_parseable() {
    let dirs: Vec<_> = BUILTIN_SKILLS.dirs().collect();
    assert!(!dirs.is_empty(), "内置技能包不应为空");
    let mut names: Vec<String> = Vec::new();
    for d in dirs {
        // include_dir 对非 ASCII 目录名的路径组件比较不可靠，按文件名匹配绕开。
        let f = d
            .files()
            .find(|f| {
                f.path()
                    .file_name()
                    .map(|n| n == "SKILL.md")
                    .unwrap_or(false)
            })
            .unwrap_or_else(|| panic!("技能 {} 缺 SKILL.md", d.path().display()));
        let text = String::from_utf8_lossy(f.contents());
        let (fm, _) = parse_skill_md(&text)
            .unwrap_or_else(|e| panic!("技能 {} 解析失败：{e}", d.path().display()));
        assert!(
            valid_name(fm.name.as_deref().unwrap_or("")),
            "技能 {} name 非法",
            d.path().display()
        );
        assert!(
            valid_description(fm.description.as_deref().unwrap_or("")),
            "技能 {} description 缺失或超长",
            d.path().display()
        );
        names.push(fm.name.unwrap_or_default());
    }
    // 同名技能互相覆盖成静默丢技能，构建期拦住。
    let mut sorted = names.clone();
    sorted.sort();
    sorted.dedup();
    assert_eq!(sorted.len(), names.len(), "技能名重复：{names:?}");
}
