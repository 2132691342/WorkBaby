<script setup lang="ts">
// 模型服务面板：内置模板一键填好，只需粘 Key 就能用。
import { computed, onMounted, reactive, ref } from 'vue'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import AppIcon from '../common/AppIcon.vue'
import PageState from '../common/PageState.vue'

const store = useSettingsStore()
const editing = ref<string | null>(null)
const busy = ref(false)
const testing = ref<string | null>(null)
const testResult = ref<Record<string, { ok: boolean; detail: string }>>({})
const extraModel = ref('')

const templates = [
  { api: 'openai', name: 'OpenAI', base_url: 'https://api.openai.com/v1', models: 'gpt-4o,gpt-4o-mini' },
  { api: 'anthropic', name: 'Anthropic', base_url: 'https://api.anthropic.com', models: 'claude-sonnet-4-5,claude-haiku-4-5' },
  { api: 'openai', name: 'DeepSeek', base_url: 'https://api.deepseek.com/v1', models: 'deepseek-chat,deepseek-reasoner' },
  { api: 'openai', name: '月之暗面', base_url: 'https://api.moonshot.cn/v1', models: 'moonshot-v1-32k' },
  { api: 'openai', name: '阿里百炼', base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1', models: 'qwen-max,qwen-plus' },
  { api: 'ollama', name: '本地 Ollama', base_url: 'http://127.0.0.1:11434', models: 'qwen3:8b' },
]

const form = reactive({ id: '', name: '', api: 'openai', base_url: '', key: '', models: '' })
const fetched = ref<string[]>([])

const needKey = computed(() => form.api !== 'ollama')
const chosen = computed(() => form.models.split(',').map((s) => s.trim()).filter(Boolean))
const options = computed(() => [...new Set([...chosen.value, ...fetched.value])])

function pick(t: (typeof templates)[number]) {
  editing.value = 'new'
  form.id = ''
  form.name = t.name
  form.api = t.api
  form.base_url = t.base_url
  form.key = ''
  form.models = t.models
  fetched.value = []
  extraModel.value = ''
}

function edit(id: string) {
  const p = store.providers.find((x) => x.id === id)
  if (!p) return
  editing.value = id
  form.id = id
  form.name = p.name
  form.api = p.api
  form.base_url = p.base_url
  form.key = ''
  form.models = p.models.join(',')
  fetched.value = []
  extraModel.value = ''
}

function reset() {
  editing.value = null
  form.id = ''
  form.name = ''
  form.api = 'openai'
  form.base_url = ''
  form.key = ''
  form.models = ''
  fetched.value = []
  extraModel.value = ''
}

// 拉取上游模型列表：保存前也能先看这家到底有哪些模型
async function pullModels() {
  if (!form.id) return
  const list = await store.loadModels(form.id)
  fetched.value = list
}

function toggleModel(m: string) {
  const set = new Set(chosen.value)
  if (set.has(m)) set.delete(m)
  else set.add(m)
  form.models = [...set].join(',')
}

function addExtra() {
  const m = extraModel.value.trim()
  if (!m) return
  if (!chosen.value.includes(m)) form.models = [...chosen.value, m].join(',')
  extraModel.value = ''
}

async function save() {
  if (!form.name || !chosen.value.length) return
  busy.value = true
  try {
    const body = {
      name: form.name,
      api: form.api,
      base_url: form.base_url,
      api_key: form.key || undefined,
      models: chosen.value,
    }
    if (form.id) await api.providers.update(form.id, body)
    else await api.providers.upsert(body)
    await store.loadProviders()
    reset()
  } finally {
    busy.value = false
  }
}

async function remove(id: string) {
  await api.providers.remove(id)
  await store.loadProviders()
}

async function test(id: string) {
  testing.value = id
  try {
    const r = await api.providers.test(id)
    testResult.value[id] = { ok: r.ok, detail: r.detail || (r.ok ? '连接正常' : '连接失败') }
  } catch (e) {
    testResult.value[id] = { ok: false, detail: (e as Error).message }
  } finally {
    testing.value = null
  }
}

async function setDefault(id: string) {
  await api.providers.setDefault(id)
  await store.loadProviders()
}

onMounted(() => store.loadProviders())
</script>

<template>
  <div class="prov">
    <div class="tpl-row">
      <span class="t-plate">选择服务商，填入 API Key 即可</span>
      <button v-for="t in templates" :key="t.name" class="chip" type="button" @click="pick(t)">{{ t.name }}</button>
    </div>

    <div v-if="editing" class="card p-sm form-card">
      <h3>{{ form.id ? '编辑模型服务' : '添加模型服务' }}</h3>
      <div class="form-grid">
        <label class="field">
          <span class="lb">名称</span>
          <input v-model="form.name" class="input" placeholder="给这个服务起个名字" />
        </label>
        <label class="field">
          <span class="lb">接口类型</span>
          <select v-model="form.api" class="input">
            <option value="openai">OpenAI 兼容</option>
            <option value="anthropic">Anthropic</option>
            <option value="ollama">Ollama（本地）</option>
          </select>
        </label>
        <label class="field span2">
          <span class="lb">接口地址</span>
          <input v-model="form.base_url" class="input" placeholder="https://..." />
        </label>
        <label v-if="needKey" class="field span2">
          <span class="lb">API Key</span>
          <input v-model="form.key" class="input" type="password" placeholder="粘贴密钥，保存后加密存储" />
        </label>
        <div class="field span2">
          <span class="lb">这个服务能用哪些模型（可多选）</span>
          <div class="mrow">
            <button
              v-for="m in options"
              :key="m"
              class="chip"
              :class="{ on: chosen.includes(m) }"
              type="button"
              @click="toggleModel(m)"
            >
              <AppIcon v-if="chosen.includes(m)" name="check" size="ic-xs" />
              {{ m }}
            </button>
            <span v-if="store.modelsLoading" class="muted">正在读取模型列表…</span>
            <span v-else-if="!options.length" class="muted">模板已带常用模型；保存后可从上游拉取完整列表</span>
          </div>
          <div class="flex-r mrow-acts">
            <input
              v-model="extraModel"
              class="input"
              placeholder="列表里没有？在这里手打模型名，点「加上」"
              @keydown.enter="addExtra"
            />
            <button class="btn btn-outline btn-sm" :disabled="!extraModel.trim()" @click="addExtra">加上</button>
            <button
              v-if="form.id"
              class="btn btn-outline btn-sm"
              :disabled="store.modelsLoading"
              title="去服务商那里问一遍有哪些模型"
              @click="pullModels"
            >
              {{ store.modelsLoading ? '拉取中…' : '拉取模型列表' }}
            </button>
          </div>
          <div class="hint">至少选一个；保存后在对话页底部也能随时换模型</div>
        </div>
      </div>
      <div class="flex-r form-acts">
        <button class="btn btn-primary btn-sm" :disabled="busy || !chosen.length" @click="save">
          {{ busy ? '保存中…' : '保存' }}
        </button>
        <button class="btn btn-ghost btn-sm" @click="reset">取消</button>
        <span v-if="!form.id" class="acts-hint">保存后就能去拉取上游模型列表</span>
      </div>
    </div>

    <PageState
      :loading="store.providersLoading"
      :error="store.error"
      :empty="!store.providers.length && !editing"
      empty-title="还没有配置模型服务"
      empty-sub="点上面的服务商模板，填一个 Key 就能用"
    >
      <div class="rowlist">
        <div v-for="p in store.providers" :key="p.id" class="rli">
          <div class="grow">
            <h5>
              {{ p.name }}
              <span v-if="p.is_default" class="tag b-primary">默认</span>
              <span v-if="!p.enabled" class="tag">停用</span>
            </h5>
            <p class="mono">{{ p.base_url || '—' }} · {{ p.models.join(', ') }}</p>
            <p v-if="testResult[p.id]" :class="testResult[p.id].ok ? 't-ok' : 't-bad'">
              {{ testResult[p.id].detail }}
            </p>
          </div>
          <div class="flex-r">
            <button class="btn btn-sm btn-ghost" :disabled="testing === p.id" @click="test(p.id)">
              {{ testing === p.id ? '测试中…' : '测试' }}
            </button>
            <button v-if="!p.is_default" class="btn btn-sm btn-ghost" @click="setDefault(p.id)">设默认</button>
            <button class="btn btn-sm btn-ghost" @click="edit(p.id)">编辑</button>
            <button class="btn btn-sm btn-danger-ghost" @click="remove(p.id)">删除</button>
          </div>
        </div>
      </div>
    </PageState>
  </div>
</template>

<style scoped>
.prov {
  min-width: 0;
}
.tpl-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-2);
  align-items: center;
  margin-bottom: var(--wb-sp-4);
  min-width: 0;
}
/* 模板 chips 单独一行，标题不再和按钮抢同一行的宽度 */
.tpl-row .t-plate {
  flex: 1 0 100%;
}
.form-card {
  margin-bottom: var(--wb-sp-4);
}
.form-card h3 {
  margin: 0 0 var(--wb-sp-3);
  font-size: var(--wb-fs-md);
  font-weight: 600;
  color: var(--wb-ink);
}
.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--wb-sp-3);
  min-width: 0;
}
.form-grid .field {
  min-width: 0;
}
@media (max-width: 720px) {
  .form-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .form-grid .span2 {
    grid-column: auto;
  }
}
.field .lb {
  font-size: var(--wb-fs-label);
  font-weight: 500;
  color: var(--wb-ink-2);
}
.mrow {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--wb-sp-2);
  min-height: var(--wb-ctl-h);
  min-width: 0;
}
/* 模型名的手打输入必须能收缩，两个按钮才能始终留在同一行 */
.mrow-acts .input {
  flex: 1 1 0;
  min-width: 0;
}
.form-acts {
  margin-top: var(--wb-sp-3);
  align-items: center;
}
.form-acts .acts-hint {
  margin-left: auto;
  font-size: var(--wb-fs-hint);
  color: var(--wb-muted);
}
.t-ok { color: var(--wb-success); font-size: var(--wb-fs-sm); }
.t-bad { color: var(--wb-danger); font-size: var(--wb-fs-sm); }
</style>
