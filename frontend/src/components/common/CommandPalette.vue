<script setup lang="ts">
// 命令面板：一个输入框搜全部动作——跳页、全局动作、最近会话、技能命令。
// 用一个统一入口替代「记住每个功能在哪一页」；键盘优先，鼠标可选。
// 复用斜杠命令的数据源（内置 + 引擎技能）与草稿桥（chat.draftOverride），不另造一套。
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useChatStore } from '@/stores/chat'
import { useBuiltinCommands } from '@/composables/useBuiltinCommands'
import { useCommandPalette } from '@/composables/useCommandPalette'
import type { SessionRow, SlashCommand } from '@/types/api'

const router = useRouter()
const { t } = useI18n()
const app = useAppStore()
const chat = useChatStore()
const builtins = useBuiltinCommands()
const palette = useCommandPalette()

/** 面板里的一条命令。run 在选中时执行。 */
interface Cmd {
  id: string
  group: 'nav' | 'action' | 'session' | 'command'
  label: string
  hint: string
  run: () => void | Promise<void>
}

const query = ref('')
const active = ref(0)
const inputEl = ref<HTMLInputElement | null>(null)
const recent = ref<SessionRow[]>([])

// 页面导航：直接从路由表生成，跳转目标与导航 rail 永远一致。
const navCmds = computed<Cmd[]>(() =>
  router
    .getRoutes()
    .filter((r) => r.name && r.meta?.titleKey)
    .map((r) => ({
      id: `nav:${String(r.name)}`,
      group: 'nav' as const,
      label: t(String(r.meta.titleKey)),
      hint: r.path,
      run: () => void router.push({ name: r.name }),
    })),
)

// 全局动作：跨页都用得上的少数几个，不塞进每个页面的角落。
const actionCmds = computed<Cmd[]>(() => {
  const nextTheme = app.theme === 'dark' ? 'light' : 'dark'
  const nextLocale = app.settings['ui.locale'] === 'en' ? 'zh-CN' : 'en'
  return [
    {
      id: 'action:new',
      group: 'action',
      label: t('palette.newSession'),
      hint: t('palette.newSessionHint'),
      run: () => void router.push({ path: '/chat' }),
    },
    {
      id: 'action:theme',
      group: 'action',
      label: t('palette.toggleTheme'),
      hint: t(nextTheme === 'dark' ? 'palette.toDark' : 'palette.toLight'),
      run: () => void app.saveSettings({ 'ui.theme': nextTheme }),
    },
    {
      id: 'action:locale',
      group: 'action',
      label: t('palette.toggleLocale'),
      hint: nextLocale === 'en' ? 'English' : '中文',
      run: () => void app.saveSettings({ 'ui.locale': nextLocale }),
    },
  ]
})

// 最近会话：打开即取，超过 5 条只留最近的。
const sessionCmds = computed<Cmd[]>(() =>
  recent.value.slice(0, 5).map((row) => ({
    id: `session:${row.session_id}`,
    group: 'session' as const,
    label: row.session_title || t('common.untitled'),
    hint: row.session_model || '',
    run: () =>
      void router.push({ path: '/chat', query: { session: row.session_path } }),
  })),
)

// 技能/命令：内置的直接跑，引擎技能把命令词填进输入框（复用斜杠菜单的两种语义）。
const commandCmds = computed<Cmd[]>(() => {
  const out: Cmd[] = builtins.items().map((b) => ({
    id: `cmd:${b.key}`,
    group: 'command' as const,
    label: `/${b.key}`,
    hint: b.desc,
    run: async () => {
      await builtins.run(b.key)
    },
  }))
  const seen = new Set(out.map((c) => c.id.slice(4)))
  for (const c of chat.commands as SlashCommand[]) {
    if (seen.has(c.command_name)) continue
    const isSkill = c.command_name.startsWith('skill:')
    const isPrompt = c.command_source === 'prompt'
    out.push({
      id: `cmd:${c.command_name}`,
      group: 'command',
      label: `/${isSkill ? c.command_name.slice(5) : c.command_name}`,
      hint: isPrompt ? c.command_desc || '' : isSkill ? t('chat.slashSkillTag') : c.command_desc || '',
      run: () => {
        // 提示词模板填整段正文，其余命令填命令词——与斜杠菜单同一语义。
        chat.draftOverride = isPrompt ? c.command_body || '' : `/${c.command_name} `
        void router.push({ path: '/chat' })
      },
    })
  }
  return out
})

const all = computed<Cmd[]>(() => [
  ...navCmds.value,
  ...actionCmds.value,
  ...sessionCmds.value,
  ...commandCmds.value,
])

// 过滤：对显示名与说明做不区分大小写的子串匹配，够用就不上模糊打分。
const filtered = computed<Cmd[]>(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return all.value
  return all.value.filter(
    (c) => c.label.toLowerCase().includes(q) || c.hint.toLowerCase().includes(q),
  )
})

const groups: { key: Cmd['group']; title: string }[] = [
  { key: 'nav', title: t('palette.groupNav') },
  { key: 'action', title: t('palette.groupAction') },
  { key: 'session', title: t('palette.groupSession') },
  { key: 'command', title: t('palette.groupCommand') },
]

const grouped = computed(() =>
  groups
    .map((g) => ({ ...g, items: filtered.value.filter((c) => c.group === g.key) }))
    .filter((g) => g.items.length > 0),
)

// 扁平序：方向键在这上面移动，跨分组连续。
const flat = computed<Cmd[]>(() => grouped.value.flatMap((g) => g.items))

watch(query, () => {
  active.value = 0
})

function move(delta: number): void {
  const total = flat.value.length
  if (total === 0) return
  active.value = (active.value + delta + total) % total
}

async function choose(cmd: Cmd | undefined): Promise<void> {
  if (!cmd) return
  palette.hide()
  query.value = ''
  await cmd.run()
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    move(1)
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    move(-1)
  } else if (event.key === 'Enter') {
    event.preventDefault()
    void choose(flat.value[active.value])
  } else if (event.key === 'Escape') {
    event.preventDefault()
    palette.hide()
  }
}

// 打开即取最近会话：面板要的是「一眼看到最近在干什么」，进来才拉不算慢。
watch(
  () => palette.open.value,
  async (open) => {
    if (!open) return
    active.value = 0
    await nextTick(() => inputEl.value?.focus())
    try {
      const list = await api.sessions()
      recent.value = list.sessions ?? []
    } catch {
      recent.value = []
    }
  },
)

onMounted(() => {
  if (palette.open.value) void nextTick(() => inputEl.value?.focus())
})
</script>

<template>
  <transition name="wb-fade">
    <div v-if="palette.open.value" class="palette-backdrop" @click="palette.hide()">
      <div class="palette" @click.stop @keydown="onKeydown">
        <div class="palette-search">
          <AppIcon name="search" :size="16" class="palette-lead" />
          <input
            ref="inputEl"
            v-model="query"
            class="palette-input"
            :placeholder="t('palette.placeholder')"
            autocomplete="off"
          />
          <kbd class="palette-esc">Esc</kbd>
        </div>

        <div class="palette-list">
          <template v-if="flat.length">
            <section v-for="g in grouped" :key="g.key">
              <p class="palette-group">{{ g.title }}</p>
              <button
                v-for="cmd in g.items"
                :key="cmd.id"
                class="palette-row"
                :class="{ active: flat[active]?.id === cmd.id }"
                @mouseenter="active = flat.findIndex((c) => c.id === cmd.id)"
                @click="void choose(cmd)"
              >
                <span class="palette-label">{{ cmd.label }}</span>
                <span class="palette-hint">{{ cmd.hint }}</span>
              </button>
            </section>
          </template>
          <p v-else class="palette-empty">{{ t('palette.empty') }}</p>
        </div>
      </div>
    </div>
  </transition>
</template>

<style scoped>
.palette-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  justify-content: center;
  padding-top: 12vh;
  background: rgb(15 23 42 / 0.32);
}
.palette {
  width: min(620px, 92vw);
  max-height: 70vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--wb-raised);
  border: 1px solid var(--wb-line-strong);
  border-radius: var(--wb-xl);
  box-shadow: var(--wb-shadow-lg);
}
.palette-search {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3) var(--wb-sp-4);
  border-bottom: 1px solid var(--wb-line);
}
.palette-lead {
  color: var(--wb-faint);
  flex: none;
}
.palette-input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.palette-esc {
  flex: none;
  padding: 2px 6px;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-sm);
}
.palette-list {
  overflow-y: auto;
  padding: var(--wb-sp-2);
}
.palette-group {
  margin: var(--wb-sp-2) var(--wb-sp-2) var(--wb-sp-1);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.palette-row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border: none;
  border-radius: var(--wb-md);
  background: transparent;
  cursor: pointer;
  text-align: left;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

/* 整行可点项：按下只缩不放（放大会溢出被裁），缩放刻度复用全站令牌 */
.palette-row:active {
  transform: scale(var(--wb-press-row-active));
  transition-duration: 60ms;
}
.palette-row.active {
  background: var(--wb-selected);
}
.palette-label {
  flex: none;
  max-width: 55%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.palette-hint {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-xs);
  color: var(--wb-faint);
}
.palette-empty {
  margin: 0;
  padding: var(--wb-sp-6);
  text-align: center;
  font-size: var(--wb-fs-sm);
  color: var(--wb-faint);
}
</style>
