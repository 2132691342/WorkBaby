// 打包主进程与 preload：esbuild 单文件输出；native 与 electron 保持外部依赖。
import { build } from 'esbuild'
import { mkdirSync } from 'node:fs'

mkdirSync('dist/main', { recursive: true })
mkdirSync('dist/preload', { recursive: true })

await build({
  entryPoints: ['electron/main/index.ts'],
  bundle: true,
  platform: 'node',
  format: 'cjs',
  outfile: 'dist/main/index.js',
  external: ['electron', 'better-sqlite3'],
  logLevel: 'info',
})

await build({
  entryPoints: ['electron/preload/index.ts'],
  bundle: true,
  platform: 'node',
  format: 'cjs',
  outfile: 'dist/preload/index.cjs',
  external: ['electron'],
  logLevel: 'info',
})
