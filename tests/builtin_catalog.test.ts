//! 内置清单落位：packages 声明的启停改写、内置插件的默认开关、技能落位与目录搬移。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'

// 两个服务都经 context 摸 electron 与数据目录，桩掉 app 即可。
vi.mock('electron', () => ({ app: { isPackaged: false } }))

let root = ''

beforeEach(async () => {
  root = fs.mkdtempSync(path.join(os.tmpdir(), 'wb-catalog-'))
  process.env.LOCALAPPDATA = root
  const { initContext } = await import('../electron/main/context')
  const { paths } = await import('../electron/main/context')
  const { openDb } = await import('../electron/main/repo/db')
  initContext()
  openDb(paths().wb_db_path)
})

afterEach(async () => {
  const { closeDb } = await import('../electron/main/repo/db')
  closeDb()
  fs.rmSync(root, { recursive: true, force: true })
})

describe('内置插件清单', () => {
  it('清单里每个包名唯一，且版本是精确 pin', async () => {
    const { BUILTIN_PLUGINS } = await import('../electron/main/service/plugin_catalog')
    const names = BUILTIN_PLUGINS.map((p) => p.plugin_name)
    expect(new Set(names).size, '内置清单不得有重复包名').toBe(names.length)
    for (const item of BUILTIN_PLUGINS) {
      expect(item.plugin_version, `${item.plugin_name} 必须 pin 精确版本`).toMatch(/^\d+\.\d+\.\d+/)
    }
    expect(BUILTIN_PLUGINS).toHaveLength(12)
  })

  it('停用态写 autoload:false，再启用回到裸来源串且不改顺序', async () => {
    const settings = await import('../electron/main/service/plugin_settings')
    settings.writePackages([])
    settings.declarePackage('pi-web-access', 'npm:pi-web-access@0.42.0', true)
    settings.declarePackage('pi-inspect', 'npm:pi-inspect@0.5.0', false)

    let entries = settings.listPackages()
    expect(entries).toHaveLength(2)
    expect(entries[0]).toBe('npm:pi-web-access@0.42.0')
    expect(entries[1]).toEqual({ source: 'npm:pi-inspect@0.5.0', autoload: false })
    expect(settings.entryEnabled(entries[1])).toBe(false)

    settings.setPackageEnabled('pi-inspect', true)
    entries = settings.listPackages()
    // 顺序不动：原位替换，不然界面上刚点过的行会跳走
    expect(settings.packageNameOf(settings.sourceOf(entries[1]))).toBe('pi-inspect')
    expect(entries[1]).toBe('npm:pi-inspect@0.5.0')

    settings.setPackageEnabled('pi-web-access', false)
    entries = settings.listPackages()
    expect(settings.entryEnabled(entries[0])).toBe(false)
    expect(settings.entryEnabled(entries[1])).toBe(true)
  })

  it('包名解析剥掉 npm: 前缀与版本，作用域包保留 @', async () => {
    const { packageNameOf } = await import('../electron/main/service/plugin_settings')
    expect(packageNameOf('npm:pi-web-access@0.42.0')).toBe('pi-web-access')
    expect(packageNameOf('npm:@juicesharp/rpiv-todo@2.13.9')).toBe('@juicesharp/rpiv-todo')
    expect(packageNameOf('context-mode')).toBe('context-mode')
  })

  it('撤声明后 packages 键消失，不留空数组', async () => {
    const settings = await import('../electron/main/service/plugin_settings')
    settings.writePackages([])
    settings.declarePackage('pi-inspect', 'npm:pi-inspect@0.5.0', true)
    settings.undeclarePackage('pi-inspect')
    const raw = JSON.parse(fs.readFileSync(settings.piSettingsPath(), 'utf8')) as Record<string, unknown>
    expect('packages' in raw, '空清单不该留 [] 键').toBe(false)
  })

  it('npmCommand：内置 node 就绪时写绝对路径，未就绪时一个字不写', async () => {
    const { paths } = await import('../electron/main/context')
    const settings = await import('../electron/main/service/plugin_settings')

    // 初次启动：内置 node 还没解压——留空才是对的，写半成品路径会让 pi 启动时找不到 npm。
    settings.ensureNpmCommand()
    expect(fs.existsSync(settings.piSettingsPath()), '运行时就绪前不该写 settings.json').toBe(false)

    // 运行时到位：npmCommand 必须两条（node 绝对路径 + npm-cli.js）
    const node_dir = paths().wb_node_dir
    fs.mkdirSync(node_dir, { recursive: true })
    fs.writeFileSync(path.join(node_dir, 'node.exe'), '', 'utf8')
    settings.ensureNpmCommand()

    const raw = JSON.parse(fs.readFileSync(settings.piSettingsPath(), 'utf8')) as { npmCommand: string[] }
    expect(raw.npmCommand).toHaveLength(2)
    expect(raw.npmCommand[0]).toMatch(/node\.exe$/)
    expect(raw.npmCommand[0], '必须是沙箱内的 node，不能是裸命令').toBe(path.join(node_dir, 'node.exe'))
    expect(raw.npmCommand[1]).toMatch(/npm-cli\.js$/)
  })
})

describe('内置技能清单', () => {
  it('清单目录都在 assets/skills 下真实存在且带 SKILL.md', async () => {
    const { BUILTIN_SKILLS, builtinSkillsSource } = await import('../electron/main/service/skill_catalog')
    const source = builtinSkillsSource()
    for (const item of BUILTIN_SKILLS) {
      const file = path.join(source, item.skill_dir, 'SKILL.md')
      expect(fs.existsSync(file), `清单列了 ${item.skill_dir} 但资产里没有 SKILL.md`).toBe(true)
    }
  })

  it('清单无重复目录', async () => {
    const { BUILTIN_SKILLS } = await import('../electron/main/service/skill_catalog')
    const dirs = BUILTIN_SKILLS.map((s) => s.skill_dir)
    expect(new Set(dirs).size).toBe(dirs.length)
  })

  it('落位：启用态进 skills，默认停用态直接进禁用区，重启不覆盖已有目录', async () => {
    const { paths } = await import('../electron/main/context')
    const { BUILTIN_SKILLS, builtinSkillsSource } = await import('../electron/main/service/skill_catalog')
    const skill = await import('../electron/main/service/skill_service')

    // 资产目录用真实 assets，落位目标在临时数据目录
    const assets = builtinSkillsSource()
    expect(fs.existsSync(assets), 'assets/skills 应当存在').toBe(true)

    skill.provisionSkills()
    const on_by_default = BUILTIN_SKILLS.filter((s) => s.skill_default_on)
    const off_by_default = BUILTIN_SKILLS.filter((s) => !s.skill_default_on)
    expect(on_by_default.length).toBeGreaterThan(0)

    for (const item of on_by_default) {
      expect(
        fs.existsSync(path.join(paths().wb_pi_agent_dir, 'skills', item.skill_dir, 'SKILL.md')),
        `${item.skill_dir} 应在启用区`,
      ).toBe(true)
    }
    for (const item of off_by_default) {
      expect(
        fs.existsSync(path.join(paths().wb_pi_agent_dir, 'skills_disabled', item.skill_dir, 'SKILL.md')),
        `${item.skill_dir} 应在禁用区`,
      ).toBe(true)
    }

    // 用户改动过某个技能：第二次落位不得覆盖
    const probe = on_by_default[0].skill_dir
    const target = path.join(paths().wb_pi_agent_dir, 'skills', probe, 'SKILL.md')
    fs.writeFileSync(target, '---\nname: edited\n---\n本地改动\n', 'utf8')
    skill.provisionSkills()
    expect(fs.readFileSync(target, 'utf8'), '落位不得覆盖用户改动').toContain('本地改动')
  })

  it('开关靠搬目录：skanl 列表里禁用项仍可见但 enabled=false', async () => {
    const { paths } = await import('../electron/main/context')
    const { BUILTIN_SKILLS } = await import('../electron/main/service/skill_catalog')
    const skill = await import('../electron/main/service/skill_service')

    skill.provisionSkills()
    const probe = BUILTIN_SKILLS.find((s) => s.skill_default_on)!
    const active = path.join(paths().wb_pi_agent_dir, 'skills', probe.skill_dir, 'SKILL.md')

    skill.skillToggle(active, false)
    expect(fs.existsSync(active), '停用后文件不该还在启用区').toBe(false)
    expect(
      fs.existsSync(path.join(paths().wb_pi_agent_dir, 'skills_disabled', probe.skill_dir, 'SKILL.md')),
    ).toBe(true)

    const listed = skill.skillList().skills.find((s) => s.name === probe.skill_dir || s.path.endsWith(probe.skill_dir + path.sep + 'SKILL.md'))
    expect(listed, '停用的技能仍要在清单里可见').toBeTruthy()
    expect(listed!.enabled).toBe(false)

    skill.skillToggle(listed!.path, true)
    expect(fs.existsSync(active)).toBe(true)
  })
})
