//! 设置持久化：写入 → 关库 → 重开 → 原样读回，含工作区历史维护。
import { describe, expect, it } from 'vitest'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { openDb, closeDb } from '../electron/main/repo/db'
import { getSetting, saveSettings, setWorkspace, getWorkspace, forgetWorkspace, getWorkspaceHistory } from '../electron/main/repo/settings_repo'

describe('设置持久化', () => {
  it('跨进程重开一字不差；工作区历史置顶去重且遗忘联动当前', () => {
    const db = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'wb-test-')), 't.db')
    openDb(db)
    saveSettings({ 'ui.theme': 'dark', 'ui.font_scale': '1.2' })
    setWorkspace('D:\a')
    setWorkspace('D:\b')
    setWorkspace('D:\a')
    closeDb()

    // 重开：模拟进程重启后的回放。
    openDb(db)
    expect(getSetting('ui.theme')).toBe('dark')
    expect(getSetting('ui.font_scale')).toBe('1.2')
    expect(getSetting('missing')).toBeNull()
    expect(getWorkspace()).toBe('D:\a')
    expect(getWorkspaceHistory().map((r) => r.workspace_path)).toEqual(['D:\a', 'D:\b'])
    forgetWorkspace('D:\a')
    expect(getWorkspace()).toBe('')
    expect(getWorkspaceHistory().map((r) => r.workspace_path)).toEqual(['D:\b'])
    closeDb()
  })
})
