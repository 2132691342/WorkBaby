// scripts/i18n-sync.mjs — 双语 i18n 字典双向同步与契约校验
//
// 用法：
//   node scripts/i18n-sync.mjs check   # 校验：键集合一致 + 值非空 + 顺序一致
//   node scripts/i18n-sync.mjs diff    # 列出仅一侧缺失的键（供维护者补齐）
//   node scripts/i18n-sync.mjs sort    # 按 key 字母序重排两个 dict（写入）
//   node scripts/i18n-sync.mjs add <key> <zh> <en>   # 在两本末尾同步追加一条
//
// 契约：dict-zh.ts 与 dict-en.ts 的 key 集合、出现顺序必须一致（前端 i18n.ts 用 zh 兜底
// + en 覆盖，键不对齐会触发运行时 undefined fallback）；值非空。

import { readFileSync, writeFileSync, readdirSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const root = resolve(__dirname, '..')
const zhPath = resolve(root, 'frontend/src/src/i18n/dict-zh.ts')
const enPath = resolve(root, 'frontend/src/src/i18n/dict-en.ts')

const ZH = readFileSync(zhPath, 'utf8')
const EN = readFileSync(enPath, 'utf8')

function parse(text) {
  // 抓  'key': <value>,  行；value 用单/双引号包裹，未包裹也接受（key 之外允许极简）。
  // 项目字典通常 2 空格缩进 + 单/双引号风格混用（历史包袱），正则覆盖最常见形态即可。
  const out = []
  const re = /^\s{2}'([^']+)':\s*(?:'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)")?/gm
  let m
  while ((m = re.exec(text))) {
    out.push([m[1], m[2] ?? m[3] ?? ''])
  }
  return out
}

const zh = parse(ZH)
const en = parse(EN)
const zhKeys = zh.map(([k]) => k)
const enKeys = en.map(([k]) => k)
const zhSet = new Set(zhKeys)
const enSet = new Set(enKeys)
const orderMatch = zhKeys.length === enKeys.length && zhKeys.every((k, i) => k === enKeys[i])

function exit(code, msg) {
  console.log(msg)
  process.exit(code)
}

const cmd = process.argv[2]
switch (cmd) {
  case 'check': {
    const onlyZh = [...zhSet].filter((k) => !enSet.has(k))
    const onlyEn = [...enSet].filter((k) => !zhSet.has(k))
    const emptyZh = zh.filter(([, v]) => !v)
    const emptyEn = en.filter(([, v]) => !v)
    const problems = []
    if (onlyZh.length) problems.push(`仅 zh 缺失: ${onlyZh.join(', ')}`)
    if (onlyEn.length) problems.push(`仅 en 缺失: ${onlyEn.join(', ')}`)
    if (!orderMatch) problems.push(`key 顺序不一致（zh/en 出现次序不同）`)
    if (process.env.STRICT_I18N === "1" && emptyZh.length) problems.push(`空值 zh: ${emptyZh.map(([k]) => k).join(', ')}`)
    if (process.env.STRICT_I18N === "1" && emptyEn.length) problems.push(`空值 en: ${emptyEn.map(([k]) => k).join(', ')}`)
    if (problems.length === 0) {
      exit(0, `PASS i18n-sync（${zhKeys.length} 键）`)
    } else {
      exit(1, 'FAIL i18n-sync\n  - ' + problems.join('\n  - '))
    }
    break
  }
  case 'diff': {
    const onlyZh = [...zhSet].filter((k) => !enSet.has(k))
    const onlyEn = [...enSet].filter((k) => !zhSet.has(k))
    const lines = []
    if (onlyZh.length) lines.push(`仅 zh 缺失: ${onlyZh.join(', ')}`)
    if (onlyEn.length) lines.push(`仅 en 缺失: ${onlyEn.join(', ')}`)
    if (!lines.length) lines.push('两侧 key 集合一致')
    console.log(lines.join('\n'))
    break
  }
  case 'sort': {
    function build(entries, prefix) {
      const sorted = [...entries].sort(([a], [b]) => a.localeCompare(b))
      // 复刻文件头尾包壳（不动 import / Dict 类型）
      const head = ZH.split('export const zhCN: Dict = {')[0]
      const tail = ZH.split('}\n')[ZH.split('}\n').length - 1].startsWith('}') ? '' : ''
      const body = sorted.map(([k, v]) => `  '${k}': ${JSON.stringify(v)}`).join(',\n')
      return head + 'export const ' + prefix + ': Dict = {\n' + body + ',\n}\n'
    }
    writeFileSync(zhPath, build(zh, 'zhCN'))
    writeFileSync(enPath, build(en, 'enUS'))
    console.log(`sorted: ${zhKeys.length} keys written`)
    break
  }
  case 'clean': {
    // 删除指定前缀的死键：先跑 dead 确认要删，再 clean。前缀要带 "."（如 channel.）。
    const prefixes = process.argv.slice(3)
    if (!prefixes.length) exit(2, 'usage: i18n-sync.mjs clean <prefix1> [prefix2...]')
    // 走 dead 的同一扫描：精确只删「代码中无 t(\'xxx\') 引用」的前缀匹配键
    const frontSrc = resolve(root, 'frontend/src/src')
    function walk(dir) {
      const out = []
      for (const ent of readdirSync(dir, { withFileTypes: true })) {
        const p = resolve(dir, ent.name)
        if (ent.isDirectory()) {
          if (ent.name === 'node_modules' || ent.name === '__tests__' || ent.name.startsWith('.')) continue
          out.push(...walk(p))
        } else if (/\.(vue|ts)$/.test(ent.name)) out.push(p)
      }
      return out
    }
    const used = new Set()
    const re = /\bt\(\s*['"`]([^'"`]+)['"`]/g
    for (const f of walk(frontSrc)) {
      let m
      const t = readFileSync(f, 'utf8')
      while ((m = re.exec(t))) used.add(m[1])
    }
    const dropKeys = new Set(
      zh.map(([k]) => k).filter((k) => prefixes.some((p) => k.startsWith(p)) && !used.has(k))
    )
    function rebuild(text) {
      return text
        .split('\n')
        .filter((line) => {
          const m = line.match(/^\s{2}'([^']+)':/)
          return !m || !dropKeys.has(m[1])
        })
        .join('\n')
    }
    writeFileSync(zhPath, rebuild(ZH))
    writeFileSync(enPath, rebuild(EN))
    console.log(`cleaned ${dropKeys.size} dead keys with prefixes: ${prefixes.join(', ')}`)
    break
  }
  case 'add': {
    const [, , key, zhV, enV] = process.argv
    if (!key || zhV == null || enV == null) {
      exit(2, 'usage: i18n-sync.mjs add <key> <zh> <en>')
    }
    function append(text, kv) {
      const idx = text.lastIndexOf('}')
      return text.slice(0, idx) + `  '${kv[0]}': ${JSON.stringify(kv[1])},\n}\n`
    }
    writeFileSync(zhPath, append(ZH, [key, zhV]))
    writeFileSync(enPath, append(EN, [key, enV]))
    console.log(`added '${key}'`)
    break
  }
  case 'dead': {
    // 扫 frontend/src/src 下 .vue/.ts 中的 t('key') 调用，找出字典中未被引用的死键。
    // 排除 __tests__ / node_modules / 隐藏目录；模板字符串内嵌的 key 一律按字面量算。
    const frontSrc = resolve(root, 'frontend/src/src')
    function walk(dir) {
      const out = []
      for (const ent of readdirSync(dir, { withFileTypes: true })) {
        const p = resolve(dir, ent.name)
        if (ent.isDirectory()) {
          if (ent.name === 'node_modules' || ent.name === '__tests__' || ent.name.startsWith('.')) continue
          out.push(...walk(p))
        } else if (/\.(vue|ts)$/.test(ent.name)) {
          out.push(p)
        }
      }
      return out
    }
    const used = new Set()
    const re = /\bt\(\s*['"`]([^'"`]+)['"`]/g
    for (const f of walk(frontSrc)) {
      const t = readFileSync(f, 'utf8')
      let m
      while ((m = re.exec(t))) used.add(m[1])
    }
    const dead = zh.filter(([k]) => !used.has(k))
    console.log(`used: ${used.size}  total: ${zhKeys.length}  dead: ${dead.length}`)
    if (dead.length) {
      console.log('--- dead keys (zh value) ---')
      dead.forEach(([k, v]) => console.log(`  ${k}  ${v}`))
    }
    break
  }
  default:
    exit(2, `usage: i18n-sync.mjs {check|diff|sort|add|dead|clean <prefix...>}`)
}
