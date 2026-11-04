//! 技能管理：内置清单落位、扫描、开关（移目录）、增删改与跨引擎复制。
import fs from 'node:fs'
import path from 'node:path'
import { paths } from '../context'
import { dbConn } from '../repo/db'
import { ensureDir } from '../pkg/fsutil'
import { parseFrontmatter } from '../pkg/frontmatter'
import { appError, wrapError } from '../pkg/apperror'
import { CODE_SKILL_BAD, CODE_SKILL_DELETE, CODE_SKILL_SAVE } from '../domain/errors'
import { broadcast } from '../sse/hub'
import { logInfo, logWarn } from '../pkg/log'
import { currentWorkspace } from './settings_service'
import { BUILTIN_SKILLS, builtinSkillOf, builtinSkillsSource } from './skill_catalog'
import type { SkillListDataVO, SkillRowVO, SkillSourceKind } from '../domain/types_data'

/** 技能目录布局。 */
function managedDir(): string {
  return path.join(paths().wb_pi_agent_dir, 'skills')
}

function hiddenDir(): string {
  return path.join(paths().wb_pi_agent_dir, 'skills_disabled')
}

/** 工作区项目技能目录；未选工作区给一个永远不会命中的占位路径。 */
function projectSkillsDir(): string {
  const ws = currentWorkspace()
  return ws ? path.join(ws, '.pi', 'skills') : path.join(paths().wb_pi_agent_dir, '.no-project')
}

/** 把一个技能目录从随包资产复制进托管目录。 */
function copySkill(skill_dir: string, target: string): boolean {
  try {
    fs.cpSync(path.join(builtinSkillsSource(), skill_dir), target, { recursive: true })
    return true
  } catch (err) {
    logWarn(`技能包落位失败 ${skill_dir}：${err instanceof Error ? err.message : String(err)}`)
    return false
  }
}

/**
 * 首启把内置清单里的技能落进沙箱：启用态进 skills，停用态直接进禁用区。
 * 已存在的目录不动——用户在 GUI 里改过的内容不该被下次启动覆盖。
 */
export function provisionSkills(): void {
  ensureDir(managedDir())
  ensureDir(hiddenDir())
  let copied = 0
  for (const item of BUILTIN_SKILLS) {
    const active = path.join(managedDir(), item.skill_dir)
    const hidden = path.join(hiddenDir(), item.skill_dir)
    if (fs.existsSync(active) || fs.existsSync(hidden)) continue
    const target = item.skill_default_on ? active : hidden
    if (copySkill(item.skill_dir, target)) copied += 1
  }
  if (copied > 0) {
    logInfo(`内置技能落位：${copied} 个`)
    broadcast('skill:status', { engine_kind: 'pi', provisioned: copied })
  }
}

/** 技能开关意图（DB）；没有记录时按清单默认值给。 */
function skillEnabled(name: string, fallback: boolean): boolean {
  const row = dbConn().prepare('SELECT skill_enabled FROM skill_toggle WHERE skill_name = ?').get(name) as
    | { skill_enabled: number }
    | undefined
  return row ? row.skill_enabled === 1 : fallback
}

function setSkillEnabled(name: string, enabled: boolean): void {
  dbConn()
    .prepare(
      'INSERT INTO skill_toggle (skill_name, skill_enabled) VALUES (?, ?) ' +
        'ON CONFLICT(skill_name) DO UPDATE SET skill_enabled = excluded.skill_enabled',
    )
    .run(name, enabled ? 1 : 0)
}

/** 本地副本与随包原件是否一致；非内置技能恒 false。 */
function skillModified(skill_dir: string, local_dir: string): boolean {
  if (!builtinSkillOf(skill_dir)) return false
  const original = path.join(builtinSkillsSource(), skill_dir)
  if (!fs.existsSync(original)) return false
  try {
    const a = fs.readFileSync(path.join(original, 'SKILL.md'), 'utf8')
    const b = fs.readFileSync(path.join(local_dir, 'SKILL.md'), 'utf8')
    return a !== b
  } catch {
    return false
  }
}

/** 扫一个目录成技能行。 */
function scanDir(root: string, source: SkillSourceKind, editable: boolean): SkillRowVO[] {
  const rows: SkillRowVO[] = []
  let names: string[] = []
  try {
    names = fs.readdirSync(root)
  } catch {
    return rows
  }
  for (const skill_dir of names) {
    const dir = path.join(root, skill_dir)
    const skill_file = path.join(dir, 'SKILL.md')
    if (!fs.existsSync(skill_file)) continue
    let text = ''
    try {
      text = fs.readFileSync(skill_file, 'utf8')
    } catch {
      continue
    }
    const fm = parseFrontmatter(text)
    const builtin = builtinSkillOf(skill_dir)
    const name = fm.name || skill_dir
    rows.push({
      name,
      description: fm.description,
      path: skill_file,
      dir,
      source,
      enabled: source === 'project' ? true : skillEnabled(skill_dir, builtin?.skill_default_on ?? true),
      editable,
      kind: 'skill',
      skill_group: builtin?.skill_group ?? '',
      skill_requires: builtin?.skill_requires ?? '',
      skill_modified: skillModified(skill_dir, dir),
    })
  }
  return rows
}

/**
 * 技能总表：启用区 + 禁用区 + 工作区项目技能。
 * 禁用区的行 enabled=false，界面能看见、开关能打开——不然停用就等于消失。
 */
export function skillList(): SkillListDataVO {
  const managed = scanDir(managedDir(), 'managed', true).map((row) => {
    const builtin = builtinSkillOf(path.basename(row.dir))
    row.source = builtin ? 'builtin' : 'bundled'
    return row
  })
  const disabled = scanDir(hiddenDir(), 'managed', true).map((row) => {
    const builtin = builtinSkillOf(path.basename(row.dir))
    row.source = builtin ? 'builtin' : 'bundled'
    row.enabled = false
    return row
  })
  const project = scanDir(projectSkillsDir(), 'project', false)
  const skills = [...managed, ...disabled, ...project]
  skills.sort((a, b) => {
    if (a.skill_group !== b.skill_group) return a.skill_group.localeCompare(b.skill_group)
    return a.name.localeCompare(b.name)
  })
  const present = new Set([...managed, ...disabled].map((row) => path.basename(row.dir)))
  return {
    skills,
    builtin_total: BUILTIN_SKILLS.length,
    builtin_installed: BUILTIN_SKILLS.filter((item) => present.has(item.skill_dir)).length,
  }
}

/** 校验技能路径落在托管 / 禁用 / 项目目录内。 */
function guardSkillPath(skill_path: string): { dir: string; file: string } {
  const resolved = path.resolve(skill_path)
  const roots = [managedDir(), hiddenDir(), projectSkillsDir()]
  const hit = roots.find((root) => resolved.startsWith(path.resolve(root) + path.sep))
  if (!hit) throw appError(CODE_SKILL_BAD, '技能路径越界', resolved)
  return { dir: path.dirname(resolved), file: resolved }
}

/** 读技能正文。 */
export function skillContent(skill_path: string): { content: string } {
  const { file } = guardSkillPath(skill_path)
  try {
    return { content: fs.readFileSync(file, 'utf8') }
  } catch (err) {
    throw wrapError(CODE_SKILL_BAD, '技能读取失败', err)
  }
}

/** 保存技能正文。 */
export function skillSave(_engine: string, skill_path: string, text: string): { content: string } {
  const { file } = guardSkillPath(skill_path)
  try {
    fs.writeFileSync(file, text, 'utf8')
  } catch (err) {
    throw wrapError(CODE_SKILL_SAVE, '技能保存失败', err)
  }
  return { content: text }
}

/** 开关技能：把目录搬进 / 搬出禁用区，pi 自然不再发现。 */
export function skillToggle(skill_path: string, enabled: boolean): { ok: boolean } {
  const { dir } = guardSkillPath(skill_path)
  const skill_dir = path.basename(dir)
  const active = path.join(managedDir(), skill_dir)
  const hidden = path.join(hiddenDir(), skill_dir)
  try {
    if (enabled && fs.existsSync(hidden)) {
      fs.renameSync(hidden, active)
    } else if (!enabled && fs.existsSync(active)) {
      fs.renameSync(active, hidden)
    }
  } catch (err) {
    throw wrapError(CODE_SKILL_SAVE, '技能开关失败', err)
  }
  setSkillEnabled(skill_dir, enabled)
  broadcast('skill:status', { engine_kind: 'pi', skill_dir, enabled })
  return { ok: true }
}

/** 把内置技能重置回随包原件（用户改坏了时的恢复口）。 */
export function skillReset(skill_path: string): { ok: boolean } {
  const { dir } = guardSkillPath(skill_path)
  const skill_dir = path.basename(dir)
  const original = path.join(builtinSkillsSource(), skill_dir)
  if (!fs.existsSync(original)) throw appError(CODE_SKILL_BAD, `不是内置技能，无法重置：${skill_dir}`)
  const was_enabled = dir.startsWith(path.resolve(managedDir()))
  try {
    fs.rmSync(dir, { recursive: true, force: true })
    fs.cpSync(original, dir, { recursive: true })
  } catch (err) {
    throw wrapError(CODE_SKILL_SAVE, '技能重置失败', err)
  }
  setSkillEnabled(skill_dir, was_enabled)
  broadcast('skill:status', { engine_kind: 'pi', skill_dir, reset: true })
  return { ok: true }
}

/** 新建技能：managed 目录下一套 SKILL.md。 */
export function skillCreate(name: string, description: string, body: string): { path: string } {
  const safe = name.trim().toLowerCase().replace(/[^a-z0-9-]+/g, '-')
  if (!safe || safe === '-') throw appError(CODE_SKILL_BAD, '技能名只能是字母数字与短横线')
  const dir = path.join(managedDir(), safe)
  if (fs.existsSync(dir) || fs.existsSync(path.join(hiddenDir(), safe))) {
    throw appError(CODE_SKILL_BAD, `技能已存在：${safe}`)
  }
  fs.mkdirSync(dir, { recursive: true })
  const file = path.join(dir, 'SKILL.md')
  const content = `---\nname: ${safe}\ndescription: ${description.trim() || safe}\n---\n\n${body.trim()}\n`
  try {
    fs.writeFileSync(file, content, 'utf8')
  } catch (err) {
    throw wrapError(CODE_SKILL_SAVE, '技能创建失败', err)
  }
  return { path: file }
}

/** 删除技能：只允许删托管与禁用区的。 */
export function skillDelete(skill_path: string): { ok: boolean } {
  const { dir } = guardSkillPath(skill_path)
  if (path.resolve(dir) === path.resolve(projectSkillsDir())) {
    throw appError(CODE_SKILL_DELETE, '项目技能请在项目内自行删除')
  }
  const skill_dir = path.basename(dir)
  try {
    fs.rmSync(dir, { recursive: true, force: true })
  } catch (err) {
    throw wrapError(CODE_SKILL_DELETE, '技能删除失败', err)
  }
  setSkillEnabled(skill_dir, true)
  broadcast('skill:status', { engine_kind: 'pi', skill_dir, deleted: true })
  return { ok: true }
}

/** 把项目技能抄进托管目录（工作区技能长期化的唯一路径）。 */
export function skillCopy(skill_path: string): { path: string } {
  const { dir, file } = guardSkillPath(skill_path)
  const skill_dir = path.basename(dir)
  if (dir.startsWith(path.resolve(managedDir()))) return { path: file }
  const target = path.join(managedDir(), skill_dir)
  if (fs.existsSync(target)) throw appError(CODE_SKILL_SAVE, `托管区已有同名技能：${skill_dir}`)
  try {
    fs.cpSync(dir, target, { recursive: true })
  } catch (err) {
    throw wrapError(CODE_SKILL_SAVE, '技能复制失败', err)
  }
  broadcast('skill:status', { engine_kind: 'pi', skill_dir, copied: true })
  return { path: path.join(target, 'SKILL.md') }
}

/** 已启用的技能数（健康面板用）。 */
export function skillCount(): number {
  return skillList().skills.filter((s) => s.enabled).length
}
