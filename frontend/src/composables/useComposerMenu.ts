// 输入区的两个弹出菜单：`/` 命令 与 `@` 文件引用，同一时刻只开一个。
// 拆出来是因为菜单的开关时机与输入区的发送逻辑无关，
// 放一起会让「什么时候弹菜单」和「回车干什么」纠缠不清。
import { computed, ref, watch } from 'vue'
import type { ComputedRef, Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '@/stores/chat'
import { useBuiltinCommands, type BuiltinKey } from '@/composables/useBuiltinCommands'
import type { SlashCommand } from '@/types/api'

/** 菜单里的一项。builtin 由界面执行，engine 只是填进输入框。 */
export interface MenuItem {
  /** 稳定键：内置命令用 BuiltinKey，引擎命令用 name */
  key: string
  /** 显示名（不带斜杠） */
  name: string
  /** 一句话说明 */
  desc: string
  /** 来源决定选中后做什么 */
  source: 'builtin' | 'engine'
  /** 选中后真正填进输入框的命令词；缺省用 name（显示名与命令词不同的只有技能命令） */
  fill?: string
}

type MenuKind = 'slash' | 'mention' | null

/**
 * 绑定两个菜单到草稿。
 *
 * pickPrompt 在选中引擎命令时被调用，由输入区决定「怎么填进框、焦点放哪」；
 * 内置命令由这里直接执行（它不需要输入框参与）。
 */
export function useComposerMenu(
  draft: Ref<string>,
  pickPrompt: (name: string) => void,
) {
  const chat = useChatStore()
  const builtins = useBuiltinCommands()
  const { t } = useI18n()

  const menu = ref<MenuKind>(null)
  const menuIndex = ref(0)
  const busyCmd = ref('')

  /** 输入到一半的 @ 触发引用菜单；`/` 开头且还没空格时触发命令菜单。 */
  function detect(value: string): void {
    if (/@\S*$/.test(value)) {
      menu.value = 'mention'
      return
    }
    if (value.startsWith('/') && !value.includes(' ')) {
      menu.value = 'slash'
      menuIndex.value = 0
      return
    }
    menu.value = null
  }

  watch(draft, detect)

  /**
   * 过滤词变了就把高亮拉回第一项。
   *
   * 不这么做的话：候选列表从 8 项缩到 2 项，而高亮还停在第 7 项——
   * 此时按回车选中的是一个看不见的条目（或者什么都没发生）。
   */
  watch(
    () => draft.value.slice(1),
    () => {
      if (menu.value === 'slash') menuIndex.value = 0
    },
  )

  const mentionOpen = computed(() => menu.value === 'mention')

  /** 合并后的命令表：内置在前、引擎在后，按输入过滤。 */
  const slashItems: ComputedRef<MenuItem[]> = computed(() => {
    const query = draft.value.slice(1).toLowerCase()
    const builtinItems = builtins.items().map((b) => ({ ...b, source: 'builtin' as const }))
    // 引擎也会报出与内置同名的命令（如 omp 的 compact / export）：
    // 同一个动作出现两次只会让人犹豫点哪个。内置的语义更明确，留它。
    const builtinKeys = new Set<string>(builtinItems.map((b) => b.key))
    const engineItems: MenuItem[] = chat.commands
      .filter((c: SlashCommand) => !builtinKeys.has(c.name))
      .map((c: SlashCommand) => {
        // pi 把技能报成 skill:名字：显示名去掉前缀，选中时仍填完整命令。
        const isSkill = c.name.startsWith('skill:')
        const base = c.description || c.input?.hint || ''
        return {
          key: c.name,
          name: isSkill ? c.name.slice('skill:'.length) : c.name,
          fill: c.name,
          desc: isSkill ? (base ? `${t('chat.slashSkillTag')} · ${base}` : t('chat.slashSkillTag')) : base,
          source: 'engine' as const,
        }
      })
    const merged: MenuItem[] = [...builtinItems, ...engineItems]
    if (!query) return merged
    return merged.filter((item) => item.name.toLowerCase().includes(query))
  })

  // 斜杠菜单只要处于「/ 开头」状态就显示：没有匹配项时也要告诉用户
  // 「没有匹配的」，而不是菜单默默消失——后者看起来像按了没反应。
  const slashOpen = computed(() => menu.value === 'slash')

  /** 跑一条内置命令，把结果话术转成提示。 */
  async function runBuiltin(key: string): Promise<void> {
    busyCmd.value = key
    try {
      const note = await builtins.run(key as BuiltinKey)
      // 空串表示「不需要提示」（例如只是把目光引到工具条）。
      if (note) chat.pushNotice('info', note)
    } catch (err) {
      chat.promptError = err instanceof Error ? err.message : String(err)
    } finally {
      busyCmd.value = ''
    }
  }

  /** 选中一条：内置的直接跑，引擎的交给输入区去填。 */
  function applyItem(item: MenuItem): void {
    menu.value = null
    if (item.source === 'engine') {
      pickPrompt(item.fill ?? item.name)
      return
    }
    void runBuiltin(item.key)
  }

  /** 上下移动选中项。 */
  function move(delta: number): void {
    const total = slashItems.value.length
    if (total === 0) return
    menuIndex.value = (menuIndex.value + delta + total) % total
  }

  function close(): void {
    menu.value = null
  }

  function openMention(): void {
    menu.value = 'mention'
  }

  return {
    menu,
    menuIndex,
    busyCmd,
    mentionOpen,
    slashOpen,
    slashItems,
    applyItem,
    move,
    close,
    openMention,
  }
}
