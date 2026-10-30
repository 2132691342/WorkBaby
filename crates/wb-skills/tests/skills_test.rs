//! wb-skills 场景测试：frontmatter 解析与内置技能包完整性。
//! 内置技能包坏一个就是运行时静默缺技能，必须在构建期就拦住。

use wb_skills::frontmatter::{parse_skill_md, valid_description, valid_name};
use wb_skills::registry::BUILTIN_SKILLS;

/// 块标量（`description: |`）是最常见的写法，多行要完整拼回来。
#[test]
fn block_scalar_description_is_parsed() {
    let md = "---\nname: tdd\ndescription: |\n  line one\n  line two\nversion: 1.0.0\n---\n\n# 正文\n内容";
    let (fm, body) = parse_skill_md(md).unwrap();
    assert_eq!(fm.name.as_deref(), Some("tdd"));
    assert!(fm.description.unwrap().contains("line two"));
    assert!(body.contains("# 正文"));
}

/// 技能名规则：小写字母数字与单连字符；description 不能为空也不能超长。
#[test]
fn skill_name_and_description_rules_are_enforced() {
    assert!(!valid_description("  "));
    assert!(valid_name("tdd"));
    assert!(!valid_name("-bad-"));
    assert!(!valid_name("has--double"));
}

/// 内置包里每个技能目录都必须带可解析的 SKILL.md。
#[test]
fn builtin_pack_is_complete_and_parseable() {
    let dirs: Vec<_> = BUILTIN_SKILLS.dirs().collect();
    assert!(!dirs.is_empty(), "内置技能包不应为空");
    for d in dirs {
        // 不能用 d.get_file("SKILL.md")：目录名含中文与括号时，
        // include_dir 的路径组件比较对非 ASCII 不可靠，会误报「缺 SKILL.md」。
        // 直接遍历目录下的文件按文件名匹配，绕开路径解析。
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
            "技能 {} 的 name 非法",
            d.path().display()
        );
        assert!(
            valid_description(fm.description.as_deref().unwrap_or("")),
            "技能 {} 的 description 缺失或超长",
            d.path().display()
        );
    }
}
