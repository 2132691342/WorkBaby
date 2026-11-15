// i18n 文案的编译门禁：每条 locale 字符串都必须能被 vue-i18n 的编译器吃掉。
// 背景：`@` `%` `|` 这些字符在 ICU 消息语法里是保留字，裸写会让整条消息
// 编译失败——界面不报错、不显示，只是静默少一段文案。这种失败肉眼很难发现，
// 所以在构建流程里拦一道。正确的转义写法是 `{'@'}`。
//
// 用法：node scripts/check-i18n.mjs   （退出码非 0 即有坏文案）
import { createRequire } from 'node:module'
import { readdirSync, statSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const require = createRequire(import.meta.url)
const { baseCompile } = require('@intlify/message-compiler')

const here = dirname(fileURLToPath(import.meta.url))
const localesDir = join(here, '..', 'src', 'locales')

// 一条文案占一整行：键可以是裸键、单引号键或双引号键，值用单/双引号包住。
// 三种键都要认——en 的 locale 全部写成 "key": "value"，只认裸键会让整批 en
// 文案一条都不进检查。
// 行锚定（^...$）不只是为了整齐：注释行永远不满足「键 + 冒号 + 引号值 + 行尾」，
// 所以不必剥注释——`http://` 这种含 `//` 的值也不会被误当注释切掉。
// 合法行尾允许尾随逗号与行尾注释。值里的转义（\" 之类）用 \\. 跳过，
// 否则提前撞上转义引号会把一条文案截成两半。
// 只覆盖字面量；模板串与拼接串不进 locale 文件，不需要解析 TS 模块。
const ENTRY = /^\s*(?:([A-Za-z0-9_.$]+)|'([^']*)'|"([^"]*)")\s*:\s*(?:'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)")\s*,?\s*(?:\/\/.*)?$/gm

const files = []
;(function walk(dir) {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name)
    if (statSync(full).isDirectory()) walk(full)
    else if (name.endsWith('.ts')) files.push(full)
  }
})(localesDir)

let checked = 0
const bad = []

for (const file of files) {
  const src = readFileSync(file, 'utf8')
  let m
  while ((m = ENTRY.exec(src)) !== null) {
    const key = m[1] ?? m[2] ?? m[3]
    const value = m[4] ?? m[5]
    checked++
    try {
      baseCompile(value, { onError: (err) => { throw new Error(err.message) } })
    } catch (err) {
      bad.push({ file: file.replace(here, ''), key, value, reason: String(err.message).split('\n')[0] })
    }
  }
}

for (const item of bad) {
  console.error(`✗ ${item.file}  ${item.key} = ${JSON.stringify(item.value)}\n    ${item.reason}`)
}
console.log(`i18n: ${checked} 条文案，${bad.length} 条编译失败`)
process.exit(bad.length === 0 ? 0 : 1)
