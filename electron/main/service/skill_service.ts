//! 技能管理：内置技能包落位、扫描、开关（移目录）、增删改与跨引擎复制。
import fs from 'node:fs'
import path from 'node:path'
import { paths } from '../context'
import { dbConn } from '../repo/db'
import { ensureDir } from '../pkg/fsutil'
import { parseFrontmatter } from '../pkg/frontmatter'
import { appError, wrapError } from '../pkg/apperror'
import { CODE_SKILL_BAD, CODE_SKILL_DELETE, CODE_SKILL_SAVE } from '../domain/errors'
import { currentWorkspace } from './settings_service'
import type { SkillRowVO } from '../domain/types_data'

/** 技能目录布局。 */
function managedDir(): string {
  return path.join(paths().wb_pi_agent_dir, 'skills')
}

function hiddenDir(): string {
  return path.join(paths().wb_pi_agent_dir, 'skills_disabled')
}

/** 工作区项目技能目录；未选工作区给空串。 */
function projectSkillsDir(): string {
  const ws = currentWorkspace()
  return ws ? path.join(ws, '.pi', 'skills') : path.join(paths().wb_pi_agent_dir, '.no-project')
}

/** 启动时把 assets/skills 的内置包复制进技能目录；已存在的不动（保住用户改动）。 */
export function provisionSkills(): void {
  const from = path.join(paths().wb_assets_dir, 'skills')
  ensureDir(managedDir())
  ensureDir(hiddenDir())
  let names: string[] = []
  try {
    names = fs.readdirSync(from, { withFileTypes: true }).filter((d) => d.isDirectory()).map((d) => d.name)
  } catch {
    return
  }
  let copied = 0
  for (const name of names) {
    if (!fs.existsSync(path.join(from, name, 'SKILL.md'))) continue
    const target = path.join(managedDir(), name)
    if (fs.existsSync(target) || fs.existsSync(path.join(hiddenDir(), name))) continue
    try {
      fs.cpSync(path.join(from, name), target, { recursive: true })
      copied += 1
    } catch (err) {
      // 单个包复制失败不阻塞其余技能。
      console.warn(`技能包复制失败 ${name}`, err)
    }
  }
  if (copied > 0) console.log(`内置技能落位：${copied} 个`)
}

function skillEnabled(name: string): boolean {
  const row = dbConn().prepare('SELECT skill_enabled FROM skill_toggle WHERE skill_name = ?').get(name) as
    | { skill_enabled: number }
    | undefined
  return row ? row.skill_enabled === 1 : true
}

function setSkillEnabled(name: string, enabled: boolean): void {
  dbConn()
    .prepare(
      'INSERT INTO skill_toggle (skill_name, skill_enabled) VALUES (?, ?) ' +
        'ON CONFLICT(skill_name) DO UPDATE SET skill_enabled = excluded.skill_enabled',
    )
    .run(name, enabled ? 1 : 0)
}

function scanDir(root: string, source: SkillRowVO['source'], editable: boolean): SkillRowVO[] {
  const rows: SkillRowVO[] = []
  let names: string[] = []
  try {
    names = fs.readdirSync(root)
  } catch {
    return rows
  }
  for (const name of names) {
    const dir = path.join(root, name)
    const skill_file = path.join(dir, 'SKILL.md')
    if (!fs.existsSync(skill_file)) continue
    const text = fs.readFileSync(skill_file, 'utf8')
    const fm = parseFrontmatter(text)
    rows.push({
      name: fm.name || name,
      description: fm.description,
      path: skill_file,
      dir,
      source,
      enabled: skillEnabled(fm.name || name),
      editable,
      kind: 'skill',
    })
  }
  return rows
}

/** 技能总表：内置（managed）+ 工作区项目技能。 */
export function skillList(): { skills: SkillRowVO[] } {
  const rows = [...scanDir(managedDir(), 'managed', true), ...scanDir(projectSkillsDir(), 'project', true)]
  rows.sort((a, b) => a.name.localeCompare(b.name))
  return { skills: rows }
}

/** 校验技能路径落在托管目录内。 */
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
  const name = path.basename(dir)
  const active = path.join(managedDir(), name)
  const hidden = path.join(hiddenDir(), name)
  const project = path.join(projectSkillsDir(), name)
  try {
    if (enabled && fs.existsSync(hidden)) {
      fs.renameSync(hidden, active)
    } else if (!enabled && fs.existsSync(active)) {
      fs.renameSync(active, hidden)
    } else if (enabled && fs.existsSync(project)) {
      return { ok: true }
    }
  } catch (err) {
    throw wrapError(CODE_SKILL_SAVE, '技能开关失败', err)
  }
  setSkillEnabled(name, enabled)
  return { ok: true }
}

/** 新建技能：managed 目录下一套 SKILL.md。 */
export function skillCreate(name: string, description: string, body: string): { path: string } {
  const safe = name.trim().toLowerCase().replace(/[^a-z0-9-]+/g, '-')
  if (!safe || safe === '-') throw appError(CODE_SKILL_BAD, '技能名只能是字母数字与短横线')
  const dir = path.join(managedDir(), safe)
  if (fs.existsSync(dir)) throw appError(CODE_SKILL_BAD, `技能已存在：${safe}`)
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

/** 删除技能：只允许删托管目录里的。 */
export function skillDelete(skill_path: string): { ok: boolean } {
  const { dir } = guardSkillPath(skill_path)
  if (path.resolve(dir) === path.resolve(projectSkillsDir())) {
    throw appError(CODE_SKILL_DELETE, '项目技能请在项目内自行删除')
  }
  try {
    fs.rmSync(dir, { recursive: true, force: true })
  } catch (err) {
    throw wrapError(CODE_SKILL_DELETE, '技能删除失败', err)
  }
  setSkillEnabled(path.basename(dir), true)
  return { ok: true }
}

/** 跨引擎复制：目标只有 pi，等价于把项目技能抄进托管目录。 */
export function skillCopy(skill_path: string): { path: string } {
  const { dir, file } = guardSkillPath(skill_path)
  const name = path.basename(dir)
  if (path.resolve(dir) === path.resolve(managedDir())) return { path: file }
  const target = path.join(managedDir(), name)
  try {
    fs.cpSync(dir, target, { recursive: true })
  } catch (err) {
    throw wrapError(CODE_SKILL_SAVE, '技能复制失败', err)
  }
  return { path: path.join(target, 'SKILL.md') }
}

/** 技能总数（健康面板用）。 */
export function skillCount(): number {
  return skillList().skills.length
}
