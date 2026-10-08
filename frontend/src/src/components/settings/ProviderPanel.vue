<script setup lang="ts">
// 模型服务面板：内置模板一键填好，只需粘 Key 就能用。
// 每个模型还能单独设上下文窗口、温度、top_p 与图像 / 工具调用能力。
import { computed, onMounted, reactive, ref, watch } from 'vue'
import * as api from '../../api'
import type { ModelConfigVO } from '../../types/api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import AppIcon from '../common/AppIcon.vue'
import PageState from '../common/PageState.vue'

const store = useSettingsStore()
const toast = useToastStore()
const editing = ref<string | null>(null)
const busy = ref(false)
const testing = ref<string | null>(null)
const testResult = ref<Record<string, { ok: boolean; detail: string }>>({})

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

// ---- API Key：已存密钥显示为掩码，眼睛显隐；「更换」是显式动作 ----
// 明文只在用户点眼睛时才向后端要一次，编辑期间不动它时保存不会覆盖旧密钥。
const KEY_MASK = '••••••••••••••••••••'
const keyHas = ref(false) // 该服务已保存密钥（服务端 has_key）
const keyEdit = ref(true) // 正在输入新密钥（新服务默认，编辑已存密钥默认关闭）
const keyShown = ref(false) // 明文显示中
const storedKey = ref<string | null>(null) // 拉取到的明文缓存
const keyBusy = ref(false)

const keyValue = computed(() => {
  if (keyEdit.value) return form.key
  if (keyShown.value) return storedKey.value ?? ''
  return KEY_MASK
})

function onKeyInput(e: Event) {
  if (keyEdit.value) form.key = (e.target as HTMLInputElement).value
}

async function toggleKeyShown() {
  // 正在输入新密钥：只是明文 / 密文切换
  if (keyEdit.value) {
    keyShown.value = !keyShown.value
    return
  }
  if (keyShown.value) {
    keyShown.value = false
    return
  }
  if (storedKey.value === null && form.id) {
    keyBusy.value = true
    try {
      storedKey.value = (await api.providers.reveal(form.id)).api_key
    } catch (e) {
      toast.bad(`读取密钥失败：${(e as Error)?.message || '请重试'}`)
      return
    } finally {
      keyBusy.value = false
    }
  }
  keyShown.value = true
}

// 「更换」：清空输入并进入编辑态；留空保存则保持原密钥
function startReplaceKey() {
  keyEdit.value = true
  keyShown.value = true
  form.key = ''
}

const needKey = computed(() => form.api !== 'ollama')
const chosen = computed(() => form.models.split(',').map((s) => s.trim()).filter(Boolean))
const options = computed(() => [...new Set([...chosen.value, ...fetched.value])])
// 模型可能上百个：搜索式下拉 + 截断，已选的用 chips 管理
const modelFilter = ref('')
const dropdownOpen = ref(false)
const MAX_SHOWN = 30

const matchedOptions = computed(() => {
  const q = modelFilter.value.trim().toLowerCase()
  return q ? options.value.filter((m) => m.toLowerCase().includes(q)) : options.value
})
const shownOptions = computed(() => matchedOptions.value.slice(0, MAX_SHOWN))
const hiddenCount = computed(() => Math.max(0, matchedOptions.value.length - MAX_SHOWN))

// 从下拉选中或手打回车都会落到这里；已存在的不重复加
function addModel(m: string) {
  const name = m.trim()
  if (!name) return
  if (!chosen.value.includes(name)) form.models = [...chosen.value, name].join(',')
  modelFilter.value = ''
  dropdownOpen.value = false
}

function removeModel(m: string) {
  form.models = chosen.value.filter((x) => x !== m).join(',')
}

function pick(t: (typeof templates)[number]) {
  editing.value = 'new'
  form.id = ''
  form.name = t.name
  form.api = t.api
  form.base_url = t.base_url
  form.key = ''
  form.models = t.models
  fetched.value = []
  modelFilter.value = ''
  keyHas.value = false
  keyEdit.value = true
  keyShown.value = false
  storedKey.value = null
}

// 切接口类型时地址跟着换：同一家服务商的 openai 入口和 anthropic 入口不是一个路径，
// 不联动的话用户会拿着 openai 地址打 anthropic 协议，必然 404。
const API_URL_MAP: Record<string, Record<string, string>> = {
  'https://api.deepseek.com/v1': { anthropic: 'https://api.deepseek.com/anthropic' },
  'https://api.deepseek.com/anthropic': { openai: 'https://api.deepseek.com/v1' },
}
watch(
  () => form.api,
  (next) => {
    const mapped = API_URL_MAP[form.base_url.trim().replace(/\/+$/, '')]?.[next]
    if (mapped) form.base_url = mapped
  },
)

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
  modelFilter.value = ''
  // 已存密钥显示掩码；没有密钥才直接进入输入态
  keyHas.value = p.has_key
  keyEdit.value = !p.has_key
  keyShown.value = false
  storedKey.value = null
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
  modelFilter.value = ''
  cfgOpen.value = false
  cfgRows.value = {}
  keyHas.value = false
  keyEdit.value = true
  keyShown.value = false
  storedKey.value = null
}

// 拉取上游模型列表：已保存的服务按 id 拉；还没保存的把连接信息直接发给后端
const pulling = ref(false)
async function pullModels() {
  pulling.value = true
  try {
    let list: string[] = []
    if (form.id) {
      list = await store.loadModels(form.id)
    } else {
      list = await api.providers.fetchModels({ api: form.api, base_url: form.base_url, api_key: form.key || undefined })
    }
    if (!list.length) toast.bad('上游没有返回模型列表，检查接口地址与服务商控制台')
    else toast.ok(`已拉取 ${list.length} 个模型`)
    fetched.value = list
  } catch (e) {
    toast.bad(`拉取模型列表失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    pulling.value = false
  }
}

// 新服务商还没落库：先把服务商存了拿到 id，再存模型参数——参数必须有地方挂
async function ensureProviderSaved(): Promise<string | null> {
  if (form.id) return form.id
  if (!form.name || !chosen.value.length) {
    toast.bad('先填名称并至少选一个模型，才能保存模型参数')
    return null
  }
  busy.value = true
  try {
    await api.providers.upsert({
      name: form.name,
      api: form.api,
      base_url: form.base_url,
      api_key: keyEdit.value && form.key ? form.key : undefined,
      models: chosen.value,
    })
    await store.loadProviders()
    const saved = store.providers.find((x) => x.name === form.name)
    if (!saved) {
      toast.bad('保存成功但没找到新服务商，请重试')
      return null
    }
    form.id = saved.id
    return saved.id
  } catch (e) {
    toast.bad(`先保存服务商失败：${(e as Error)?.message || '请检查名称、地址与密钥'}`)
    return null
  } finally {
    busy.value = false
  }
}

// ---- 模型参数与能力：目录认不出的模型，只有用户自己知道真实数字 ----
interface CfgRow {
  context_window: string
  temperature: string
  top_p: string
  vision: boolean
  tool_call: boolean
  window_known: boolean
  max_output: number
  saving: boolean
  saved: boolean
}

const cfgOpen = ref(false)
const cfgRows = ref<Record<string, CfgRow>>({})

function applyConfig(m: string, vo: ModelConfigVO) {
  cfgRows.value[m] = {
    context_window: vo.window_known ? String(vo.context_window) : '',
    temperature: String(vo.temperature),
    top_p: String(vo.top_p),
    vision: vo.vision,
    tool_call: vo.tool_call,
    window_known: vo.window_known,
    max_output: vo.max_output,
    saving: false,
    saved: false,
  }
}

function blankRow(): CfgRow {
  return {
    context_window: '', temperature: '0.25', top_p: '0.75',
    vision: false, tool_call: true, window_known: false, max_output: 0, saving: false, saved: false,
  }
}

// 行对象必须先于渲染存在，模板里的 v-model 才有落点；随后用服务端值覆盖
async function loadCfg(m: string) {
  try {
    applyConfig(m, await api.models.config(m, form.id || undefined))
  } catch {
    // 拉不到就保留缺省行
  }
}

watch([cfgOpen, chosen], () => {
  if (!cfgOpen.value) return
  for (const m of chosen.value) {
    if (!cfgRows.value[m]) cfgRows.value[m] = blankRow()
    void loadCfg(m)
  }
})

async function saveCfg(m: string) {
  const row = cfgRows.value[m]
  if (!row) return
  const pid = await ensureProviderSaved()
  if (!pid) return
  row.saving = true
  try {
    const vo = await api.models.saveConfig({
      provider_id: pid,
      model: m,
      context_window: Math.max(0, Math.round(Number(row.context_window) || 0)),
      max_output: row.max_output,
      temperature: Math.min(2, Math.max(0, Number(row.temperature) || 0)),
      top_p: Math.min(1, Math.max(0, Number(row.top_p) || 0)),
      vision: row.vision,
      tool_call: row.tool_call,
    })
    applyConfig(m, vo)
    row.saved = true
    toast.ok(`${m} 的参数已保存`)
    setTimeout(() => (row.saved = false), 1500)
  } catch (e) {
    toast.bad(`保存模型参数失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    row.saving = false
  }
}

async function save() {
  if (!form.name || !chosen.value.length) return
  busy.value = true
  try {
    const body = {
      name: form.name,
      api: form.api,
      base_url: form.base_url,
      // 只有在「更换 / 新填」且确实有内容时才提交密钥，其余情况不动已存的
      api_key: keyEdit.value && form.key ? form.key : undefined,
      models: chosen.value,
    }
    if (form.id) await api.providers.update(form.id, body)
    else await api.providers.upsert(body)
    await store.loadProviders()
    reset()
    toast.ok(form.id ? '模型服务已更新' : '模型服务已添加')
  } catch (e) {
    toast.bad(`保存失败：${(e as Error)?.message || '请检查填写内容后重试'}`)
  } finally {
    busy.value = false
  }
}

// 删除是危险操作：两段式确认，第一次点进入确认态，3 秒不点自动还原
const confirmingDelete = ref('')
let confirmTimer = 0
function askRemove(id: string) {
  if (confirmingDelete.value === id) {
    confirmingDelete.value = ''
    void doRemove(id)
    return
  }
  confirmingDelete.value = id
  clearTimeout(confirmTimer)
  confirmTimer = window.setTimeout(() => (confirmingDelete.value = ''), 3000)
}

async function doRemove(id: string) {
  try {
    await api.providers.remove(id)
    await store.loadProviders()
    toast.ok('模型服务已删除')
  } catch (e) {
    toast.bad(`删除失败：${(e as Error)?.message || '请重试'}`)
  }
}

async function test(id: string) {
  testing.value = id
  try {
    const r = await api.providers.test(id)
    testResult.value[id] = { ok: r.ok, detail: r.detail || (r.ok ? '连接正常' : '连接失败') }
    if (r.ok) toast.ok(`${r.model} 连接正常`)
    else toast.bad(`连接失败：${testResult.value[id].detail}`)
  } catch (e) {
    testResult.value[id] = { ok: false, detail: (e as Error).message }
    toast.bad(`连接失败：${(e as Error).message}`)
  } finally {
    testing.value = null
  }
}

async function setDefault(id: string) {
  try {
    await api.providers.setDefault(id)
    await store.loadProviders()
    toast.ok('已设为默认服务')
  } catch (e) {
    toast.bad(`设置默认失败：${(e as Error)?.message || '请重试'}`)
  }
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
          <div class="key-row">
            <input
              class="input"
              :type="keyShown ? 'text' : 'password'"
              :readonly="!keyEdit"
              :value="keyValue"
              :placeholder="keyEdit ? '粘贴密钥，保存后加密存储' : ''"
              @input="onKeyInput"
            />
            <button
              class="icon-btn"
              type="button"
              :title="keyShown ? '隐藏' : keyHas && !keyEdit ? '显示已保存的密钥' : '显示'"
              :class="{ 'is-loading': keyBusy }"
              @click="toggleKeyShown"
            >
              <AppIcon :name="keyShown ? 'eye-off' : 'eye'" size="ic-sm" />
            </button>
            <button v-if="keyHas && !keyEdit" class="btn btn-sm btn-ghost" type="button" @click="startReplaceKey">
              更换
            </button>
          </div>
          <span class="key-note">
            <template v-if="keyHas && !keyEdit">已保存密钥（加密存储），点眼睛查看</template>
            <template v-else-if="keyHas && keyEdit">留空保存则保持原密钥</template>
          </span>
        </label>
        <div class="field span2">
          <span class="lb">这个服务能用哪些模型（可多选）</span>

          <!-- 已选模型：数量少，chips 一目了然，可单独移除 -->
          <div v-if="chosen.length" class="mrow">
            <span v-for="m in chosen" :key="m" class="chip on">
              {{ m }}
              <button class="chip-x" type="button" aria-label="移除" @click="removeModel(m)">
                <AppIcon name="close" size="ic-xs" />
              </button>
            </span>
          </div>

          <!-- 搜索式下拉：上游几百个模型也能选，输入即过滤 -->
          <div class="mselect">
            <input
              v-model="modelFilter"
              class="input"
              placeholder="搜索或手打模型名，回车加上"
              @focus="dropdownOpen = true"
              @keydown.enter.prevent="addModel(modelFilter.trim())"
            />
            <div v-if="dropdownOpen && matchedOptions.length" class="mselect-pop">
              <button
                v-for="m in shownOptions"
                :key="m"
                class="pop-row"
                type="button"
                @mousedown.prevent="addModel(m)"
              >
                <AppIcon v-if="chosen.includes(m)" name="check" size="ic-xs" />
                <span v-else class="pr-gap" />
                {{ m }}
              </button>
              <div v-if="hiddenCount > 0" class="pop-row muted">
                还有 {{ hiddenCount }} 个匹配项，继续输入缩小范围
              </div>
            </div>
            <div v-else-if="dropdownOpen && modelFilter.trim()" class="mselect-pop">
              <button class="pop-row" type="button" @mousedown.prevent="addModel(modelFilter.trim())">
                <AppIcon name="plus" size="ic-xs" />
                添加自定义模型「{{ modelFilter.trim() }}」
              </button>
            </div>
          </div>
          <div @focusout="dropdownOpen = false">
            <div class="flex-r mrow-acts">
              <span class="hint">
                {{ store.modelsLoading ? '正在读取模型列表…' : options.length ? '' : '拉取列表后在这里选择，也可以手打' }}
              </span>
              <span class="sp" />
              <button
                class="btn btn-outline btn-sm"
                :class="{ 'is-loading': pulling }"
                :disabled="pulling || (needKey && !form.id && !form.key)"
                :title="form.id ? '去服务商那里问一遍有哪些模型' : '按上面填的地址与密钥去问一遍'"
                @click="pullModels"
              >
                {{ pulling || store.modelsLoading ? '拉取中' : '拉取模型列表' }}
              </button>
            </div>
            <div class="hint">至少选一个；保存后在对话页底部也能随时换模型</div>
          </div>
        </div>

        <!-- 模型参数与能力：不展开也能用（走内置目录），展开后每项都能手填 -->
        <div class="field span2">
          <button class="cfg-toggle" type="button" @click="cfgOpen = !cfgOpen">
            <AppIcon :name="cfgOpen ? 'chevron-down' : 'chevron-right'" size="ic-xs" />
            模型参数与能力
            <span class="cfg-tip">上下文窗口、温度、top_p、识图 / 工具调用</span>
          </button>
          <div v-if="cfgOpen" class="cfg-list">
            <div v-for="m in chosen" :key="m" class="cfg-row">
              <div class="cfg-name" :title="m">{{ m }}</div>
              <label class="cfg-field">
                <span>上下文窗口</span>
                <input
                  v-model="cfgRows[m].context_window"
                  class="input input-sm"
                  inputmode="numeric"
                  placeholder="自动"
                />
              </label>
              <label class="cfg-field">
                <span>温度</span>
                <input v-model="cfgRows[m].temperature" class="input input-sm" inputmode="decimal" />
              </label>
              <label class="cfg-field">
                <span>Top P</span>
                <input v-model="cfgRows[m].top_p" class="input input-sm" inputmode="decimal" />
              </label>
              <label class="cfg-check" title="能不能看图">
                <input v-model="cfgRows[m].vision" type="checkbox" />
                <span>识图</span>
              </label>
              <label class="cfg-check" title="能不能调用工具">
                <input v-model="cfgRows[m].tool_call" type="checkbox" />
                <span>工具调用</span>
              </label>
              <button
                class="btn btn-outline btn-sm"
                :class="{ 'is-loading': cfgRows[m]?.saving }"
                :disabled="cfgRows[m]?.saving"
                @click="saveCfg(m)"
              >
                {{ cfgRows[m]?.saved ? '已保存' : '保存' }}
              </button>
            </div>
            <div class="hint">窗口留空 = 跟随内置目录；改完点保存立刻生效，对话里按这里的值算上下文水位</div>
          </div>
        </div>
      </div>
      <div class="flex-r form-acts">
        <button class="btn btn-primary btn-sm" :class="{ 'is-loading': busy }" :disabled="busy || !chosen.length" @click="save">
          保存
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
            <button
              class="btn btn-sm btn-ghost"
              :class="{ 'is-loading': testing === p.id }"
              :disabled="testing === p.id"
              @click="test(p.id)"
            >
              {{ testing === p.id ? '测试中' : '测试' }}
            </button>
            <button v-if="!p.is_default" class="btn btn-sm btn-ghost" @click="setDefault(p.id)">设默认</button>
            <button class="btn btn-sm btn-ghost" @click="edit(p.id)">编辑</button>
            <button class="btn btn-sm btn-danger-ghost" @click="askRemove(p.id)">
              {{ confirmingDelete === p.id ? '再点一次确认删除' : '删除' }}
            </button>
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
/* API Key 行：输入 + 眼睛 + 更换同一行，掩码态呈现等宽点阵 */
.key-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
}
.key-row .input {
  flex: 1;
  min-width: 0;
}
.key-row .input[readonly] {
  font-family: var(--font-mono);
  color: var(--wb-ink-2);
  cursor: default;
}
.key-note {
  font-size: var(--wb-fs-hint);
  color: var(--wb-muted);
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

/* 搜索式模型下拉：上游几百个模型时的唯一入口 */
.mselect {
  position: relative;
}
.mselect-pop {
  position: absolute;
  left: 0;
  right: 0;
  top: calc(100% + 4px);
  z-index: var(--wb-z-pop);
  max-height: 280px;
  overflow-y: auto;
  padding: 4px;
  border-radius: var(--wb-radius);
  background: var(--wb-surface-solid);
  border: 1px solid var(--wb-border-strong);
  box-shadow: var(--wb-shadow-pop);
}
.mselect-pop .pop-row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border: 0;
  border-radius: var(--wb-radius-sm);
  background: transparent;
  font-size: var(--wb-fs-sm);
  font-family: var(--font-mono);
  color: var(--wb-ink);
  text-align: left;
  cursor: pointer;
}
.mselect-pop .pop-row:hover {
  background: var(--wb-tint);
}
.mselect-pop .pr-gap {
  width: 14px;
  flex: none;
}
.chip-x {
  display: inline-grid;
  place-items: center;
  border: 0;
  background: transparent;
  padding: 0;
  cursor: pointer;
  color: inherit;
  border-radius: var(--wb-radius-full);
  transition:
    color var(--wb-dur-fast) var(--wb-ease),
    background var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.chip-x:hover:not(:disabled) {
  color: var(--wb-danger);
  transform: scale(1.06);
}
.chip-x:active:not(:disabled) {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
  transform: scale(0.92);
}
.chip-x:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.chip-x.is-loading {
  pointer-events: none;
  cursor: progress;
}

/* 模型参数折叠区 */
.cfg-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: var(--wb-ctl-h-sm);
  padding: 0 var(--wb-sp-2);
  margin-left: calc(-1 * var(--wb-sp-2));
  border: 0;
  border-radius: var(--wb-radius-sm);
  background: transparent;
  font-size: var(--wb-fs-label);
  font-weight: 600;
  color: var(--wb-ink-2);
  cursor: pointer;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.cfg-toggle:hover:not(:disabled) {
  background: var(--wb-tint);
  color: var(--wb-ink);
}
.cfg-toggle:active:not(:disabled) {
  background: var(--wb-tint-lg);
  color: var(--wb-ink);
  transform: scale(0.97);
}
.cfg-toggle:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.cfg-tip {
  font-weight: 400;
  color: var(--wb-muted);
}
.cfg-list {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  margin-top: var(--wb-sp-2);
  padding: var(--wb-sp-3);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius);
  background: var(--wb-surface-2);
}
.cfg-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--wb-sp-3);
}
.cfg-name {
  flex: 0 1 220px;
  min-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
}
.cfg-field {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.cfg-field > span {
  font-size: var(--wb-fs-hint);
  color: var(--wb-muted);
}
.cfg-field .input-sm {
  width: 96px;
  height: var(--wb-ctl-h-sm);
  font-size: var(--wb-fs-sm);
}
.cfg-check {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
  cursor: pointer;
  user-select: none;
}
.cfg-check input {
  accent-color: var(--wb-primary);
}
</style>
