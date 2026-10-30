//! 三类开关：内置功能 / 技能 / 工具。

use wb_pkg::Result;

use super::db::Repo;

impl Repo {
    /// 读功能开关；未记录时返回 None（调用方用默认值）。
    pub fn feature_enabled(&self, feature_code: &str) -> Result<Option<bool>> {
        let conn = self.conn.lock().unwrap();
        let mut st = conn
            .prepare("SELECT enabled FROM feature_toggle WHERE feature_code = ?1")
            .map_err(super::db::db_err)?;
        let mut rows = st.query([feature_code]).map_err(super::db::db_err)?;
        match rows.next().map_err(super::db::db_err)? {
            Some(r) => Ok(Some(r.get::<_, i64>(0).map_err(super::db::db_err)? != 0)),
            None => Ok(None),
        }
    }

    /// 写功能开关。
    pub fn feature_set_enabled(&self, feature_code: &str, enabled: bool) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        conn.execute(
            "INSERT INTO feature_toggle(feature_code, enabled) VALUES(?1, ?2)
             ON CONFLICT(feature_code) DO UPDATE SET enabled = ?2",
            [feature_code, if enabled { "1" } else { "0" }],
        )
        .map_err(super::db::db_err)?;
        Ok(())
    }

    /// 读技能开关；未记录 = 开。
    pub fn skill_enabled(&self, skill_name: &str) -> Result<bool> {
        let conn = self.conn.lock().unwrap();
        let mut st = conn
            .prepare("SELECT enabled FROM skill_toggle WHERE skill_name = ?1")
            .map_err(super::db::db_err)?;
        let mut rows = st.query([skill_name]).map_err(super::db::db_err)?;
        match rows.next().map_err(super::db::db_err)? {
            Some(r) => Ok(r.get::<_, i64>(0).map_err(super::db::db_err)? != 0),
            None => Ok(true),
        }
    }

    /// 写技能开关。
    pub fn skill_set_enabled(&self, skill_name: &str, enabled: bool) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        conn.execute(
            "INSERT INTO skill_toggle(skill_name, enabled) VALUES(?1, ?2)
             ON CONFLICT(skill_name) DO UPDATE SET enabled = ?2",
            [skill_name, if enabled { "1" } else { "0" }],
        )
        .map_err(super::db::db_err)?;
        Ok(())
    }

    /// 读工具开关；未记录 = 开。
    pub fn tool_enabled(&self, tool_name: &str) -> Result<bool> {
        let conn = self.conn.lock().unwrap();
        let mut st = conn
            .prepare("SELECT enabled FROM tool_toggle WHERE tool_name = ?1")
            .map_err(super::db::db_err)?;
        let mut rows = st.query([tool_name]).map_err(super::db::db_err)?;
        match rows.next().map_err(super::db::db_err)? {
            Some(r) => Ok(r.get::<_, i64>(0).map_err(super::db::db_err)? != 0),
            None => Ok(true),
        }
    }

    /// 写工具开关。
    pub fn tool_set_enabled(&self, tool_name: &str, enabled: bool) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        conn.execute(
            "INSERT INTO tool_toggle(tool_name, enabled) VALUES(?1, ?2)
             ON CONFLICT(tool_name) DO UPDATE SET enabled = ?2",
            [tool_name, if enabled { "1" } else { "0" }],
        )
        .map_err(super::db::db_err)?;
        Ok(())
    }
}
