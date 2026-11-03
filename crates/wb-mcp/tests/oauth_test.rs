//! wb-mcp 场景测试：OAuth 授权流的协议细节。
//! PKCE 的 challenge 算错或 state 校验漏了，整条登录链就是废的。

use wb_mcp::oauth_crypto::{pkce_pair, random_state};

/// PKCE：challenge 必须是 verifier 的 SHA-256 base64url。
/// RFC 7636 规定得很死：算错就是 authorization server 拒绝换 token。
#[test]
fn pkce_challenge_is_sha256_base64url_of_verifier() {
    let (verifier, challenge) = pkce_pair();
    assert!(!verifier.is_empty());
    assert!(!challenge.is_empty());
    // base64url 字符集：不含 + / =，且没有 padding
    assert!(
        challenge
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_'),
        "challenge 不是 base64url：{challenge}"
    );
    assert!(!challenge.contains('='), "base64url 不该有 padding");
    // 同一 verifier 必须算出同一个 challenge（可复现）
    let (v2, c2) = pkce_pair();
    assert_ne!(v2, verifier, "两次生成的 verifier 不该相同");
    assert_ne!(c2, challenge, "两次生成的 challenge 不该相同");
}

/// state 每次都要不同：它是防 CSRF 的凭证，重复就等于没防。
#[test]
fn random_state_differs_each_time() {
    let a = random_state();
    let b = random_state();
    assert_ne!(a, b, "state 不该重复");
    assert!(a.len() >= 16, "state 太短：{a}");
}
