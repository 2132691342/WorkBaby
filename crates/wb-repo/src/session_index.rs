//! 会话索引：列表数据源（真相在 JSONL，这里只做查询加速）。

use wb_domain::session::SessionIndexDO;
use wb_pkg::Result;

use super::db::Repo;

impl Repo {
    /// 全量会话索引（按最后活动倒序）。
    pub fn sessions_list(&self) -> Result<Vec<SessionIndexDO>> {
        let conn = self.conn.lock().unwrap();
        let mut st = conn
            .prepare(
                "SELECT session_id, session_name, session_workspace, session_provider,
                        session_model, session_message_count, session_last_active_at, session_summary
                 FROM session_index ORDER BY session_last_active_at DESC",
            )
            .map_err(super::db::db_err)?;
        let rows = st
            .query_map([], |r| {
                Ok(SessionIndexDO {
                    session_id: r.get(0)?,
                    session_name: r.get(1)?,
                    session_workspace: r.get(2)?,
                    session_provider: r.get(3)?,
                    session_model: r.get(4)?,
                    session_message_count: r.get(5)?,
                    session_last_active_at: r.get(6)?,
                    session_summary: r.get(7)?,
                })
            })
            .map_err(super::db::db_err)?;
        let mut out = Vec::new();
        for row in rows {
            out.push(row.map_err(super::db::db_err)?);
        }
        Ok(out)
    }

    /// upsert 一条索引。
    pub fn sessions_upsert(&self, s: &SessionIndexDO) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        conn.execute(
            "INSERT INTO session_index(session_id, session_name, session_workspace, session_provider,
                                       session_model, session_message_count, session_last_active_at, session_summary)
             VALUES(?1,?2,?3,?4,?5,?6,?7,?8)
             ON CONFLICT(session_id) DO UPDATE SET
               session_name=?2, session_workspace=?3, session_provider=?4, session_model=?5,
               session_message_count=?6, session_last_active_at=?7, session_summary=?8",
            rusqlite::params![
                s.session_id,
                s.session_name,
                s.session_workspace,
                s.session_provider,
                s.session_model,
                s.session_message_count,
                s.session_last_active_at,
                s.session_summary,
            ],
        )
        .map_err(super::db::db_err)?;
        Ok(())
    }

    /// 删除一条索引。
    pub fn sessions_delete(&self, session_id: &str) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        conn.execute(
            "DELETE FROM session_index WHERE session_id = ?1",
            [session_id],
        )
        .map_err(super::db::db_err)?;
        Ok(())
    }

    /// 更新运行态字段（模型/名称/消息数/摘要/活动时间）。
    pub fn sessions_touch(&self, session_id: &str, s: &SessionIndexDO) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        conn.execute(
            "UPDATE session_index SET session_name=?2, session_provider=?3, session_model=?4,
                    session_message_count=?5, session_last_active_at=?6, session_summary=?7
             WHERE session_id = ?1",
            rusqlite::params![
                session_id,
                s.session_name,
                s.session_provider,
                s.session_model,
                s.session_message_count,
                s.session_last_active_at,
                s.session_summary,
            ],
        )
        .map_err(super::db::db_err)?;
        Ok(())
    }
}
