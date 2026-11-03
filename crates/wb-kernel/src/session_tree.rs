//! 会话树：从 JSONL 的父子链还原出分叉结构，供界面画出「我走过哪条路」。
//! 与 session.rs 分开：树是纯派生视图，session.rs 是存储与回放，两套生命周期。

use serde::Serialize;

use super::session::{Entry, EntryWrap, Msg, SessionFile};

/// 会话树的一个节点：一条消息 + 它下面的分支。
#[derive(Debug, Clone, Serialize)]
pub struct TreeNode {
    /// 条目 id
    pub node_entry_id: String,
    /// 父条目 id（根为 null）
    pub node_parent_entry_id: Option<String>,
    /// 一句话摘要（用户消息正文；助手消息取工具名提示）
    pub node_preview: String,
    /// 是不是用户消息（只有用户消息能当分支点）
    pub node_is_user: bool,
    /// 是否在当前路径上（leaf→root 回放的那条线）
    pub node_on_path: bool,
    /// 子节点（分支从这里长出来）
    pub node_children: Vec<TreeNode>,
}

/// 整棵会话树：从根到叶的路径 + 挂在这条路径上的其它分支。
/// 节点是所有消息条目（用户与助手都算）：只留用户消息的话，助手回复成了看不见的空洞。
/// `node_is_user` 标出哪些能当分支点；`node_on_path` 标出当前路径，界面据此高亮主干。
pub fn build_tree(sf: &SessionFile) -> TreeNode {
    use std::collections::HashMap;
    let by_id: HashMap<&str, &EntryWrap> = sf
        .entries
        .iter()
        .map(|e| (e.entry_id.as_str(), e))
        .collect();
    let on_path: std::collections::HashSet<&str> = sf
        .path_to_leaf()
        .iter()
        .map(|e| e.entry_id.as_str())
        .collect();
    // 消息条目才是节点；模型变更、压缩、用量等条目对用户没有分支意义
    let mut children_of: HashMap<Option<&str>, Vec<&EntryWrap>> = HashMap::new();
    for e in sf.entries.iter() {
        if !is_message(e) {
            continue;
        }
        // 父节点不是消息时（模型变更等夹在中间）上溯到最近的消息祖先
        let parent = nearest_message_ancestor(e, &by_id);
        children_of.entry(parent).or_default().push(e);
    }
    for v in children_of.values_mut() {
        v.sort_by_key(|e| e.timestamp);
    }
    let mut root_entries: Vec<&EntryWrap> = children_of.get(&None).cloned().unwrap_or_default();
    root_entries.sort_by_key(|e| e.timestamp);
    fn build(
        e: &EntryWrap,
        on_path: &std::collections::HashSet<&str>,
        children_of: &HashMap<Option<&str>, Vec<&EntryWrap>>,
    ) -> TreeNode {
        let kids = children_of
            .get(&Some(e.entry_id.as_str()))
            .map(|v| v.iter().map(|c| build(c, on_path, children_of)).collect())
            .unwrap_or_default();
        TreeNode {
            node_entry_id: e.entry_id.clone(),
            node_parent_entry_id: e.parent_entry_id.clone(),
            node_preview: preview_of(e),
            node_is_user: is_user_entry(e),
            node_on_path: on_path.contains(e.entry_id.as_str()),
            node_children: kids,
        }
    }
    let branches: Vec<TreeNode> = root_entries
        .iter()
        .map(|r| build(r, &on_path, &children_of))
        .collect();
    if branches.len() == 1 {
        return branches.into_iter().next().unwrap();
    }
    // 多个根（会话从中间导入过时可能有）拼成一条虚拟主干
    TreeNode {
        node_entry_id: String::new(),
        node_parent_entry_id: None,
        node_preview: String::new(),
        node_is_user: false,
        node_on_path: true,
        node_children: branches,
    }
}

/// 条目是不是用户消息（只有用户消息能当分支点）。
fn is_user_entry(e: &EntryWrap) -> bool {
    matches!(
        &e.payload,
        Entry::Message {
            message: Msg::User { .. }
        }
    )
}

/// 条目是不是消息。
fn is_message(e: &EntryWrap) -> bool {
    matches!(&e.payload, Entry::Message { .. })
}

/// 最近的消息祖先 id（None = 没有祖先，是根）。
fn nearest_message_ancestor<'a>(
    e: &EntryWrap,
    by_id: &std::collections::HashMap<&'a str, &'a EntryWrap>,
) -> Option<&'a str> {
    let mut cur = e.parent_entry_id.as_deref();
    // 链上条目数有上限：坏文件成环时不能死循环
    for _ in 0..10_000 {
        let id = cur?;
        let node = by_id.get(id)?;
        if is_message(node) {
            return Some(node.entry_id.as_str());
        }
        cur = node.parent_entry_id.as_deref();
    }
    None
}

/// 节点的摘要文案。
fn preview_of(e: &EntryWrap) -> String {
    if let Entry::Message {
        message: Msg::User { content, .. },
    } = &e.payload
    {
        return content.trim().replace('\n', " ");
    }
    String::new()
}
