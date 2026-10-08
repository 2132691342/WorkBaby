<script setup lang="ts">
// 技能面板：新建 / 导入 / 开关 / 看正文 / 删除。
// 只有开关的面板不叫「能用」：想把自己那套流程固化下来，必须有条不依赖命令行的路。
import { onMounted, ref } from 'vue'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import { OpenFileDialog, OpenDirectoryDialog } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'
import PageState from '../common/PageState.vue'

const store = useSettingsStore()
const toast = useToastStore()
const open = ref<string | null>(null)
const body = ref('')
const busy = ref(false)
const err = ref('')
const creating = ref(false)
const draft = ref({ name: '', description: '', body: '' })

const TEMPLATE = `## 什么时候用

（写清触发场景，例如：用户提到要做周报时）

## 怎么做

1. 第一步…
2. 第二步…

## 注意

（容易出错的地方、需要的工具）`

const sourceName: Record<string, string> = {
  builtin: '内置',
  global: '我的',
  workspace: '工作区',
}

async function toggle(id: string, enabled: boolean) {
  err.value = ''
  try {
    await api.skills.toggle(id, enabled)
    await store.loadSkills()
    toast.ok(enabled ? '技能已启用' : '技能已停用')
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function expand(id: string) {
  err.value = ''
  if (open.value === id) {
    open.value = null
    return
  }
  const r = await api.skills.content(id)
  body.value = r.content
  open.value = id
}

function startCreate() {
  creating.value = true
  err.value = ''
  draft.value = { name: '', description: '', body: TEMPLATE }
}

async function submitCreate() {
  err.value = ''
  if (!draft.value.name.trim()) {
    err.value = '先给它起个名字（只能用小写字母、数字和连字符）'
    return
  }
  busy.value = true
  try {
    await api.skills.create({ ...draft.value })
    creating.value = false
    await store.loadSkills()
    toast.ok('技能已创建')
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function importFiles(fromDir: boolean) {
  err.value = ''
  try {
    const p = fromDir ? await OpenDirectoryDialog('') : await OpenFileDialog('', '')
    if (!p) return
    const r = await api.skills.import([p])
    if (!r.imported) {
      err.value = '没找到 SKILL.md。技能是一个文件夹，里面放着 SKILL.md 文件。'
      return
    }
    await store.loadSkills()
    toast.ok(`已导入 ${r.imported} 个技能`)
  } catch (e) {
    err.value = (e as Error).message
  }
}

// 删除两段式确认：与模型服务删除同一交互
const confirming = ref('')
let confirmTimer = 0
function askRemove(id: string) {
  if (confirming.value === id) {
    confirming.value = ''
    void remove(id)
    return
  }
  confirming.value = id
  clearTimeout(confirmTimer)
  confirmTimer = window.setTimeout(() => (confirming.value = ''), 3000)
}

async function remove(id: string) {
  err.value = ''
  try {
    await api.skills.remove(id)
    if (open.value === id) open.value = null
    await store.loadSkills()
    toast.ok('技能已删除')
  } catch (e) {
    err.value = (e as Error).message
  }
}

onMounted(() => store.loadSkills())
</script>

<template>
  <div class="skills">
    <div class="toolbar">
      <button class="btn btn-primary" type="button" @click="startCreate">
        <AppIcon name="plus" size="ic-xs" /> 新建技能
      </button>
      <button class="btn" type="button" @click="importFiles(false)">
        <AppIcon name="download" size="ic-xs" /> 导入 SKILL.md
      </button>
      <button class="btn" type="button" @click="importFiles(true)">
        <AppIcon name="folder" size="ic-xs" /> 导入文件夹
      </button>
    </div>
    <p class="hint">
      技能是给助手的「操作秘籍」。写清什么时候用、怎么做，它就会照着做。
    </p>

    <p v-if="err" class="alert is-bad">{{ err }}</p>

    <div v-if="creating" class="card p-sm draft">
      <h3>新建技能</h3>
      <div class="field">
        <label>名字</label>
        <input v-model="draft.name" class="input" placeholder="例如 monthly-report" />
        <span class="hint">只能用小写字母、数字和连字符</span>
      </div>
      <div class="field">
        <label>一句话说明</label>
        <input v-model="draft.description" class="input" placeholder="做什么用，什么时候会用它" />
      </div>
      <div class="field">
        <label>怎么做</label>
        <textarea v-model="draft.body" class="textarea" rows="12" />
      </div>
      <div class="acts">
        <button class="btn" type="button" @click="creating = false">取消</button>
        <button class="btn btn-primary" type="button" :disabled="busy" @click="submitCreate">
          {{ busy ? '保存中…' : '保存' }}
        </button>
      </div>
    </div>

    <PageState
      :loading="store.skillsLoading"
      :error="store.error"
      :empty="!store.skills.length && !creating"
      empty-title="还没有技能"
      empty-sub="点右上角「新建技能」写一个，或导入别人分享的 SKILL.md"
    >
      <div class="rowlist">
        <div
          v-for="s in store.skills"
          :key="s.id"
          class="rli skill"
          :class="{ 'is-open': open === s.id }"
        >
          <div class="grow main" @click="expand(s.id)">
            <h5>
              {{ s.name }}
              <span class="tag">{{ sourceName[s.source] || s.source }}</span>
            </h5>
            <p>{{ s.description || '（无描述）' }}</p>
          </div>
          <button
            class="switch"
            :class="{ 'is-on': s.enabled }"
            type="button"
            :title="s.enabled ? '点击停用' : '点击启用'"
            @click="toggle(s.id, !s.enabled)"
          />
          <button
            v-if="s.source !== 'builtin'"
            class="icon-btn is-sm is-danger"
            :class="{ 'is-confirm': confirming === s.id }"
            type="button"
            :title="confirming === s.id ? '再点一次确认删除' : '删除'"
            @click.stop="askRemove(s.id)"
          >
            <AppIcon name="trash" size="ic-xs" />
          </button>
        </div>
      </div>
      <pre v-if="open" class="skill-body"><code>{{ body }}</code></pre>
    </PageState>
  </div>
</template>

<style scoped>
.skills {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  min-width: 0;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
  min-width: 0;
}
.hint {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.draft {
  margin-bottom: var(--wb-sp-3);
}
.field {
  display: grid;
  gap: var(--wb-sp-1);
  margin-bottom: var(--wb-sp-3);
  min-width: 0;
}
.field label {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.acts {
  display: flex;
  gap: var(--wb-sp-2);
  justify-content: flex-end;
}
.skill {
  cursor: pointer;
}
/* 技能行是「标题 + 开关 + 删除」的紧凑行：不能吃 .grow 的整行独占规则；
   flex-basis 必须 0——auto 会按最长内容（nowrap 描述）预估宽度，把开关挤到下一行 */
.skill .main {
  flex: 1 1 0;
  min-width: 0;
  cursor: pointer;
}
/* 删除钮走全局 .icon-btn（五态齐全），这里只叠「二次确认」的常亮态：
   scoped 重定义 .icon-btn 会连带抹掉 :active 缩放 / :disabled / .is-loading 三态。 */
.icon-btn.is-confirm {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
.skill-body {
  margin-top: var(--wb-sp-3);
  max-height: 420px;
  overflow: auto;
  padding: var(--wb-sp-4);
  border-radius: var(--wb-radius);
  background: var(--wb-surface-2);
  border: 1px solid var(--wb-border);
  font-size: var(--wb-fs-sm);
  white-space: pre-wrap;
}
</style>
