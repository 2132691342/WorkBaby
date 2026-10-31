//! 用量统计：天/模型粒度的 upsert 与聚合查询。

use wb_domain::stats::{StatsRESP, UsageDailyDO, UsageModelDO};
use wb_pkg::Result;

use super::db::Repo;

impl Repo {
    /// 累加一条用量（按天/模型/会话粒度）。
    #[allow(clippy::too_many_arguments)]
    pub fn usage_add(
        &self,
        day: &str,
        provider_id: &str,
        model_id: &str,
        session_id: &str,
        input: i64,
        output: i64,
        cache_read: i64,
        cache_write: i64,
        cost: f64,
    ) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        conn.execute(
            "INSERT INTO usage_daily(day, provider_id, model_id, session_id,
                                     input_tokens, output_tokens, cache_read, cache_write, cost_usd)
             VALUES(?1,?2,?3,?4,?5,?6,?7,?8,?9)
             ON CONFLICT(day, provider_id, model_id, session_id) DO UPDATE SET
               input_tokens = input_tokens + ?5,
               output_tokens = output_tokens + ?6,
               cache_read = cache_read + ?7,
               cache_write = cache_write + ?8,
               cost_usd = cost_usd + ?9",
            rusqlite::params![
                day,
                provider_id,
                model_id,
                session_id,
                input,
                output,
                cache_read,
                cache_write,
                cost
            ],
        )
        .map_err(super::db::db_err)?;
        Ok(())
    }

    /// 聚合查询（span：day7 / day30 / all）。
    pub fn usage_stats(&self, days: i64) -> Result<StatsRESP> {
        let conn = self.conn.lock().unwrap();
        let since = day_key_offset(-days);
        let mut daily = Vec::new();
        {
            let mut st = conn
                .prepare(
                    "SELECT day, SUM(input_tokens), SUM(output_tokens), SUM(cache_read), SUM(cache_write), SUM(cost_usd)
                     FROM usage_daily WHERE day >= ?1 GROUP BY day ORDER BY day",
                )
                .map_err(super::db::db_err)?;
            let rows = st
                .query_map([since.clone()], |r| {
                    Ok(UsageDailyDO {
                        daily_day: r.get(0)?,
                        daily_input: r.get(1)?,
                        daily_output: r.get(2)?,
                        daily_cache_read: r.get(3)?,
                        daily_cache_write: r.get(4)?,
                        daily_cost: r.get(5)?,
                    })
                })
                .map_err(super::db::db_err)?;
            for row in rows {
                daily.push(row.map_err(super::db::db_err)?);
            }
        }
        let mut by_model = Vec::new();
        {
            let mut st = conn
                .prepare(
                    "SELECT provider_id, model_id, SUM(input_tokens), SUM(output_tokens), SUM(cost_usd)
                     FROM usage_daily WHERE day >= ?1 GROUP BY provider_id, model_id ORDER BY SUM(cost_usd) DESC",
                )
                .map_err(super::db::db_err)?;
            let rows = st
                .query_map([since.clone()], |r| {
                    Ok(UsageModelDO {
                        model_provider: r.get(0)?,
                        model_id: r.get(1)?,
                        model_input: r.get(2)?,
                        model_output: r.get(3)?,
                        model_cost: r.get(4)?,
                    })
                })
                .map_err(super::db::db_err)?;
            for row in rows {
                by_model.push(row.map_err(super::db::db_err)?);
            }
        }
        Ok(StatsRESP {
            resp_daily: daily,
            resp_by_model: by_model,
        })
    }
}

/// 相对今天的日期键（yyyy-MM-dd，本机时区）。
fn day_key_offset(days: i64) -> String {
    let (y, m, d) = chrono_days(days);
    format!("{y:04}-{m:02}-{d:02}")
}

/// 毫秒时间戳对应的本地日期键（历史统计回填用）。
pub fn day_key_of_ms(ms: i64) -> String {
    use chrono::{Datelike, Local, TimeZone};
    let date = Local
        .timestamp_millis_opt(ms)
        .single()
        .map(|t| t.date_naive())
        .unwrap_or_else(|| Local::now().date_naive());
    format!("{:04}-{:02}-{:02}", date.year(), date.month(), date.day())
}

/// 用 chrono 求偏移日期。
fn chrono_days(days: i64) -> (i32, u32, u32) {
    use chrono::{Datelike, Duration, Local};
    let date = Local::now().date_naive() + Duration::days(days);
    (date.year(), date.month(), date.day())
}

/// 今天的日期键（写统计时用）。
pub fn today_key() -> String {
    day_key_offset(0)
}
