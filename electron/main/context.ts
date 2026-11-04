//! 全局运行时：路径布局、鉴权令牌、监听端口；装配时填好，之后只读。
import path from 'node:path'
import crypto from 'node:crypto'
import { app } from 'electron'

export interface WbPaths {
  /** 数据根：%LOCALAPPDATA%\WorkBaby（开发态 -dev 后缀） */
  wb_data_dir: string
  /** 内置运行时解压根：data\runtime */
  wb_runtime_dir: string
  /** 内置 node 目录：runtime\node */
  wb_node_dir: string
  /** 内置 Git 目录：runtime\PortableGit */
  wb_git_dir: string
  /** 内置 Python 目录：runtime\python */
  wb_python_dir: string
  /** pi 的 agent 目录（PI_CODING_AGENT_DIR）：data\pi */
  wb_pi_agent_dir: string
  /** 内置资产（zip / 技能 / 文档） */
  wb_assets_dir: string
  /** 内置 pi 扩展（workbaby-gate 等） */
  wb_extension_dir: string
  wb_log_dir: string
  wb_db_path: string
}

interface WbContext {
  wb_paths: WbPaths
  /** HTTP 鉴权令牌：每次启动随机 */
  wb_token: string
  /** HTTP 端口：装配时定 */
  wb_port: number
}

let ctx: WbContext | null = null

/** 计算路径布局；在 app.ready 之前调用。 */
export function initContext(): WbPaths {
  const data = app.isPackaged
    ? path.join(process.env.LOCALAPPDATA ?? '.', 'WorkBaby')
    : path.join(process.env.LOCALAPPDATA ?? '.', 'WorkBaby-dev')
  // 开发态产物在 dist/main 下，项目根即其上两级；打包态资产在 resources/assets。
  const assets = app.isPackaged
    ? path.join(process.resourcesPath, 'assets')
    : path.join(__dirname, '..', '..', 'assets')
  const extensions = app.isPackaged
    ? path.join(process.resourcesPath, 'extension')
    : path.join(__dirname, '..', '..', 'resources', 'extension')
  const paths: WbPaths = {
    wb_data_dir: data,
    wb_runtime_dir: path.join(data, 'runtime'),
    wb_node_dir: path.join(data, 'runtime', 'node'),
    wb_git_dir: path.join(data, 'runtime', 'PortableGit'),
    wb_python_dir: path.join(data, 'runtime', 'python'),
    wb_pi_agent_dir: path.join(data, 'pi'),
    wb_assets_dir: assets,
    wb_extension_dir: extensions,
    wb_log_dir: path.join(data, 'logs'),
    wb_db_path: path.join(data, 'workbaby.db'),
  }
  ctx = { wb_paths: paths, wb_token: crypto.randomBytes(24).toString('base64url'), wb_port: 0 }
  return paths
}

/** 取上下文；没初始化就抛——装配顺序错误应当立刻炸出来。 */
export function context(): WbContext {
  if (!ctx) throw new Error('context 未初始化：先调 initContext()')
  return ctx
}

export function paths(): WbPaths {
  return context().wb_paths
}

/** 装配完成后回填端口。 */
export function setPort(port: number): void {
  context().wb_port = port
}

export function token(): string {
  return context().wb_token
}
