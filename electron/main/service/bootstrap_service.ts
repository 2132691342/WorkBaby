//! 引导总装：Bootstrap、引擎状态与健康面板的视图组装。
import fs from 'node:fs'
import path from 'node:path'
import { app } from 'electron'
import { paths } from '../context'
import { saveSettings, getSetting, getWorkspaceHistory } from '../repo/settings_repo'
import { broadcast } from '../sse/hub'
import { PI_CAPS, EMPTY_CAPS } from '../domain/types_core'
import type {
  BootstrapVO,
  EngineHealthVO,
  EngineIdentityVO,
  EngineInfoVO,
  EngineLayoutVO,
  EngineStatusVO,
  HealthDataVO,
  RunRowVO,
} from '../domain/types_core'
import { nodeVersion, piInstalled, piVersion, nodeExe, piEntry } from './engine/runtime'
import { currentWorkspace, configFiles, settingsAll } from './settings_service'
import { chatRunInfo, chatRestart } from './chat/chat_service'
import { sessionCount } from './session_service'
import { skillCount } from './skill_service'
import { mcpCount } from './mcp_service'
import { pluginList } from './plugin_service'

const KEY_DETECTED_AT = 'engine.detected_at'

/** pi 引擎身份。 */
export function piIdentity(): EngineIdentityVO {
  const installed = piInstalled()
  return {
    kind: 'pi',
    exe: installed ? piEntry() : '',
    version: installed ? piVersion() : '',
    source: installed ? 'custom' : '',
    channel: 'wb-installer',
    caps: PI_CAPS,
  }
}

/** pi 目录布局。 */
export function piLayout(): EngineLayoutVO {
  const p = paths()
  return {
    kind: 'pi',
    root: p.wb_pi_agent_dir,
    agent: p.wb_pi_agent_dir,
    sessions: path.join(p.wb_pi_agent_dir, 'sessions'),
    skills: path.join(p.wb_pi_agent_dir, 'skills'),
    plugins: path.join(p.wb_pi_agent_dir, 'plugins'),
    models: path.join(p.wb_pi_agent_dir, 'models.json'),
    settings: path.join(p.wb_pi_agent_dir, 'settings.json'),
    exists: fs.existsSync(p.wb_pi_agent_dir),
  }
}

function engineInfo(): EngineInfoVO {
  const identity = piIdentity()
  return { ...identity, installed: piInstalled(), layout: piLayout() }
}

/** 引擎状态总览。 */
export function engineStatus(): EngineStatusVO {
  const run = chatRunInfo()
  const running: RunRowVO[] = run.running
    ? [
        {
          kind: 'pi',
          work_dir: currentWorkspace(),
          pid: run.pid,
          session_id: run.session_id,
          running: true,
          protocol: run.protocol,
          started_at: run.started_at,
          call_count: run.call_count,
          busy: run.busy,
        },
      ]
    : []
  return {
    engines: [engineInfo()],
    running,
    install: {
      pi: piInstalled() ? '' : '点击应用页的「安装」由内置 npm 自动完成',
      omp: '本壳不管理 omp 引擎',
    },
    detecting: false,
    detected_at: Number(getSetting(KEY_DETECTED_AT) ?? 0),
  }
}

/** 重新探测：刷新版本与时间戳。 */
export async function enginesDetect(): Promise<EngineStatusVO> {
  await nodeVersion()
  saveSettings({ [KEY_DETECTED_AT]: String(Date.now()) })
  broadcast('engine:detected', { engine_kind: 'pi' })
  return engineStatus()
}

/** 重启引擎。 */
export async function enginesRestart(_engine?: string): Promise<{ ok: boolean }> {
  chatRestart()
  return { ok: true }
}

/** 停止引擎实例。 */
export async function enginesStop(_engine: string, _work_dir: string): Promise<{ stopped: boolean }> {
  chatRestart()
  return { stopped: true }
}

/** 启动总包。 */
export function bootstrapView(): BootstrapVO {
  const p = paths()
  const identity = piIdentity()
  return {
    version: app.getVersion(),
    engines: [identity],
    caps: { pi: PI_CAPS, omp: EMPTY_CAPS },
    active: 'pi',
    workspace: currentWorkspace(),
    profile: '',
    settings: settingsAll(),
    layouts: { pi: piLayout() },
    workspaces: getWorkspaceHistory(),
    skills: { pi: skillCount() },
    sessions: { pi: sessionCount() },
    install: {
      pi: piInstalled() ? '' : 'npm install @earendil-works/pi-coding-agent',
      omp: '',
    },
  }
}

/** 健康面板。 */
export async function healthView(): Promise<HealthDataVO> {
  const installed = piInstalled()
  const run = chatRunInfo()
  const plugins = pluginList().plugins.length
  const health: EngineHealthVO = {
    engine_kind: 'pi',
    engine_label: 'pi coding agent',
    installed,
    exe_path: installed ? piEntry() : '',
    version: installed ? piVersion() : '',
    channel_kind: 'wb-installer',
    channel_label: '内置安装器',
    latest_version: '',
    outdated: false,
    install_hint: installed ? '' : '到「应用」页点安装',
    running: run.running,
    busy: run.busy,
    pid: run.pid,
    protocol: run.protocol,
    started_at: run.started_at,
    call_count: run.call_count,
    session_count: sessionCount(),
    skill_count: skillCount(),
    plugin_count: plugins,
    mcp_count: mcpCount(),
    session_count_known: true,
    skill_count_known: true,
    plugin_count_known: true,
    mcp_count_known: true,
    layout_resolved: fs.existsSync(paths().wb_pi_agent_dir),
    config_files: configFiles().entries,
    root_dir: paths().wb_pi_agent_dir,
    agent_dir: paths().wb_pi_agent_dir,
    dir_exists: fs.existsSync(paths().wb_pi_agent_dir),
    caps: { ...PI_CAPS },
  }
  if (!installed) health.probe_unknown_reason = 'missing'
  return {
    engines: [health],
    active_kind: 'pi',
    workspace: currentWorkspace(),
    refreshing: false,
    detecting: false,
    ready: installed,
    detected_at: Number(getSetting(KEY_DETECTED_AT) ?? 0),
    total_sessions: health.session_count,
    total_skills: health.skill_count,
    total_plugins: plugins,
    total_mcp: health.mcp_count,
    totals_partial: false,
    live_engines: run.running ? 1 : 0,
    max_engines: 1,
    engine_runs: run.running
      ? [
          {
            engine_kind: 'pi',
            engine_label: 'pi coding agent',
            work_dir: currentWorkspace(),
            process_id: run.pid,
            protocol_version: run.protocol,
            started_at: run.started_at,
            call_count: run.call_count,
            busy: run.busy,
            session_id: run.session_id,
          },
        ]
      : [],
  }
}

/** 引擎可执行文件（日志与自检用）。 */
export function engineExePath(): string {
  return piInstalled() ? piEntry() : nodeExe()
}
