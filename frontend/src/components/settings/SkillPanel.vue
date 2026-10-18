<script setup lang="ts">
// 技能面板：列的就是 PI 会加载的那些技能（用户目录 / 工作区 / 插件包自带）。
// 启停靠把目录移出 PI 的扫描范围，不是在自己的库里记一个开关。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { SkillVO } from '../../types/api'
import { OpenFileDialog } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'

const store = useSettingsStore()
const toast = useToastStore()
const busy = ref('')
const opened = ref<SkillVO | null>(null)
const content = ref('')
const filter = ref('')

const SOURCE: Record<string, { label: string; hint: string }> = {
  global: { label: '我的技能', hint: '~/.pi/agent/skills' },
  workspace: { label: '工作区', hint: '项目的 .pi/skills' },
  package: { label: '插件包', hint: '随包安装，界面只读' },
}

const list = computed(() => {
  const q = filter.value.trim().toLowerCase()
  const items = store.skills ?? []
  if (!q) return items
  return items.filter(
    (s) => s.name.toLowerCase().includes(q) || (s.description || '').toLowerCase().includes(q),
  )
})

const grouped = computed(() => {
  const out: Array<{ key: string; label: string; hint: string; items: SkillVO[] }> = []
  for (const key of ['global', 'workspace', 'package']) {
    const items = list.value.filter((s) => s.source === key)
    if (items.length) out.push({ key, label: SOURCE[key].label, hint: SOURCE[key].hint, items })
  }
  return out
})

async function reload() {
  await store.loadSkills()
}

async function toggle(s: SkillVO) {
  if (busy.value) return
  busy.value = s.id
  try {
    await api.skills.toggle(s.id, !s.enabled)
    await reload()
    toast.ok(s.enabled ? `已停用「${s.name}」` : `已启用「${s.name}」`)
  } catch (e) {
    toast.bad(`切换失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function open(s: SkillVO) {
  opened.value = s
  content.value = ''
  try {
    const r = await api.skills.content(s.id)
    content.value = r.content
  } catch (e) {
    content.value = `读取失败：${(e as Error)?.message || '请重试'}`
  }
}

async function remove(s: SkillVO) {
  if (busy.value) return
  busy.value = `del:${s.id}`
  try {
    await api.skills.remove(s.id)
    await reload()
    toast.ok(`已删除「${s.name}」`)
  } catch (e) {
    toast.bad(`删除失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function importFolder() {
  if (busy.value) return
  busy.value = 'import'
  try {
    const path = await OpenFileDialog('选择技能目录里的 SKILL.md', 'SKILL.md')
    if (!path) return
    const r = await api.skills.import([path])
    await reload()
    TOAST_IMPORT(r)
  } catch (e) {
    toast.bad(`导入失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

function TOAST_IMPORT(r: { imported: number; skipped: string[] }) {
  if (r.imported) toast.ok(`导入了 ${r.imported} 个技能`)
  if (r.skipped?.length) toast.info(`跳过：${r.skipped.slice(0, 3).join('；')}`)
  if (!r.imported && !r.skipped?.length) toast.info('没有找到可导入的技能')
}

onMounted(() => void reload())
</script>

<template>
  <div class="panel">
    <header class="pn-head">
      <div>
        <h3>技能</h3>
        <p class="pn-sub">这里是 PI 真正会加载的技能目录；停用只是把它移出扫描范围，文件还在。</p>
      </div>
      <div class="acts">
        <input v-model="filter" class="input search" placeholder="搜索技能" />
        <button class="btn" type="button" :class="{ 'is-loading': busy === 'import' }" :disabled="!!busy" @click="importFolder">
          导入
        </button>
        <button class="btn" type="button" :disabled="!!busy" @click="reload">刷新</button>
      </div>
    </header>

    <div v-if="!store.skills.length" class="empty card">
      <span class="em-ic"><AppIcon name="book" /></span>
      <b>还没有技能</b>
      <p>技能是给助手的操作手册：一个目录里放一个 SKILL.md，它会在需要时自己读。</p>
      <button class="btn" type="button" @click="importFolder">导入一个技能</button>
    </div>

    <section v-for="g in grouped" :key="g.key" class="group">
      <div class="g-head">
        <b>{{ g.label }}</b>
        <span class="muted mono">{{ g.hint }}</span>
        <span class="sp" />
        <span class="muted">{{ g.items.length }} 个</span>
      </div>
      <div class="rows">
        <article v-for="s in g.items" :key="s.id" class="row card" :class="{ 'is-off': !s.enabled }">
          <button class="r-main" type="button" @click="open(s)">
            <b>{{ s.name }}</b>
            <span class="r-desc">{{ s.description || '（没有说明）' }}</span>
          </button>
          <span class="tag" :class="{ ok: s.enabled }">{{ s.enabled ? '启用' : '停用' }}</span>
          <button
            v-if="s.source !== 'package'"
            class="btn btn-sm"
            type="button"
            :class="{ 'is-loading': busy === s.id }"
            :disabled="!!busy"
            @click="toggle(s)"
          >
            {{ s.enabled ? '停用' : '启用' }}
          </button>
          <button
            v-if="s.source === 'global'"
            class="btn btn-sm danger"
            type="button"
            :class="{ 'is-loading': busy === `del:${s.id}` }"
            :disabled="!!busy"
            @click="remove(s)"
          >
            删除
          </button>
        </article>
      </div>
    </section>

    <Transition name="scrim">
      <div v-if="opened" class="scrim" @click.self="opened = null">
        <div class="drawer">
          <header class="dr-head">
            <b>{{ opened.name }}</b>
            <span class="muted mono">{{ opened.location }}</span>
            <span class="sp" />
            <button class="icon-btn" type="button" title="关闭" @click="opened = null">
              <AppIcon name="close" size="ic-sm" />
            </button>
          </header>
          <pre class="dr-body">{{ content || '读取中…' }}</pre>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}
.pn-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
}
.pn-head h3 {
  font-size: var(--wb-fs-lg);
  color: var(--wb-ink);
}
.pn-sub {
  margin-top: 4px;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.acts {
  display: flex;
  gap: var(--wb-sp-2);
  align-items: center;
}
.search {
  width: 180px;
  height: var(--wb-ctl-h-sm);
}
.group {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.g-head {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.g-head b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.sp {
  flex: 1;
}
.rows {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-3) var(--wb-sp-4);
}
.row.is-off {
  opacity: 0.62;
}
.r-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
  border: none;
  background: none;
  text-align: left;
  cursor: pointer;
  padding: 0;
}
.r-main b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.r-desc {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8) var(--wb-sp-4);
  text-align: center;
}
.em-ic {
  width: var(--wb-tile);
  height: var(--wb-tile);
  border-radius: var(--wb-radius);
  display: grid;
  place-items: center;
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
}
.empty p {
  color: var(--wb-muted);
  font-size: var(--wb-fs-xs);
  max-width: 440px;
  line-height: 1.7;
}
.scrim {
  position: fixed;
  inset: 0;
  background: var(--wb-scrim);
  display: flex;
  justify-content: flex-end;
  z-index: 60;
}
.drawer {
  width: min(620px, 92vw);
  background: var(--wb-surface);
  border-left: 1px solid var(--wb-border);
  display: flex;
  flex-direction: column;
  animation: drawer-in var(--wb-dur) var(--wb-ease);
}
@keyframes drawer-in {
  from {
    transform: translateX(16px);
    opacity: 0.6;
  }
}
.dr-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3) var(--wb-sp-4);
  border-bottom: 1px solid var(--wb-line);
}
.dr-body {
  flex: 1;
  overflow: auto;
  margin: 0;
  padding: var(--wb-sp-4);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  line-height: 1.7;
  color: var(--wb-ink-2);
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
