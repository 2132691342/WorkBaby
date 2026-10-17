<script setup lang="ts">
// MCP 设置面板：整段编辑 mcp.json 原文，保存前后端各校验一次，坏配置一个字符都不落盘。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import type { MCPConfigRESP, MCPTemplateVO, MCPToolVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import PageState from '../common/PageState.vue'
import { useToastStore } from '../../stores/toast'

const toast = useToastStore()
const cfg = ref<MCPConfigRESP | null>(null)
const loading = ref(true)
const error = ref('')

const raw = ref('')
const dirty = ref(false)
const saving = ref(false)
// 本地 JSON 语法错误：先拦一道，省一次注定失败的往返
const syntaxError = ref('')

// 模板三档一次取全：面板要列三个按钮，逐个请求是浪费
const templates = ref<MCPTemplateVO[]>([])

async function load() {
  loading.value = true
  error.value = ''
  try {
    // 工具清单读不到不挡面板：它只影响每条上是「已声明」还是「内核已注册」。
    const [c, tpls, tools] = await Promise.all([
      api.mcp.config(),
      api.mcp.templates(),
      api.mcp.tools().catch(() => []),
    ])
    cfg.value = c
    raw.value = c.raw
    templates.value = tpls
    toolNames.value = tools ?? []
    dirty.value = false
    syntaxError.value = ''
  } catch (e) {
    error.value = (e as Error)?.message || '读不到 MCP 配置'
  } finally {
    loading.value = false
  }
}
onMounted(load)

// 保存前先本地解析：语法错就说错在哪一行，不让它走一趟网络再原样弹回来
function checkSyntax(): boolean {
  const text = raw.value
  if (!text.trim()) {
    syntaxError.value = '配置文件是空的'
    return false
  }
  try {
    JSON.parse(text)
  } catch (e) {
    const m = /position (\d+)/.exec((e as Error).message)
    let where = ''
    if (m) {
      const line = text.slice(0, Number(m[1])).split('\n').length
      where = `（第 ${line} 行附近）`
    }
    syntaxError.value = `JSON 语法不对${where}：${(e as Error).message}`
    return false
  }
  syntaxError.value = ''
  return true
}

async function save() {
  if (saving.value) return
  if (!checkSyntax()) {
    toast.bad(syntaxError.value)
    return
  }
  saving.value = true
  try {
    cfg.value = await api.mcp.save({ raw: raw.value })
    dirty.value = false
    toast.ok('配置已保存并重载：新服务已连上，改过的已重连')
  } catch (e) {
    // 9106 的 message 里带着后端指出的具体哪一项不合格，原样弹给用户
    toast.bad(`保存失败：${(e as Error)?.message || '请重试'}`)
    await load()
  } finally {
    saving.value = false
  }
}

async function useTemplate(kind: string) {
  const tpl = templates.value.find((t) => t.kind === kind)
  if (!tpl) return
  raw.value = tpl.raw
  dirty.value = true
  syntaxError.value = ''
  toast.ok(`已填入「${tpl.label}」模板，改完点「保存并重载」才生效`)
}

// 重新加载：按当前文件重拉内核进程，然后看内核报了哪些外部工具前缀。
// 这里没有「探测」：连接是内核做的，它也不上报每个服务的工具清单，
// 任何状态灯都会是猜出来的。
const reloading = ref(false)
const toolNames = ref<MCPToolVO[]>([])
async function reload() {
  if (reloading.value) return
  reloading.value = true
  try {
    cfg.value = await api.mcp.reload()
    toolNames.value = await api.mcp.tools()
    toast.ok(`已重新加载，内核侧看到 ${toolNames.value.length} 个外部工具条目`)
  } catch (e) {
    toast.bad(`重新加载失败：${(e as Error)?.message || '检查命令与参数'}`)
  } finally {
    reloading.value = false
  }
}

// declared 判断某个服务的工具条目有没有出现在内核的清单里。
function declared(name: string) {
  return toolNames.value.some((t) => t.server === name)
}

const parseError = computed(() => cfg.value?.parse_error || '')
const servers = computed(() => cfg.value?.servers || [])
const lineCount = computed(() => raw.value.split('\n').length)
const savedText = computed(() => (dirty.value ? '有未保存的修改' : '已保存'))
</script>

<template>
  <PageState :loading="loading" :error="error">
    <header class="pane-head">
      <div>
        <h2>MCP 外部工具</h2>
        <p>
          把你机器上跑的 MCP 服务写进 <span class="mono">mcp.json</span>，它的工具会和内置工具一起
          出现在助手面前。外部工具一律「每次先问」，停用某个条目把 <span class="mono">disabled</span> 设成
          <span class="mono">true</span>，不要删掉——改完保存即刻重载。
        </p>
      </div>
      <button
        class="btn btn-primary btn-sm"
        :class="{ 'is-loading': saving }"
        :disabled="saving || !dirty"
        type="button"
        @click="save"
      >
        <AppIcon v-if="!saving" name="check" size="ic-xs" /> {{ saving ? '保存中…' : '保存并重载' }}
      </button>
    </header>

    <div class="meta card p-sm">
      <span class="mk">配置文件</span>
      <span class="mono path">{{ cfg?.path }}</span>
      <span class="sp" />
      <span class="tag" :class="{ ok: !dirty, muted: dirty }">{{ savedText }}</span>
      <span class="tag muted">{{ lineCount }} 行</span>
    </div>

    <p v-if="parseError" class="alert bad">
      <AppIcon name="alert" size="ic-sm" />
      这份配置读不通，界面显示的是上次能用的那份：{{ parseError }}
    </p>

    <div class="tpl-row">
      <span class="t-plate">填入模板（会覆盖编辑器里的内容）</span>
      <button
        v-for="t in templates"
        :key="t.kind"
        class="chip"
        type="button"
        :title="t.hint"
        :disabled="saving"
        @click="useTemplate(t.kind)"
      >
        {{ t.label }}
      </button>
    </div>

    <div class="edit-grid">
      <section class="card editor">
        <div class="ed-head">
          <span class="lb">mcp.json</span>
          <span v-if="syntaxError" class="err">{{ syntaxError }}</span>
        </div>
        <textarea
          v-model="raw"
          class="input mono code"
          spellcheck="false"
          aria-label="MCP 配置 JSON"
          @input="dirty = true"
        />
        <footer class="ed-foot">
          <span>字段：</span>
          <span class="mono dim">command</span><span class="dim">必填</span>
          <span class="mono dim">args</span>
          <span class="mono dim">env</span>
          <span class="mono dim">cwd</span>
          <span class="mono dim">disabled</span>
          <span class="mono dim">timeout</span><span class="dim">秒，缺省 30</span>
        </footer>
      </section>

      <section class="card parsed">
        <div class="ed-head">
          <span class="lb">解析结果</span>
          <span class="dim">{{ servers.length }} 个服务</span>
          <button
            class="btn btn-sm btn-ghost"
            :class="{ 'is-loading': reloading }"
            :disabled="reloading || !servers.length"
            title="按当前文件重新拉起内核，再看它报回来的外部工具条目"
            type="button"
            @click="reload"
          >
            {{ reloading ? '重新加载中…' : '重新加载' }}
          </button>
        </div>
        <p v-if="!servers.length" class="blank">
          还没有任何服务。点上面的「本地命令」模板，把 <span class="mono">command</span> 换成你自己的启动命令。
        </p>
        <ul v-else class="srv-list">
          <li v-for="s in servers" :key="s.name" class="srv">
            <div class="srv-main">
              <div class="srv-title">
                <b>{{ s.name }}</b>
                <span
                  v-if="s.disabled"
                  class="led off"
                  role="status"
                  aria-label="已停用"
                  title="已停用：不启动、工具不注册"
                />
                <span v-if="s.disabled" class="tag muted">已停用</span>
                <span v-else class="tag" :class="{ ok: declared(s.name) }">
                  {{ declared(s.name) ? '内核已注册' : '已声明' }}
                </span>
              </div>
              <p class="mono dim cmd">{{ s.command }} {{ s.args.join(' ') }}</p>
              <p v-if="s.cwd" class="dim hint">工作目录 {{ s.cwd }}</p>
              <p v-if="s.env_keys.length" class="dim hint">
                附加环境变量 {{ s.env_keys.join('、') }}
              </p>
            </div>
          </li>
        </ul>
        <p class="foot-note">
          「已声明」只说明配置里有它：连不连得上由内核负责，它连不上时的原因在内核的日志里。
          想让状态变成「内核已注册」，点上面的「重新加载」。工具的全局启停在「工具」页。
        </p>
      </section>
    </div>
  </PageState>
</template>

<style scoped>
.pane-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--wb-sp-4);
  margin-bottom: var(--wb-sp-5);
}
.pane-head h2 {
  font-size: var(--wb-fs-lg);
  color: var(--wb-ink);
}
.pane-head p {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  margin-top: var(--wb-sp-1);
  max-width: 620px;
  line-height: var(--wb-lh-base);
}
.p-sm {
  padding: var(--wb-sp-3) var(--wb-sp-4);
}
.dim {
  color: var(--wb-muted);
}
.meta {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  margin-bottom: var(--wb-sp-3);
  flex-wrap: wrap;
}
.meta .mk {
  font-size: var(--wb-fs-label);
  font-weight: 600;
  color: var(--wb-ink-2);
}
.meta .path {
  font-size: var(--wb-fs-hint);
  color: var(--wb-muted);
  word-break: break-all;
}
.meta .sp {
  flex: 1;
}
.tag.ok {
  color: var(--wb-success);
  border-color: var(--wb-success);
}
.tag.muted {
  color: var(--wb-muted);
}
.alert {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3) var(--wb-sp-4);
  border-radius: var(--wb-radius-sm);
  font-size: var(--wb-fs-sm);
  margin-bottom: var(--wb-sp-4);
  line-height: var(--wb-lh-base);
}
.alert.bad {
  background: var(--wb-danger-soft);
  border-left: 3px solid var(--wb-danger);
  color: var(--wb-danger);
}
.alert .ic {
  margin-top: 2px;
  flex: none;
}
.tpl-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-2);
  align-items: center;
  margin-bottom: var(--wb-sp-4);
}
.tpl-row .t-plate {
  flex: 1 0 100%;
  font-size: var(--wb-fs-label);
  font-weight: 600;
  color: var(--wb-ink-2);
}
.edit-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(0, 1fr);
  gap: var(--wb-sp-4);
  align-items: start;
}
@media (max-width: 900px) {
  .edit-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
.ed-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-3);
  margin-bottom: var(--wb-sp-3);
}
.ed-head .lb {
  font-size: var(--wb-fs-label);
  font-weight: 600;
  color: var(--wb-ink-2);
}
.ed-head .err {
  font-size: var(--wb-fs-hint);
  color: var(--wb-danger);
  text-align: right;
}
.editor .code {
  width: 100%;
  min-height: 420px;
  resize: vertical;
  line-height: 1.6;
  tab-size: 2;
  white-space: pre;
  overflow-wrap: normal;
  overflow-x: auto;
}
.ed-foot {
  margin-top: var(--wb-sp-3);
  padding-top: var(--wb-sp-3);
  border-top: 1px solid var(--wb-line);
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  font-size: var(--wb-fs-hint);
  color: var(--wb-ink-2);
}
.ed-foot .mono {
  padding: 1px 6px;
  border-radius: var(--wb-radius-xs);
  background: var(--wb-tint);
  color: var(--wb-primary);
}
.blank {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
}
.srv-list {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
/* 服务行本身不可点（可点的是里面的「探测」按钮），所以不挂 hover 变色——
   悬停变色会让用户以为整行能点什么，点了没反应比不给反馈更让人恼火 */
.srv {
  display: flex;
  justify-content: space-between;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-3);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius);
  background: var(--wb-surface-2);
}
.srv-main {
  min-width: 0;
  flex: 1;
}
.srv-title {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
}
.srv-title b {
  color: var(--wb-ink);
}
.led {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: none;
}
.led.ok {
  background: var(--wb-success);
}
.led.bad {
  background: var(--wb-danger);
}
.led.off {
  background: var(--wb-muted);
}
.cmd {
  font-size: var(--wb-fs-hint);
  margin-top: 2px;
  word-break: break-all;
}
.hint {
  font-size: var(--wb-fs-hint);
  margin-top: 1px;
}
.foot-note {
  margin-top: var(--wb-sp-4);
  font-size: var(--wb-fs-hint);
  color: var(--wb-muted);
}
</style>
