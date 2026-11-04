//! 内置技能清单：随壳分发的 22 个策展技能包，首启落位沙箱技能目录。
import fs from 'node:fs'
import path from 'node:path'
import { paths } from '../context'

/** 一个内置技能的静态描述。 */
export interface BuiltinSkill {
  /** 目录名（与 assets/skills 下一致） */
  skill_dir: string
  /** 界面上的分类标签 */
  skill_group: string
  /** 是否默认启用 */
  skill_default_on: boolean
  /** 依赖说明；需要外部程序时在界面上点明 */
  skill_requires?: string
}

/** 内置技能清单。按组排布，同组内顺序即界面顺序。 */
export const BUILTIN_SKILLS: BuiltinSkill[] = [
  { skill_dir: 'skill-creator', skill_group: '元能力', skill_default_on: true },
  { skill_dir: 'skill-scanner', skill_group: '元能力', skill_default_on: true },
  { skill_dir: 'skill-vetter', skill_group: '元能力', skill_default_on: true },
  { skill_dir: 'prompt-engineering-expert', skill_group: '元能力', skill_default_on: true },
  { skill_dir: 'task-alignment', skill_group: '任务流程', skill_default_on: true },
  { skill_dir: 'task-implement', skill_group: '任务流程', skill_default_on: true },
  { skill_dir: 'tdd', skill_group: '任务流程', skill_default_on: true },
  { skill_dir: 'diagnose', skill_group: '任务流程', skill_default_on: true },
  { skill_dir: 'handoff', skill_group: '任务流程', skill_default_on: true },
  { skill_dir: 'setup-pre-commit', skill_group: '任务流程', skill_default_on: true },
  { skill_dir: 'github', skill_group: '开发', skill_default_on: true, skill_requires: 'gh CLI' },
  { skill_dir: '全栈开发', skill_group: '开发', skill_default_on: true },
  { skill_dir: '前端开发', skill_group: '开发', skill_default_on: true },
  { skill_dir: 'Impeccable（前端设计工具集）', skill_group: '开发', skill_default_on: true },
  { skill_dir: 'mcp-builder', skill_group: '开发', skill_default_on: true },
  { skill_dir: 'deep-research', skill_group: '研究', skill_default_on: true },
  { skill_dir: 'canvas-design', skill_group: '文档产出', skill_default_on: true },
  { skill_dir: 'markitdown-skill', skill_group: '文档产出', skill_default_on: false },
  { skill_dir: 'md-to-pdf-cjk', skill_group: '文档产出', skill_default_on: true, skill_requires: 'Python reportlab' },
  { skill_dir: 'minimax-docx', skill_group: '文档产出', skill_default_on: true, skill_requires: '.NET SDK' },
  { skill_dir: 'minimax-pdf', skill_group: '文档产出', skill_default_on: true },
  { skill_dir: 'minimax-xlsx', skill_group: '文档产出', skill_default_on: false },
]

/** 按目录名取内置描述。 */
export function builtinSkillOf(skill_dir: string): BuiltinSkill | undefined {
  return BUILTIN_SKILLS.find((s) => s.skill_dir === skill_dir)
}

/** assets/skills 的绝对路径。 */
export function builtinSkillsSource(): string {
  return path.join(paths().wb_assets_dir, 'skills')
}

/**
 * 磁盘上有、清单里没登记的技能目录。
 * 界面上归为「随包附带」而不是内置——它们不享受版本管理，删了不会回来。
 */
export function unbundledSkillDirs(): string[] {
  const source = builtinSkillsSource()
  let names: string[] = []
  try {
    names = fs.readdirSync(source, { withFileTypes: true }).filter((d) => d.isDirectory()).map((d) => d.name)
  } catch {
    return []
  }
  return names.filter((name) => !builtinSkillOf(name) && fs.existsSync(path.join(source, name, 'SKILL.md')))
}
