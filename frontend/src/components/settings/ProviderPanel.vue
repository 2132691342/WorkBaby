<script setup lang="ts">
// 模型服务面板：读写的就是 PI 的 models.json，界面与 pi 命令共用同一份配置。
import { computed, onMounted, reactive, ref } from 'vue'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { ModelREQ, ProviderVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import PnHead from '../common/PnHead.vue'

const store = useSettingsStore()
const toast = useToastStore()

const APIS = [
  { key: 'openai-completions', label: 'OpenAI 兼容（最常用）' },
  { key: 'openai-responses', label: 'OpenAI Responses' },
  { key: 'anthropic-messages', label: 'Anthropic Messages' },
  { key: 'google-generative-ai', label: 'Google Generative AI' },
  { key: 'azure-openai-responses', label: 'Azure OpenAI Responses' },
]

const editing = ref('')
const busy = ref('')
const picked = ref<ProviderVO | null>(null)

const form = reactive({
  id: '',
  name: '',
  api: 'openai-completions',
  base_url: '',
  api_key: '',
  models: [] as ModelREQ[],
})
const fetched = ref<string[]>([])
const manual = ref('')

const providers = computed(() => store.providers?.providers ?? [])
const path = computed(() => store.providers?.path || '')

function reset() {
  editing.value = ''
  picked.value = null
  form.id = ''
  form.name = ''
  form.api = 'openai-completions'
  form.base_url = ''
  form.api_key = ''
  form.models = []
  fetched.value = []
  manual.value = ''
}

function edit(p: ProviderVO) {
  editing.value = p.id
  picked.value = p
  form.id = p.id
  form.name = p.name
  form.api = p.api || 'openai-completions'
  form.base_url = p.base_url
  form.api_key = ''
  form.models = p.models.map((m) => ({
    id: m.id,
    name: m.name,
    reasoning: m.reasoning,
    vision: m.vision,
    context_window: m.context_window,
    max_tokens: m.max_tokens,
  }))
  fetched.value = []
  manual.value = ''
}

function addModel(id: string) {
  const name = id.trim()
  if (!name || form.models.some((m) => m.id === name)) return
  form.models.push({ id: name, name, reasoning: false, vision: true, context_window: 0, max_tokens: 0 })
}

function addManual() {
  for (const line of manual.value.split(/[\s,]+/)) addModel(line)
  manual.value = ''
}

async function fetchModels() {
  if (busy.value) return
  busy.value = 'fetch'
  try {
    const resp = await api.providers.fetchModels({
      api: form.api,
      base_url: form.base_url,
      api_key: form.api_key || undefined,
      id: form.id || undefined,
    })
    fetched.value = resp.models ?? []
    if (!fetched.value.length) toast.info('上游没有返回模型列表，手动填也行')
  } catch (e) {
    toast.bad(`拉取模型失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function save() {
  if (busy.value) return
  if (!form.name.trim() && !form.id) {
    toast.bad('先给它起个名字')
    return
  }
  busy.value = 'save'
  try {
    await api.providers.upsert({
      id: form.id || undefined,
      name: form.name,
      api: form.api,
      base_url: form.base_url,
      api_key: form.api_key || undefined,
      models: form.models,
    })
    await store.refreshProviderData()
    toast.ok('已写进 PI 的 models.json')
    reset()
  } catch (e) {
    toast.bad(`保存失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function remove(p: ProviderVO) {
  if (busy.value) return
  busy.value = `del:${p.id}`
  try {
    await api.providers.remove(p.id)
    await store.refreshProviderData()
    if (editing.value === p.id) reset()
    toast.ok(`已删除 ${p.name}`)
  } catch (e) {
    toast.bad(`删除失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function test(p: ProviderVO) {
  if (busy.value) return
  busy.value = `test:${p.id}`
  try {
    const r = await api.providers.test(p.id)
    if (r.ok) toast.ok(r.message || '连上了')
    else toast.bad(r.message || '连不上')
  } catch (e) {
    toast.bad(`测试失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function reveal(p: ProviderVO) {
  if (busy.value) return
  busy.value = `key:${p.id}`
  try {
    const r = await api.providers.reveal(p.id)
    toast.ok(`密钥：${r.api_key || '（空）'}`)
  } catch (e) {
    toast.bad(`读取密钥失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function setDefault(p: ProviderVO) {
  const model = p.models[0]?.id
  if (!model) {
    toast.bad('这个服务下还没有模型')
    return
  }
  if (busy.value) return
  busy.value = `def:${p.id}`
  try {
    await api.providers.setDefault(p.id, model)
    await store.refreshBoot()
    await store.loadProviders()
    toast.ok(`默认模型：${model}`)
  } catch (e) {
    toast.bad(`设默认失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

onMounted(() => {
  if (!store.providers) void store.loadProviders()
})
</script>

<template>
  <div class="panel">
    <PnHead icon="cpu" title="模型服务">
      <template #sub>
        读写的就是 PI 的 <span class="mono">{{ path || 'models.json' }}</span>：这里加的，<span class="mono">pi</span> 命令也立刻能用。
      </template>
      <el-button type="primary" @click="reset">新增服务</el-button>
    </PnHead>

    <div v-if="!providers.length" class="empty card">
      <span class="em-ic"><AppIcon name="cpu" /></span>
      <b>还没有任何服务商</b>
      <p>填一个 OpenAI 兼容的地址与密钥即可开始（大多数中转网关都属于这一类）。</p>
      <el-button @click="reset">新增服务</el-button>
    </div>

    <div class="cards">
      <article v-for="p in providers" :key="p.id" class="pcard card" :class="{ 'is-on': editing === p.id }">
        <div class="pc-top">
          <span class="pc-avatar">{{ (p.name || p.id).slice(0, 1).toUpperCase() }}</span>
          <div class="pc-id">
            <b>{{ p.name || p.id }}</b>
            <span class="pc-meta">{{ p.api }} · {{ p.models.length }} 个模型</span>
          </div>
          <span v-if="p.is_default" class="tag ok">默认</span>
        </div>
        <div class="pc-url mono">{{ p.base_url || '（未填地址）' }}</div>
        <div class="pc-key">
          <AppIcon name="shield" size="ic-2xs" />
          <span class="mono">{{ p.has_key ? p.key_hint : '（没配密钥）' }}</span>
          <button v-if="p.has_key" class="lnk" type="button" @click="reveal(p)">显示</button>
        </div>
        <div class="pc-models">
          <span v-for="m in p.models.slice(0, 4)" :key="m.id" class="chip sm">{{ m.name || m.id }}</span>
          <span v-if="p.models.length > 4" class="muted">+{{ p.models.length - 4 }}</span>
        </div>
        <div class="pc-acts">
          <el-button size="small" :loading="busy === `test:${p.id}`" :disabled="!!busy" @click="test(p)">测试</el-button>
          <el-button size="small" type="primary" plain :loading="busy === `def:${p.id}`" :disabled="!!busy || p.is_default" @click="setDefault(p)">设默认</el-button>
          <el-button size="small" :disabled="!!busy" @click="edit(p)">编辑</el-button>
          <el-popconfirm title="删除后 pi 命令也用不了它，确定？" width="240" @confirm="remove(p)">
            <template #reference>
              <el-button size="small" type="danger" plain :loading="busy === `del:${p.id}`" :disabled="!!busy">删除</el-button>
            </template>
          </el-popconfirm>
        </div>
      </article>
    </div>

    <section class="editor card">
      <header class="ed-head">
        <b>{{ editing ? `编辑 ${picked?.name || editing}` : '新增服务' }}</b>
        <span class="muted">字段与 PI 的 models.json 一一对应</span>
      </header>
      <div class="grid2">
        <label class="field">
          <span>显示名</span>
          <el-input v-model="form.name" placeholder="例：本地网关 / DeepSeek" />
        </label>
        <label class="field">
          <span>接口类型</span>
          <el-select v-model="form.api">
            <el-option v-for="a in APIS" :key="a.key" :label="a.label" :value="a.key" />
          </el-select>
        </label>
        <label class="field">
          <span>接口地址</span>
          <el-input v-model="form.base_url" class="mono" placeholder="https://api.example.com/v1" />
        </label>
        <label class="field">
          <span>API Key {{ editing ? '（留空表示不改）' : '' }}</span>
          <el-input v-model="form.api_key" class="mono" type="password" show-password placeholder="sk-…" />
        </label>
      </div>

      <div class="models">
        <div class="md-head">
          <b>模型</b>
          <span class="muted">{{ form.models.length }} 个</span>
          <span class="sp" />
          <el-button size="small" :loading="busy === 'fetch'" :disabled="!!busy" @click="fetchModels">
            从上游拉取
          </el-button>
        </div>
        <div class="md-add">
          <el-input v-model="manual" placeholder="手动填模型名，空格或逗号分隔" @keydown.enter.prevent="addManual" />
          <el-button size="small" @click="addManual">加入</el-button>
        </div>
        <div v-if="fetched.length" class="md-pick">
          <button v-for="m in fetched" :key="m" class="chip sm" type="button" :disabled="form.models.some((x) => x.id === m)" @click="addModel(m)">
            {{ m }}
          </button>
        </div>
        <div v-if="form.models.length" class="md-list">
          <div v-for="(m, i) in form.models" :key="m.id" class="md-row">
            <el-input v-model="m.id" class="mono" placeholder="模型 id" size="small" />
            <el-input v-model="m.name" placeholder="显示名" size="small" />
            <el-input-number v-model="m.context_window" placeholder="窗口" size="small" :min="0" :controls="false" class="num" />
            <el-input-number v-model="m.max_tokens" placeholder="输出上限" size="small" :min="0" :controls="false" class="num" />
            <el-checkbox v-model="m.vision" size="small">识图</el-checkbox>
            <el-checkbox v-model="m.reasoning" size="small">思考</el-checkbox>
            <el-button size="small" type="danger" link @click="form.models.splice(i, 1)">移除</el-button>
          </div>
        </div>
        <p v-else class="muted">还没加模型：模型清单决定界面上的模型选择器里有什么。</p>
      </div>

      <div class="ed-acts">
        <el-button type="primary" :loading="busy === 'save'" :disabled="!!busy" @click="save">
          保存
        </el-button>
        <el-button v-if="editing" @click="reset">取消</el-button>
        <span class="sp" />
        <span class="muted">保存后重启空闲的 PI 进程，新配置立刻生效</span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}
.cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(340px, 100%), 1fr));
  gap: var(--wb-sp-3);
}
.pcard {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-4);
  transition: border-color var(--wb-dur-fast) var(--wb-ease);
}
.pcard.is-on {
  border-color: var(--wb-primary);
}
.pc-top {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
}
.pc-avatar {
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border-radius: var(--wb-radius-sm);
  display: grid;
  place-items: center;
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
  font-weight: 700;
}
.pc-id {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}
.pc-id b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.pc-meta {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.pc-url {
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pc-key {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.pc-models {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.pc-acts {
  display: flex;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
  margin-top: auto;
}
.editor {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-4);
}
.ed-head {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-3);
}
.ed-head b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.grid2 {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--wb-sp-3);
}
/* 窄窗时表单双列会挤：单列兜底 */
@media (max-width: 720px) {
  .grid2 {
    grid-template-columns: minmax(0, 1fr);
  }
}
.field {
  display: flex;
  flex-direction: column;
  gap: 5px;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.models {
  border-top: 1px solid var(--wb-line);
  padding-top: var(--wb-sp-3);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.md-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.md-add {
  display: flex;
  gap: var(--wb-sp-2);
}
.md-pick {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  max-height: 120px;
  overflow: auto;
}
.md-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.md-row {
  display: grid;
  grid-template-columns: 1.4fr 1fr 100px 100px auto auto auto;
  gap: 6px;
  align-items: center;
}
.md-row .num {
  width: 100%;
}
.ed-acts {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.sp {
  flex: 1;
}
.lnk {
  border: none;
  background: none;
  color: var(--wb-primary);
  font-size: var(--wb-fs-xs);
  cursor: pointer;
}
.lnk.danger {
  color: var(--wb-danger);
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
  max-width: 420px;
  line-height: 1.7;
}
</style>
