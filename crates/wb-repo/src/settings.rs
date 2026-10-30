//! 设置 KV：界面偏好的读写。

use wb_domain::settings::SettingKV;
use wb_pkg::Result;

use super::db::Repo;

impl Repo {
    /// 读全部设置。
    pub fn settings_all(&self) -> Result<Vec<SettingKV>> {
        let conn = self.conn.lock().unwrap();
        let mut st = conn
            .prepare("SELECT key, value FROM settings_kv ORDER BY key")
            .map_err(super::db::db_err)?;
        let rows = st
            .query_map([], |r| {
                Ok(SettingKV {
                    setting_key: r.get(0)?,
                    setting_value: r.get(1)?,
                })
            })
            .map_err(super::db::db_err)?;
        let mut out = Vec::new();
        for row in rows {
            out.push(row.map_err(super::db::db_err)?);
        }
        Ok(out)
    }

    /// 读单键；缺省返回 None。
    pub fn settings_get(&self, key: &str) -> Result<Option<String>> {
        let conn = self.conn.lock().unwrap();
        let mut st = conn
            .prepare("SELECT value FROM settings_kv WHERE key = ?1")
            .map_err(super::db::db_err)?;
        let mut rows = st.query([key]).map_err(super::db::db_err)?;
        match rows.next().map_err(super::db::db_err)? {
            Some(r) => Ok(Some(r.get(0).map_err(super::db::db_err)?)),
            None => Ok(None),
        }
    }

    /// 写单键（upsert）。
    pub fn settings_set(&self, key: &str, value: &str) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_millis() as i64;
        conn.execute(
            "INSERT INTO settings_kv(key, value, updated_at) VALUES(?1, ?2, ?3)
             ON CONFLICT(key) DO UPDATE SET value = ?2, updated_at = ?3",
            [key, value, &now.to_string()],
        )
        .map_err(super::db::db_err)?;
        Ok(())
    }
}
