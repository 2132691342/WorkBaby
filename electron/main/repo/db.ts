//! SQLite：连接、迁移与表访问的根；手写迁移，user_version 步进。
import Database from 'better-sqlite3'
import fs from 'node:fs'
import { ensureDir } from '../pkg/fsutil'

let db: Database.Database | null = null

/** 打开（或复用）连接：WAL + 忙等 5s，进程内单写。 */
export function openDb(db_path: string): Database.Database {
  if (db) return db
  ensureDir(db_path.replace(/[/\\][^/\\]+$/, ''))
  db = new Database(db_path)
  db.pragma('journal_mode = WAL')
  db.pragma('busy_timeout = 5000')
  migrate(db)
  return db
}

/** 测试与关停用。 */
export function closeDb(): void {
  db?.close()
  db = null
}

/** 拿当前连接；没开就抛。 */
export function dbConn(): Database.Database {
  if (!db) throw new Error('DB 未打开：先调 openDb()')
  return db
}

function migrate(conn: Database.Database): void {
  const version = (conn.pragma('user_version', { simple: true }) as number) ?? 0
  if (version < 1) {
    conn.exec(`
      CREATE TABLE IF NOT EXISTS settings_kv (
        setting_key TEXT PRIMARY KEY,
        setting_value TEXT NOT NULL
      );
      CREATE TABLE IF NOT EXISTS skill_toggle (
        skill_name TEXT PRIMARY KEY,
        skill_enabled INTEGER NOT NULL
      );
      CREATE TABLE IF NOT EXISTS plugin_toggle (
        plugin_name TEXT PRIMARY KEY,
        plugin_enabled INTEGER NOT NULL
      );
      CREATE TABLE IF NOT EXISTS usage_daily (
        usage_day TEXT NOT NULL,
        provider_name TEXT NOT NULL,
        model_id TEXT NOT NULL,
        session_id TEXT NOT NULL,
        input_tokens INTEGER NOT NULL DEFAULT 0,
        output_tokens INTEGER NOT NULL DEFAULT 0,
        cache_read_tokens INTEGER NOT NULL DEFAULT 0,
        cache_write_tokens INTEGER NOT NULL DEFAULT 0,
        cost REAL NOT NULL DEFAULT 0,
        PRIMARY KEY (usage_day, provider_name, model_id, session_id)
      );
    `)
    conn.pragma('user_version = 1')
  }
}
