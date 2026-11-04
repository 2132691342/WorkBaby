//! 环境组装：给子进程的 PATH 与变量全部先指到内置运行时，再回退系统。
import fs from 'node:fs'
import { paths } from '../context'

/** 已存在的目录才进 PATH；不存在的直接跳过。 */
function existing(dirs: string[]): string[] {
  return dirs.filter((d) => d && fs.existsSync(d))
}

/**
 * 组装子进程环境：内置 node / git / python 在前，系统 PATH 在后，
 * 这样 agent 的 bash / python 一律先命中内置运行时。
 */
export function toolEnv(extra: Record<string, string> = {}): Record<string, string> {
  const p = paths()
  const prepended = existing([
    p.wb_node_dir,
    `${p.wb_git_dir}\\cmd`,
    `${p.wb_git_dir}\\bin`,
    `${p.wb_git_dir}\\usr\\bin`,
    p.wb_python_dir,
  ])
  const env: Record<string, string> = { ...process.env, ...extra } as Record<string, string>
  env.PATH = [...prepended, process.env.PATH ?? ''].join(';')
  // 让 git / npm 全部落在数据目录内，不污染用户全局配置。
  env.HOME = p.wb_data_dir
  env.USERPROFILE = p.wb_data_dir
  return env
}
