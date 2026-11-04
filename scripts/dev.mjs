//! 开发态脚本：构建主进程 → 起 Vite → 起 Electron；任一退出即全停。
import { spawn, spawnSync } from 'node:child_process'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const isWin = process.platform === 'win32'
const npm = isWin ? 'npm.cmd' : 'npm'

console.log('[wb-dev] 构建主进程 ...')
const build = spawnSync(npm, ['run', 'build:main'], { stdio: 'inherit', shell: isWin })
if (build.status !== 0) {
  console.error('[wb-dev] 主进程构建失败，退出')
  process.exit(build.status ?? 1)
}

console.log('[wb-dev] 启动 Vite ...')
const vite = spawn(npm, ['run', 'dev', '--prefix', 'frontend'], {
  stdio: 'inherit',
  shell: isWin,
})

// 给 Vite 一点起服务的时间再拉 Electron（前端自带连接失败重试，不会白屏死等）。
await new Promise((r) => setTimeout(r, 4000))

console.log('[wb-dev] 启动 Electron ...')
const electronPath = require('electron')
const electron = spawn(electronPath, ['dist/main/index.js'], {
  stdio: 'inherit',
  env: { ...process.env, WB_DEV: '1' },
  shell: false,
})

const stop = new Promise((resolve) => {
  electron.on('exit', (code) => resolve(code))
  vite.on('exit', () => {
    try {
      electron.kill()
    } catch {
      /* 已退出 */
    }
    resolve(0)
  })
})

const code = await stop
try {
  vite.kill()
} catch {
  /* 已退出 */
}
process.exit(typeof code === 'number' ? code : 0)
