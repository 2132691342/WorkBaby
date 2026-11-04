//! 环境组装：给子进程的 PATH 与变量全部先指到内置运行时，再回退系统。
import fs from 'node:fs'
import path from 'node:path'
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
  // npm 的全局前缀与缓存也锁进数据目录：装什么都落在沙箱里，不碰用户 %APPDATA%。
  env.npm_config_cache = path.join(p.wb_data_dir, 'cache', 'npm')
  env.npm_config_prefix = path.join(p.wb_data_dir, 'cache', 'npm-global')
  env.npm_config_update_notifier = 'false'
  env.npm_config_fund = 'false'
  env.npm_config_audit = 'false'
  // 让 git / npm 全部落在数据目录内，不污染用户全局配置。
  env.HOME = p.wb_data_dir
  env.USERPROFILE = p.wb_data_dir
  // pi 的家：任何一层子进程（bash / python / 插件）都认这个变量。
  env.PI_CODING_AGENT_DIR = p.wb_pi_agent_dir
  // 壳自己管升级与遥测，引擎不必再探远端，也不上报。
  env.PI_SKIP_VERSION_CHECK = '1'
  env.PI_TELEMETRY = '0'
  return env
}
