//! 错误码契约：每个码都要有中文与英文文案。
//!
//! 漏一个码的表现是「英文界面里蹦出一句中文」或「只有 details 没有结论」，
//! 这属于要发布时才被用户撞见的那类问题，所以在这里一次性守住。

use wb_domain::messages::{default_message, is_known};

/// 与 docs/ERROR-CODES.md 的码表一致；新增码必须先改文档再改这里。
const ALL_CODES: &[&str] = &[
    "1001", "1002", "1003", "1004", "1017", "1020", "1021", "1024", "1030", "2001", "2002", "2004",
    "3001", "3002", "3003", "3005", "3109", "3111", "3200", "5001", "5002", "5003", "5004", "5005",
    "5006", "5100", "7001", "7002", "7003", "7004", "7100", "7101", "7102", "7103", "7301", "8001",
    "8002", "8003", "8004", "8005", "9101", "9102", "9103", "9701", "9703", "9704",
];

#[test]
fn every_business_code_has_both_languages() {
    for code in ALL_CODES {
        assert!(is_known(code), "码 {code} 没有默认文案");
        let zh = default_message(code, "zh-CN");
        let en = default_message(code, "en");
        assert!(!zh.is_empty(), "码 {code} 缺中文文案");
        assert!(!en.is_empty(), "码 {code} 缺英文文案");
        assert_ne!(zh, en, "码 {code} 的中英文文案完全相同，等于没翻译");
    }
}

#[test]
fn unknown_code_falls_back_to_empty_string() {
    assert!(!is_known("9999"), "没登记的码不能被当作已收录");
    assert_eq!(default_message("9999", "zh-CN"), "");
    assert_eq!(default_message("9999", "en"), "");
}
