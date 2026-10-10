<script setup lang="ts">
// 技能面板：新建 / 导入 / 开关 / 看正文 / 删除。
// 只有开关的面板不叫「能用」：想把自己那套流程固化下来，必须有条不依赖命令行的路。
import { onBeforeUnmount, onMounted, ref } from 'vue'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import { OpenFileDialog } from '../../../wailsjs/go/main/App'
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

// busyId 标记正在操作的行：开关 / 展开 / 删除共用，同一时刻只跑一个，
// 否则连点会打出重复请求，开关状态最后谁赢取决于返回顺序。
const busyId = ref('')

async function toggle(id: string, enabled: boolean) {
  if (busyId.value) return
  busyId.value = id
  err.value = ''
  try {
    await api.skills.toggle(id, enabled)
    await store.loadSkills()
    toast.ok(enabled ? '技能已启用' : '技能已停用')
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busyId.value = ''
  }
}

async function expand(id: string) {
  if (open.value === id) {
    open.value = null
    return
  }
  if (busyId.value) return
  err.value = ''
  busyId.value = id
  try {
    const r = await api.skills.content(id)
    body.value = r.content
    open.value = id
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busyId.value = ''
  }
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

// importing 覆盖真正的导入请求段：原生对话框本身有系统反馈，
// 选完文件后的解析与落盘才是需要转圈的那一段。
const importing = ref(false)
async function importMd() {
  if (importing.value) return
  err.value = ''
  const p = await OpenFileDialog('', '*.md')
  if (!p) return
  importing.value = true
  try {
    const r = await api.skills.import([p])
    if (!r.imported) {
      err.value = '这个文件里没有解析出技能。技能是一个 SKILL.md，或一个 zip 包。'
      return
    }
    await store.loadSkills()
    toast.ok(`已导入 ${r.imported} 个技能`)
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    importing.value = false
  }
}

// 技能包导入：浏览器读 zip 内容转 base64 上传，后端解压落盘。
// 用 <input type=file> 而不是原生对话框：前端要拿到文件字节，对话框只给路径。
const zipInput = ref<HTMLInputElement | null>(null)
function arrayBufferToBase64(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf)
  let bin = ''
  const CHUNK = 0x8000
  for (let i = 0; i < bytes.length; i += CHUNK) {
    bin += String.fromCharCode(...bytes.subarray(i, i + CHUNK))
  }
  return btoa(bin)
}

async function importZipFile(file: File) {
  if (importing.value) return
  err.value = ''
  importing.value = true
  try {
    const data = arrayBufferToBase64(await file.arrayBuffer())
    const r = await api.skills.importZip(file.name, data)
    if (!r.imported) {
      err.value = r.skipped.length
        ? `没有可导入的技能：${r.skipped.join('、')}`
        : '压缩包里没有找到 SKILL.md。技能包里每个技能一个文件夹，各含一个 SKILL.md。'
      return
    }
    await store.loadSkills()
    toast.ok(`已导入 ${r.imported} 个技能`)
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    importing.value = false
    if (zipInput.value) zipInput.value.value = ''
  }
}

function onZipChange(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (f) void importZipFile(f)
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
onBeforeUnmount(() => clearTimeout(confirmTimer))

async function remove(id: string) {
  if (busyId.value) return
  busyId.value = id
  err.value = ''
  try {
    await api.skills.remove(id)
    if (open.value === id) open.value = null
    await store.loadSkills()
    toast.ok('技能已删除')
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busyId.value = ''
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
      <button
        class="btn"
        type="button"
        :class="{ 'is-loading': importing }"
        :disabled="importing"
        @click="zipInput?.click()"
      >
        <AppIcon name="download" size="ic-xs" /> 导入技能包（.zip）
      </button>
      <input
        ref="zipInput"
        type="file"
        accept=".zip"
        hidden
        @change="onZipChange"
      />
      <button
        class="btn"
        type="button"
        :class="{ 'is-loading': importing }"
        :disabled="importing"
        @click="importMd"
      >
        <AppIcon name="doc" size="ic-xs" /> 导入 SKILL.md
      </button>
    </div>
    <p class="hint">
      技能是给助手的「操作秘籍」。写清什么时候用、怎么做，它就会照着做。
      导入支持别人分享的技能包（zip）或单个 SKILL.md 文件。
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
        <button class="btn btn-primary" type="button" :class="{ 'is-loading': busy }" :disabled="busy" @click="submitCreate">
          保存
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
          <!-- 展开区用 button 而不是 div：键盘要能 Tab 到，读屏要知道它是可展开的 -->
          <button
            class="grow main"
            type="button"
            :class="{ 'is-loading': busyId === s.id }"
            :aria-expanded="open === s.id"
            @click="expand(s.id)"
          >
            <h5>
              {{ s.name }}
              <span class="tag">{{ sourceName[s.source] || s.source }}</span>
            </h5>
            <p>{{ s.description || '（无描述）' }}</p>
          </button>
          <button
            class="switch"
            type="button"
            role="switch"
            :aria-checked="s.enabled"
            :aria-label="s.enabled ? `停用技能 ${s.name}` : `启用技能 ${s.name}`"
            :class="{ 'is-on': s.enabled, 'is-loading': busyId === s.id }"
            :title="s.enabled ? '点击停用' : '点击启用'"
            :disabled="busyId === s.id"
            @click.stop="toggle(s.id, !s.enabled)"
          />
          <button
            v-if="s.source !== 'builtin'"
            class="icon-btn is-sm is-danger"
            :class="{ 'is-confirm': confirming === s.id, 'is-loading': busyId === s.id }"
            type="button"
            :title="confirming === s.id ? '再点一次确认删除' : '删除'"
            :disabled="busyId === s.id"
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
