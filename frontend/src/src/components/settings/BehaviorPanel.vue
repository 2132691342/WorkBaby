<script setup lang="ts">
// 行为面板：只管「助手怎么做事、怎么待着」的事。
// 主题字体在「外观」，两者刻意分开——用户找设置时是带着目的来的，
// 「我想让它别老弹窗确认」和「我想把界面调暗」不该挤在同一屏里互相干扰。
import { onMounted, ref } from 'vue'
import * as api from '../../api'
import type { ModelCapability, RuntimeInfo } from '../../types/api'
import { useSettingsStore } from '../../stores/settings'
import { AutoStartEnabled, OpenDirectoryDialog, SetAutoStart } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'

const store = useSettingsStore()

const autoStart = ref(false)
const autoStartErr = ref('')
const runtimeInfo = ref<RuntimeInfo | null>(null)
const runtimeErr = ref('')
const cap = ref<ModelCapability | null>(null)
const winInput = ref('')
const winErr = ref('')

// 权限档取值必须与后端 domain.PermissionXxx 一致，否则设置写入直接被拒。
const PERMISSIONS = [
  { key: 'ask', name: '先问我', desc: '读写文件、跑命令都先问我一句，推荐新手使用' },
  { key: 'auto_edit', name: '少打扰', desc: '改文件不问，跑命令和跑脚本仍然要审批' },
  { key: 'yolo', name: '全自动', desc: '不再询问，直接执行（只建议在自己电脑上用）' },
]

const permission = () => store.values['permission'] || 'ask'
const minimizeToTray = () => store.values['minimize_to_tray'] !== 'false'
const sendOnEnter = () => store.values['send_on_enter'] !== 'false'

async function loadAutoStart() {
  try {
    autoStart.value = await AutoStartEnabled()
  } catch {
    autoStart.value = false
  }
}

async function toggleAutoStart() {
  autoStartErr.value = ''
  const next = !autoStart.value
  try {
    await SetAutoStart(next)
    autoStart.value = await AutoStartEnabled()
  } catch (e) {
    autoStartErr.value = (e as Error)?.message || '设置失败，请重启后再试一次'
    autoStart.value = await AutoStartEnabled().catch(() => autoStart.value)
  }
}

async function loadRuntime() {
  runtimeErr.value = ''
  try {
    runtimeInfo.value = await api.runtime.status()
  } catch (e) {
    runtimeErr.value = (e as Error)?.message || '读取运行时状态失败'
  }
}

// 当前模型的上下文窗口：本地没资料就显示「未知」并允许手填，
// 而不是拿一个猜出来的数字冒充权威。
async function loadCapability() {
  const model = store.boot?.default_model || store.values['default_model']
  if (!model) return
  try {
    cap.value = await api.models.capability(model)
    winInput.value = store.values['context_window'] || ''
  } catch {
    cap.value = null
  }
}

async function pickWorkspace() {
  const dir = await OpenDirectoryDialog('选择工作目录')
  if (!dir) return
  await store.setValue('workspace', dir)
  await store.loadBoot()
}

async function saveWindow() {
  winErr.value = ''
  const v = winInput.value.trim()
  if (!v) {
    await store.setValue('context_window', '')
    cap.value && (cap.value.known = false)
    return
  }
  const n = Number(v)
  if (!Number.isFinite(n) || n <= 0) {
    winErr.value = '请填一个大于 0 的数字'
    return
  }
  await store.setValue('context_window', String(Math.floor(n)))
  await loadCapability()
}

onMounted(async () => {
  await Promise.all([loadAutoStart(), loadRuntime(), loadCapability()])
})
</script>

<template>
  <div class="bp">
    <div class="card p-sm">
      <h3>动手前先问我</h3>
      <p class="hint">助手做「危险」的事之前要不要先问你。改了立刻对下一轮生效。</p>
      <div class="perm-row">
        <button
          v-for="o in PERMISSIONS"
          :key="o.key"
          class="perm"
          :class="{ 'is-on': permission() === o.key }"
          type="button"
          @click="store.setValue('permission', o.key)"
        >
          <b>{{ o.name }}</b>
          <span>{{ o.desc }}</span>
        </button>
      </div>
    </div>

    <div class="card p-sm">
      <h3>工作目录</h3>
      <p class="hint">助手能读写的范围就是这里，界面上所有文件都相对它解析。</p>
      <div class="ws-row">
        <code class="ws-path">{{ store.values['workspace'] || store.boot?.workspace || '未设置' }}</code>
        <button class="btn btn-sm btn-primary" type="button" @click="pickWorkspace">
          <AppIcon name="folder" size="ic-xs" /> 更换
        </button>
      </div>
    </div>

    <div class="card p-sm">
      <h3>后台与启动</h3>
      <label class="row">
        <span class="grow">
          <b>关闭窗口时收进托盘</b>
          <span>关掉窗口后助手仍在后台待命，右下角托盘图标可以再唤起</span>
        </span>
        <input
          class="wb-switch"
          type="checkbox"
          :checked="minimizeToTray()"
          @change="store.setValue('minimize_to_tray', ($event.target as HTMLInputElement).checked ? 'true' : 'false')"
        />
      </label>
      <label class="row">
        <span class="grow">
          <b>开机自动启动</b>
          <span>开机登录后自动在后台待命，要用时点右下角托盘图标</span>
        </span>
        <input class="wb-switch" type="checkbox" :checked="autoStart" @change="toggleAutoStart" />
      </label>
      <p v-if="autoStartErr" class="hint err">{{ autoStartErr }}</p>
    </div>

    <div class="card p-sm">
      <h3>对话</h3>
      <label class="row">
        <span class="grow">
          <b>回车发送</b>
          <span>关闭后回车换行，发送要用 Ctrl+Enter</span>
        </span>
        <input
          class="wb-switch"
          type="checkbox"
          :checked="sendOnEnter()"
          @change="store.setValue('send_on_enter', ($event.target as HTMLInputElement).checked ? 'true' : 'false')"
        />
      </label>

      <div class="cap" v-if="cap">
        <div class="cap-head">
          <AppIcon name="brain" size="ic-sm" />
          <b>当前模型：{{ cap.id }}</b>
        </div>
        <div class="cap-grid">
          <span>上下文窗口</span>
          <span>
            <template v-if="cap.known">{{ (cap.context_window / 1000).toFixed(0) }}K</template>
            <template v-else><i class="unknown">未知（按 128K 估算）</i></template>
          </span>
          <span>最大输出</span>
          <span>{{ (cap.max_output / 1000).toFixed(0) }}K</span>
          <span>支持思考</span>
          <span>{{ cap.thinking ? '是' : '否' }}</span>
          <span>支持识图</span>
          <span>{{ cap.vision ? '是' : '否' }}</span>
        </div>
        <p v-if="cap.note && !cap.known" class="hint">{{ cap.note }}</p>
        <div class="cap-fix">
          <input
            v-model="winInput"
            class="inp"
            type="number"
            min="1024"
            step="1024"
            placeholder="手填上下文窗口（如 200000）"
          />
          <button class="btn btn-sm btn-outline" type="button" @click="saveWindow">保存</button>
        </div>
        <p v-if="winErr" class="hint err">{{ winErr }}</p>
      </div>
    </div>

    <div class="card p-sm">
      <h3>内置运行时</h3>
      <div v-if="runtimeErr" class="hint err">{{ runtimeErr }}</div>
      <div v-else-if="runtimeInfo" class="kv">
        <dt>内置 Python</dt>
        <dd>
          <span class="led" :class="runtimeInfo.python_exe ? 'g' : 'r'" />
          <template v-if="runtimeInfo.python_exe">
            可用 · {{ runtimeInfo.python_version }}
          </template>
          <template v-else>未就绪</template>
        </dd>
        <template v-if="!runtimeInfo.python_exe">
          <dt>原因</dt>
          <dd class="why">{{ runtimeInfo.python_error || '未知' }}</dd>
          <dt>归档路径</dt>
          <dd class="mono">{{ runtimeInfo.archive_path || '未找到' }}</dd>
        </template>
      </div>
      <button class="btn btn-sm btn-ghost act" type="button" @click="loadRuntime">
        <AppIcon name="refresh" size="ic-xs" /> 重新检测
      </button>
    </div>

    <div class="card p-sm">
      <h3>关于</h3>
      <div class="kv">
        <dt>版本</dt>
        <dd class="mono">{{ store.boot?.version || '—' }}</dd>
        <dt>数据目录</dt>
        <dd class="mono">{{ store.values['workspace'] ? '见上方工作目录' : '—' }}</dd>
        <dt>日志</dt>
        <dd class="mono">%APPDATA%\WorkBaby\logs\app.log</dd>
      </div>
    </div>
  </div>
</template>

<style scoped>
.bp {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
  min-width: 0;
}
.hint {
  color: var(--wb-muted);
  font-size: var(--wb-fs-sm);
  margin: var(--wb-sp-2) 0 var(--wb-sp-3);
}
.hint.err {
  color: var(--wb-danger);
  margin-bottom: 0;
}
.perm-row {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--wb-sp-2);
}
@media (max-width: 860px) {
  .perm-row {
    grid-template-columns: minmax(0, 1fr);
  }
}
.perm {
  text-align: left;
  display: grid;
  gap: 2px;
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  background: var(--wb-surface);
  color: var(--wb-ink);
  cursor: pointer;
  min-width: 0;
  transition: border-color var(--wb-dur) var(--wb-ease), background var(--wb-dur) var(--wb-ease);
}
.perm:hover {
  border-color: var(--wb-border-strong);
}
.perm.is-on {
  border-color: var(--wb-primary);
  background: var(--wb-primary-soft);
}
.perm b {
  font-size: var(--wb-fs-md);
}
.perm span {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-3) 0;
  cursor: pointer;
  border-top: 1px solid var(--wb-line);
}
.row:first-of-type {
  border-top: none;
}
.row .grow {
  display: grid;
  gap: 2px;
  min-width: 0;
}
.row b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.row span {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.ws-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
}
.ws-path {
  flex: 1;
  min-width: 0;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
  padding: var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-raise);
  border: 1px solid var(--wb-line);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cap {
  margin-top: var(--wb-sp-2);
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-line);
  background: var(--wb-surface-2);
  display: grid;
  gap: var(--wb-sp-2);
}
.cap-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  color: var(--wb-ink);
  font-size: var(--wb-fs-sm);
}
.cap-grid {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: var(--wb-sp-1) var(--wb-sp-3);
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
}
.cap-grid > span:nth-child(odd) {
  color: var(--wb-muted);
}
.unknown {
  color: var(--wb-warning);
  font-style: italic;
}
.cap-fix {
  display: flex;
  gap: var(--wb-sp-2);
  min-width: 0;
}
.inp {
  flex: 1;
  min-width: 0;
  height: var(--wb-ctl-h);
  padding: 0 var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border-strong);
  background: var(--wb-surface);
  color: var(--wb-ink);
  font-size: var(--wb-fs-sm);
  font-family: var(--font-mono);
}
.inp:focus {
  outline: none;
  border-color: var(--wb-primary);
  box-shadow: 0 0 0 3px var(--wb-primary-soft);
}
.act {
  margin-top: var(--wb-sp-2);
}
.why {
  color: var(--wb-danger);
  word-break: break-all;
}
</style>
